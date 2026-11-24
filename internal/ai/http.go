//go:build kmdai

package ai

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

// limits bound every request, so a broken or hostile server can make kmd
// wait or allocate only so much. The values come from the measurements in
// issue #44: a cold whole document takes about 25 s to the first token on a
// CPU, and llama.cpp sends a keep-alive comment every 30 s while it works.
type limits struct {
	connect time.Duration // to establish the connection
	header  time.Duration // from sending the request to the end of the headers
	idle    time.Duration // between two reads of the body
	total   time.Duration // for the whole exchange
	maxBody int64         // decoded body bytes
	maxLine int           // a header, chunk-size or event-stream line
}

var defaultLimits = limits{
	connect: 5 * time.Second,
	header:  5 * time.Second,
	idle:    60 * time.Second,
	total:   180 * time.Second,
	maxBody: 8 << 20, // ~2048 tokens of answer plus reasoning, ~400 bytes of JSON each
	maxLine: 64 << 10,
}

// maxHeaders bounds the number of response header lines.
const maxHeaders = 100

var (
	errTooLarge = errors.New("response too large")
	errLongLine = errors.New("response line too long")
	errTotal    = errors.New("total time limit") // the cause when limits.total runs out
)

// Client talks to the local model server with a minimal HTTP/1.1 client of
// its own rather than net/http: no TLS, proxies, redirects, cookies or
// connection reuse, and every read bounded in size and time.
type Client struct {
	ep  endpoint
	key string
	lim limits
}

// New checks the configuration and reads the key file. It does not connect.
func New(cfg Config) (*Client, error) {
	ep, err := parseEndpoint(cfg.URL)
	if err != nil {
		return nil, err
	}
	key, err := readKey(cfg.KeyFile)
	if err != nil {
		return nil, err
	}
	return &Client{ep: ep, key: key, lim: defaultLimits}, nil
}

// response is the body of a 200 response; Close it when done.
type response struct {
	ctx    context.Context // the request's, with the total timeout
	body   io.Reader
	conn   net.Conn
	cancel context.CancelFunc
	stop   func() bool
}

func (r *response) Close() {
	r.stop()
	r.cancel()
	_ = r.conn.Close()
}

// do sends one request on a new connection and reads the response headers.
// Cancelling ctx closes the connection, which interrupts any read at once.
func (c *Client) do(ctx context.Context, method, path string, body []byte) (*response, error) {
	ctx, cancel := context.WithTimeoutCause(ctx, c.lim.total, errTotal)
	d := net.Dialer{Timeout: c.lim.connect}
	conn, err := d.DialContext(ctx, c.ep.network, c.ep.address)
	if err != nil {
		err = c.explain(ctx, fmt.Errorf("cannot reach the local AI server at %s: %w", c.ep.address, err))
		cancel()
		return nil, err
	}
	resp := &response{ctx: ctx, conn: conn, cancel: cancel, stop: context.AfterFunc(ctx, func() { _ = conn.Close() })}
	if err := c.exchange(resp, method, path, body); err != nil {
		err = c.explain(ctx, err) // before Close cancels ctx
		resp.Close()
		return nil, err
	}
	return resp, nil
}

// exchange writes the request and reads the headers into resp.
func (c *Client) exchange(resp *response, method, path string, body []byte) error {
	_ = resp.conn.SetDeadline(time.Now().Add(c.lim.header))
	if _, err := resp.conn.Write(c.request(method, path, body)); err != nil {
		return err
	}
	ic := &idleConn{Conn: resp.conn}
	br := bufio.NewReaderSize(ic, c.lim.maxLine)
	h, err := readHeader(br)
	if err != nil {
		return err
	}
	ic.idle = c.lim.idle // from now on, each read has its own deadline (and ctx the total)
	resp.body = h.body(br, c.lim.maxBody)
	return h.check(resp.body)
}

func (c *Client) request(method, path string, body []byte) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "%s %s HTTP/1.1\r\nHost: %s\r\nUser-Agent: kmd\r\nConnection: close\r\n", method, path, c.ep.host)
	if c.key != "" {
		fmt.Fprintf(&b, "Authorization: Bearer %s\r\n", c.key)
	}
	if body != nil {
		fmt.Fprintf(&b, "Content-Type: application/json\r\nContent-Length: %d\r\n", len(body))
	}
	b.WriteString("\r\n")
	b.Write(body)
	return b.Bytes()
}

// explain turns a failure caused by a deadline into a sentence.
func (c *Client) explain(ctx context.Context, err error) error {
	var ne net.Error
	switch {
	case context.Cause(ctx) == errTotal:
		return fmt.Errorf("the local AI server did not finish within %v", c.lim.total)
	case ctx.Err() != nil:
		return ctx.Err() // the caller cancelled, or its own deadline passed
	case errors.As(err, &ne) && ne.Timeout():
		return fmt.Errorf("the local AI server stopped responding: %w", err)
	}
	return err
}

// idleConn gives every read its own deadline, so a server that stops
// sending is noticed even while the whole answer is still within limits.
type idleConn struct {
	net.Conn
	idle time.Duration // 0: keep the deadline already set
}

func (c *idleConn) Read(p []byte) (int, error) {
	if c.idle > 0 {
		_ = c.Conn.SetReadDeadline(time.Now().Add(c.idle))
	}
	return c.Conn.Read(p)
}

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
		return header{}, fmt.Errorf("not an HTTP/1.x response: %q", printable(line, 40))
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
			return fmt.Errorf("unsupported transfer encoding %q", printable(v, 40))
		}
		h.chunked = true
	case "content-length":
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 0 {
			return fmt.Errorf("invalid content length %q", printable(v, 40))
		}
		h.length = n
	}
	return nil
}

// body decodes the body that follows the headers, capped at maxBody.
func (h header) body(br *bufio.Reader, maxBody int64) io.Reader {
	var r io.Reader = br
	switch {
	case h.chunked:
		r = &chunkedReader{r: br}
	case h.length >= 0:
		r = io.LimitReader(br, h.length)
	}
	return &cappedReader{r: r, left: maxBody}
}

// check rejects anything but 200 OK, with the server's message if it gave
// one. kmd never follows a redirect: the destination is the configured one.
func (h header) check(body io.Reader) error {
	switch {
	case h.status == 200:
		return nil
	case h.status >= 300 && h.status < 400:
		return fmt.Errorf("the local AI server answered %d %s: kmd does not follow redirects", h.status, printable(h.reason, 40))
	}
	msg := printable(h.reason, 40)
	b, _ := io.ReadAll(io.LimitReader(body, 4<<10))
	if m := errorMessage(b); m != "" {
		msg = m
	}
	return fmt.Errorf("the local AI server answered %d: %s", h.status, msg)
}

// readLine reads one CRLF- or LF-terminated line no longer than the reader's
// buffer, so a server cannot make kmd buffer an endless line.
func readLine(br *bufio.Reader) (string, error) {
	b, err := br.ReadSlice('\n')
	if errors.Is(err, bufio.ErrBufferFull) {
		return "", errLongLine
	} else if errors.Is(err, io.EOF) {
		return "", io.ErrUnexpectedEOF
	} else if err != nil {
		return "", err
	}
	return string(bytes.TrimSuffix(b[:len(b)-1], []byte("\r"))), nil
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
		if line, err := readLine(c.r); err != nil || line != "" {
			return errors.Join(errors.New("malformed chunk"), err)
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
	if err != nil || size == "" || size[0] == '+' {
		return fmt.Errorf("malformed chunk size %q", printable(size, 20))
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

// cappedReader fails, rather than ending quietly, once a body exceeds its cap:
// a truncated answer must not look complete.
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
