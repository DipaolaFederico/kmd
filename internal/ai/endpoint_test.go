//go:build kmdai

package ai

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestParseEndpoint(t *testing.T) {
	for _, tc := range []struct{ url, network, address, host string }{
		{"unix:///run/user/1000/kmd-llama.sock", "unix", "/run/user/1000/kmd-llama.sock", "localhost"},
		{"http://127.0.0.1:8091", "tcp", "127.0.0.1:8091", "127.0.0.1:8091"},
		{"http://127.0.0.1:8091/", "tcp", "127.0.0.1:8091", "127.0.0.1:8091"},
		{"http://127.1.2.3:8091", "tcp", "127.1.2.3:8091", "127.1.2.3:8091"},
		{"http://localhost:8091", "tcp", "127.0.0.1:8091", "127.0.0.1:8091"},
		{"http://[::1]:8091", "tcp", "[::1]:8091", "[::1]:8091"},
		{"http://[::ffff:127.0.0.1]:8091", "tcp", "127.0.0.1:8091", "127.0.0.1:8091"},
		{"http://127.0.0.1", "tcp", "127.0.0.1:80", "127.0.0.1:80"},
	} {
		ep, err := parseEndpoint(tc.url)
		if err != nil || ep != (endpoint{tc.network, tc.address, tc.host}) {
			t.Errorf("parseEndpoint(%q) = %+v, %v", tc.url, ep, err)
		}
	}
}

func TestParseEndpointRejects(t *testing.T) {
	for _, raw := range []string{
		"http://example.com:8091",       // DNS
		"http://192.168.1.10:8091",      // another machine
		"http://0.0.0.0:8091",           // not loopback
		"http://localhost.:8091",        // would need a resolver
		"http://127.0.0.1.nip.io:8091",  // a name that resolves to loopback
		"http://2130706433:8091",        // 127.0.0.1 as a number
		"http://[fe80::1%25lo]:8091",    // zone
		"https://127.0.0.1:8091",        // TLS
		"ftp://127.0.0.1",               // scheme
		"127.0.0.1:8091",                // no scheme
		"http://user:pw@127.0.0.1:8091", // credentials in the URL
		"http://127.0.0.1:8091/v1",      // path
		"http://127.0.0.1:8091/?a=b",    // query
		"http://127.0.0.1:8091#x",       // fragment
		"http://127.0.0.1:0",            // port
		"http://127.0.0.1:99999",        // port
		"unix://host/sock",              // host in a socket URL
		"unix:relative.sock",            // opaque
		"unix:///",                      // no socket
		"%zz",                           // not a URL
	} {
		if ep, err := parseEndpoint(raw); err == nil {
			t.Errorf("parseEndpoint(%q) = %+v, want an error", raw, ep)
		} else if !strings.Contains(err.Error(), "ai-url") {
			t.Errorf("parseEndpoint(%q): %v does not name ai-url", raw, err)
		}
	}
}

func writeKey(t *testing.T, body string, mode os.FileMode) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "ai-key")
	if err := os.WriteFile(p, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestReadKey(t *testing.T) {
	for _, tc := range []struct{ path, want string }{
		{"", ""},
		{filepath.Join(t.TempDir(), "missing"), ""},
		{writeKey(t, "s3cr3t-KEY\n", 0o600), "s3cr3t-KEY"},
		{writeKey(t, "s3cr3t\r\n", 0o400), "s3cr3t"},
		{writeKey(t, "s3cr3t", 0o600), "s3cr3t"},
	} {
		if got, err := readKey(tc.path); got != tc.want || err != nil {
			t.Errorf("readKey(%q) = %q, %v; want %q", tc.path, got, err, tc.want)
		}
	}
}

func TestReadKeyRejects(t *testing.T) {
	cases := map[string]string{
		"empty":      writeKey(t, "\n", 0o600),
		"two lines":  writeKey(t, "a\nb\n", 0o600),
		"header":     writeKey(t, "a\r\nX-Evil: 1", 0o600),
		"space":      writeKey(t, "a b", 0o600),
		"escape":     writeKey(t, "a\x1b[2J", 0o600),
		"non-ASCII":  writeKey(t, "clé", 0o600),
		"too large":  writeKey(t, strings.Repeat("k", maxKeyFile+1), 0o600),
		"directory":  t.TempDir(),
		"unreadable": filepath.Join(writeKey(t, "k", 0o600), "not-a-dir"),
	}
	if runtime.GOOS != "windows" {
		cases["group-readable"] = writeKey(t, "k\n", 0o640)
		cases["world-readable"] = writeKey(t, "k\n", 0o604)
	}
	for name, path := range cases {
		if key, err := readKey(path); err == nil {
			t.Errorf("%s: readKey = %q, want an error", name, key)
		}
	}
}

func TestNew(t *testing.T) {
	c, err := New(Config{URL: "http://localhost:9", KeyFile: writeKey(t, "k\n", 0o600)})
	if err != nil || c.key != "k" || c.ep.address != "127.0.0.1:9" || c.lim != defaultLimits {
		t.Errorf("New = %+v, %v", c, err)
	}
	if _, err := New(Config{URL: "http://example.com"}); err == nil {
		t.Error("New accepted a non-loopback URL")
	}
	if _, err := New(Config{URL: "http://127.0.0.1:9", KeyFile: writeKey(t, "k\n", 0o644)}); err == nil && runtime.GOOS != "windows" {
		t.Error("New accepted a world-readable key file")
	}
}
