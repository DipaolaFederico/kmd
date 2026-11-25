package termimg

import (
	"bytes"
	"encoding/base64"
	"image/png"
	"strconv"
	"strings"
)

// renderITerm2 emits an OSC 1337 inline file sized to a cols×rows cell box.
// A PNG is sent as it is (Load checked its structure); a JPEG or GIF is
// decoded by kmd, scaled to the target pixel size and sent as a PNG of its
// first frame, so the terminal's decoders never see the document's bytes.
// The terminal moves the cursor past the image, so the frame is a single
// line.
func renderITerm2(img *Image, l layout) (Frame, error) {
	data := img.data
	if img.format != "png" {
		src, err := img.decode()
		if err != nil {
			return Frame{}, err
		}
		var buf bytes.Buffer
		enc := png.Encoder{CompressionLevel: png.BestSpeed}
		if err := enc.Encode(&buf, resize(src, l.pxW, l.pxH)); err != nil {
			return Frame{}, err
		}
		data = buf.Bytes()
	}
	var b strings.Builder
	b.Grow(base64.StdEncoding.EncodedLen(len(data)) + 96)
	b.WriteString("\x1b]1337;File=inline=1;size=")
	b.WriteString(strconv.Itoa(len(data)))
	b.WriteString(";width=")
	b.WriteString(strconv.Itoa(l.cols))
	b.WriteString(";height=")
	b.WriteString(strconv.Itoa(l.rows))
	b.WriteString(";preserveAspectRatio=1:")
	enc := base64.NewEncoder(base64.StdEncoding, &b)
	_, _ = enc.Write(data) // strings.Builder never fails
	_ = enc.Close()
	b.WriteByte('\a')
	return Frame{Lines: []string{b.String()}, Cols: l.cols, Rows: l.rows}, nil
}
