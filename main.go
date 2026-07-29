// FindSenryu4Slack detects Japanese senryu (5-7-5 mora poems) in Slack
// messages and replies when it finds one. It is a Socket Mode bot: it opens an
// outbound WebSocket to Slack, so it needs no public endpoint. The 5-7-5
// detection is delegated to go-haiku (kagome + UniDic), whose dictionary is
// loaded once at startup and kept warm in memory for low-latency responses.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	haiku "github.com/0x307e/go-haiku"
	"github.com/ikawaha/kagome-dict/uni"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel()}))
	slog.SetDefault(logger)

	botToken := os.Getenv("SLACK_BOT_TOKEN")
	appToken := os.Getenv("SLACK_APP_TOKEN")
	if err := validateTokens(botToken, appToken); err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	// Load the UniDic dictionary into the go-haiku analyzer once. This is the
	// one expensive step; keeping the process resident keeps it warm.
	haiku.UseDict(uni.Dict())

	api := slack.New(botToken, slack.OptionAppLevelToken(appToken))

	// Resolve our own bot/user IDs so we never react to our own messages.
	auth, err := api.AuthTest()
	if err != nil {
		slog.Error("Slack auth test failed (check SLACK_BOT_TOKEN)", "error", err)
		os.Exit(1)
	}
	slog.Info("authenticated", "bot_user_id", auth.UserID, "team", auth.Team)

	client := socketmode.New(api)
	bot := &bot{api: api, client: client, selfUserID: auth.UserID}

	// Optional health endpoint for hosts that require a listening port
	// (e.g. AWS App Runner). Render Background Workers do not need it.
	if port := os.Getenv("PORT"); port != "" {
		go serveHealth(port)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go bot.handleEvents(ctx)

	slog.Info("connecting to Slack via Socket Mode")
	if err := client.RunContext(ctx); err != nil && ctx.Err() == nil {
		slog.Error("socket mode client stopped with error", "error", err)
		os.Exit(1)
	}
	slog.Info("shutting down")
}

type bot struct {
	api        *slack.Client
	client     *socketmode.Client
	selfUserID string
}

// handleEvents consumes the Socket Mode event stream until ctx is cancelled.
func (b *bot) handleEvents(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-b.client.Events:
			if !ok {
				return
			}
			slog.Debug("received socketmode event", "type", evt.Type)
			switch evt.Type {
			case socketmode.EventTypeConnecting:
				slog.Info("connecting…")
			case socketmode.EventTypeConnected:
				slog.Info("connected")
			case socketmode.EventTypeConnectionError:
				slog.Warn("connection error", "data", evt.Data)
			case socketmode.EventTypeEventsAPI:
				// Ack immediately (Slack requires ack within 3s), then process.
				if evt.Request != nil {
					b.client.Ack(*evt.Request)
				}
				eventsAPI, ok := evt.Data.(slackevents.EventsAPIEvent)
				if !ok {
					slog.Debug("ignored non-events-api payload", "type", fmt.Sprintf("%T", evt.Data))
					continue
				}
				slog.Debug("received events-api event", "event_type", eventsAPI.Type, "inner_type", eventsAPI.InnerEvent.Type)
				b.handleEventsAPI(eventsAPI)
			}
		}
	}
}

func (b *bot) handleEventsAPI(event slackevents.EventsAPIEvent) {
	if event.Type != slackevents.CallbackEvent {
		slog.Debug("ignored non-callback event", "event_type", event.Type)
		return
	}
	msg, ok := event.InnerEvent.Data.(*slackevents.MessageEvent)
	if !ok {
		slog.Debug("ignored non-message event", "inner_type", event.InnerEvent.Type, "data_type", fmt.Sprintf("%T", event.InnerEvent.Data))
		return
	}
	b.handleMessage(msg)
}

// handleMessage runs the detection pipeline on a single message event and
// replies in-thread when a senryu is found. Mirrors the Discord bot's
// messageCreate filtering (main.go:509).
func (b *bot) handleMessage(m *slackevents.MessageEvent) {
	slog.Debug(
		"received message",
		"channel", m.Channel,
		"channel_type", m.ChannelType,
		"user", m.User,
		"bot_id", m.BotID,
		"subtype", m.SubType,
		"text", m.Text,
	)
	// Only act on brand-new top-level messages: skip edits/deletes/joins
	// (any SubType) and anything posted by a bot (including ourselves).
	if m.SubType != "" {
		slog.Debug("ignored message with subtype", "subtype", m.SubType)
		return
	}
	if m.BotID != "" {
		slog.Debug("ignored bot message", "bot_id", m.BotID)
		return
	}
	if m.User == "" {
		slog.Debug("ignored message without user")
		return
	}
	if m.User == b.selfUserID {
		slog.Debug("ignored self message", "user", m.User)
		return
	}
	// DMs are not supported (parity with the Discord bot).
	if m.ChannelType == "im" || m.ChannelType == "mpim" {
		slog.Debug("ignored dm message", "channel_type", m.ChannelType)
		return
	}

	senryu, ok := DetectSenryu(m.Text)
	if !ok {
		slog.Debug("no senryu detected", "text", m.Text)
		return
	}

	// Reply threaded under the original message (or in the existing thread),
	// and broadcast it so the notification is also visible in the channel.
	threadTS := m.ThreadTimeStamp
	if threadTS == "" {
		threadTS = m.TimeStamp
	}
	text := fmt.Sprintf("川柳を検出しました！\n「%s」", senryu)
	if _, _, err := b.api.PostMessage(
		m.Channel,
		slack.MsgOptionText(text, false),
		slack.MsgOptionTS(threadTS),
		// Broadcast the threaded reply so it also appears in the channel
		// timeline, not only inside the thread.
		slack.MsgOptionBroadcast(),
		slack.MsgOptionDisableLinkUnfurl(),
	); err != nil {
		slog.Warn("failed to post senryu reply", "error", err, "channel", m.Channel)
		return
	}
	slog.Info("senryu detected", "channel", m.Channel, "user", m.User, "senryu", senryu)
}

// validateTokens checks the two required Slack tokens are present and have the
// expected prefixes, giving a clear error before we attempt to connect.
func validateTokens(botToken, appToken string) error {
	if botToken == "" {
		return fmt.Errorf("SLACK_BOT_TOKEN is required")
	}
	if !strings.HasPrefix(botToken, "xoxb-") {
		return fmt.Errorf("SLACK_BOT_TOKEN should start with 'xoxb-' (bot token)")
	}
	if appToken == "" {
		return fmt.Errorf("SLACK_APP_TOKEN is required")
	}
	if !strings.HasPrefix(appToken, "xapp-") {
		return fmt.Errorf("SLACK_APP_TOKEN should start with 'xapp-' (app-level token with connections:write)")
	}
	return nil
}

func logLevel() slog.Level {
	switch strings.ToLower(os.Getenv("LOG_LEVEL")) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func serveHealth(port string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Warn("health server stopped", "error", err)
	}
}
