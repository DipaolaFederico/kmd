package termimg

import (
	"image"
	"strconv"
)

// Fixed sixel palette: a 6×7×6 RGB cube (252 colors, green gets the extra
// level because the eye is most sensitive to it) with 4×4 ordered dithering.
// A fixed palette needs no analysis pass, which keeps encoding fast.
const (
	levelsR     = 6
	levelsG     = 7
	levelsB     = 6
	paletteSize = levelsR * levelsG * levelsB
	transparent = 0xff // palette index marking a transparent pixel
)

// bayer4 is the 4×4 ordered dithering matrix (values 0..15).
var bayer4 = [16]uint8{0, 8, 2, 10, 12, 4, 14, 6, 3, 11, 1, 9, 15, 7, 13, 5}

func renderSixel(img *Image, l layout) (Frame, error) {
	src, err := img.decode()
	if err != nil {
		return Frame{}, err
	}
	m := resize(src, l.pxW, l.pxH)
	return Frame{Lines: []string{string(encodeSixel(m))}, Cols: l.cols}, nil
}

// quantLUT maps a channel value to a cube level for each dither threshold:
// level = ⌊v·(L−1)/255 + (2t+1)/32⌋.
func quantLUT(levels int) *[16][256]uint8 {
	var lut [16][256]uint8
	for t := range lut {
		for v := range lut[t] {
			lut[t][v] = uint8((v*(levels-1)*32 + (2*t+1)*255) / (255 * 32)) // #nosec G115 -- result < levels ≤ 7
		}
	}
	return &lut
}

// quantize maps every pixel of m to a palette index (transparent for alpha
// below alphaCutoff) and reports which palette entries are used.
func quantize(m *image.NRGBA) (idx []uint8, used *[paletteSize]bool) {
	w, h := m.Rect.Dx(), m.Rect.Dy()
	lr, lg := quantLUT(levelsR), quantLUT(levelsG)
	lb := lr // levelsB == levelsR
	idx = make([]uint8, w*h)
	used = new([paletteSize]bool)
	for y := 0; y < h; y++ {
		row := m.Pix[y*m.Stride : y*m.Stride+w*4]
		out := idx[y*w : y*w+w]
		for x := range out {
			p := row[x*4 : x*4+4 : x*4+4]
			if p[3] < alphaCutoff {
				out[x] = transparent
				continue
			}
			t := bayer4[(y&3)<<2|x&3]
			c := lr[t][p[0]]*levelsG*levelsB + lg[t][p[1]]*levelsB + lb[t][p[2]]
			out[x] = c
			used[c] = true
		}
	}
	return idx, used
}

// encodeSixel encodes m as a DEC sixel image: P2=1 keeps unset pixels
// transparent, raster attributes declare the size, palette registers are
// defined in percent RGB, and runs of identical sixels are RLE-compressed.
func encodeSixel(m *image.NRGBA) []byte {
	w, h := m.Rect.Dx(), m.Rect.Dy()
	idx, used := quantize(m)

	out := make([]byte, 0, 64+w*h/2)
	out = append(out, "\x1bP0;1;0q\"1;1;"...)
	out = strconv.AppendInt(out, int64(w), 10)
	out = append(out, ';')
	out = strconv.AppendInt(out, int64(h), 10)
	for c := range used {
		if !used[c] {
			continue
		}
		r, g, b := c/(levelsG*levelsB), c/levelsB%levelsG, c%levelsB
		out = append(out, '#')
		out = strconv.AppendInt(out, int64(c), 10)
		out = append(out, ";2;"...)
		out = strconv.AppendInt(out, int64((r*100+(levelsR-1)/2)/(levelsR-1)), 10)
		out = append(out, ';')
		out = strconv.AppendInt(out, int64((g*100+(levelsG-1)/2)/(levelsG-1)), 10)
		out = append(out, ';')
		out = strconv.AppendInt(out, int64((b*100+(levelsB-1)/2)/(levelsB-1)), 10)
	}

	// Per band: bits[c*w+x] holds the 6 vertical pixels of color c at x;
	// only the [minX, maxX] span of colors present in the band is touched.
	bits := make([]uint8, paletteSize*w)
	var minX, maxX [paletteSize]int
	var present [paletteSize]bool
	colors := make([]uint8, 0, paletteSize)
	for y0 := 0; y0 < h; y0 += 6 {
		for r := 0; r < 6 && y0+r < h; r++ {
			row := idx[(y0+r)*w : (y0+r)*w+w]
			for x, c := range row {
				if c == transparent {
					continue
				}
				bits[int(c)*w+x] |= 1 << r
				switch {
				case !present[c]:
					present[c] = true
					minX[c], maxX[c] = x, x
					colors = append(colors, c)
				case x < minX[c]:
					minX[c] = x
				case x > maxX[c]:
					maxX[c] = x
				}
			}
		}
		for i, c := range colors {
			if i > 0 {
				out = append(out, '$') // carriage return: overlay next color
			}
			out = append(out, '#')
			out = strconv.AppendInt(out, int64(c), 10)
			out = appendRun(out, '?', minX[c])
			span := bits[int(c)*w+minX[c] : int(c)*w+maxX[c]+1]
			for j := 0; j < len(span); {
				k := j + 1
				for k < len(span) && span[k] == span[j] {
					k++
				}
				out = appendRun(out, '?'+span[j], k-j)
				j = k
			}
			clear(span)
			present[c] = false
		}
		colors = colors[:0]
		if y0+6 < h {
			out = append(out, '-') // next band
		}
	}
	return append(out, "\x1b\\"...)
}

// appendRun appends n copies of sixel character ch, RLE-encoded when shorter.
func appendRun(b []byte, ch byte, n int) []byte {
	if n > 3 {
		b = append(b, '!')
		b = strconv.AppendInt(b, int64(n), 10)
		return append(b, ch)
	}
	for ; n > 0; n-- {
		b = append(b, ch)
	}
	return b
}
