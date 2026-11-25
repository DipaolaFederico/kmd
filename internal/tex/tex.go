// Package tex renders LaTeX math for the terminal using Unicode: Inline
// writes a formula on one line (x², √x, α, ½), Display lays it out in 2D
// (stacked fractions, limits above and below big operators, tall
// delimiters, matrices).
//
// The renderer is a small recursive-descent parser producing a tree of
// nodes, plus two back ends: a linear one (Inline) and a box layout one
// (Display) where boxes carry a width, a height and a baseline. Input is
// untrusted: rendering never panics, recursion and input size are bounded,
// and control characters are dropped. Unknown commands are shown verbatim
// with the Muted class.
package tex

import (
	"strings"
	"unicode/utf8"

	"github.com/DipaolaFederico/kmd/internal/styled"
)

// Inline renders a LaTeX formula on a single line using Unicode
// (x^2 → x², \frac{a}{b} → a/b or (a+1)/(b+c), \sqrt{x} → √x, \alpha → α).
func Inline(src string) styled.Line {
	if len(src) > maxSource {
		return verbatim(src)
	}
	root, tag, ok := parse(src)
	if !ok {
		return verbatim(src)
	}
	r := linear{}
	r.node(root)
	if tag != "" {
		r.emit("  ("+tag+")", styled.Plain)
	}
	return trimRight(merge(r.out))
}

// trimRight drops trailing blanks (left by a dangling operator).
func trimRight(l styled.Line) styled.Line {
	for len(l) > 0 {
		last := &l[len(l)-1]
		last.Text = strings.TrimRight(last.Text, " ")
		if last.Text != "" {
			break
		}
		l = l[:len(l)-1]
	}
	if len(l) == 0 {
		return nil
	}
	return l
}

// Display renders a LaTeX formula in 2D (stacked fractions, limits above
// and below big operators, tall delimiters, matrices) within width columns.
// All returned lines have the same width. If the 2D layout cannot fit,
// it falls back to the Inline rendering wrapped to width.
func Display(src string, width int) []styled.Line {
	width = max(width, 1)
	if len(src) > maxSource {
		return wrap(verbatim(src), width)
	}
	root, tag, ok := parse(src)
	if !ok {
		return wrap(verbatim(src), width)
	}
	if height(root) > maxRows {
		return wrap(Inline(src), width)
	}
	b := layout(root, false)
	if b.w == 0 {
		return nil
	}
	if tag != "" {
		t := textBox("("+tag+")", styled.Plain)
		if gap := width - b.w - t.w; gap >= 2 {
			t.base = 0
			b = hcat(b, blankBox(gap, 1), t)
		}
	}
	if b.w > width {
		return wrap(Inline(src), width)
	}
	out := make([]styled.Line, len(b.rows))
	for i, row := range b.rows {
		out[i] = merge(row)
	}
	return out
}

// verbatim shows the source itself, with control characters blanked.
func verbatim(src string) styled.Line {
	var b strings.Builder
	for _, r := range src {
		if isControl(r) {
			r = ' '
		}
		b.WriteRune(r)
	}
	return styled.Line{{Text: b.String(), Class: styled.Muted}}
}

// merge joins adjacent spans of the same class and drops empty ones.
func merge(l styled.Line) styled.Line {
	if len(l) == 0 {
		return nil
	}
	out := make(styled.Line, 0, len(l))
	var b strings.Builder
	cur := l[0].Class
	flush := func() {
		if b.Len() > 0 {
			out = append(out, styled.Span{Text: b.String(), Class: cur})
			b.Reset()
		}
	}
	for _, s := range l {
		if s.Text == "" {
			continue
		}
		if s.Class != cur {
			flush()
			cur = s.Class
		}
		b.WriteString(s.Text)
	}
	flush()
	return out
}

// cell is one character (with its combining marks) of a line being wrapped.
type cell struct {
	text  string
	class styled.Class
	w     int
}

// maxForeignMarks bounds the zero-width runes a cell takes from a later
// span: they cannot be sliced from the cell's own span, so they are
// copied, and copying an unbounded run of them would be quadratic.
const maxForeignMarks = 64

// cellsOf splits l into characters, each with its zero-width runes
// (combining marks).
func cellsOf(l styled.Line) []cell {
	var cells []cell
	owner, start := -1, 0 // span and offset where the last cell starts
	for k, s := range l {
		for i := 0; i < len(s.Text); {
			r, size := utf8.DecodeRuneInString(s.Text[i:])
			t := s.Text[i : i+size]
			i += size
			if n := len(cells); n > 0 && styled.RuneWidth(r) == 0 {
				// a longer slice of the same span costs nothing, however
				// many follow (the verbatim fallback of a hostile formula
				// is one span of thousands of them)
				switch c := &cells[n-1]; {
				case owner == k:
					c.text = s.Text[start:i]
				case len(c.text) < maxForeignMarks:
					c.text += t
				}
				continue
			}
			cells = append(cells, cell{text: t, class: s.Class})
			owner, start = k, i-size
		}
	}
	return cells
}

// wrap breaks l into lines at most width columns wide, preferring breaks
// at spaces, and pads them to a common width.
func wrap(l styled.Line, width int) []styled.Line {
	cells := cellsOf(l)
	for i := range cells {
		cells[i].w = styled.Width(cells[i].text)
	}
	var lines []styled.Line
	widest := 0
	for start := 0; ; {
		for start < len(cells) && cells[start].text == " " {
			start++
		}
		if start >= len(cells) {
			break
		}
		end, w, brk := start, 0, -1
		for end < len(cells) && w+cells[end].w <= width {
			if cells[end].text == " " {
				brk = end
			}
			w += cells[end].w
			end++
		}
		switch {
		case end == start: // a single character wider than width
			end++
		case end < len(cells) && cells[end].text != " " && brk > start:
			end = brk
		}
		stop := end
		for stop > start && cells[stop-1].text == " " {
			stop--
		}
		line := make(styled.Line, 0, stop-start)
		for _, c := range cells[start:stop] {
			line = append(line, styled.Span{Text: c.text, Class: c.class})
		}
		line = merge(line)
		widest = max(widest, styled.LineWidth(line))
		lines = append(lines, line)
		start = end
	}
	for i, line := range lines {
		if pad := widest - styled.LineWidth(line); pad > 0 {
			lines[i] = merge(append(line, styled.Span{Text: spaces(pad)}))
		}
	}
	return lines
}
