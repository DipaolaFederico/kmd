// Package styled defines the small vocabulary shared by kmd's sub-renderers
// (diagrams, math, images): text spans tagged with a semantic class that the
// terminal renderer maps to theme colors, plus Unicode-aware width helpers.
package styled

import (
	"sort"
	"unicode"
	"unicode/utf8"
)

// Class is the semantic role of a span; the renderer maps it to a theme color.
type Class uint8

// Span classes. Sub-renderers only pick a role; colors come from the theme.
const (
	Plain   Class = iota // default text color
	Muted                // secondary text, hints, fallbacks
	Border               // frames, boxes, lifelines
	Edge                 // connectors between nodes
	Arrow                // arrow heads
	Label                // edge / message labels
	Title                // diagram titles, headers
	Node                 // text inside nodes / participants
	Var                  // math variables (rendered italic)
	Num                  // numbers
	Op                   // operators and relations
	Symbol               // big operators, delimiters, special symbols
	Func                 // named functions (sin, log, lim)
	Series0              // Series0..Series7: categorical colors (charts)
	Series1
	Series2
	Series3
	Series4
	Series5
	Series6
	Series7
)

// Series returns the categorical class for index i (cycling through 8).
func Series(i int) Class {
	series := [...]Class{Series0, Series1, Series2, Series3, Series4, Series5, Series6, Series7}
	return series[(i%8+8)%8]
}

// Span is a run of text with a single class.
type Span struct {
	Text  string
	Class Class
}

// Line is one terminal line made of spans.
type Line []Span

// LineWidth returns the number of columns a line occupies.
func LineWidth(l Line) int {
	w := 0
	for _, s := range l {
		w += Width(s.Text)
	}
	return w
}

// wide lists the code point ranges terminals render two columns wide
// (East Asian Wide/Fullwidth and emoji presentation).
var wide = [...][2]rune{
	{0x1100, 0x115f}, {0x231a, 0x231b}, {0x2329, 0x232a}, {0x23e9, 0x23ec}, {0x23f0, 0x23f0},
	{0x23f3, 0x23f3}, {0x25fd, 0x25fe}, {0x2614, 0x2615}, {0x2648, 0x2653}, {0x267f, 0x267f},
	{0x2693, 0x2693}, {0x26a1, 0x26a1}, {0x26aa, 0x26ab}, {0x26bd, 0x26be}, {0x26c4, 0x26c5},
	{0x26ce, 0x26ce}, {0x26d4, 0x26d4}, {0x26ea, 0x26ea}, {0x26f2, 0x26f3}, {0x26f5, 0x26f5},
	{0x26fa, 0x26fa}, {0x26fd, 0x26fd}, {0x2705, 0x2705}, {0x270a, 0x270b}, {0x2728, 0x2728},
	{0x274c, 0x274c}, {0x274e, 0x274e}, {0x2753, 0x2755}, {0x2757, 0x2757}, {0x2795, 0x2797},
	{0x27b0, 0x27b0}, {0x27bf, 0x27bf}, {0x2b1b, 0x2b1c}, {0x2b50, 0x2b50}, {0x2b55, 0x2b55},
	{0x2e80, 0x303e}, {0x3041, 0x33ff}, {0x3400, 0x4dbf}, {0x4e00, 0x9fff}, {0xa000, 0xa4cf},
	{0xa960, 0xa97f}, {0xac00, 0xd7a3}, {0xf900, 0xfaff}, {0xfe10, 0xfe19}, {0xfe30, 0xfe6f},
	{0xff00, 0xff60}, {0xffe0, 0xffe6}, {0x1f004, 0x1f004}, {0x1f0cf, 0x1f0cf}, {0x1f18e, 0x1f18e},
	{0x1f191, 0x1f19a}, {0x1f200, 0x1f202}, {0x1f210, 0x1f23b}, {0x1f240, 0x1f248}, {0x1f250, 0x1f251},
	{0x1f260, 0x1f265}, {0x1f300, 0x1f64f}, {0x1f680, 0x1f6ff}, {0x1f7e0, 0x1f7eb}, {0x1f90c, 0x1f9ff},
	{0x1fa70, 0x1faff}, {0x20000, 0x2fffd}, {0x30000, 0x3fffd},
}

// RuneWidth returns the number of terminal columns r occupies (0, 1 or 2).
func RuneWidth(r rune) int {
	switch {
	case r == '\t':
		return 1 // painted as a single space
	case r < 0x20:
		return 0
	case r < 0x7f:
		return 1
	case r < 0xa0:
		return 0
	case r == 0x200b || r == 0x200d || (r >= 0xfe00 && r <= 0xfe0f):
		return 0
	case r < 0x300:
		return 1
	case unicode.In(r, unicode.Mn, unicode.Me, unicode.Cf):
		return 0
	}
	i := sort.Search(len(wide), func(i int) bool { return wide[i][1] >= r })
	if i < len(wide) && r >= wide[i][0] {
		return 2
	}
	return 1
}

// Width returns the number of terminal columns s occupies.
func Width(s string) int {
	ascii := true
	for i := 0; i < len(s); i++ {
		if c := s[i]; c >= utf8.RuneSelf || (c < 0x20 && c != '\t') || c == 0x7f {
			ascii = false
			break
		}
	}
	if ascii {
		return len(s)
	}
	w, last := 0, 0
	for _, r := range s {
		if r == 0xfe0f && last == 1 { // emoji presentation selector widens the base
			w++
			last = 2
			continue
		}
		last = RuneWidth(r)
		w += last
	}
	return w
}
