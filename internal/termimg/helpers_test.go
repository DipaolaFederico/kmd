package termimg

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// gradient returns a w×h opaque test image.
func gradient(w, h int) *image.NRGBA {
	m := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			m.SetNRGBA(x, y, color.NRGBA{uint8(x * 255 / max(w-1, 1)), uint8(y * 255 / max(h-1, 1)), 128, 255})
		}
	}
	return m
}

// noise returns a w×h image of random opaque pixels (incompressible PNG).
func noise(w, h int, seed uint64) *image.NRGBA {
	r := rand.New(rand.NewPCG(seed, seed))
	m := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := range m.Pix {
		m.Pix[i] = uint8(r.Uint32())
	}
	for i := 3; i < len(m.Pix); i += 4 {
		m.Pix[i] = 255
	}
	return m
}

func encodePNG(t testing.TB, m image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, m); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func encodeJPEG(t testing.TB, m image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := jpeg.Encode(&b, m, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func encodeGIF(t testing.TB, m image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := gif.Encode(&b, m, nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func writeFile(t testing.TB, dir, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func dataURI(mime string, data []byte) string {
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
}

// pngHeader returns a PNG signature plus a valid IHDR claiming w×h pixels
// and no image data: enough for DecodeConfig, nothing to decode.
func pngHeader(w, h uint32) []byte {
	var b bytes.Buffer
	b.WriteString("\x89PNG\r\n\x1a\n")
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:], w)
	binary.BigEndian.PutUint32(ihdr[4:], h)
	ihdr[8], ihdr[9] = 8, 6 // 8-bit RGBA
	_ = binary.Write(&b, binary.BigEndian, uint32(len(ihdr)))
	chunk := append([]byte("IHDR"), ihdr...)
	b.Write(chunk)
	_ = binary.Write(&b, binary.BigEndian, crc32.ChecksumIEEE(chunk))
	return b.Bytes()
}

func mustLoadBytes(t testing.TB, data []byte) *Image {
	t.Helper()
	img, err := validate(data)
	if err != nil {
		t.Fatal(err)
	}
	return img
}

// decodedSixel is the result of parsing a sixel stream in tests.
type decodedSixel struct {
	w, h    int
	palette map[int][3]int
	pix     []int // color register per pixel, -1 = unset
}

// decodeSixel is a strict sixel decoder used to verify encodeSixel output.
func decodeSixel(s string) (*decodedSixel, error) {
	const head = "\x1bP0;1;0q\"1;1;"
	if !strings.HasPrefix(s, head) || !strings.HasSuffix(s, "\x1b\\") {
		return nil, fmt.Errorf("bad framing %q", s[:min(len(s), 20)])
	}
	body := s[len(head) : len(s)-2]
	num := func() (int, error) {
		i := 0
		for i < len(body) && body[i] >= '0' && body[i] <= '9' {
			i++
		}
		n, err := strconv.Atoi(body[:i])
		body = body[i:]
		return n, err
	}
	d := &decodedSixel{palette: map[int][3]int{}}
	var err error
	if d.w, err = num(); err != nil {
		return nil, err
	}
	if body == "" || body[0] != ';' {
		return nil, fmt.Errorf("bad raster")
	}
	body = body[1:]
	if d.h, err = num(); err != nil {
		return nil, err
	}
	d.pix = make([]int, d.w*d.h)
	for i := range d.pix {
		d.pix[i] = -1
	}
	x, y, cur := 0, 0, -1
	put := func(ch byte, n int) error {
		if ch < '?' || ch > '~' {
			return fmt.Errorf("bad sixel char %q", ch)
		}
		bits := ch - '?'
		for ; n > 0; n-- {
			for r := 0; r < 6; r++ {
				if bits&(1<<r) == 0 {
					continue
				}
				if x >= d.w || y+r >= d.h {
					return fmt.Errorf("pixel %d,%d out of bounds", x, y+r)
				}
				if cur < 0 {
					return fmt.Errorf("no color selected")
				}
				if d.pix[(y+r)*d.w+x] != -1 {
					return fmt.Errorf("pixel %d,%d set twice", x, y+r)
				}
				d.pix[(y+r)*d.w+x] = cur
			}
			x++
		}
		return nil
	}
	for body != "" {
		c := body[0]
		body = body[1:]
		switch {
		case c == '#':
			n, err := num()
			if err != nil {
				return nil, err
			}
			if strings.HasPrefix(body, ";2;") {
				body = body[3:]
				var rgb [3]int
				for i := range rgb {
					if i > 0 {
						if body == "" || body[0] != ';' {
							return nil, fmt.Errorf("bad color")
						}
						body = body[1:]
					}
					if rgb[i], err = num(); err != nil || rgb[i] > 100 {
						return nil, fmt.Errorf("bad color component")
					}
				}
				d.palette[n] = rgb
			} else {
				if _, ok := d.palette[n]; !ok {
					return nil, fmt.Errorf("undefined color %d", n)
				}
				cur = n
			}
		case c == '!':
			n, err := num()
			if err != nil || body == "" {
				return nil, fmt.Errorf("bad repeat")
			}
			ch := body[0]
			body = body[1:]
			if err := put(ch, n); err != nil {
				return nil, err
			}
		case c == '$':
			x = 0
		case c == '-':
			x, y = 0, y+6
		default:
			if err := put(c, 1); err != nil {
				return nil, err
			}
		}
	}
	return d, nil
}

// checkSixel encodes m and verifies the decoded pixels match quantize(m).
func checkSixel(t testing.TB, m *image.NRGBA) {
	t.Helper()
	out := string(encodeSixel(m))
	d, err := decodeSixel(out)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	w, h := m.Rect.Dx(), m.Rect.Dy()
	if d.w != w || d.h != h {
		t.Fatalf("size %dx%d, want %dx%d", d.w, d.h, w, h)
	}
	idx, _ := quantize(m)
	for i, c := range idx {
		want := int(c)
		if c == transparent {
			want = -1
		}
		if d.pix[i] != want {
			t.Fatalf("pixel %d: got %d want %d", i, d.pix[i], want)
		}
	}
}
