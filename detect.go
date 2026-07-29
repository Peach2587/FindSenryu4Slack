package main

import (
	"log/slog"
	"regexp"
	"strings"
	"unicode"

	haiku "github.com/0x307e/go-haiku"
)

// Detection pipeline ported from FindSenryu4Discord (main.go), with the
// platform-specific text handling rewritten for Slack markup instead of
// Discord markup. The 5-7-5 mora analysis itself is delegated unchanged to
// go-haiku (kagome + UniDic).

// Slack markup patterns.
//
// Slack wraps mentions/channels/links/special commands in angle brackets and
// HTML-escapes any literal <, >, & the user typed. That means a real "<...>"
// sequence in event text is always a Slack entity, never user prose, so its
// presence is a reliable signal that the message is not a clean poem.
var (
	// reSlackEntity matches Slack auto-formatted entities: user mentions
	// (<@U…>), channel mentions (<#C…|name>), special mentions (<!here>,
	// <!subteam^…>), and links (<http…|text>, <mailto:…>).
	reSlackEntity = regexp.MustCompile(`<[@#!][^>]*>|<[a-z]+:[^>]*>`)
	// reBareURL is a safety net for URLs that arrive without angle brackets.
	reBareURL = regexp.MustCompile(`https?://\S+`)
)

// containsSlackTokens reports whether s contains Slack-specific tokens
// (mentions, channel refs, special mentions, links) that should exclude the
// message from senryu detection. Mirrors containsDiscordTokens.
func containsSlackTokens(s string) bool {
	return reSlackEntity.MatchString(s) || reBareURL.MatchString(s)
}

var (
	reFencedCodeBlock = regexp.MustCompile("(?s)```.*?```")
	reInlineCode      = regexp.MustCompile("`[^`]+`")
	// reEmojiShortcode matches Slack emoji shortcodes like :smile:, :+1:,
	// :100:. Both a leading and trailing colon are required, so clock times
	// ("10:30") and lone colons are left untouched.
	reEmojiShortcode = regexp.MustCompile(`:[a-z0-9_'+-]+:`)
)

// stripCodeBlocks removes fenced and inline code spans. Ported unchanged;
// Slack uses the same ``` / ` code syntax as Discord.
func stripCodeBlocks(s string) string {
	s = reFencedCodeBlock.ReplaceAllString(s, "")
	s = reInlineCode.ReplaceAllString(s, "")
	return s
}

// stripEmojiShortcodes removes :emoji: shortcodes so decorative emoji do not
// pollute the mora analysis or drag the Japanese-char ratio below threshold.
func stripEmojiShortcodes(s string) string {
	return reEmojiShortcode.ReplaceAllString(s, "")
}

// slackUnescaper reverses Slack's HTML escaping of the three reserved
// characters. Slack sends &amp; &lt; &gt; in message text.
var slackUnescaper = strings.NewReplacer("&lt;", "<", "&gt;", ">", "&amp;", "&")

// unescapeSlack converts Slack's HTML-escaped reserved characters back to
// their literal form for analysis.
func unescapeSlack(s string) string {
	return slackUnescaper.Replace(s)
}

// findHaikuSafe wraps haiku.Find with recover to prevent tokenizer panics from
// crashing the bot. Ported verbatim from the Discord bot (main.go:810).
func findHaikuSafe(content string, rule []int) (result []string) {
	defer func() {
		if r := recover(); r != nil {
			slog.Warn("recovered from panic in haiku.Find", "error", r, "content_len", len(content))
			result = nil
		}
	}()
	return haiku.Find(content, rule)
}

// haikuSpansNewline reports whether the matched senryu is stitched together
// across a newline in the original content (which should be rejected). Ported
// verbatim from the Discord bot (main.go:842).
func haikuSpansNewline(content, haikuResult string) bool {
	if !strings.Contains(content, "\n") {
		return false
	}
	matched := strings.ReplaceAll(haikuResult, " ", "")
	return !strings.Contains(content, matched)
}

// japaneseCharRatioThreshold is the minimum ratio of Japanese characters
// (Hiragana, Katakana, Han) to total non-space characters required for a
// message to be considered "Japanese-rich" and eligible for senryu detection.
const japaneseCharRatioThreshold = 0.5

// isJapaneseRune reports whether r is a Japanese script character (Hiragana,
// Katakana, or Han) or one of the two katakana marks treated as Japanese.
func isJapaneseRune(r rune) bool {
	return unicode.In(r, unicode.Hiragana, unicode.Katakana, unicode.Han) ||
		r == 'ー' || // Katakana long vowel mark (U+30FC)
		r == '・' // Katakana middle dot (U+30FB)
}

// countJapanese returns the number of Japanese script characters in s.
func countJapanese(s string) int {
	n := 0
	for _, r := range s {
		if isJapaneseRune(r) {
			n++
		}
	}
	return n
}

// isJapaneseRich reports whether at least japaneseCharRatioThreshold of the
// non-space characters are Japanese. Ported verbatim from the Discord bot
// (main.go:855).
func isJapaneseRich(s string) bool {
	var total, jp int
	for _, r := range s {
		if unicode.IsSpace(r) {
			continue
		}
		total++
		if isJapaneseRune(r) {
			jp++
		}
	}
	if total == 0 {
		return false
	}
	return float64(jp)/float64(total) >= japaneseCharRatioThreshold
}

// coversWholeContent reports whether the matched verse accounts for
// essentially every Japanese character in content, i.e. the message *is* the
// verse rather than merely *containing* it. go-haiku's Find joins segments
// with spaces, so those are stripped before counting.
func coversWholeContent(content, result string) bool {
	matched := strings.ReplaceAll(result, " ", "")
	return countJapanese(matched) >= countJapanese(content)
}

// PoemKind identifies which Japanese verse form was detected.
type PoemKind int

const (
	// KindSenryu is the 5-7-5 form (also senryu/haiku metre).
	KindSenryu PoemKind = iota
	// KindTanka is the 5-7-5-7-7 form.
	KindTanka
	// KindShichiShichi is the 7-7 lower verse (shimo-no-ku).
	KindShichiShichi
)

// Label returns the Japanese name used in the notification message.
func (k PoemKind) Label() string {
	switch k {
	case KindTanka:
		return "短歌"
	case KindShichiShichi:
		return "七七"
	default:
		return "川柳"
	}
}

// poemRule pairs a verse form with its mora pattern.
type poemRule struct {
	kind PoemKind
	rule []int
	// wholeOnly requires the match to cover the whole message. It guards the
	// very short 7-7 form, which otherwise fires on ordinary sentences that
	// merely contain a 7-7 fragment (e.g. "明日の会議は何時からでしたっけ").
	wholeOnly bool
}

// poemRules lists the supported verse forms in detection-precedence order.
// Order matters: tanka (5-7-5-7-7) contains a 5-7-5 prefix, so it must be
// tried before senryu or a tanka would be reported as a mere senryu. The 7-7
// lower verse is tried last because it is the shortest and most permissive
// pattern, so it should only match when nothing longer does — and only when it
// spans the whole message (wholeOnly).
var poemRules = []poemRule{
	{kind: KindTanka, rule: []int{5, 7, 5, 7, 7}},
	{kind: KindSenryu, rule: []int{5, 7, 5}},
	{kind: KindShichiShichi, rule: []int{7, 7}, wholeOnly: true},
}

// DetectPoem runs the full detection pipeline on a raw Slack message text and
// returns the matched verse (segments space-separated, as go-haiku returns
// it), its kind, and true if one is found. Verse forms are tried in
// poemRules precedence order. It performs no I/O.
func DetectPoem(raw string) (string, PoemKind, bool) {
	if containsSlackTokens(raw) {
		return "", 0, false
	}
	content := stripCodeBlocks(raw)
	content = stripEmojiShortcodes(content)
	content = unescapeSlack(content)
	if !isJapaneseRich(content) {
		return "", 0, false
	}
	for _, pr := range poemRules {
		h := findHaikuSafe(content, pr.rule)
		if len(h) == 0 || haikuSpansNewline(content, h[0]) {
			continue
		}
		if pr.wholeOnly && !coversWholeContent(content, h[0]) {
			continue
		}
		return h[0], pr.kind, true
	}
	return "", 0, false
}

// DetectSenryu is a backwards-compatible wrapper that reports only whether any
// supported verse form was detected.
func DetectSenryu(raw string) (string, bool) {
	poem, _, ok := DetectPoem(raw)
	return poem, ok
}
