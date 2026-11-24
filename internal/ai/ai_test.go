//go:build kmdai

package ai

import (
	"net/url"
	"testing"
)

func TestDefaultURL(t *testing.T) {
	for _, tc := range []struct{ runtimeDir, want string }{
		{"", "http://127.0.0.1:8091"},
		{"/run/user/1000", "unix:///run/user/1000/kmd-llama.sock"},
	} {
		if got := DefaultURL(func(string) string { return tc.runtimeDir }); got != tc.want {
			t.Errorf("DefaultURL(%q) = %q, want %q", tc.runtimeDir, got, tc.want)
		}
	}
	const odd = "/run/a b#c?d%e"
	u, err := url.Parse(DefaultURL(func(string) string { return odd }))
	if err != nil || u.Path != odd+"/kmd-llama.sock" {
		t.Errorf("odd runtime directory: %v, %v", u, err)
	}
}
