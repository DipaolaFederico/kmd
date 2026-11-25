package termimg

import (
	"encoding/base64"
	"testing"
)

func FuzzDataURI(f *testing.F) {
	f.Add(dataURI("image/png", encodePNG(f, gradient(3, 2))))
	f.Add(dataURI("image/gif", encodeGIF(f, gradient(2, 3))))
	f.Add(dataURI("image/jpeg", encodeJPEG(f, gradient(9, 9))))
	f.Add("data:image/png;base64," + base64.StdEncoding.EncodeToString(pngHeader(1e5, 1e5)))
	f.Add("data:,")
	f.Add("data:image/png;base64,%41%41==")
	f.Fuzz(func(t *testing.T, s string) {
		img, err := new(Loader).Load("data:"+s, "")
		if err != nil {
			return
		}
		if img.w <= 0 || img.h <= 0 || img.w > maxDimension || img.h > maxDimension || img.w*img.h > maxPixels {
			t.Fatalf("accepted %d×%d", img.w, img.h)
		}
		if img.w*img.h > 1<<20 {
			return // keep iterations fast
		}
		for _, p := range []Protocol{Blocks, Kitty, ITerm2, Sixel} {
			fr, err := Render(img, p, 20, 10, Cell{})
			if err != nil {
				continue
			}
			if fr.Cols < 1 || fr.Cols > 20 || len(fr.Lines) < 1 {
				t.Fatalf("%v: bad frame %d cols %d lines", p, fr.Cols, len(fr.Lines))
			}
		}
	})
}

func FuzzSixel(f *testing.F) {
	f.Add(uint8(1), uint8(1), uint64(0), uint8(0))
	f.Add(uint8(13), uint8(7), uint64(42), uint8(128))
	f.Add(uint8(64), uint8(6), uint64(7), uint8(255))
	f.Fuzz(func(t *testing.T, w, h uint8, seed uint64, alphaMask uint8) {
		iw, ih := int(w)%96+1, int(h)%96+1
		m := noise(iw, ih, seed)
		for i := 3; i < len(m.Pix); i += 4 {
			m.Pix[i] |= alphaMask & m.Pix[i-1] // vary transparency
			if alphaMask == 0 {
				m.Pix[i] = m.Pix[i-2]
			}
		}
		checkSixel(t, m)
		// Renderer geometry must hold for arbitrary sizes too.
		l := fit(iw, ih, int(w)%10, int(h)%10, Cell{W: int(seed % 12), H: int(seed % 25)})
		if l.cols < 1 || l.rows < 1 || l.pxW < 1 || l.pxH < 1 || l.pxW > iw || l.pxH > ih {
			t.Fatalf("bad layout %+v for %d×%d", l, iw, ih)
		}
	})
}
