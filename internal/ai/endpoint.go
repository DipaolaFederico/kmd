//go:build kmdai

package ai

import (
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
)

// endpoint is where the server listens, as net.Dial wants it.
type endpoint struct {
	network string // "unix" or "tcp"
	address string // socket path, or IP:port
	host    string // the Host header
}

// parseEndpoint accepts only destinations on this machine: a Unix socket or
// a loopback IP. localhost is mapped to 127.0.0.1 here rather than resolved,
// so neither DNS nor /etc/hosts can send the document elsewhere.
func parseEndpoint(raw string) (endpoint, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return endpoint{}, fmt.Errorf("ai-url %q: %w", raw, err)
	}
	bad := func(why string) (endpoint, error) {
		return endpoint{}, fmt.Errorf("ai-url %q: %s", raw, why)
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return bad("only a scheme, a host or socket path, and a port are allowed")
	}
	switch u.Scheme {
	case "unix":
		if u.Host != "" || !filepath.IsAbs(u.Path) || strings.HasSuffix(u.Path, "/") {
			return bad("want unix:///absolute/path/to/socket")
		}
		return endpoint{network: "unix", address: u.Path, host: "localhost"}, nil
	case "http":
		if u.Path != "" && u.Path != "/" {
			return bad("the server must be at the root, with no path")
		}
		return loopbackTCP(u, bad)
	default:
		return bad("want unix:// or http:// (kmd has no TLS: the server is on this machine)")
	}
}

// loopbackTCP checks the host and port of an http:// ai-url.
func loopbackTCP(u *url.URL, bad func(string) (endpoint, error)) (endpoint, error) {
	port := u.Port()
	if port == "" {
		port = "80"
	}
	if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
		return bad("invalid port")
	}
	name := u.Hostname()
	if name == "localhost" {
		name = "127.0.0.1"
	}
	ip, err := netip.ParseAddr(name)
	if err != nil || !ip.IsLoopback() || ip.Zone() != "" {
		return bad("not a loopback address: use 127.0.0.1, [::1], localhost or a unix:// socket")
	}
	addr := net.JoinHostPort(ip.Unmap().String(), port)
	return endpoint{network: "tcp", address: addr, host: addr}, nil
}
