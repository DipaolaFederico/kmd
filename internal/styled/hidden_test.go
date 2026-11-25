package styled

import (
	"testing"
	"unicode"
)

func TestHidden(t *testing.T) {
	hidden := []rune{
		0x00, 0x1b, 0x7f, 0x85, 0x9b, // controls
		0xad,                                                   // soft hyphen
		0x061c, 0x200e, 0x200f, 0x202a, 0x202e, 0x2066, 0x2069, // bidi marks and controls
		0x200b, 0x2060, 0x206f, 0xfeff, 0xfff9, // zero-width space, word joiner, BOM…
		0x2028, 0x2029, // line and paragraph separators
		0xe0001, 0xe0041, 0xe007f, // tag characters
	}
	for _, r := range hidden {
		if !Hidden(r) {
			t.Errorf("Hidden(%U) = false", r)
		}
	}
	shown := []rune{' ', 'a', '~', 0xa0, 0xe9, 0x200c, 0x200d, 0xfe0f, 0xfe0e, 0x2014, 0x65e5, 0x1f600, 0x0301}
	for _, r := range shown {
		if Hidden(r) {
			t.Errorf("Hidden(%U) = true", r)
		}
	}
}

// TestHiddenMatchesUnicode checks Hidden on every rune against its
// definition: Hidden writes the ranges of category Cf out for speed, and a
// Go release with a newer Unicode version may add some.
func TestHiddenMatchesUnicode(t *testing.T) {
	want := func(r rune) bool {
		switch {
		case r < 0x20 || r >= 0x7f && r < 0xa0, r == 0x2028, r == 0x2029:
			return true
		case r == 0x200c || r == 0x200d:
			return false
		}
		return unicode.Is(unicode.Cf, r)
	}
	for r := rune(0); r <= unicode.MaxRune; r++ {
		if Hidden(r) != want(r) {
			t.Errorf("Hidden(%U) = %v, want %v (Unicode %s)", r, Hidden(r), want(r), unicode.Version)
		}
	}
}

func TestStrip(t *testing.T) {
	tests := []struct{ in, keep, want string }{
		{"plain text", "", "plain text"},
		{"a\tb\nc", "\t\n", "a\tb\nc"},
		{"a\tb\nc\rd", "", "abcd"},
		{"\x1b[31mred\x1b[0m", "", "[31mred[0m"},
		{"curl https://e.x/install\u200b.sh | sh", "", "curl https://e.x/install.sh | sh"},
		{"user\u202e \u2066// admin\u2069", "", "user // admin"},
		{"tag\U000E0041\U000E007F", "", "tag"},
		{"bad \x9b2J utf8", "", "bad \ufffd2J utf8"},
		{"emoji \U0001F468\u200d\U0001F469\u200d\U0001F467 \u2764\ufe0f", "", "emoji \U0001F468\u200d\U0001F469\u200d\U0001F467 \u2764\ufe0f"},
		{"\u0645\u06cc\u200c\u062e\u0648\u0627\u0647\u0645", "", "\u0645\u06cc\u200c\u062e\u0648\u0627\u0647\u0645"},
		{"café — 日本", "", "café — 日本"},
	}
	for _, tc := range tests {
		if got := Strip(tc.in, tc.keep); got != tc.want {
			t.Errorf("Strip(%q, %q) = %q, want %q", tc.in, tc.keep, got, tc.want)
		}
	}
}

func TestStripCleanDoesNotAllocate(t *testing.T) {
	s := "café — 日本語 and plain ASCII text"
	if n := testing.AllocsPerRun(100, func() { _ = Strip(s, "\t") }); n != 0 {
		t.Errorf("Strip of clean text allocates %v times", n)
	}
}

func BenchmarkStrip(b *testing.B) {
	for _, tc := range []struct{ name, s string }{
		{"ascii", "The quick brown fox jumps over the lazy dog, again and again."},
		{"arabic", "\u0627\u0644\u0633\u0644\u0627\u0645 \u0639\u0644\u064a\u0643\u0645 \u0648\u0631\u062d\u0645\u0629 \u0627\u0644\u0644\u0647 \u0648\u0628\u0631\u0643\u0627\u062a\u0647"},
		{"emoji", "\U0001F600 \U0001F680 \u2764\ufe0f \U0001F468\u200d\U0001F469\u200d\U0001F467 \U0001F389"},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.SetBytes(int64(len(tc.s)))
			for b.Loop() {
				_ = Strip(tc.s, "\t")
			}
		})
	}
}
