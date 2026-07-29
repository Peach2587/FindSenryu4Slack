package main

import (
	"testing"

	haiku "github.com/0x307e/go-haiku"
	"github.com/ikawaha/kagome-dict/uni"
)

// The 5-7-5 analysis needs the dictionary loaded, so initialize it once for
// the whole test binary.
func TestMain(m *testing.M) {
	haiku.UseDict(uni.Dict())
	m.Run()
}

func TestContainsSlackTokens(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"plain japanese", "古池や蛙飛び込む水の音", false},
		{"user mention", "<@U12345> 古池や蛙飛び込む水の音", true},
		{"user mention with name", "<@U12345|taro> こんにちは", true},
		{"channel mention", "<#C12345|general> 古池や蛙飛び込む水の音", true},
		{"special mention here", "<!here> 集合", true},
		{"subteam mention", "<!subteam^S123|team> よろしく", true},
		{"slack link", "詳しくは <https://example.com|こちら>", true},
		{"mailto link", "<mailto:a@example.com|メール>", true},
		{"bare url", "https://example.com 古池や蛙飛び込む水の音", true},
		{"time is not a token", "10:30 に集合", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := containsSlackTokens(tt.input); got != tt.want {
				t.Errorf("containsSlackTokens(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestStripCodeBlocks(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"no code", "古池や蛙飛び込む水の音", "古池や蛙飛び込む水の音"},
		{"fenced", "```\n古池や蛙飛び込む水の音\n```", ""},
		{"inline", "`古池や蛙飛び込む水の音`", ""},
		{"inline mixed", "前 `code` 後", "前  後"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stripCodeBlocks(tt.input); got != tt.want {
				t.Errorf("stripCodeBlocks(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestStripEmojiShortcodes(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"single emoji", "古池や蛙飛び込む水の音 :smile:", "古池や蛙飛び込む水の音 "},
		{"plus one", ":+1:", ""},
		{"time untouched", "10:30", "10:30"},
		{"lone colon", "これは: テスト", "これは: テスト"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stripEmojiShortcodes(tt.input); got != tt.want {
				t.Errorf("stripEmojiShortcodes(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestUnescapeSlack(t *testing.T) {
	in := "a &lt;b&gt; &amp; c"
	want := "a <b> & c"
	if got := unescapeSlack(in); got != want {
		t.Errorf("unescapeSlack(%q) = %q, want %q", in, got, want)
	}
}

func TestIsJapaneseRich(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"all kanji", "古池蛙飛込水音", true},
		{"mixed japanese", "古池や蛙飛びこむ水の音", true},
		{"japanese with spaces", "古池や 蛙飛びこむ 水の音", true},
		{"japanese with punctuation", "古池や！蛙飛びこむ？水の音", true},
		{"mostly english", "hello world this is a test", false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isJapaneseRich(tt.input); got != tt.want {
				t.Errorf("isJapaneseRich(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestHaikuSpansNewline(t *testing.T) {
	tests := []struct {
		name    string
		content string
		result  string
		want    bool
	}{
		{"no newline", "古池や蛙飛びこむ水の音", "古池や 蛙飛びこむ 水の音", false},
		{"result spans newline", "古池や蛙飛びこむ\n水の音", "古池や 蛙飛びこむ 水の音", true},
		{"three lines", "古池や\n蛙飛びこむ\n水の音", "古池や 蛙飛びこむ 水の音", true},
		{"complete haiku after newline", "こんにちは\n古池や蛙飛びこむ水の音", "古池や 蛙飛びこむ 水の音", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := haikuSpansNewline(tt.content, tt.result); got != tt.want {
				t.Errorf("haikuSpansNewline(%q, %q) = %v, want %v", tt.content, tt.result, got, tt.want)
			}
		})
	}
}

// TestDetectSenryu exercises the full pipeline end to end.
func TestDetectSenryu(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"classic senryu", "古池や蛙飛び込む水の音", true},
		{"senryu with trailing emoji", "古池や蛙飛び込む水の音 :smile:", true},
		{"not a senryu", "今日はいい天気ですね", false},
		{"contains mention", "<@U123> 古池や蛙飛び込む水の音", false},
		{"contains url", "古池や蛙飛び込む水の音 https://example.com", false},
		{"english only", "hello world this is not japanese", false},
		{"spans newline", "古池や蛙飛びこむ\n水の音", false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, got := DetectSenryu(tt.input)
			if got != tt.want {
				t.Errorf("DetectSenryu(%q) matched = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
