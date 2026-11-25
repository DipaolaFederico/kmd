//go:build kmdai

package ai

import (
	"bufio"
	"bytes"
	"io"
)

// readEvents parses a server-sent event stream incrementally and calls on
// with the data of each event as soon as its blank line arrives. Lines and
// the data of one event are bounded by maxLine; comments (llama.cpp's
// keep-alive pings) and fields other than data are ignored. The slice passed
// to on is reused: on must not keep it. An event cut off by the end of the
// stream is dropped, as the specification says.
func readEvents(r io.Reader, maxLine int, on func(data []byte) error) error {
	// a second buffer over the one in Client.exchange: that one reads the
	// framing, this one the decoded body, which is all readEvents sees
	br := bufio.NewReaderSize(r, maxLine)
	data := make([]byte, 0, 1024)
	pending := false // a data field was seen since the last dispatch
	for {
		line, err := rawLine(br)
		if err != nil {
			return err // io.EOF included: the caller decides if the stream was complete
		}
		field, value, _ := bytes.Cut(line, []byte(":"))
		switch {
		case len(line) == 0:
			if pending {
				if err := on(data); err != nil {
					return err
				}
			}
			data, pending = data[:0], false
		case string(field) == "data":
			if pending {
				data = append(data, '\n')
			}
			data, pending = append(data, bytes.TrimPrefix(value, []byte(" "))...), true
			if len(data) > maxLine {
				return errLongLine
			}
		}
	}
}
