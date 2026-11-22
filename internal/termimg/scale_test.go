package termimg

import (
	"image"
	"image/color"
	"testing"
)

func TestResizeAverage(t *testing.T) {
	m := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	m.SetNRGBA(0, 0, color.NRGBA{200, 0, 0, 255})
	m.SetNRGBA(1, 0, color.NRGBA{0, 200, 0, 255})
	m.SetNRGBA(0, 1, color.NRGBA{0, 0, 200, 255})
	// (1,1) transparent: must not darken the average color.
	got := resize(m, 1, 1).NRGBAAt(0, 0)
	want := color.NRGBA{67, 67, 67, 191}
	if got != want {
		t.Fatalf("got %v want %v", got, want)
	}
	if got := resize(image.NewNRGBA(image.Rect(0, 0, 4, 4)), 2, 2).NRGBAAt(1, 1); got != (color.NRGBA{}) {
		t.Fatalf("transparent got %v", got)
	}
}

func TestResizeUpscaleAndEmpty(t *testing.T) {
	m := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	m.SetNRGBA(0, 0, red)
	m.SetNRGBA(1, 0, blue)
	up := resize(m, 4, 3)
	if up.NRGBAAt(1, 2) != red || up.NRGBAAt(2, 0) != blue {
		t.Fatalf("nearest upscale wrong: %v %v", up.NRGBAAt(1, 2), up.NRGBAAt(2, 0))
	}
	if e := resize(image.NewNRGBA(image.Rect(0, 0, 0, 0)), 2, 2); e.Rect.Dx() != 2 {
		t.Fatal("empty source")
	}
}

// TestResizeFastPaths checks every fast path against the generic At path.
func TestResizeFastPaths(t *testing.T) {
	r := image.Rect(3, 5, 3+23, 5+17) // non-zero origin
	src := gradient(23, 17)
	src.SetNRGBA(4, 4, color.NRGBA{10, 20, 30, 77})

	rgba := image.NewRGBA(r)
	nrgba := image.NewNRGBA(r)
	gray := image.NewGray(r)
	pal := image.NewPaletted(r, color.Palette{color.Transparent, red, blue, color.White})
	for y := 0; y < 17; y++ {
		for x := 0; x < 23; x++ {
			c := src.NRGBAAt(x, y)
			rgba.Set(r.Min.X+x, r.Min.Y+y, c)
			nrgba.SetNRGBA(r.Min.X+x, r.Min.Y+y, c)
			gray.Set(r.Min.X+x, r.Min.Y+y, c)
			pal.Set(r.Min.X+x, r.Min.Y+y, c)
		}
	}
	imgs := []image.Image{rgba, nrgba, gray, pal}
	for _, ratio := range []image.YCbCrSubsampleRatio{
		image.YCbCrSubsampleRatio444, image.YCbCrSubsampleRatio422, image.YCbCrSubsampleRatio420,
		image.YCbCrSubsampleRatio440, image.YCbCrSubsampleRatio411, image.YCbCrSubsampleRatio410,
	} {
		y := image.NewYCbCr(r, ratio)
		for i := range y.Y {
			y.Y[i] = uint8(i * 7)
		}
		for i := range y.Cb {
			y.Cb[i], y.Cr[i] = uint8(i*13), uint8(255-i*5)
		}
		imgs = append(imgs, y)
	}
	for _, m := range imgs {
		for _, sz := range [][2]int{{23, 17}, {7, 5}, {1, 1}} {
			fast := resize(m, sz[0], sz[1])
			slow := resize(opaqueWrapper{m}, sz[0], sz[1])
			for i := range fast.Pix {
				if d := int(fast.Pix[i]) - int(slow.Pix[i]); d < -1 || d > 1 {
					t.Fatalf("%T %v: byte %d fast %d slow %d", m, sz, i, fast.Pix[i], slow.Pix[i])
				}
			}
		}
	}
}

// opaqueWrapper hides the concrete type to force the generic At path.
type opaqueWrapper struct{ image.Image }
