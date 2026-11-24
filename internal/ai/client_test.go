//go:build kmdai

package ai

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// collect runs Chat and returns the answer and the reasoning it streamed.
func collect(ctx context.Context, c *Client, req Request) (answer, reasoning string, err error) {
	err = c.Chat(ctx, req, func(d Delta) error {
		answer += d.Content
		reasoning += d.Reasoning
		return nil
	})
	return answer, reasoning, err
}

func newClient(t *testing.T, url string) *Client {
	t.Helper()
	c, err := New(Config{URL: url})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// sse writes one event per payload, flushed at once like a streaming server.
func sse(w http.ResponseWriter, payloads ...string) {
	w.Header().Set("Content-Type", "text/event-stream")
	for _, p := range payloads {
		fmt.Fprintf(w, "data: %s\n\n", p)
		w.(http.Flusher).Flush()
	}
}

func TestChat(t *testing.T) {
	var got struct {
		header http.Header
		host   string
		body   map[string]any
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.header, got.host = r.Header, r.Host
		if r.Method != "POST" || r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&got.body)
		fmt.Fprint(w, ": ping\n\n")
		sse(w,
			`{"choices":[{"delta":{"role":"assistant"}}]}`,
			`{"choices":[{"delta":{"reasoning_content":"hmm"}}]}`,
			`{"choices":[{"delta":{"content":"Hola"}}]}`,
			`{"choices":[{"delta":{"content":", mundo"}}],"error":null}`,
			`[DONE]`)
	}))
	defer srv.Close()
	c := newClient(t, srv.URL)
	c.key = "s3cr3t"
	answer, reasoning, err := collect(context.Background(), c, Request{
		Messages:  []Message{{"system", "Be brief."}, {"user", "Hi"}},
		MaxTokens: 2048,
	})
	if err != nil || answer != "Hola, mundo" || reasoning != "hmm" {
		t.Fatalf("Chat = %q, %q, %v", answer, reasoning, err)
	}
	if a := got.header.Get("Authorization"); a != "Bearer s3cr3t" {
		t.Errorf("Authorization = %q", a)
	}
	if got.host != strings.TrimPrefix(srv.URL, "http://") {
		t.Errorf("Host = %q", got.host)
	}
	want := `{"max_tokens":2048,"messages":[{"content":"Be brief.","role":"system"},{"content":"Hi","role":"user"}],"stream":true}`
	if b, _ := json.Marshal(got.body); string(b) != want {
		t.Errorf("request body\n got %s\nwant %s", b, want)
	}
}

func TestChatUnixSocket(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "s")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Skip("no Unix sockets:", err)
	}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "localhost" || r.Header.Get("Authorization") != "" {
			t.Errorf("Host %q, Authorization %q", r.Host, r.Header.Get("Authorization"))
		}
		sse(w, `{"choices":[{"delta":{"content":"ok"}}]}`, `[DONE]`)
	}))
	srv.Listener = ln
	srv.Start()
	defer srv.Close()
	answer, _, err := collect(context.Background(), newClient(t, "unix://"+sock), Request{})
	if err != nil || answer != "ok" {
		t.Errorf("Chat over a Unix socket = %q, %v", answer, err)
	}
}

func TestChatStopsWhenCallbackFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		sse(w, `{"choices":[{"delta":{"content":"a"}}]}`, `{"choices":[{"delta":{"content":"b"}}]}`, `[DONE]`)
	}))
	defer srv.Close()
	stop := errors.New("enough")
	n := 0
	err := newClient(t, srv.URL).Chat(context.Background(), Request{}, func(Delta) error { n++; return stop })
	if !errors.Is(err, stop) || n != 1 {
		t.Errorf("Chat = %v after %d calls, want %v after 1", err, n, stop)
	}
}

func TestInfo(t *testing.T) {
	for _, tc := range []struct {
		props string // "" = 404, as on servers other than llama.cpp
		want  ServerInfo
	}{
		{`{"modalities":{"vision":true,"audio":false}}`, ServerInfo{"LFM3-VL-2B[2J", true}},
		{`{"modalities":{"vision":false}}`, ServerInfo{"LFM3-VL-2B[2J", false}},
		{"", ServerInfo{"LFM3-VL-2B[2J", false}},
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.URL.Path == "/v1/models":
				fmt.Fprint(w, `{"object":"list","data":[{"id":"LFM3-VL-2B\u001b[2J"}]}`)
			case r.URL.Path == "/props" && tc.props != "":
				fmt.Fprint(w, tc.props)
			default:
				http.NotFound(w, r)
			}
		}))
		info, err := newClient(t, srv.URL).Info(context.Background())
		srv.Close()
		if err != nil || info != tc.want {
			t.Errorf("props %q: Info = %+v, %v; want %+v", tc.props, info, err, tc.want)
		}
	}
}

func TestInfoErrors(t *testing.T) {
	for _, body := range []string{"not json", ""} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if body == "" {
				http.Error(w, `{"error":{"message":"no model loaded"}}`, http.StatusServiceUnavailable)
				return
			}
			fmt.Fprint(w, body)
		}))
		_, err := newClient(t, srv.URL).Info(context.Background())
		srv.Close()
		if err == nil {
			t.Errorf("Info with %q: no error", body)
		}
	}
	// nothing listening
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := ln.Addr().String()
	ln.Close()
	if _, err := newClient(t, "http://"+addr).Info(context.Background()); err == nil || !strings.Contains(err.Error(), "cannot reach") {
		t.Errorf("Info with no server: %v", err)
	}
}

// rawServer answers every connection with serve, after reading the request
// headers, so tests can send what no well-behaved HTTP server would.
func rawServer(t *testing.T, serve func(net.Conn)) *Client {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				br := bufio.NewReader(conn)
				for {
					if line, err := br.ReadString('\n'); err != nil || line == "\r\n" {
						break
					}
				}
				serve(conn)
			}()
		}
	}()
	return newClient(t, "http://"+ln.Addr().String())
}

// reply writes s and closes the connection.
func reply(s string) func(net.Conn) {
	return func(c net.Conn) { _, _ = io.WriteString(c, s) }
}

const okChunked = "HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n"

func chunked(s string) string { return fmt.Sprintf("%x\r\n%s\r\n", len(s), s) }

func TestChatRaw(t *testing.T) {
	done := "data: [DONE]\n\n"
	event := `data: {"choices":[{"delta":{"content":"ok"}}]}` + "\n\n"
	for name, resp := range map[string]string{
		"content length": fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Length: %d\r\n\r\n%s", len(event+done), event+done),
		"until close":    "HTTP/1.0 200 OK\r\n\r\n" + event + done,
		"chunked":        okChunked + chunked(event[:10]) + chunked(event[10:]+done) + "0\r\n\r\n",
		"chunk extension and trailer": okChunked + fmt.Sprintf("%X;x=y\r\n", len(event)) + event + "\r\n" +
			chunked(done) + "0\r\nX-Trailer: 1\r\n\r\n",
		"CRLF events": "HTTP/1.1 200 OK\r\n\r\n" + strings.ReplaceAll(event+done, "\n", "\r\n"),
	} {
		answer, _, err := collect(context.Background(), rawServer(t, reply(resp)), Request{})
		if err != nil || answer != "ok" {
			t.Errorf("%s: Chat = %q, %v", name, answer, err)
		}
	}
}

func TestChatRejects(t *testing.T) {
	long := strings.Repeat("x", defaultLimits.maxLine)
	event := `data: {"choices":[{"delta":{"content":"ok"}}]}` + "\n\n"
	for _, tc := range []struct{ name, resp, want string }{
		{"redirect", "HTTP/1.1 302 Found\r\nLocation: http://evil.example/\r\n\r\n", "does not follow redirects"},
		{"server error", "HTTP/1.1 500 Internal Server Error\r\n\r\n" +
			`{"error":{"message":"out of memory\u001b]52;c;x"}}`, "500: out of memory]52;c;x"},
		{"plain error", "HTTP/1.1 503 Busy\u001b[2J\r\n\r\n", "503: Busy[2J"},
		{"not HTTP", "SSH-2.0-OpenSSH\r\n\r\n", "not an HTTP/1.x response"},
		{"bad status", "HTTP/1.1 2000 OK\r\n\r\n", "not an HTTP/1.x response"},
		{"empty", "", "unexpected EOF"},
		{"long header", "HTTP/1.1 200 OK\r\nX: " + long + "\r\n\r\n", "line too long"},
		{"many headers", "HTTP/1.1 200 OK\r\n" + strings.Repeat("X: y\r\n", maxHeaders+1) + "\r\n", "too many response headers"},
		{"gzip", "HTTP/1.1 200 OK\r\nTransfer-Encoding: gzip\r\n\r\n", "unsupported transfer encoding"},
		{"bad length", "HTTP/1.1 200 OK\r\nContent-Length: -1\r\n\r\n", "invalid content length"},
		{"bad chunk", okChunked + "zz\r\n", "malformed chunk size"},
		{"signed chunk", okChunked + "+5\r\nhello\r\n", "malformed chunk size"},
		{"chunk overrun", okChunked + "2\r\nhello\r\n", "malformed chunk"},
		{"short chunk", okChunked + "ff\r\nhello", "unexpected EOF"},
		{"many trailers", okChunked + "0\r\n" + strings.Repeat("X: y\r\n", maxHeaders+1), "too many trailers"},
		{"long event line", "HTTP/1.1 200 OK\r\n\r\ndata: " + long, "line too long"},
		{"long event", "HTTP/1.1 200 OK\r\n\r\n" + strings.Repeat("data: "+long[:1000]+"\n", 70), "line too long"},
		{"no DONE", "HTTP/1.1 200 OK\r\n\r\n" + event, "ended before the server said it was done"},
		{"error event", "HTTP/1.1 200 OK\r\n\r\n" + `data: {"error":{"message":"context too long"}}` + "\n\n", "failed: context too long"},
		{"string error", "HTTP/1.1 200 OK\r\n\r\n" + `data: {"error":"boom"}` + "\n\n", "failed: boom"},
		{"odd error", "HTTP/1.1 200 OK\r\n\r\n" + `data: {"error":42}` + "\n\n", "failed: unknown error"},
		{"bad event", "HTTP/1.1 200 OK\r\n\r\ndata: {\n\n", "invalid event"},
	} {
		_, _, err := collect(context.Background(), rawServer(t, reply(tc.resp)), Request{})
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: Chat error %v, want %q", tc.name, err, tc.want)
		}
	}
}

// An endless stream of small, valid events stops at the body cap.
func TestChatEndlessStream(t *testing.T) {
	c := rawServer(t, func(conn net.Conn) {
		_, _ = io.WriteString(conn, "HTTP/1.1 200 OK\r\n\r\n")
		ev := []byte(`data: {"choices":[{"delta":{"content":"spam"}}]}` + "\n\n")
		for {
			if _, err := conn.Write(ev); err != nil {
				return
			}
		}
	})
	c.lim.maxBody = 1 << 20
	n := 0
	err := c.Chat(context.Background(), Request{}, func(Delta) error { n++; return nil })
	if !errors.Is(err, errTooLarge) || n == 0 {
		t.Errorf("Chat = %v after %d events, want %v", err, n, errTooLarge)
	}
}

// A body exactly at the cap is complete, not too large.
func TestCappedReaderAtCap(t *testing.T) {
	b, err := io.ReadAll(&cappedReader{r: strings.NewReader("abc"), left: 3})
	if err != nil || string(b) != "abc" {
		t.Errorf("ReadAll = %q, %v", b, err)
	}
}

// hang sends what it is given and then nothing, keeping the connection open.
func hang(s string) func(net.Conn) {
	return func(c net.Conn) {
		_, _ = io.WriteString(c, s)
		_, _ = io.Copy(io.Discard, c) // until kmd closes it
	}
}

func TestTimeouts(t *testing.T) {
	stream := okChunked + chunked(`data: {"choices":[{"delta":{"content":"a"}}]}`+"\n\n")
	for _, tc := range []struct {
		name, sent, want string
		lim              func(*limits)
	}{
		{"headers", "HTTP/1.1 200 OK\r\n", "stopped responding", func(l *limits) { l.header = 50 * time.Millisecond }},
		{"idle", stream, "stopped responding", func(l *limits) { l.idle = 50 * time.Millisecond }},
		{"total", stream, "did not finish within", func(l *limits) { l.total = 100 * time.Millisecond }},
	} {
		c := rawServer(t, hang(tc.sent))
		tc.lim(&c.lim)
		start := time.Now()
		_, _, err := collect(context.Background(), c, Request{})
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: Chat error %v, want %q", tc.name, err, tc.want)
		}
		if d := time.Since(start); d > 5*time.Second {
			t.Errorf("%s: took %v", tc.name, d)
		}
	}
}

// Cancelling the context closes the connection at once, mid-stream.
func TestCancel(t *testing.T) {
	closed := make(chan struct{})
	c := rawServer(t, func(conn net.Conn) {
		_, _ = io.WriteString(conn, okChunked+chunked(`data: {"choices":[{"delta":{"content":"a"}}]}`+"\n\n"))
		_, _ = io.Copy(io.Discard, conn)
		close(closed) // kmd closed its end
	})
	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error, 1)
	go func() {
		errc <- c.Chat(ctx, Request{}, func(Delta) error { cancel(); return nil })
	}()
	select {
	case err := <-errc:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Chat = %v, want %v", err, context.Canceled)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Chat did not return after cancel")
	}
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("the connection stayed open after cancel")
	}
}

// Cancelled before the server even answers: the wait for headers stops too.
func TestCancelWhileWaitingForHeaders(t *testing.T) {
	c := rawServer(t, hang(""))
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := c.Info(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Info = %v, want %v", err, context.DeadlineExceeded)
	}
}

func TestPrintable(t *testing.T) {
	if got := printable("a\x1b]52;c;x\x07bé\r\nc", 100); got != "a]52;c;xbc" {
		t.Errorf("printable = %q", got)
	}
	if got := printable(strings.Repeat("a", 10), 4); got != "aaaa" {
		t.Errorf("printable cap = %q", got)
	}
}
