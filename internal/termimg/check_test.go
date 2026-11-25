package termimg

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"testing"
)

// jpegSegment appends a marker and its segment to b.
func jpegSegment(b []byte, marker byte, body ...byte) []byte {
	b = append(b, 0xff, marker)
	b = binary.BigEndian.AppendUint16(b, uint16(len(body)+2))
	return append(b, body...)
}

// syntheticJPEG returns SOI, a w×h frame header, the given number of
// scans (a header and one byte of data each) and EOI: enough for the walk.
func syntheticJPEG(w, h, scans int) []byte {
	b := jpegSegment([]byte{0xff, 0xd8}, 0xc0, 8, byte(h>>8), byte(h), byte(w>>8), byte(w), 1, 1, 0x11, 0)
	for range scans {
		b = jpegSegment(b, 0xda, 1, 1, 0, 0, 63, 0)
		b = append(b, 0x12, 0xff, 0x00, 0xff, 0xd3, 0x34) // stuffed 0xff, RST3
	}
	return append(b, 0xff, 0xd9)
}

func TestCheckJPEG(t *testing.T) {
	good := encodeJPEG(t, noise(64, 48, 1))
	if n, err := checkJPEG(good, 64, 48); err != nil || n != len(good) {
		t.Fatalf("encoded JPEG: %d of %d, %v", n, len(good), err)
	}
	trailer := append(bytes.Clone(good), "\xff\xd8 and more"...)
	if n, err := checkJPEG(trailer, 64, 48); err != nil || n != len(good) {
		t.Errorf("trailer: %d of %d, %v", n, len(good), err)
	}
	if n, err := checkJPEG(syntheticJPEG(5, 7, maxJPEGScans), 5, 7); err != nil || n == 0 {
		t.Errorf("%d scans: %v", maxJPEGScans, err)
	}
	sof := bytes.Index(good, []byte{0xff, 0xc0})
	secondFrame := append(bytes.Clone(good[:sof]), jpegSegment(nil, 0xc2, 8, 0x20, 0, 0x20, 0, 1, 1, 0x11, 0)...)
	secondFrame = append(secondFrame, good[sof:]...)
	frame57 := jpegSegment([]byte{0xff, 0xd8}, 0xc0, 8, 0, 7, 0, 5, 1, 1, 0x11, 0)
	bad := map[string]struct {
		data []byte
		w, h int // the size DecodeConfig read
	}{
		"no SOI":          {good[2:], 64, 48},
		"truncated":       {good[:len(good)-100], 64, 48},
		"no EOI":          {good[:len(good)-2], 64, 48},
		"second SOI":      {append([]byte{0xff, 0xd8}, good...), 64, 48},
		"short segment":   {append([]byte{0xff, 0xd8, 0xff, 0xe0, 0, 1}, good[2:]...), 64, 48},
		"second frame":    {secondFrame, 64, 48},
		"long segment":    {jpegSegment([]byte{0xff, 0xd8}, 0xe0, make([]byte, 10)...)[:10], 5, 7},
		"scan, no frame":  {jpegSegment([]byte{0xff, 0xd8}, 0xda, 1, 1, 0, 0, 63, 0), 5, 7},
		"too many scans":  {syntheticJPEG(5, 7, maxJPEGScans+1), 5, 7},
		"no scan":         {append(frame57, 0xff, 0xd9), 5, 7},
		"garbage":         {append(jpegSegment(frame57, 0xe0), 'x', 0xff, 0xd9), 5, 7},
		"other size":      {syntheticJPEG(6, 7, 1), 5, 7},
		"marker at end":   {[]byte{0xff, 0xd8, 0xff, 0xff}, 5, 7},
		"length past end": {[]byte{0xff, 0xd8, 0xff, 0xe0, 0}, 5, 7},
	}
	for name, c := range bad {
		if _, err := checkJPEG(c.data, c.w, c.h); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := validate(good[:len(good)-100]); !errors.Is(err, ErrUnsupported) {
		t.Errorf("validate does not walk JPEGs: %v", err)
	}
}

// gifFile returns a GIF of a w×h logical screen with the given frames
// (each x, y, width, height), without color tables and with empty image
// data: enough for the walk.
func gifFile(w, h int, frames ...[4]int) []byte {
	b := []byte("GIF89a")
	b = binary.LittleEndian.AppendUint16(b, uint16(w))
	b = binary.LittleEndian.AppendUint16(b, uint16(h))
	b = append(b, 0, 0, 0)
	b = append(b, 0x21, 0xf9, 4, 0, 0, 0, 0, 0) // graphic control extension
	for _, f := range frames {
		b = append(b, 0x2c)
		for _, v := range f {
			b = binary.LittleEndian.AppendUint16(b, uint16(v))
		}
		b = append(b, 0, 2, 1, 0x44, 0) // packed, LZW size, one sub-block, end
	}
	return append(b, 0x3b)
}

func TestCheckGIF(t *testing.T) {
	m := image.NewPaletted(image.Rect(0, 0, 30, 20), color.Palette{color.Black, color.White})
	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, &gif.GIF{Image: []*image.Paletted{m, m}, Delay: []int{1, 1}}); err != nil {
		t.Fatal(err)
	}
	good := buf.Bytes()
	if n, err := checkGIF(good, 30, 20); err != nil || n != len(good) {
		t.Fatalf("encoded GIF: %d of %d, %v", n, len(good), err)
	}
	trailer := append(bytes.Clone(good), "more"...)
	if n, err := checkGIF(trailer, 30, 20); err != nil || n != len(good) {
		t.Errorf("trailer: %d of %d, %v", n, len(good), err)
	}
	big := [4]int{0, 0, 8000, 5000}
	if _, err := checkGIF(gifFile(8000, 5000, big, big, big, big, big), 8000, 5000); err != nil {
		t.Errorf("five 40-megapixel frames: %v", err)
	}
	bad := map[string][]byte{
		"truncated header": good[:10],
		"truncated":        good[:len(good)-10],
		"no trailer":       good[:len(good)-1],
		"no image":         gifFile(30, 20),
		"frame outside":    gifFile(30, 20, [4]int{20, 0, 11, 20}),
		"too many pixels":  gifFile(8000, 5000, big, big, big, big, big, big),
		"bad LZW size":     bytes.Replace(gifFile(30, 20, [4]int{0, 0, 1, 1}), []byte{0, 2, 1, 0x44}, []byte{0, 9, 1, 0x44}, 1),
		"unknown block":    append(gifFile(30, 20, [4]int{0, 0, 1, 1})[:13], '!'-1),
		"descriptor cut":   append(gifFile(30, 20)[:21], 0x2c, 0, 0),
	}
	for name, data := range bad {
		w, h := 30, 20
		if name == "too many pixels" {
			w, h = 8000, 5000
		}
		if _, err := checkGIF(data, w, h); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestCheckStructureFormats(t *testing.T) {
	if _, err := checkStructure(nil, "bmp", 1, 1); err == nil {
		t.Error("unknown format accepted")
	}
}

func BenchmarkCheckJPEG(b *testing.B) {
	data := encodeJPEG(b, noise(1024, 768, 3))
	b.SetBytes(int64(len(data)))
	for b.Loop() {
		if _, err := checkJPEG(data, 1024, 768); err != nil {
			b.Fatal(err)
		}
	}
}
