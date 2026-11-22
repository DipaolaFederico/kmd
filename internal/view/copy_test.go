package view

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/DipaolaFederico/kmd/internal/render"
)

func TestCopyCode(t *testing.T) {
	lines := make([]string, 100)
	page := &Page{Lines: lines, Code: []render.CodeBlock{
		{Lang: "go", Text: "a := 1\nb := 2", Start: 2, End: 5},
		{Text: "evil\x1b]52;c;x\x07\u202etxt\r\n\tok", Start: 6, End: 8},
		{Lang: "sh", Text: "ls", Start: 50, End: 52},
	}}
	var got []string
	h := &harness{model: &model{page: page, rows: 12, cols: 60, sel: -1, copied: -1, opt: Options{
		Copy: func(s string) error { got = append(got, s); return nil },
	}}}
	h.keys("c")
	if len(got) != 1 || got[0] != "a := 1\nb := 2" || h.msg != "copied 2 lines of go: a := 1 · c: next" {
		t.Fatalf("first: %q %q", got, h.msg)
	}
	h.keys("c") // the next block on screen, cleaned
	if got[1] != "evil]52;c;xtxt\n\tok" || h.msg != "copied 2 lines: evil]52;c;xtxt · c: next" {
		t.Fatalf("second: %q %q", got[1], h.msg)
	}
	h.keys("c") // wraps to the first block on screen
	if got[2] != got[0] {
		t.Errorf("third: %q", got[2])
	}
	h.top = 45
	h.keys("c")
	if got[3] != "ls" || h.msg != "copied 1 line of sh: ls" {
		t.Errorf("only block on screen: %q %q", got[3], h.msg)
	}
	h.top = 20
	h.keys("c")
	if h.msg != "no code block on screen" || len(got) != 4 {
		t.Errorf("nothing on screen: %q", h.msg)
	}
	// a block whose top is above the screen is not copied, even if its
	// last lines show (it is not what the user is looking at)
	h.top = 51
	h.keys("c")
	if h.msg != "no code block on screen" || len(got) != 4 {
		t.Errorf("block scrolled past: %q", h.msg)
	}

	h.top = 45
	h.opt.Copy = func(string) error { return errors.New("tty gone") }
	h.keys("c")
	if h.msg != "tty gone" {
		t.Errorf("copy error: %q", h.msg)
	}
	h.opt.Copy = nil
	h.keys("c")
	if h.msg != "no clipboard available" {
		t.Errorf("no clipboard: %q", h.msg)
	}
	page.Code[2].Text = strings.Repeat("x", maxCopy+1)
	h.opt.Copy = func(string) error { t.Error("copied a huge block"); return nil }
	h.keys("c")
	if !strings.HasPrefix(h.msg, "code block too large") {
		t.Errorf("too large: %q", h.msg)
	}
}

func TestOSC52(t *testing.T) {
	s := osc52("héllo\x07\x1b")
	if !strings.HasPrefix(s, "\x1b]52;c;") || !strings.HasSuffix(s, "\x07") {
		t.Fatalf("framing: %q", s)
	}
	payload := s[len("\x1b]52;c;") : len(s)-1]
	if strings.ContainsAny(payload, "\x07\x1b") {
		t.Error("payload can end the sequence")
	}
	if dec, err := base64.StdEncoding.DecodeString(payload); err != nil || string(dec) != "héllo\x07\x1b" {
		t.Errorf("payload: %q %v", dec, err)
	}
}

func TestClipboardCommand(t *testing.T) {
	env := func(kv ...string) func(string) string {
		m := map[string]string{}
		for i := 0; i+1 < len(kv); i += 2 {
			m[kv[i]] = kv[i+1]
		}
		return func(k string) string { return m[k] }
	}
	have := func(names ...string) func(string) (string, error) {
		return func(n string) (string, error) {
			for _, h := range names {
				if h == n {
					return "/usr/bin/" + n, nil
				}
			}
			return "", errors.New("not found")
		}
	}
	for _, c := range []struct {
		name string
		env  func(string) string
		look func(string) (string, error)
		goos string
		want string
	}{
		{"wayland", env("WAYLAND_DISPLAY", "wayland-0"), have("wl-copy", "xclip"), "linux", "/usr/bin/wl-copy"},
		{"xwayland only", env("WAYLAND_DISPLAY", "wayland-0"), have("xclip"), "linux", "/usr/bin/xclip -selection clipboard"},
		{"x11", env("DISPLAY", ":0"), have("xsel"), "linux", "/usr/bin/xsel --clipboard --input"},
		{"no tool", env("DISPLAY", ":0"), have(), "linux", ""},
		{"console", env(), have("wl-copy"), "linux", ""},
		{"ssh", env("SSH_TTY", "/dev/pts/1", "DISPLAY", ":0"), have("xclip"), "linux", ""},
		{"macos", env(), have("pbcopy"), "darwin", "/usr/bin/pbcopy"},
	} {
		if got := strings.Join(clipboardCommand(c.env, c.look, c.goos), " "); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}
