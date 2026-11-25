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
	"time"
)

// limits bound every request, so a broken or hostile server can make kmd
// wait or allocate only so much. The values come from issue #44. llama.cpp
// sends the headers as soon as it accepts a request (8 ms for a cold 26 KB
// document whose first token took 25 s), so the long wait for the first
// token falls under idle, and while it works it sends a keep-alive comment
// every 30 s.
type limits struct {
	connect  time.Duration // to establish the connection
	header   time.Duration // from sending the request to the end of the headers
	idle     time.Duration // between two reads of the body
	total    time.Duration // for the whole exchange
	maxBytes int64         // everything read from the server, framing included
	maxLine  int           // a header, chunk-size or event-stream line
}

var defaultLimits = limits{
	connect: 5 * time.Second,
	header:  5 * time.Second,
	idle:    60 * time.Second,
	total:   180 * time.Second,
	// 2048 tokens of answer and reasoning at ~420 bytes of event each is
	// ~0.9 MB; 8 MiB leaves room for chattier servers.
	maxBytes: 8 << 20,
	maxLine:  64 << 10,
}

// ErrUnreachable is returned, wrapped, when nothing answers at the configured
// address: the server is not running, or listens elsewhere.
var ErrUnreachable = errors.New("cannot reach the local AI server")

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
	stop   func() bool // unregisters the AfterFunc that closes conn on cancel
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
		err = c.explain(ctx, fmt.Errorf("%w at %s: %w", ErrUnreachable, c.ep.address, err))
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

// exchange writes the request and reads the headers into resp. The headers,
// and an error response's body, must arrive within the header limit; only
// the body of a 200 response gets the idle limit.
func (c *Client) exchange(resp *response, method, path string, body []byte) error {
	_ = resp.conn.SetDeadline(time.Now().Add(c.lim.header))
	if _, err := resp.conn.Write(c.request(method, path, body)); err != nil {
		return err
	}
	ic := &idleConn{Conn: resp.conn}
	// the cap sits under the line reader, so chunk framing counts too
	br := bufio.NewReaderSize(&cappedReader{r: ic, left: c.lim.maxBytes}, c.lim.maxLine)
	h, err := readHeader(br)
	if err != nil {
		return err
	}
	resp.body = h.body(br)
	if err := h.check(resp.body); err != nil {
		return err
	}
	ic.idle = c.lim.idle // from now on, each read has its own deadline (and ctx the total)
	return nil
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
	case errors.Is(context.Cause(ctx), errTotal):
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
