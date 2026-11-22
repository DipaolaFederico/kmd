package diagram

import (
	"github.com/DipaolaFederico/kmd/internal/styled"
)

// shape is the visual style of a graph node.
type shape uint8

const (
	shapeRect       shape = iota // A[text]
	shapeRound                   // A(text)
	shapeStadium                 // A([text])
	shapeSubroutine              // A[[text]]
	shapeDatabase                // A[(text)]
	shapeCircle                  // A((text))
	shapeDecision                // A{text}
	shapeHexagon                 // A{{text}}
	shapeAsym                    // A>text]
	shapePara                    // A[/text/]
	shapeParaAlt                 // A[\text\]
	shapeStart                   // state diagram start marker
	shapeEnd                     // state diagram end marker
	shapeBar                     // state diagram fork/join bar
)

// box is a sized node: its shape, wrapped label and outer dimensions.
type box struct {
	shape shape
	lines []string
	w, h  int
	lr    bool
}

// makeBox wraps label to max columns and computes the node's size.
func makeBox(sh shape, label string, max int, lr bool) box {
	b := box{shape: sh, lr: lr}
	switch sh {
	case shapeStart, shapeEnd:
		b.w, b.h = 1, 1
		return b
	case shapeBar:
		if lr {
			b.w, b.h = 1, 3
		} else {
			b.w, b.h = 7, 1
		}
		return b
	}
	b.lines = wrap(label, max)
	nl := len(b.lines)
	b.w, b.h = maxWidth(b.lines)+4, nl+2
	switch sh {
	case shapeDatabase:
		b.h = nl + 3
	case shapeCircle:
		b.h = nl + 4
	case shapePara, shapeParaAlt:
		b.w += b.h - 1
	}
	return b
}

// extent returns the first and last column the outline occupies on row r.
func (b *box) extent(r int) (int, int) {
	switch b.shape {
	case shapePara:
		return b.h - 1 - r, b.w - 1 - r
	case shapeParaAlt:
		return r, b.w - b.h + r
	case shapeCircle, shapeHexagon:
		if r == 0 || r == b.h-1 {
			return 1, b.w - 2
		}
	}
	return 0, b.w - 1
}

// tbRange returns the columns usable as ports on the top or bottom side.
func (b *box) tbRange(bottom bool) (int, int) {
	lo, hi := 1, b.w-2
	switch {
	case b.w < 3:
		lo, hi = 0, b.w-1
	case b.shape == shapeCircle:
		lo, hi = 2, b.w-3
	case b.shape == shapePara && !bottom, b.shape == shapeParaAlt && bottom:
		lo, hi = b.h, b.w-2
	case b.shape == shapePara, b.shape == shapeParaAlt:
		lo, hi = 1, b.w-b.h-1
	}
	if lo > hi {
		lo = b.w / 2
		hi = lo
	}
	return lo, hi
}

// lrRange returns the rows usable as ports on the left or right side.
func (b *box) lrRange() (int, int) {
	lo, hi := 1, b.h-2
	switch {
	case b.h < 3:
		lo, hi = 0, b.h-1
	case b.shape == shapeDatabase:
		lo = 2
	case b.shape == shapeCircle:
		lo, hi = 2, b.h-3
	}
	if lo > hi {
		lo = b.h / 2
		hi = lo
	}
	return lo, hi
}

// outline returns the left char, right char and border fill for row r.
func (b *box) outline(r int) (l, rt, fill rune) {
	top, bot := r == 0, r == b.h-1
	mid := r == b.h/2
	pick := func(t1, t2, b1, b2, m1, m2 rune) (rune, rune, rune) {
		switch {
		case top:
			return t1, t2, '─'
		case bot:
			return b1, b2, '─'
		}
		return m1, m2, ' '
	}
	switch b.shape {
	case shapeRound:
		return pick('╭', '╮', '╰', '╯', '│', '│')
	case shapeStadium:
		return pick('╭', '╮', '╰', '╯', '(', ')')
	case shapeSubroutine:
		return pick('╓', '╖', '╙', '╜', '║', '║')
	case shapeDatabase:
		if r == 1 {
			return '├', '┤', '─'
		}
		return pick('╭', '╮', '╰', '╯', '│', '│')
	case shapeCircle:
		return pick('╭', '╮', '╰', '╯', '│', '│')
	case shapeDecision:
		if mid && !top && !bot {
			return '<', '>', ' '
		}
		return pick('╱', '╲', '╲', '╱', '│', '│')
	case shapeHexagon:
		if mid && !top && !bot {
			return '<', '>', ' '
		}
		return pick('─', '─', '─', '─', '│', '│')
	case shapeAsym:
		if mid && !top && !bot {
			return '>', '│', ' '
		}
		return pick('╲', '┐', '╱', '┘', '│', '│')
	case shapePara:
		return pick('╱', '╱', '╱', '╱', '╱', '╱')
	case shapeParaAlt:
		return pick('╲', '╲', '╲', '╲', '╲', '╲')
	}
	return pick('┌', '┐', '└', '┘', '│', '│')
}

// draw paints the node outline and its centered label at (x, y).
func (b *box) draw(c *canvas, x, y int) {
	switch b.shape {
	case shapeStart:
		c.put(x, y, '●', styled.Node)
		return
	case shapeEnd:
		c.put(x, y, '◉', styled.Node)
		return
	case shapeBar:
		for i := 0; i < b.w; i++ {
			for j := 0; j < b.h; j++ {
				if b.lr {
					c.put(x+i, y+j, '┃', styled.Border)
				} else {
					c.put(x+i, y+j, '━', styled.Border)
				}
			}
		}
		return
	}
	for r := 0; r < b.h; r++ {
		lc, rc := b.extent(r)
		l, rt, fill := b.outline(r)
		cls := styled.Border
		c.put(x+lc, y+r, l, cls)
		c.fill(x+lc+1, y+r, rc-lc-1, fill, cls)
		c.put(x+rc, y+r, rt, cls)
	}
	if b.shape == shapeCircle && b.h >= 4 {
		c.put(x, y+1, '╭', styled.Border)
		c.put(x+1, y+1, '╯', styled.Border)
		c.put(x+b.w-2, y+1, '╰', styled.Border)
		c.put(x+b.w-1, y+1, '╮', styled.Border)
		c.put(x, y+b.h-2, '╰', styled.Border)
		c.put(x+1, y+b.h-2, '╮', styled.Border)
		c.put(x+b.w-2, y+b.h-2, '╭', styled.Border)
		c.put(x+b.w-1, y+b.h-2, '╯', styled.Border)
	}
	first, last := 1, b.h-2
	switch b.shape {
	case shapeDatabase:
		first = 2
	case shapeCircle:
		first, last = 2, b.h-3
	}
	top := first + (last-first+1-len(b.lines))/2
	for i, line := range b.lines {
		r := top + i
		lc, rc := b.extent(r)
		inner := rc - lc - 1
		c.text(x+lc+1+(inner-styled.Width(line))/2, y+r, line, styled.Node)
	}
}

// portGlyph returns the border rune to use where a line meets the outline,
// or the current rune when the outline is not a plain straight side.
func portGlyph(cur rune, side uint8, st lineStyle) rune {
	thick := st == styleThick
	switch {
	case cur == '─' && side == bitDown:
		if thick {
			return '┰'
		}
		return '┬'
	case cur == '─' && side == bitUp:
		if thick {
			return '┸'
		}
		return '┴'
	case cur == '│' && side == bitRight:
		if thick {
			return '┝'
		}
		return '├'
	case cur == '│' && side == bitLeft:
		if thick {
			return '┥'
		}
		return '┤'
	}
	return cur
}
