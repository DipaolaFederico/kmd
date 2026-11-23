//go:build kmdai

// Package ai is kmd's experimental local AI assistant. It is compiled only
// with -tags kmdai, so the released binary never contains it and keeps its
// promise of making no network requests. kmd talks only to a model server
// the user runs on the same machine; it never installs, downloads or starts
// one. See docs/experimental/ai.md.
package ai

import "path/filepath"

// Config says where the local model server is and how to authenticate.
type Config struct {
	// URL is unix:///path/to/socket or http://127.0.0.1:PORT.
	URL string
	// KeyFile holds the server's API key, if it needs one ("" = none).
	KeyFile string
}

// fallbackURL is used without a per-user runtime directory for a socket.
// Not 8080: other software often takes it.
const fallbackURL = "http://127.0.0.1:8091"

// Defaults returns the configuration for the keys the user did not set. A
// Unix socket in $XDG_RUNTIME_DIR is preferred: only its owner can reach it,
// unlike a TCP port that any local user can connect to.
func Defaults(getenv func(string) string) Config {
	c := Config{URL: fallbackURL}
	if dir := getenv("XDG_RUNTIME_DIR"); dir != "" {
		c.URL = "unix://" + filepath.Join(dir, "kmd-llama.sock")
	}
	if x := getenv("XDG_CONFIG_HOME"); x != "" {
		c.KeyFile = filepath.Join(x, "kmd", "ai-key")
	} else if h := getenv("HOME"); h != "" {
		c.KeyFile = filepath.Join(h, ".config", "kmd", "ai-key")
	}
	return c
}
