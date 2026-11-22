package termimg

import "math"

// layout is the on-screen geometry of an image.
type layout struct {
	cols, rows int // cell box occupied by the image
	pxW, pxH   int // target pixel size, aspect preserved, ≤ natural size
	cellW      int // effective cell width in pixels
	cellH      int // effective cell height in pixels
}

// fit scales a w×h pixel image to fit within maxCols×maxRows cells
// (non-positive limits are unlimited) without enlarging it and keeping its
// aspect ratio. Both cols and rows are at least 1.
func fit(w, h, maxCols, maxRows int, cell Cell) layout {
	cw, ch := cell.W, cell.H
	if cw <= 0 || ch <= 0 {
		cw, ch = defaultCellW, defaultCellH
	}
	tw, th := float64(w), float64(h)
	if maxCols > 0 && tw > float64(maxCols*cw) {
		th = th * float64(maxCols*cw) / tw
		tw = float64(maxCols * cw)
	}
	if maxRows > 0 && th > float64(maxRows*ch) {
		tw = tw * float64(maxRows*ch) / th
		th = float64(maxRows * ch)
	}
	return layout{
		cols:  clampInt(ceil(tw/float64(cw)), 1, maxCols),
		rows:  clampInt(ceil(th/float64(ch)), 1, maxRows),
		pxW:   max(1, int(math.Round(tw))),
		pxH:   max(1, int(math.Round(th))),
		cellW: cw,
		cellH: ch,
	}
}

// ceil rounds up, tolerating floating point noise just above an integer.
func ceil(f float64) int { return int(math.Ceil(f - 1e-9)) }

// clampInt clamps v to [lo, hi]; a non-positive hi means no upper bound.
func clampInt(v, lo, hi int) int {
	if hi > 0 && v > hi {
		v = hi
	}
	return max(v, lo)
}
