package view

import (
	"encoding/base64"
	"strconv"
	"strings"

	"github.com/DipaolaFederico/kmd/internal/styled"
)

// maxCopy bounds what is sent to the clipboard (terminals cap OSC 52
// payloads; xterm's default is about 100 KB of base64).
const maxCopy = 64 << 10

// copyCode copies a code block on screen to the clipboard: the first one,
// or the one after the block copied last if that is still on screen, so
// pressing c again walks through the blocks on screen. A block counts as on
// screen when its top border (with the language label) is: a block whose
// last line peeks in at the top is not what the user is looking at. The
// status line shows the start of what was copied.
func (m *model) copyCode() {
	var vis []int
	for i, c := range m.page.Code {
		if c.Start >= m.top && c.Start < m.top+m.viewH() {
			vis = append(vis, i)
		}
	}
	if len(vis) == 0 {
		m.msg = "no code block on screen"
		return
	}
	pick := vis[0]
	for k, i := range vis {
		if i == m.copied && k+1 < len(vis) {
			pick = vis[k+1]
		}
	}
	m.copied = pick
	c := m.page.Code[pick]
	text := c.Text
	if len(text) <= 4*maxCopy { // cleaning only removes characters
		text = clipboardText(text)
	}
	if len(text) > maxCopy {
		m.msg = "code block too large to copy (" + strconv.Itoa(len(text)>>10) + " KiB)"
		return
	}
	if m.opt.Copy == nil {
		m.msg = "no clipboard available"
		return
	}
	if err := m.opt.Copy(text); err != nil {
		m.msg = errText(err)
		return
	}
	n := strings.Count(text, "\n") + 1
	what := "copied " + strconv.Itoa(n) + " line"
	if n != 1 {
		what += "s"
	}
	if c.Lang != "" {
		what += " of " + clip(c.Lang, 20)
	}
	first, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
	what += ": " + clip(first, 40)
	if len(vis) > 1 {
		what += " · c: next"
	}
	m.msg = what
}

// clipboardText removes the characters a paste must not carry, which the
// screen does not show either (styled.Hidden): control characters other
// than tab and newline (a pasted ESC could drive the terminal or the
// shell's line editor), bidi controls, and zero-width and other format
// characters, which would make the copied command differ from the one on
// screen.
func clipboardText(s string) string { return styled.Strip(s, "\t\n") }

// osc52 is the escape sequence that asks the terminal to put text in the
// clipboard. The payload is base64, so it cannot end the sequence early.
func osc52(text string) string {
	return "\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte(text)) + "\x07"
}

// clipboardCommand picks the program that sets the desktop clipboard, or
// nil to use OSC 52: over SSH (the clipboard to fill is the one of the
// terminal, on the other machine) or without such a program. Many
// terminals ignore OSC 52 (VTE-based ones such as GNOME Terminal), so a
// local session prefers the desktop's tool.
func clipboardCommand(getenv func(string) string, lookPath func(string) (string, error), goos string) []string {
	if getenv("SSH_TTY") != "" || getenv("SSH_CONNECTION") != "" {
		return nil
	}
	var candidates [][]string
	switch {
	case goos == "darwin":
		candidates = [][]string{{"pbcopy"}}
	case getenv("WAYLAND_DISPLAY") != "":
		candidates = [][]string{{"wl-copy"}, {"xclip", "-selection", "clipboard"}, {"xsel", "--clipboard", "--input"}}
	case getenv("DISPLAY") != "":
		candidates = [][]string{{"xclip", "-selection", "clipboard"}, {"xsel", "--clipboard", "--input"}}
	}
	for _, c := range candidates {
		if p, err := lookPath(c[0]); err == nil {
			return append([]string{p}, c[1:]...)
		}
	}
	return nil
}
