//go:build kmdai

package ai

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// The HTTP/1.1 response parsing behind Client: headers, chunked decoding and
// bounded line reading.

// maxHeaders bounds the number of response header and trailer lines.
const maxHeaders = 100

// How much of a server-supplied string an error message quotes.
const (
	shortText = 40  // a header value or status reason
	longText  = 200 // a server's error message or model name
)

// header is what kmd needs from the response headers.
type header struct {
	status  int
	reason  string
	length  int64 // -1: until the connection closes
	chunked bool
}

func readHeader(br *bufio.Reader) (header, error) {
	line, err := readLine(br)
	if err != nil {
		return header{}, err
	}
	h := header{length: -1}
	proto, rest, _ := strings.Cut(line, " ")
	code, reason, _ := strings.Cut(rest, " ")
	h.status, err = strconv.Atoi(code)
	if !strings.HasPrefix(proto, "HTTP/1.") || err != nil || len(code) != 3 {
		return header{}, fmt.Errorf("not an HTTP/1.x response: %q", printable(line, shortText))
	}
	h.reason = reason
	for range maxHeaders {
		line, err := readLine(br)
		if err != nil || line == "" {
			return h, err
		}
		if err := h.field(line); err != nil {
			return header{}, err
		}
	}
	return header{}, errors.New("too many response headers")
}

func (h *header) field(line string) error {
	k, v, _ := strings.Cut(line, ":")
	v = strings.TrimSpace(v)
	switch strings.ToLower(k) {
	case "transfer-encoding":
		if !strings.EqualFold(v, "chunked") {
			return fmt.Errorf("unsupported transfer encoding %q", printable(v, shortText))
		}
		h.chunked = true
	case "content-length":
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 0 {
			return fmt.Errorf("invalid content length %q", printable(v, shortText))
		}
		h.length = n
	}
	return nil
}

// body decodes the body that follows the headers.
func (h header) body(br *bufio.Reader) io.Reader {
	switch {
	case h.chunked:
		return &chunkedReader{r: br}
	case h.length >= 0:
		return io.LimitReader(br, h.length)
	}
	return br
}

// check rejects anything but 200 OK, with the server's message if it gave
// one. kmd never follows a redirect: the destination is the configured one.
func (h header) check(body io.Reader) error {
	switch {
	case h.status == 200:
		return nil
	case h.status >= 300 && h.status < 400:
		return fmt.Errorf("the local AI server answered %d %s: kmd does not follow redirects", h.status, printable(h.reason, shortText))
	}
	msg := printable(h.reason, shortText)
	b, _ := io.ReadAll(io.LimitReader(body, 4<<10))
	if m := bodyError(b); m != "" {
		msg = m
	}
	return fmt.Errorf("the local AI server answered %d: %s", h.status, msg)
}

// rawLine reads one CRLF- or LF-terminated line, without its terminator,
// no longer than the reader's buffer, so a server cannot make kmd buffer an
// endless line. The slice is valid until the next read.
func rawLine(br *bufio.Reader) ([]byte, error) {
	b, err := br.ReadSlice('\n')
	if errors.Is(err, bufio.ErrBufferFull) {
		return nil, errLongLine
	} else if err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(b[:len(b)-1], []byte("\r")), nil
}

// readLine is rawLine for the HTTP framing, where the end of the stream
// can never come in the middle.
func readLine(br *bufio.Reader) (string, error) {
	b, err := rawLine(br)
	if errors.Is(err, io.EOF) {
		return "", io.ErrUnexpectedEOF
	}
	return string(b), err
}

// chunkedReader decodes Transfer-Encoding: chunked, which llama.cpp uses to
// stream. Chunk extensions and trailers are read and ignored.
type chunkedReader struct {
	r       *bufio.Reader
	left    int64 // bytes left in the current chunk
	started bool  // a chunk was read, so a CRLF precedes the next size
	eof     bool
}

func (c *chunkedReader) Read(p []byte) (int, error) {
	if c.eof {
		return 0, io.EOF
	}
	if c.left == 0 {
		if err := c.next(); err != nil {
			return 0, err
		}
		if c.eof {
			return 0, io.EOF
		}
	}
	n, err := c.r.Read(p[:min(int64(len(p)), c.left)])
	c.left -= int64(n)
	if errors.Is(err, io.EOF) {
		err = io.ErrUnexpectedEOF
	}
	return n, err
}

// next reads the size line of the next chunk, or the trailers after the last.
func (c *chunkedReader) next() error {
	if c.started {
		if line, err := readLine(c.r); err != nil {
			return err
		} else if line != "" {
			return errors.New("malformed chunk: no CRLF after its data")
		}
	}
	c.started = true
	line, err := readLine(c.r)
	if err != nil {
		return err
	}
	size, _, _ := strings.Cut(line, ";")
	size = strings.TrimSpace(size)
	n, err := strconv.ParseUint(size, 16, 63)
	if err != nil { // ParseUint also refuses "", signs and prefixes
		return fmt.Errorf("malformed chunk size %q", printable(size, shortText))
	}
	if n == 0 {
		c.eof = true
		return skipTrailers(c.r)
	}
	c.left = int64(n)
	return nil
}

func skipTrailers(br *bufio.Reader) error {
	for range maxHeaders {
		if line, err := readLine(br); err != nil || line == "" {
			return err
		}
	}
	return errors.New("too many trailers")
}

// cappedReader fails, rather than ending quietly, once a response exceeds
// its cap: a truncated answer must not look complete.
type cappedReader struct {
	r    io.Reader
	left int64
}

func (c *cappedReader) Read(p []byte) (int, error) {
	if c.left <= 0 {
		// at the cap: fine if the body ends exactly here
		if n, err := c.r.Read(make([]byte, 1)); n == 0 && errors.Is(err, io.EOF) {
			return 0, io.EOF
		}
		return 0, errTooLarge
	}
	n, err := c.r.Read(p[:min(int64(len(p)), c.left)])
	c.left -= int64(n)
	return n, err
}

// printable keeps the printable ASCII of a server-supplied string, at most
// max bytes of it, so an error message cannot carry terminal escapes.
func printable(s string, max int) string {
	var b strings.Builder
	for i := 0; i < len(s) && b.Len() < max; i++ {
		if s[i] >= ' ' && s[i] <= '~' {
			b.WriteByte(s[i])
		}
	}
	return b.String()
}
