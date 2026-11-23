package view

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

// key is one decoded input event.
type key struct {
	name string // "up", "tab", "enter", "char", "mouse"…
	r    rune   // the character for name == "char"
	// mouse events (SGR 1006 encoding): button code, 0-based cell, press.
	btn, x, y int
	press     bool
}

// csiKeys maps the final part of CSI / SS3 sequences to key names.
var csiKeys = map[string]string{
	"A": "up", "B": "down", "C": "right", "D": "left", "H": "home", "F": "end", "Z": "btab",
	"1~": "home", "4~": "end", "7~": "home", "8~": "end", "5~": "pgup", "6~": "pgdn", "3~": "delete",
}

// parseKeys decodes a chunk read from the terminal. An ESC not followed by
// a sequence is the Escape key (terminals send whole sequences at once).
func parseKeys(b []byte) []key {
	var out []key
	for len(b) > 0 {
		c := b[0]
		switch {
		case c == 0x1b && len(b) > 2 && b[1] == ']':
			// a terminal's answer to an OSC query (kmd asks for the
			// background color) that arrived late: not typed keys
			b = b[oscLen(b):]
			continue
		case c == 0x1b && len(b) > 2 && (b[1] == '[' || b[1] == 'O'):
			k, n := parseSeq(b)
			if k.name != "" {
				out = append(out, k)
			}
			b = b[n:]
			continue
		case c == 0x1b:
			out = append(out, key{name: "esc"})
		case c == '\r' || c == '\n':
			out = append(out, key{name: "enter"})
		case c == '\t':
			out = append(out, key{name: "tab"})
		case c == 0x7f || c == 0x08:
			out = append(out, key{name: "backspace"})
		case c < 0x20:
			out = append(out, key{name: "ctrl-" + string(rune('a'+c-1))})
		default:
			r, n := utf8.DecodeRune(b)
			out = append(out, key{name: "char", r: r})
			b = b[n:]
			continue
		}
		b = b[1:]
	}
	return out
}

// oscLen is the length of the OSC string at the start of b, up to its BEL
// or ESC \ terminator, or all of b when the terminator is missing.
func oscLen(b []byte) int {
	for i := 2; i < len(b); i++ {
		switch {
		case b[i] == '\a':
			return i + 1
		case b[i] == 0x1b && i+1 < len(b) && b[i+1] == '\\':
			return i + 2
		}
	}
	return len(b)
}

// parseSeq decodes the CSI or SS3 sequence at the start of b and returns
// the key (name "" if unknown) and the number of bytes consumed.
func parseSeq(b []byte) (key, int) {
	ss3 := b[1] == 'O'
	i := 2
	for i < len(b) && (b[i] >= 0x20 && b[i] <= 0x3f) { // parameter / intermediate bytes
		i++
	}
	if i == len(b) {
		return key{}, len(b)
	}
	params, final := string(b[2:i]), b[i]
	n := i + 1
	if !ss3 && strings.HasPrefix(params, "<") && (final == 'M' || final == 'm') {
		f := strings.Split(params[1:], ";")
		if len(f) != 3 {
			return key{}, n
		}
		btn, e1 := strconv.Atoi(f[0])
		x, e2 := strconv.Atoi(f[1])
		y, e3 := strconv.Atoi(f[2])
		if e1 != nil || e2 != nil || e3 != nil {
			return key{}, n
		}
		return key{name: "mouse", btn: btn, x: x - 1, y: y - 1, press: final == 'M'}, n
	}
	// modifiers ("1;5A" = Ctrl-Up) are ignored: the base key is enough here
	if p := strings.IndexByte(params, ';'); p >= 0 {
		params = params[:p]
	}
	if params == "1" && final != '~' {
		params = ""
	}
	return key{name: csiKeys[params+string(final)]}, n
}
