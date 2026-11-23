package term

import (
	"math"
	"strings"
	"testing"
)

func TestParseBackground(t *testing.T) {
	const da1 = "\x1b[?62;22c"
	for _, tc := range []struct {
		reply   string
		r, g, b float64
		ok      bool
	}{
		{"\x1b]11;rgb:ffff/ffff/ffff\x1b\\" + da1, 1, 1, 1, true},
		{"\x1b]11;rgb:0000/8080/FFFF\a" + da1, 0, 0x8080 / 65535.0, 1, true},
		{"\x1b]11;rgb:f/80/fff\a", 1, 0x80 / 255.0, 1, true},         // 1–4 digits per channel
		{"x\x1b]11;rgb:1/2/3\a", 1 / 15.0, 2 / 15.0, 3 / 15.0, true}, // a key typed before
		{da1, 0, 0, 0, false},                                                   // no OSC 11 support
		{"", 0, 0, 0, false},                                                    // silent terminal
		{"\x1b]11;rgb:fffff/0/0\a", 0, 0, 0, false},                             // 5 digits
		{"\x1b]11;rgb:/0/0\a", 0, 0, 0, false},                                  // empty channel
		{"\x1b]11;rgb:0/0\a", 0, 0, 0, false},                                   // two channels
		{"\x1b]11;rgb:0/0/0/0\a", 0, 0, 0, false},                               // four channels
		{"\x1b]11;rgb:0/0/0", 0, 0, 0, false},                                   // unterminated
		{"\x1b]11;rgb:0/0/0\x1b", 0, 0, 0, false},                               // half a terminator
		{"\x1b]11;rgba:0/0/0/0\a", 0, 0, 0, false},                              // other colour forms
		{"\x1b]11;#ffffff\a", 0, 0, 0, false},                                   //
		{"\x1b]11;rgb:g/0/0\a", 0, 0, 0, false},                                 // not hex
		{"\x1b]10;rgb:ffff/ffff/ffff\a", 0, 0, 0, false},                        // foreground, not background
		{"\x1b]11;rgb:-1/0/0\a", 0, 0, 0, false},                                // sign
		{strings.Repeat("x", maxReply) + "\x1b]11;rgb:f/f/f\a", 0, 0, 0, false}, // too long
	} {
		r, g, b, ok := ParseBackground([]byte(tc.reply))
		if ok != tc.ok || math.Abs(r-tc.r) > 1e-9 || math.Abs(g-tc.g) > 1e-9 || math.Abs(b-tc.b) > 1e-9 {
			t.Errorf("ParseBackground(%q) = %v,%v,%v,%v; want %v,%v,%v,%v", tc.reply, r, g, b, ok, tc.r, tc.g, tc.b, tc.ok)
		}
	}
}

func TestIsLight(t *testing.T) {
	for _, tc := range []struct {
		r, g, b float64
		light   bool
	}{
		{1, 1, 1, true},          // white
		{0.99, 0.96, 0.89, true}, // solarized light #fdf6e3
		{0.83, 0.85, 0.9, true},  // tokyo night day #d5d6db
		{0, 0, 0, false},         // black
		{0.1, 0.11, 0.15, false}, // tokyo night #1a1b26
		{0, 0.17, 0.21, false},   // solarized dark #002b36
		{0.5, 0.5, 0.5, false},   // mid grey stays dark
		{0, 0, 1, false},         // saturated blue is dark
		{0, 1, 0, true},          // saturated green is light
	} {
		if got := IsLight(tc.r, tc.g, tc.b); got != tc.light {
			t.Errorf("IsLight(%v,%v,%v) = %v", tc.r, tc.g, tc.b, got)
		}
	}
}

func TestDA1Done(t *testing.T) {
	for in, want := range map[string]bool{
		"\x1b[?62;22c":                true,
		"\x1b[?1;2c":                  true,
		"\x1b]11;rgb:0/0/0\a\x1b[?6c": true,
		"\x1b[?62;22":                 false, // incomplete
		"\x1b[?62x":                   false,
		"\x1b[62c":                    false, // not a DA1 reply
		"":                            false,
	} {
		if got := da1Done([]byte(in)); got != want {
			t.Errorf("da1Done(%q) = %v", in, got)
		}
	}
}

func FuzzBackground(f *testing.F) {
	f.Add([]byte("\x1b]11;rgb:ffff/ffff/ffff\x1b\\\x1b[?62c"))
	f.Add([]byte("\x1b]11;rgb:1/22/333\a"))
	f.Add([]byte("\x1b]11;rgb:fffff/0/0\a"))
	f.Add([]byte("\x1b[?1;2c"))
	f.Fuzz(func(t *testing.T, reply []byte) {
		r, g, b, ok := ParseBackground(reply)
		if !ok {
			if r != 0 || g != 0 || b != 0 {
				t.Fatal("colour without ok")
			}
			return
		}
		if len(reply) > maxReply {
			t.Fatalf("accepted a %d-byte reply", len(reply))
		}
		for _, c := range []float64{r, g, b} {
			if !(c >= 0 && c <= 1) {
				t.Fatalf("channel %v out of range", c)
			}
		}
		_ = da1Done(reply)
	})
}
