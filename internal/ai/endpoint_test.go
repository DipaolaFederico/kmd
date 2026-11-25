//go:build kmdai

package ai

import (
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
