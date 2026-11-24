//go:build kmdai

package ai

import (
	"bufio"
	"bytes"
	"testing"
)

// FuzzResponse feeds arbitrary bytes, as a server response, through every
// parser a response meets: headers, chunked decoding, the body cap, the
// event stream and the JSON of each event. None may panic, hand on more
// than a line's worth of data, or read past the body cap.
func FuzzResponse(f *testing.F) {
	for _, s := range []string{
		okChunked + chunked(`data: {"choices":[{"delta":{"content":"a","reasoning_content":"b"}}]}`+"\n\n") +
			chunked("data: [DONE]\n\n") + "0\r\n\r\n",
		"HTTP/1.1 200 OK\r\nContent-Length: 20\r\n\r\n: ping\r\n\r\ndata: x\r\n\r\n",
		"HTTP/1.0 500 Oops\r\n\r\n" + `{"error":{"message":"m"}}`,
		okChunked + "5;ext\r\ndata:\r\n0\r\nT: 1\r\n\r\n",
		"HTTP/1.1 200 OK\r\n\r\ndata: a\ndata: b\nid: 1\n\n" + `data: {"error":"e"}` + "\n\n",
	} {
		f.Add([]byte(s))
	}
	const maxLine, maxBody = 256, 1024
	f.Fuzz(func(t *testing.T, b []byte) {
		br := bufio.NewReaderSize(bytes.NewReader(b), maxLine)
		h, err := readHeader(br)
		if err != nil {
			return
		}
		body := h.body(br, maxBody)
		if h.check(body) != nil {
			return
		}
		total := 0
		_ = readEvents(body, maxLine, func(data []byte) error {
			if len(data) > maxLine {
				t.Fatalf("event of %d bytes, cap %d", len(data), maxLine)
			}
			total += len(data)
			return chunk(data, func(Delta) error { return nil })
		})
		if total > maxBody {
			t.Fatalf("%d bytes of events from a %d-byte body cap", total, maxBody)
		}
	})
}
