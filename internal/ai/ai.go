//go:build kmdai

// Package ai is kmd's experimental local AI assistant. It is compiled only
// with -tags kmdai, so the released binary never contains it and keeps its
// promise of making no network requests. kmd talks only to a model server
// the user runs on the same machine; it never installs, downloads or starts
// one. See docs/experimental/ai.md.
package ai

import (
	"net/url"
	"path/filepath"
)

// Config says where the local model server is and how to authenticate.
type Config struct {
	// URL is unix:///path/to/socket or http://127.0.0.1:PORT.
	URL string
	// KeyFile is the absolute path of the server's API key ("" = none).
	KeyFile string
}

// fallbackURL is used without a per-user runtime directory for a socket.
// Not 8080: other software often takes it.
const fallbackURL = "http://127.0.0.1:8091"

// DefaultURL is the server address when ai-url is not set. A Unix socket in
// $XDG_RUNTIME_DIR is preferred: only its owner can create or reach it,
// whereas any local user can listen on, or connect to, a loopback port.
func DefaultURL(getenv func(string) string) string {
	dir := getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		return fallbackURL
	}
	// escaped, so a directory name with '#', '?' or '%' still parses back
	return (&url.URL{Scheme: "unix", Path: filepath.Join(dir, "kmd-llama.sock")}).String()
}
