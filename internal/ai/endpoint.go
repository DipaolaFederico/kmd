//go:build kmdai

package ai

import (
	"errors"
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
	ep, err := endpointOf(raw)
	if err != nil {
		return endpoint{}, fmt.Errorf("ai-url %q: %w", raw, err)
	}
	return ep, nil
}

func endpointOf(raw string) (endpoint, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return endpoint{}, err
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return endpoint{}, errors.New("only a scheme, a host or socket path, and a port are allowed")
	}
	switch u.Scheme {
	case "unix":
		if u.Host != "" || !filepath.IsAbs(u.Path) || strings.HasSuffix(u.Path, "/") {
			return endpoint{}, errors.New("want unix:///absolute/path/to/socket")
		}
		return endpoint{network: "unix", address: u.Path, host: "localhost"}, nil
	case "http":
		if u.Path != "" && u.Path != "/" {
			return endpoint{}, errors.New("the server must be at the root, with no path")
		}
		return loopbackTCP(u)
	}
	return endpoint{}, errors.New("want unix:// or http:// (kmd has no TLS: the server is on this machine)")
}

// loopbackTCP checks the host and port of an http:// ai-url.
func loopbackTCP(u *url.URL) (endpoint, error) {
	port := u.Port()
	if port == "" {
		port = "80"
	}
	if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
		return endpoint{}, errors.New("invalid port")
	}
	name := u.Hostname()
	if name == "localhost" {
		name = "127.0.0.1"
	}
	ip, err := netip.ParseAddr(name)
	if err != nil || !ip.IsLoopback() || ip.Zone() != "" {
		return endpoint{}, errors.New("not a loopback address: use 127.0.0.1, [::1], localhost or a unix:// socket")
	}
	addr := net.JoinHostPort(ip.Unmap().String(), port)
	return endpoint{network: "tcp", address: addr, host: addr}, nil
}
