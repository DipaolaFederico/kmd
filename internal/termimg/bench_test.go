package termimg

import "testing"

func benchImage(b *testing.B) []byte {
	b.Helper()
	m := gradient(1024, 768)
	n := noise(1024, 768, 9)
	for i := 0; i < len(m.Pix); i += 16 { // some texture, still photo-like size
		m.Pix[i] ^= n.Pix[i] & 0x1f
	}
	return encodePNG(b, m)
}

func benchRender(b *testing.B, p Protocol, cell Cell) {
	data := benchImage(b)
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for b.Loop() {
		img, err := validate(data) // fresh image: includes the full decode
		if err != nil {
			b.Fatal(err)
		}
		if _, err := Render(img, p, 80, 0, cell); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBlocks1024x768(b *testing.B) { benchRender(b, Blocks, Cell{}) }
func BenchmarkSixel1024x768(b *testing.B)  { benchRender(b, Sixel, Cell{W: 10, H: 20}) }
func BenchmarkKittyPNG1024x768(b *testing.B) {
	benchRender(b, Kitty, Cell{W: 10, H: 20})
}

// BenchmarkEncodeOnly: scaling and encoding decoded pixels (Render decodes
// them again each time: they are not kept).
func BenchmarkEncodeOnly(b *testing.B) {
	img := mustLoadBytes(b, benchImage(b))
	src, err := img.decode()
	if err != nil {
		b.Fatal(err)
	}
	b.Run("blocks", func(b *testing.B) {
		gw, gh := blocksGeometry(img.w, img.h, 80, 0, Cell{})
		for b.Loop() {
			_ = encodeBlocks(resize(src, gw, gh))
		}
	})
	b.Run("sixel", func(b *testing.B) {
		l := fit(img.w, img.h, 80, 0, Cell{W: 10, H: 20})
		for b.Loop() {
			_ = encodeSixel(resize(src, l.pxW, l.pxH))
		}
	})
}

// BenchmarkCheckPNG: the chunk walk every PNG gets before kitty or iTerm2
// receives it as it is.
func BenchmarkCheckPNG(b *testing.B) {
	data := benchImage(b)
	b.SetBytes(int64(len(data)))
	for b.Loop() {
		if err := checkPNG(data); err != nil {
			b.Fatal(err)
		}
	}
}
