package termimg

import (
	"image"
	"math"
	"strconv"
)

// alphaCutoff: pixels with lower alpha are drawn as the terminal's default
// background, so transparent areas look right on dark and light themes.
// Pixels at or above it are drawn with their (straight) color.
const alphaCutoff = 128

// blocksGeometry returns the half-block pixel grid for an image: each cell is
// one grid pixel wide and two tall, a grid pixel being cellW × cellH/2 screen
// pixels.
func blocksGeometry(w, h, maxCols, maxRows int, cell Cell) (gw, gh int) {
	l := fit(w, h, maxCols, maxRows, cell)
	gw = clampInt(int(math.Round(float64(l.pxW)/float64(l.cellW))), 1, maxCols)
	gh = clampInt(int(math.Round(2*float64(l.pxH)/float64(l.cellH))), 1, 2*maxRows)
	return gw, gh
}

func renderBlocks(img *Image, maxCols, maxRows int, cell Cell) (Frame, error) {
	src, err := img.decode()
	if err != nil {
		return Frame{}, err
	}
	gw, gh := blocksGeometry(img.w, img.h, maxCols, maxRows, cell)
	return encodeBlocks(resize(src, gw, gh)), nil
}

// rgb is a packed 24-bit color; noColor means the terminal default.
type rgb int32

const noColor rgb = -1

func pixelColor(m *image.NRGBA, x, y int) rgb {
	if y >= m.Rect.Dy() {
		return noColor
	}
	p := m.Pix[y*m.Stride+x*4 : y*m.Stride+x*4+4 : y*m.Stride+x*4+4]
	if p[3] < alphaCutoff {
		return noColor
	}
	return rgb(p[0])<<16 | rgb(p[1])<<8 | rgb(p[2])
}

// encodeBlocks draws m with upper half blocks: foreground = top pixel,
// background = bottom pixel. Escape sequences are emitted only when a color
// changes, and every line ends with an SGR reset.
func encodeBlocks(m *image.NRGBA) Frame {
	w, h := m.Rect.Dx(), m.Rect.Dy()
	rows := (h + 1) / 2
	lines := make([]string, rows)
	buf := make([]byte, 0, w*24)
	for row := 0; row < rows; row++ {
		buf = buf[:0]
		fg, bg := noColor, noColor // state after the previous reset
		for x := 0; x < w; x++ {
			top, bot := pixelColor(m, x, 2*row), pixelColor(m, x, 2*row+1)
			var glyph string
			var wantFg, wantBg rgb
			switch {
			case top == noColor && bot == noColor:
				glyph, wantFg, wantBg = " ", fg, noColor // fg is irrelevant
			case top == noColor:
				glyph, wantFg, wantBg = "▄", bot, noColor
			case bot == noColor:
				glyph, wantFg, wantBg = "▀", top, noColor
			case top == bot && bg == top:
				glyph, wantFg, wantBg = " ", fg, bg // reuse background, fg irrelevant
			default:
				glyph, wantFg, wantBg = "▀", top, bot
			}
			if wantFg != fg {
				buf = appendSGR(buf, 38, wantFg)
				fg = wantFg
			}
			if wantBg != bg {
				buf = appendSGR(buf, 48, wantBg)
				bg = wantBg
			}
			buf = append(buf, glyph...)
		}
		buf = append(buf, "\x1b[0m"...)
		lines[row] = string(buf)
	}
	return Frame{Lines: lines, Cols: w}
}

// appendSGR appends a 24-bit color selection (base 38 = fg, 48 = bg), or the
// default color (39/49) for noColor.
func appendSGR(b []byte, base int, c rgb) []byte {
	b = append(b, "\x1b["...)
	if c == noColor {
		b = strconv.AppendInt(b, int64(base+1), 10)
		return append(b, 'm')
	}
	b = strconv.AppendInt(b, int64(base), 10)
	b = append(b, ";2;"...)
	b = strconv.AppendInt(b, int64(c>>16&0xff), 10)
	b = append(b, ';')
	b = strconv.AppendInt(b, int64(c>>8&0xff), 10)
	b = append(b, ';')
	b = strconv.AppendInt(b, int64(c&0xff), 10)
	return append(b, 'm')
}
