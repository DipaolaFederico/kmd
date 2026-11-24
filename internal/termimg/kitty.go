package termimg

import (
	"encoding/base64"
	"strconv"
	"strings"
)

// kittyChunk is the maximum base64 payload size of one kitty graphics escape.
const kittyChunk = 4096

// renderKitty transmits the image inline (t=d, never t=f: the terminal must
// not be told to read files) and displays it at the cursor without moving it
// (C=1), scaled to a cols×rows cell box. PNG files are passed through
// untouched (f=100); other formats are decoded, downscaled to the target
// pixel size and sent as raw RGB (f=24) or RGBA (f=32).
func renderKitty(img *Image, l layout) (Frame, error) {
	ctrl := make([]byte, 0, 64)
	var payload []byte
	if img.format == "png" {
		ctrl = append(ctrl, "a=T,f=100"...)
		payload = img.data
	} else {
		src, err := img.decode()
		if err != nil {
			return Frame{}, err
		}
		m := resize(src, l.pxW, l.pxH)
		if opaque(m.Pix) {
			payload = make([]byte, 0, l.pxW*l.pxH*3)
			for i := 0; i < len(m.Pix); i += 4 {
				payload = append(payload, m.Pix[i], m.Pix[i+1], m.Pix[i+2])
			}
			ctrl = append(ctrl, "a=T,f=24"...)
		} else {
			payload = m.Pix
			ctrl = append(ctrl, "a=T,f=32"...)
		}
		ctrl = append(ctrl, ",s="...)
		ctrl = strconv.AppendInt(ctrl, int64(l.pxW), 10)
		ctrl = append(ctrl, ",v="...)
		ctrl = strconv.AppendInt(ctrl, int64(l.pxH), 10)
	}
	ctrl = append(ctrl, ",t=d,C=1,c="...)
	ctrl = strconv.AppendInt(ctrl, int64(l.cols), 10)
	ctrl = append(ctrl, ",r="...)
	ctrl = strconv.AppendInt(ctrl, int64(l.rows), 10)
	ctrl = append(ctrl, ",q=2"...)

	lines := make([]string, l.rows)
	lines[0] = kittySequence(string(ctrl), payload)
	return Frame{Lines: lines, Cols: l.cols, Rows: l.rows}, nil
}

// opaque reports whether every alpha byte of straight RGBA pixels is 0xff.
func opaque(pix []uint8) bool {
	for i := 3; i < len(pix); i += 4 {
		if pix[i] != 0xff {
			return false
		}
	}
	return true
}

// kittySequence base64-encodes payload and splits it into APC escapes of at
// most kittyChunk bytes: the first carries ctrl, the following only m=, and
// m=1 marks every chunk but the last (m=0).
func kittySequence(ctrl string, payload []byte) string {
	enc := make([]byte, base64.StdEncoding.EncodedLen(len(payload)))
	base64.StdEncoding.Encode(enc, payload)
	var b strings.Builder
	b.Grow(len(enc) + (len(enc)/kittyChunk+1)*12 + len(ctrl))
	for off := 0; ; off += kittyChunk {
		end := min(off+kittyChunk, len(enc))
		b.WriteString("\x1b_G")
		if off == 0 {
			b.WriteString(ctrl)
			b.WriteByte(',')
		}
		if end < len(enc) {
			b.WriteString("m=1;")
		} else {
			b.WriteString("m=0;")
		}
		b.Write(enc[off:end])
		b.WriteString("\x1b\\")
		if end >= len(enc) {
			return b.String()
		}
	}
}
