package render

import (
	"strings"

	"github.com/DipaolaFederico/kmd/internal/styled"
)

func runeWidth(r rune) int  { return styled.RuneWidth(r) }
func strWidth(s string) int { return styled.Width(s) }

// run is a piece of text painted with a single style. A raw run carries
// pre-built escape sequences (images) that occupy w columns.
type run struct {
	text string
	st   Style
	raw  bool
	w    int
}

// rline is one rendered terminal line.
type rline []run

func lineWidth(l rline) int {
	w := 0
	for _, r := range l {
		if r.raw {
			w += r.w
		} else {
			w += strWidth(r.text)
		}
	}
	return w
}

// cut hard-splits a line into chunks of at most width columns.
func cut(l rline, width int) []rline {
	if width < 1 {
		width = 1
	}
	var out []rline
	var cur rline
	w := 0
	for _, r := range l {
		start := 0
		for idx, ch := range r.text {
			cw := runeWidth(ch)
			if w+cw > width && w > 0 {
				if idx > start {
					cur = append(cur, run{text: r.text[start:idx], st: r.st})
				}
				out = append(out, cur)
				cur, w, start = nil, 0, idx
			}
			w += cw
		}
		if start < len(r.text) {
			cur = append(cur, run{text: r.text[start:], st: r.st})
		}
	}
	return append(out, cur)
}

// wrap lays styled runs out as word-wrapped lines of at most width columns.
// A run whose text is "\n" forces a line break.
func wrap(runs []run, width int) []rline {
	if width < 1 {
		width = 1
	}
	var lines []rline
	var cur rline
	word := make(rline, 0, 4)
	curW, wordW := 0, 0
	var space run
	hasSpace := false
	emit := func() {
		if len(word) == 0 {
			return
		}
		sw := 0
		if hasSpace && curW > 0 {
			sw = 1
		}
		if curW > 0 && curW+sw+wordW > width {
			lines = append(lines, cur)
			cur, curW, sw = nil, 0, 0
		}
		if sw == 1 {
			cur = append(cur, space)
			curW++
		}
		if wordW > width-curW {
			parts := cut(append(rline{{text: strings.Repeat(" ", curW)}}, word...), width)
			if curW > 0 { // replace the placeholder padding with the real content
				parts[0] = append(cur, parts[0][1:]...)
			}
			lines = append(lines, parts[:len(parts)-1]...)
			cur = parts[len(parts)-1]
			curW = lineWidth(cur)
		} else {
			cur = append(cur, word...)
			curW += wordW
		}
		word, wordW, hasSpace = word[:0], 0, false
	}
	for _, r := range runs {
		if r.text == "\n" {
			emit()
			lines = append(lines, cur)
			cur, curW, hasSpace = nil, 0, false
			continue
		}
		t := r.text
		for len(t) > 0 {
			k := strings.IndexByte(t, ' ')
			if k == 0 {
				emit()
				hasSpace = true
				space = run{text: " ", st: r.st}
				t = strings.TrimLeft(t, " ")
				continue
			}
			seg := t
			if k > 0 {
				seg, t = t[:k], t[k:]
			} else {
				t = ""
			}
			word = append(word, run{text: seg, st: r.st})
			wordW += strWidth(seg)
		}
	}
	emit()
	return append(lines, cur)
}
