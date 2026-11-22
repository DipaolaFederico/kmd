package diagram

import (
	"strings"

	"github.com/DipaolaFederico/kmd/internal/styled"
)

// lineStyle is the stroke used for connector lines.
type lineStyle uint8

const (
	styleSolid lineStyle = iota
	styleDotted
	styleThick
)

// Direction bits a line cell connects to.
const (
	bitUp uint8 = 1 << iota
	bitDown
	bitLeft
	bitRight
)

// Junction glyphs indexed by the combination of direction bits.
var (
	glyphSolid  = [16]rune{' ', '│', '│', '│', '─', '╯', '╮', '┤', '─', '╰', '╭', '├', '─', '┴', '┬', '┼'}
	glyphDotted = [16]rune{' ', '┆', '┆', '┆', '┄', '╯', '╮', '┤', '┄', '╰', '╭', '├', '┄', '┴', '┬', '┼'}
	glyphThick  = [16]rune{' ', '┃', '┃', '┃', '━', '┛', '┓', '┫', '━', '┗', '┏', '┣', '━', '┻', '┳', '╋'}
)

// junction returns the box drawing rune for a set of direction bits.
func junction(bits uint8, st lineStyle) rune {
	switch st {
	case styleDotted:
		return glyphDotted[bits&15]
	case styleThick:
		return glyphThick[bits&15]
	}
	return glyphSolid[bits&15]
}

// cell is one terminal column of the canvas. A cell either holds text (r is
// non-zero), line directions to be merged into a junction glyph, or nothing.
type cell struct {
	r     rune
	cls   styled.Class
	bits  uint8
	style lineStyle
	cont  bool // right half of a double-width rune
}

// canvas is a fixed-size character grid that merges crossing lines.
type canvas struct {
	w, h  int
	cells []cell
}

func newCanvas(w, h int) *canvas {
	return &canvas{w: w, h: h, cells: make([]cell, w*h)}
}

func (c *canvas) at(x, y int) *cell {
	if x < 0 || y < 0 || x >= c.w || y >= c.h {
		return nil
	}
	return &c.cells[y*c.w+x]
}

// clear empties a cell, blanking the other half of a wide rune it touches.
func (c *canvas) clear(x, y int) {
	p := c.at(x, y)
	if p == nil {
		return
	}
	if p.cont {
		if l := c.at(x-1, y); l != nil {
			l.r = ' '
		}
	}
	if p.r != 0 && styled.RuneWidth(p.r) == 2 {
		if q := c.at(x+1, y); q != nil && q.cont {
			q.cont = false
			q.r = ' '
		}
	}
	*p = cell{}
}

// put writes one rune and returns the number of columns it occupies.
func (c *canvas) put(x, y int, r rune, cls styled.Class) int {
	rw := styled.RuneWidth(r)
	if rw == 0 {
		return 0
	}
	if c.at(x, y) == nil {
		return rw
	}
	if rw == 2 && x+1 >= c.w {
		r, rw = '…', 1
	}
	c.clear(x, y)
	if rw == 2 {
		c.clear(x+1, y)
		c.at(x+1, y).cont = true
	}
	p := c.at(x, y)
	p.r, p.cls = r, cls
	return rw
}

// text writes s starting at column x and returns the column after it.
func (c *canvas) text(x, y int, s string, cls styled.Class) int {
	for _, r := range s {
		x += c.put(x, y, r, cls)
	}
	return x
}

// fill writes n copies of r starting at column x.
func (c *canvas) fill(x, y, n int, r rune, cls styled.Class) {
	for i := 0; i < n; i++ {
		c.put(x+i, y, r, cls)
	}
}

// rune returns the text rune at a cell, or 0.
func (c *canvas) rune(x, y int) rune {
	if p := c.at(x, y); p != nil {
		return p.r
	}
	return 0
}

// addBits merges line directions into a cell; text cells are left alone.
func (c *canvas) addBits(x, y int, b uint8, st lineStyle, cls styled.Class) {
	p := c.at(x, y)
	if p == nil || p.r != 0 || p.cont {
		return
	}
	if p.bits == 0 {
		p.style = st
	} else if p.style != st {
		p.style = styleSolid
	}
	p.bits |= b
	p.cls = cls
}

// seg draws a horizontal or vertical line between two points (inclusive).
func (c *canvas) seg(x0, y0, x1, y1 int, st lineStyle, cls styled.Class) {
	switch {
	case y0 == y1 && x0 != x1:
		lo, hi := min(x0, x1), max(x0, x1)
		for x := lo; x <= hi; x++ {
			var b uint8
			if x > lo {
				b |= bitLeft
			}
			if x < hi {
				b |= bitRight
			}
			c.addBits(x, y0, b, st, cls)
		}
	case x0 == x1 && y0 != y1:
		lo, hi := min(y0, y1), max(y0, y1)
		for y := lo; y <= hi; y++ {
			var b uint8
			if y > lo {
				b |= bitUp
			}
			if y < hi {
				b |= bitDown
			}
			c.addBits(x0, y, b, st, cls)
		}
	}
}

// lines converts the canvas into styled lines without trailing blanks and
// without leading or trailing empty rows.
func (c *canvas) lines() []styled.Line {
	out := make([]styled.Line, 0, c.h)
	var sb strings.Builder
	for y := 0; y < c.h; y++ {
		row := c.cells[y*c.w : (y+1)*c.w]
		end := len(row)
		for end > 0 && blank(row[end-1]) {
			end--
		}
		var line styled.Line
		cur := styled.Plain
		sb.Reset()
		for x := 0; x < end; x++ {
			cl := row[x]
			if cl.cont {
				continue
			}
			r, cls := ' ', cur
			switch {
			case cl.r != 0 && cl.r != ' ':
				r, cls = cl.r, cl.cls
			case cl.r == 0 && cl.bits != 0:
				r, cls = junction(cl.bits, cl.style), cl.cls
			}
			if cls != cur && sb.Len() > 0 {
				line = append(line, styled.Span{Text: sb.String(), Class: cur})
				sb.Reset()
			}
			cur = cls
			sb.WriteRune(r)
		}
		if sb.Len() > 0 {
			line = append(line, styled.Span{Text: sb.String(), Class: cur})
		}
		out = append(out, line)
	}
	for len(out) > 0 && len(out[len(out)-1]) == 0 {
		out = out[:len(out)-1]
	}
	for len(out) > 0 && len(out[0]) == 0 {
		out = out[1:]
	}
	return out
}

func blank(cl cell) bool {
	return !cl.cont && (cl.r == 0 || cl.r == ' ') && (cl.r != 0 || cl.bits == 0)
}
