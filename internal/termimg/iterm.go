package termimg

import (
	"encoding/base64"
	"strconv"
	"strings"
)

// renderITerm2 emits an OSC 1337 inline file with the original bytes, whose
// structure Load checked (see checkStructure), sized to a cols×rows cell
// box. The terminal moves the cursor past the image, so the frame is a
// single line.
func renderITerm2(img *Image, l layout) Frame {
	var b strings.Builder
	b.Grow(base64.StdEncoding.EncodedLen(len(img.data)) + 96)
	b.WriteString("\x1b]1337;File=inline=1;size=")
	b.WriteString(strconv.Itoa(len(img.data)))
	b.WriteString(";width=")
	b.WriteString(strconv.Itoa(l.cols))
	b.WriteString(";height=")
	b.WriteString(strconv.Itoa(l.rows))
	b.WriteString(";preserveAspectRatio=1:")
	enc := base64.NewEncoder(base64.StdEncoding, &b)
	_, _ = enc.Write(img.data) // strings.Builder never fails
	_ = enc.Close()
	b.WriteByte('\a')
	return Frame{Lines: []string{b.String()}, Cols: l.cols, Rows: l.rows}
}
