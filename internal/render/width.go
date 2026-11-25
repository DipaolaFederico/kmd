package render

import (
	"math"
	"strings"

	"github.com/DipaolaFederico/kmd/internal/styled"
)

func runeWidth(r rune) int         { return styled.RuneWidth(r) }
func strWidth(s string) int        { return styled.Width(s) }
func elide(s string, n int) string { return styled.Elide(s, n) }

// run is a piece of text painted with a single style. A raw run carries
// pre-built escape sequences (images) that occupy w columns.
type run struct {
	text string
	st   Style
	raw  bool
	// rows > 0: an image placed apart (Options.Placements), text its
	// sequence; uint16 fits in the padding after raw, keeping runs small
	rows uint16
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
func wrap(runs []run, width int) []rline { return wrapUpTo(runs, width, math.MaxInt) }

// wrapUpTo is wrap stopping after the first maxLines lines.
func wrapUpTo(runs []run, width, maxLines int) []rline {
	w := &wrapper{width: max(width, 1), maxLines: maxLines, word: make(rline, 0, 4)}
	for _, r := range runs {
		if w.full() {
			return w.lines
		}
		if r.text == "\n" {
			w.emit()
			w.push(w.cur)
			w.cur, w.curW, w.hasSpace = w.cur[:0], 0, false
			continue
		}
		w.add(r)
	}
	w.emit()
	w.push(w.cur)
	return w.lines
}

// wrapper is the state of wrap: the lines so far, the line being filled
// and the word being read.
type wrapper struct {
	width, maxLines int
	lines           []rline
	cur             rline // the line being filled; lines keeps a compact copy
	curW            int
	word            rline
	wordW           int
	space           run // the space before word, if hasSpace
	hasSpace        bool
}

func (w *wrapper) full() bool { return len(w.lines) >= w.maxLines }

func (w *wrapper) push(l rline) {
	if !w.full() {
		w.lines = append(w.lines, compact(l))
	}
}

// nextWord splits t, not empty, into its first word and the rest, or
// reports that t starts with spaces (word empty, rest after them).
func nextWord(t string) (word, rest string) {
	switch k := strings.IndexByte(t, ' '); {
	case k == 0:
		return "", strings.TrimLeft(t, " ")
	case k > 0:
		return t[:k], t[k:]
	}
	return t, ""
}

// add splits r into words and spaces.
func (w *wrapper) add(r run) {
	for t := r.text; len(t) > 0 && !w.full(); {
		var seg string
		if seg, t = nextWord(t); seg == "" {
			w.emit()
			w.hasSpace = true
			w.space = run{text: " ", st: r.st}
			continue
		}
		w.word = append(w.word, run{text: seg, st: r.st})
		w.wordW += strWidth(seg)
	}
}

// emit puts the word read on the line, or on the next one when it does
// not fit, cutting it when it is wider than a line.
func (w *wrapper) emit() {
	if len(w.word) == 0 {
		return
	}
	sw := 0
	if w.hasSpace && w.curW > 0 {
		sw = 1
	}
	if w.curW > 0 && w.curW+sw+w.wordW > w.width {
		w.push(w.cur)
		w.cur, w.curW, sw = w.cur[:0], 0, 0
	}
	if sw == 1 {
		w.cur = append(w.cur, w.space)
		w.curW++
	}
	if w.wordW > w.width-w.curW {
		parts := cut(append(rline{{text: strings.Repeat(" ", w.curW)}}, w.word...), w.width)
		if w.curW > 0 { // replace the placeholder padding with the real content
			parts[0] = append(w.cur, parts[0][1:]...)
		}
		for _, l := range parts[:len(parts)-1] {
			w.push(l)
		}
		w.cur = parts[len(parts)-1]
		w.curW = lineWidth(w.cur)
	} else {
		w.cur = append(w.cur, w.word...)
		w.curW += w.wordW
	}
	w.word, w.wordW, w.hasSpace = w.word[:0], 0, false
}

// naturalWidth is the width of the widest line wrap lays runs out in
// when nothing has to wrap, measured without laying them out: a table
// measures every cell, laid out or not.
func naturalWidth(runs []run) int {
	widest, w := 0, 0
	space := false // spaces since the last word
	for _, r := range runs {
		if r.text == "\n" {
			widest, w, space = max(widest, w), 0, false
			continue
		}
		for t := r.text; t != ""; {
			var seg string
			if seg, t = nextWord(t); seg == "" {
				space = true
				continue
			}
			if space && w > 0 { // spaces collapse to one, and none starts a line
				w++
			}
			space = false
			w += strWidth(seg)
		}
	}
	return max(widest, w)
}

// compact copies a line with each stretch of runs of the same style
// merged into one. wrap makes a run per word and per space (64 bytes
// each): kept as they are, a long paragraph costs dozens of bytes of
// layout per byte of text.
func compact(l rline) rline {
	if len(l) == 0 {
		return nil
	}
	n := 1
	for i := 1; i < len(l); i++ {
		if !sameStyle(l[i-1], l[i]) {
			n++
		}
	}
	out := make(rline, 0, n)
	for i := 0; i < len(l); {
		j, size := i+1, len(l[i].text)
		for ; j < len(l) && sameStyle(l[j-1], l[j]); j++ {
			size += len(l[j].text)
		}
		r := l[i]
		if j > i+1 {
			var b strings.Builder
			b.Grow(size)
			for _, x := range l[i:j] {
				b.WriteString(x.text)
			}
			r.text = b.String()
		}
		out = append(out, r)
		i = j
	}
	return out
}

// sameStyle reports whether a and b can be painted as one run.
func sameStyle(a, b run) bool {
	return a.st == b.st && !a.raw && !b.raw && a.rows == 0 && b.rows == 0
}
