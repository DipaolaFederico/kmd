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

func BenchmarkEncodeOnly(b *testing.B) {
	img := mustLoadBytes(b, benchImage(b))
	if _, err := img.decode(); err != nil {
		b.Fatal(err)
	}
	b.Run("blocks", func(b *testing.B) {
		for b.Loop() {
			_, _ = Render(img, Blocks, 80, 0, Cell{})
		}
	})
	b.Run("sixel", func(b *testing.B) {
		for b.Loop() {
			_, _ = Render(img, Sixel, 80, 0, Cell{W: 10, H: 20})
		}
	})
}
