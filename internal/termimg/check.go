package termimg

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
)

// Limits on what a terminal decodes for kitty and iTerm2, which receive
// the document's files as they are: kmd decodes only the first frame of a
// GIF and checks the pixels of that one, but a terminal animates them all,
// and each scan of a progressive JPEG makes a decoder go over the whole
// image again.
const (
	maxGIFPixels = 5 * maxPixels // pixels of all the frames of a GIF
	maxJPEGScans = 256
)

// checkStructure walks data, an image of the given format and size, the
// way a decoder would but without decoding it, and returns the length of
// the image in it. kitty and iTerm2 receive image files as they are, so a
// terminal's decoder only ever sees a well-formed one of the size kmd
// checked. Bytes after the end of a JPEG or GIF (a phone's motion photo, a
// trailer) are left out of that length, as decoders ignore them; a PNG
// must end with its IEND chunk.
func checkStructure(data []byte, format string, w, h int) (int, error) {
	switch format {
	case "png":
		return len(data), checkPNG(data)
	case "jpeg":
		return checkJPEG(data, w, h)
	case "gif":
		return checkGIF(data, w, h)
	}
	return 0, fmt.Errorf("unsupported format %s", format)
}

// checkPNG walks the chunks of a PNG without inflating them: IHDR first,
// every length within the file, every type four letters, every CRC right,
// and IEND last with nothing after it. The walk costs a CRC over the file,
// far less than decoding it.
func checkPNG(data []byte) error {
	rest, ok := bytes.CutPrefix(data, []byte("\x89PNG\r\n\x1a\n"))
	if !ok {
		return errors.New("png: bad signature")
	}
	for first := true; len(rest) >= 12; first = false {
		n := binary.BigEndian.Uint32(rest)
		if int64(n) > int64(len(rest)-12) {
			return errors.New("png: truncated chunk")
		}
		typ, end := string(rest[4:8]), 8+int(n)
		if !chunkType(typ) || first != (typ == "IHDR") {
			return fmt.Errorf("png: unexpected chunk %q", typ)
		}
		if crc32.ChecksumIEEE(rest[4:end]) != binary.BigEndian.Uint32(rest[end:]) {
			return fmt.Errorf("png: bad checksum in %s", typ)
		}
		rest = rest[end+4:]
		if typ == "IEND" {
			if len(rest) > 0 {
				return errors.New("png: data after IEND")
			}
			return nil
		}
	}
	return errors.New("png: no IEND chunk")
}

// chunkType reports whether t is a PNG chunk type: four ASCII letters.
func chunkType(t string) bool {
	for i := 0; i < len(t); i++ {
		if c := t[i] | 0x20; c < 'a' || c > 'z' {
			return false
		}
	}
	return true
}

// checkJPEG walks the markers of a JPEG from SOI to EOI: every segment
// within the file, every frame header of the size DecodeConfig read, a
// frame header before the first scan and at most maxJPEGScans scans. It
// returns the length of the image up to its EOI marker.
func checkJPEG(data []byte, w, h int) (int, error) {
	if len(data) < 2 || data[0] != 0xff || data[1] != 0xd8 {
		return 0, errors.New("jpeg: no SOI marker")
	}
	frame, scans := false, 0
	for i := 2; ; {
		m, next, err := jpegMarker(data, i)
		if err != nil {
			return 0, err
		}
		i = next
		switch {
		case m == 0xd9: // EOI
			if scans == 0 {
				return 0, errors.New("jpeg: no scan")
			}
			return i, nil
		case m == 0x01 || m >= 0xd0 && m <= 0xd7: // TEM, RSTn: no segment
			continue
		case m == 0x00 || m == 0xd8:
			return 0, fmt.Errorf("jpeg: unexpected marker %#x", m)
		}
		if i+2 > len(data) {
			return 0, errors.New("jpeg: truncated segment")
		}
		n := int(binary.BigEndian.Uint16(data[i:]))
		if n < 2 || n > len(data)-i {
			return 0, errors.New("jpeg: truncated segment")
		}
		seg := data[i+2 : i+n]
		i += n
		switch {
		case jpegFrame(m):
			if len(seg) < 5 || int(binary.BigEndian.Uint16(seg[1:])) != h || int(binary.BigEndian.Uint16(seg[3:])) != w {
				return 0, errors.New("jpeg: frame header of another size")
			}
			frame = true
		case m == 0xda: // SOS: entropy-coded data follows the header
			if scans++; !frame || scans > maxJPEGScans {
				return 0, errors.New("jpeg: scan without a frame header, or too many scans")
			}
			i = entropyEnd(data, i)
		}
	}
}

// jpegMarker reads the marker at data[i] (0xff, any number of 0xff fill
// bytes, the marker code) and returns its code and the index after it.
func jpegMarker(data []byte, i int) (byte, int, error) {
	if i >= len(data) || data[i] != 0xff {
		return 0, 0, errors.New("jpeg: missing marker")
	}
	for i < len(data) && data[i] == 0xff {
		i++
	}
	if i >= len(data) {
		return 0, 0, errors.New("jpeg: no EOI marker")
	}
	return data[i], i + 1, nil
}

// jpegFrame reports whether m is a start-of-frame marker: SOF0–SOF15 but
// DHT, JPG and DAC.
func jpegFrame(m byte) bool {
	return m >= 0xc0 && m <= 0xcf && m != 0xc4 && m != 0xc8 && m != 0xcc
}

// entropyEnd returns the index of the marker that ends the entropy-coded
// data starting at i, or len(data): inside that data 0xff is followed by a
// stuffed 0x00 or a restart marker.
func entropyEnd(data []byte, i int) int {
	for {
		j := bytes.IndexByte(data[i:], 0xff)
		if j < 0 || i+j+1 >= len(data) {
			return len(data)
		}
		i += j
		if b := data[i+1]; b != 0x00 && b != 0xff && (b < 0xd0 || b > 0xd7) {
			return i
		}
		i++
	}
}

// checkGIF walks the blocks of a GIF up to its trailer: every block and
// sub-block within the file, every frame inside the logical screen with an
// LZW code size a decoder accepts, and at most maxGIFPixels in all the
// frames. It returns the length of the image up to its trailer.
func checkGIF(data []byte, w, h int) (int, error) {
	const header = 13 // signature and logical screen descriptor
	if len(data) < header {
		return 0, errors.New("gif: truncated header")
	}
	i, frames, pixels := header+colorTable(data[10]), 0, 0
	for {
		if i >= len(data) {
			return 0, errors.New("gif: no trailer")
		}
		var err error
		switch data[i] {
		case 0x3b: // trailer
			if frames == 0 {
				return 0, errors.New("gif: no image")
			}
			return i + 1, nil
		case 0x21: // extension: a label, then sub-blocks
			i, err = skipSubBlocks(data, i+2)
		case 0x2c: // image descriptor
			var area int
			if i, area, err = gifFrame(data, i, w, h); err == nil {
				frames, pixels = frames+1, pixels+area
				if pixels > maxGIFPixels {
					err = errors.New("gif: too many pixels in all the frames")
				}
			}
		default:
			err = fmt.Errorf("gif: unexpected block %#x", data[i])
		}
		if err != nil {
			return 0, err
		}
	}
}

// gifFrame walks the image descriptor at data[i] and the frame after it,
// and returns the index after the frame and its area in pixels.
func gifFrame(data []byte, i, w, h int) (int, int, error) {
	const desc = 10
	if desc > len(data)-i {
		return 0, 0, errors.New("gif: truncated image descriptor")
	}
	d := data[i+1 : i+desc]
	x, y := int(binary.LittleEndian.Uint16(d)), int(binary.LittleEndian.Uint16(d[2:]))
	fw, fh := int(binary.LittleEndian.Uint16(d[4:])), int(binary.LittleEndian.Uint16(d[6:]))
	if x+fw > w || y+fh > h {
		return 0, 0, errors.New("gif: frame outside the image")
	}
	i += desc + colorTable(d[8])
	if i >= len(data) || data[i] < 2 || data[i] > 8 {
		return 0, 0, errors.New("gif: bad LZW code size")
	}
	i, err := skipSubBlocks(data, i+1)
	return i, fw * fh, err
}

// colorTable returns the size of the color table a GIF descriptor's packed
// byte announces.
func colorTable(packed byte) int {
	if packed&0x80 == 0 {
		return 0
	}
	return 3 << (packed&7 + 1)
}

// skipSubBlocks returns the index after the sub-blocks starting at i: each
// a length byte and its data, ending with an empty one.
func skipSubBlocks(data []byte, i int) (int, error) {
	for {
		if i >= len(data) {
			return 0, errors.New("gif: truncated sub-block")
		}
		n := int(data[i])
		i += 1 + n
		if n == 0 {
			return i, nil
		}
	}
}
