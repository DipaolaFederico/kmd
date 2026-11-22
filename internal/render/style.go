// Package render turns a parsed Markdown document into styled terminal text.
package render

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

// Color is a 24-bit RGB color. The zero value means "terminal default".
type Color uint32

// RGB builds a Color from a 0xRRGGBB value.
func RGB(hex uint32) Color { return Color(hex&0xffffff | 1<<24) }

// Text attributes.
const (
	Bold uint8 = 1 << iota
	Italic
	Underline
	Strike
	Faint
)

// Style describes how a run of text is painted.
type Style struct {
	Fg, Bg Color
	Attr   uint8
	URL    string
}

func (s Style) sgrEqual(o Style) bool { return s.Fg == o.Fg && s.Bg == o.Bg && s.Attr == o.Attr }

// painter serializes styled lines into escape sequences.
type painter struct {
	color, truecolor, links bool
	cache                   map[Style]string // SGR sequence per style (URL excluded)
}

func (p *painter) colorCode(b *strings.Builder, c Color, base int) {
	v := uint32(c)
	r, g, bl := v>>16&255, v>>8&255, v&255
	b.WriteByte(';')
	b.WriteString(strconv.Itoa(base))
	if p.truecolor {
		b.WriteString(";2;" + strconv.Itoa(int(r)) + ";" + strconv.Itoa(int(g)) + ";" + strconv.Itoa(int(bl)))
		return
	}
	b.WriteString(";5;" + strconv.Itoa(to256(r, g, bl)))
}

// to256 maps an RGB color to the closest xterm-256 palette index.
func to256(r, g, b uint32) int {
	if r == g && g == b {
		switch {
		case r < 8:
			return 16
		case r > 238:
			return 231
		}
		return 232 + int((r-8)/10)
	}
	q := func(v uint32) uint32 {
		if v < 48 {
			return 0
		}
		if v < 115 {
			return 1
		}
		return (v - 35) / 40
	}
	return int(16 + 36*q(r) + 6*q(g) + q(b))
}

func (p *painter) sgr(out *strings.Builder, s Style) {
	s.URL = ""
	if seq, ok := p.cache[s]; ok {
		out.WriteString(seq)
		return
	}
	var b strings.Builder
	b.WriteString("\x1b[0")
	for i, code := range [...]string{";1", ";3", ";4", ";9", ";2"} {
		if s.Attr&(1<<i) != 0 {
			b.WriteString(code)
		}
	}
	if s.Fg != 0 {
		p.colorCode(&b, s.Fg, 38)
	}
	if s.Bg != 0 {
		p.colorCode(&b, s.Bg, 48)
	}
	b.WriteByte('m')
	if p.cache == nil {
		p.cache = map[Style]string{}
	}
	p.cache[s] = b.String()
	out.WriteString(b.String())
}

// safeURL drops URLs that could terminate an OSC 8 sequence early.
func safeURL(u string) string {
	if len(u) > 2048 {
		return ""
	}
	if !utf8.ValidString(u) || hasInvisible(u) {
		return ""
	}
	for i := 0; i < len(u); i++ {
		if c := u[i]; c < 0x20 || c == 0x7f || (c == 0xc2 && i+1 < len(u) && u[i+1] < 0xa0) {
			return ""
		}
	}
	return u
}

// isBidiControl reports Unicode bidi embedding/override/isolate controls,
// which can visually reorder text (CVE-2021-42574, "Trojan Source").
func isBidiControl(r rune) bool { return (r >= 0x202a && r <= 0x202e) || (r >= 0x2066 && r <= 0x2069) }

// hasInvisible reports bidi controls and zero-width/format characters that
// have no business in a URL (spoofing the hover target).
func hasInvisible(s string) bool {
	for _, r := range s {
		if isBidiControl(r) || (r >= 0x200b && r <= 0x200f) || r == 0xfeff {
			return true
		}
	}
	return false
}

// clean removes characters that could drive the terminal (C0/C1 controls,
// DEL) or visually reorder it (bidi controls) from document text. Tabs
// become spaces.
func clean(s string) string {
	dirty := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < 0x20 || c == 0x7f || (c == 0xc2 && i+1 < len(s) && s[i+1] < 0xa0) ||
			(c == 0xe2 && i+2 < len(s) && ((s[i+1] == 0x80 && s[i+2] >= 0xaa && s[i+2] <= 0xae) || (s[i+1] == 0x81 && s[i+2] >= 0xa6 && s[i+2] <= 0xa9))) {
			dirty = true
			break
		}
	}
	if !dirty {
		return s
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\t':
			b.WriteByte(' ')
		case r < 0x20 || (r >= 0x7f && r < 0xa0) || isBidiControl(r):
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Clean is clean for other packages (e.g. file names in error messages).
func Clean(s string) string { return clean(s) }

func (p *painter) line(b *strings.Builder, l rline) {
	var cur Style
	reset := false // terminal state unknown after raw output
	url := ""
	for _, r := range l {
		if r.raw {
			if url != "" {
				b.WriteString("\x1b]8;;\x1b\\")
				url = ""
			}
			b.WriteString(r.text)
			cur, reset = Style{}, true
			continue
		}
		if r.text == "" {
			continue
		}
		if p.color && (reset || !r.st.sgrEqual(cur)) {
			p.sgr(b, r.st)
			cur, reset = r.st, false
		}
		if u := safeURL(r.st.URL); p.links && u != url {
			if url != "" {
				b.WriteString("\x1b]8;;\x1b\\")
			}
			if u != "" {
				b.WriteString("\x1b]8;;" + u + "\x1b\\")
			}
			url = u
		}
		b.WriteString(clean(r.text))
	}
	if url != "" {
		b.WriteString("\x1b]8;;\x1b\\")
	}
	if p.color && !cur.sgrEqual(Style{}) {
		b.WriteString("\x1b[0m")
	}
	b.WriteByte('\n')
}
