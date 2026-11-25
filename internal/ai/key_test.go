//go:build kmdai

package ai

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

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
