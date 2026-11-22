package termimg

import (
	"image"
	"image/color"
)

// resize downsamples src to w×h pixels with a box filter (each destination
// pixel is the alpha-weighted mean of the source pixels it covers) and
// returns straight-alpha pixels. When a dimension grows, nearest-neighbour
// sampling is used for it. Every source row is converted once per
// destination row it contributes to, so downscaling costs O(source pixels).
func resize(src image.Image, w, h int) *image.NRGBA {
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	if sw <= 0 || sh <= 0 {
		return dst
	}
	xs := make([]int, w+1) // source column boundaries of each destination column
	for x := range xs {
		xs[x] = x * sw / w
	}
	row := make([]uint8, sw*4) // one source row, premultiplied RGBA
	sum := make([]uint64, w*4)
	fetch := rowFetcher(src)
	ns := make([]uint64, w)   // box area of each destination column
	invs := make([]uint64, w) // ⌈2³²/area⌉, recomputed when the box height changes
	lastRows := 0
	for y := 0; y < h; y++ {
		y0, y1 := span(y, sh, h)
		if y1-y0 != lastRows {
			lastRows = y1 - y0
			for x := range ns {
				// #nosec G115 -- both spans are positive (span/max guarantee ≥ 1).
				ns[x] = uint64(max(xs[x+1], xs[x]+1)-xs[x]) * uint64(lastRows)
				invs[x] = (1<<32 + ns[x] - 1) / ns[x]
			}
		}
		clear(sum)
		for sy := y0; sy < y1; sy++ {
			fetch(row, sy)
			for x := 0; x < w; x++ {
				x0, x1 := xs[x], max(xs[x+1], xs[x]+1)
				var r, g, bl, a uint64
				seg := row[x0*4 : x1*4]
				for i := 0; i+3 < len(seg); i += 4 {
					r += uint64(seg[i])
					g += uint64(seg[i+1])
					bl += uint64(seg[i+2])
					a += uint64(seg[i+3])
				}
				s := sum[x*4 : x*4+4 : x*4+4]
				s[0] += r
				s[1] += g
				s[2] += bl
				s[3] += a
			}
		}
		out := dst.Pix[y*dst.Stride : y*dst.Stride+w*4]
		for x := 0; x < w; x++ {
			n := ns[x]
			s := sum[x*4 : x*4+4 : x*4+4]
			o := out[x*4 : x*4+4 : x*4+4]
			switch s[3] {
			case 0:
				o[0], o[1], o[2], o[3] = 0, 0, 0, 0
			case 255 * n:
				// Opaque (the common case): straight = premultiplied mean,
				// computed with a fixed-point reciprocal instead of divisions.
				inv := invs[x]
				o[0], o[1], o[2], o[3] = mean(s[0], n, inv), mean(s[1], n, inv), mean(s[2], n, inv), 0xff
			default:
				// Un-premultiply: straight = Σpremult·255 / Σalpha.
				o[0] = unpremul(s[0], s[3])
				o[1] = unpremul(s[1], s[3])
				o[2] = unpremul(s[2], s[3])
				o[3] = uint8((s[3] + n/2) / n) // #nosec G115 -- mean of uint8 values fits in uint8
			}
		}
	}
	return dst
}

// span returns the source range [lo, hi) covered by destination index i
// when n source samples map to m destination samples; it is never empty.
func span(i, n, m int) (lo, hi int) {
	lo, hi = i*n/m, (i+1)*n/m
	return lo, max(hi, lo+1)
}

// mean returns round(sum/n) using inv = ⌈2³²/n⌉, clamped to 255.
func mean(sum, n, inv uint64) uint8 {
	return uint8(min(((sum+n/2)*inv)>>32, 255)) // #nosec G115 -- clamped to 255
}

func unpremul(c, a uint64) uint8 {
	return uint8(min((c*255+a/2)/a, 255)) // #nosec G115 -- clamped to 255
}

// rowFetcher returns a function filling dst with row y (relative to the
// image bounds) as premultiplied 8-bit RGBA, with fast paths for the types
// produced by the standard PNG, JPEG and GIF decoders.
func rowFetcher(src image.Image) func(dst []uint8, y int) {
	b := src.Bounds()
	w := b.Dx()
	switch m := src.(type) {
	case *image.RGBA:
		return func(dst []uint8, y int) {
			i := m.PixOffset(b.Min.X, b.Min.Y+y)
			copy(dst, m.Pix[i:i+w*4])
		}
	case *image.NRGBA:
		return func(dst []uint8, y int) {
			i := m.PixOffset(b.Min.X, b.Min.Y+y)
			s := m.Pix[i : i+w*4]
			for x := 0; x < w*4; x += 4 {
				a := uint32(s[x+3])
				dst[x+0] = premul(s[x+0], a)
				dst[x+1] = premul(s[x+1], a)
				dst[x+2] = premul(s[x+2], a)
				dst[x+3] = s[x+3]
			}
		}
	case *image.Gray:
		return func(dst []uint8, y int) {
			i := m.PixOffset(b.Min.X, b.Min.Y+y)
			for x, v := range m.Pix[i : i+w] {
				dst[x*4], dst[x*4+1], dst[x*4+2], dst[x*4+3] = v, v, v, 0xff
			}
		}
	case *image.Paletted:
		var lut [256][4]uint8
		for i := range lut {
			if i >= len(m.Palette) {
				break
			}
			lut[i] = rgba8(m.Palette[i])
		}
		return func(dst []uint8, y int) {
			i := m.PixOffset(b.Min.X, b.Min.Y+y)
			for x, v := range m.Pix[i : i+w] {
				copy(dst[x*4:x*4+4], lut[v][:])
			}
		}
	case *image.YCbCr:
		hs, vs := chromaShift(m.SubsampleRatio)
		return func(dst []uint8, y int) {
			ay := b.Min.Y + y
			yi := m.YOffset(b.Min.X, ay)
			cb := (ay>>vs - m.Rect.Min.Y>>vs) * m.CStride
			cmin := m.Rect.Min.X >> hs
			for x := 0; x < w; x++ {
				ci := cb + (b.Min.X+x)>>hs - cmin
				r, g, bl := color.YCbCrToRGB(m.Y[yi+x], m.Cb[ci], m.Cr[ci])
				dst[x*4], dst[x*4+1], dst[x*4+2], dst[x*4+3] = r, g, bl, 0xff
			}
		}
	}
	return func(dst []uint8, y int) {
		for x := 0; x < w; x++ {
			c := rgba8(src.At(b.Min.X+x, b.Min.Y+y))
			copy(dst[x*4:x*4+4], c[:])
		}
	}
}

// chromaShift returns the horizontal and vertical chroma subsampling shifts.
func chromaShift(r image.YCbCrSubsampleRatio) (h, v uint) {
	switch r {
	case image.YCbCrSubsampleRatio422:
		return 1, 0
	case image.YCbCrSubsampleRatio420:
		return 1, 1
	case image.YCbCrSubsampleRatio440:
		return 0, 1
	case image.YCbCrSubsampleRatio411:
		return 2, 0
	case image.YCbCrSubsampleRatio410:
		return 2, 1
	}
	return 0, 0
}

// rgba8 converts c to premultiplied 8-bit RGBA.
func rgba8(c color.Color) [4]uint8 {
	r, g, b, a := c.RGBA()
	return [4]uint8{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a >> 8)} // #nosec G115 -- 16-bit channels shifted to 8 bits
}

func premul(c uint8, a uint32) uint8 {
	return uint8((uint32(c)*a + 127) / 255) // #nosec G115 -- c·a/255 ≤ 255
}
