package termimg

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// Security limits for untrusted input.
const (
	maxFileBytes    = 20 << 20   // largest image file read from disk
	maxDataURIBytes = 10 << 20   // largest decoded data: URI payload
	maxDimension    = 8192       // largest width or height in pixels
	maxPixels       = 40_000_000 // largest width×height
)

// Load resolves src and validates it as a PNG, JPEG or GIF image. src is a
// path relative to baseDir, an absolute path, a file: URL or a base64 data:
// URI. Any other URL (http:, https:, ftp:, //host, …) returns ErrRemote:
// images are never fetched over the network.
//
// Only the header is parsed here; pixels are decoded on demand by Render.
func Load(src, baseDir string) (*Image, error) {
	src = strings.TrimSpace(src)
	if src == "" {
		return nil, fmt.Errorf("termimg: empty image source")
	}
	if strings.HasPrefix(src, "//") {
		return nil, ErrRemote
	}
	switch scheme := strings.ToLower(urlScheme(src)); scheme {
	case "":
		return loadPath(src, baseDir)
	case "data":
		data, err := parseDataURI(src)
		if err != nil {
			return nil, err
		}
		return validate(data)
	case "file":
		u, err := url.Parse(src)
		if err != nil || (u.Host != "" && u.Host != "localhost") || u.Path == "" {
			return nil, ErrRemote
		}
		return loadFile(u.Path)
	default:
		return nil, ErrRemote
	}
}

// urlScheme returns the RFC 3986 scheme of s, or "" when s looks like a
// path. Single-letter schemes are treated as paths (Windows drive letters).
func urlScheme(s string) string {
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z':
		case i > 0 && ('0' <= c && c <= '9' || c == '+' || c == '-' || c == '.'):
		case c == ':' && i > 1:
			return s[:i]
		default:
			return ""
		}
	}
	return ""
}

// loadPath resolves a (possibly relative, possibly percent-encoded) path.
func loadPath(p, baseDir string) (*Image, error) {
	if !filepath.IsAbs(p) {
		p = filepath.Join(baseDir, p)
	}
	img, err := loadFile(p)
	if errors.Is(err, fs.ErrNotExist) && strings.Contains(p, "%") {
		if u, uerr := url.PathUnescape(p); uerr == nil && u != p {
			return loadFile(u)
		}
	}
	return img, err
}

// loadFile reads and validates an image file. Only regular files are opened:
// a FIFO or device could block forever or produce unbounded data.
func loadFile(path string) (*Image, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: %s", ErrNotRegular, path)
	}
	if fi.Size() > maxFileBytes {
		return nil, fmt.Errorf("%w: %s is %d bytes", ErrTooLarge, path, fi.Size())
	}
	// #nosec G304 -- the path comes from the (untrusted) document: it was
	// checked to be a regular file above, is opened non-blocking so a FIFO
	// swapped in meanwhile cannot hang us, is re-checked on the descriptor,
	// size-limited, and sniffed as PNG/JPEG/GIF before use.
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	// Re-check on the open descriptor in case the path was swapped.
	if fi, err = f.Stat(); err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: %s", ErrNotRegular, path)
	}
	data, err := io.ReadAll(io.LimitReader(f, maxFileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxFileBytes {
		return nil, fmt.Errorf("%w: %s", ErrTooLarge, path)
	}
	return validate(data)
}

// parseDataURI decodes "data:[<mediatype>][;params];base64,<payload>".
// Only base64 payloads with an image/* (or empty) media type are accepted.
func parseDataURI(src string) ([]byte, error) {
	if len(src) > base64.StdEncoding.EncodedLen(maxDataURIBytes)+4096 {
		return nil, ErrTooLarge
	}
	meta, payload, ok := strings.Cut(src[len("data:"):], ",")
	if !ok {
		return nil, ErrDataURI
	}
	params := strings.Split(meta, ";")
	if len(params) < 2 || !strings.EqualFold(strings.TrimSpace(params[len(params)-1]), "base64") {
		return nil, fmt.Errorf("%w: only base64 payloads are supported", ErrDataURI)
	}
	if mt := strings.ToLower(strings.TrimSpace(params[0])); mt != "" && !strings.HasPrefix(mt, "image/") {
		return nil, fmt.Errorf("%w: media type %q is not an image", ErrDataURI, mt)
	}
	payload = stripSpace(payload)
	if strings.Contains(payload, "%") {
		u, err := url.PathUnescape(payload)
		if err != nil {
			return nil, ErrDataURI
		}
		payload = u
	}
	enc := base64.StdEncoding
	if len(payload)%4 != 0 {
		enc = base64.RawStdEncoding
		payload = strings.TrimRight(payload, "=")
	}
	data, err := enc.DecodeString(payload)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDataURI, err)
	}
	if len(data) > maxDataURIBytes {
		return nil, ErrTooLarge
	}
	return data, nil
}

// stripSpace removes ASCII whitespace (data URIs may be wrapped).
func stripSpace(s string) string {
	if !strings.ContainsAny(s, " \t\r\n\f") {
		return s
	}
	return strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '\r', '\n', '\f':
			return -1
		}
		return r
	}, s)
}

// validate sniffs data from its header only and enforces the dimension
// limits before anything is decoded (decompression bomb protection).
func validate(data []byte) (*Image, error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnsupported, err)
	}
	switch format {
	case "png", "jpeg", "gif":
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupported, format)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return nil, fmt.Errorf("%w: empty image", ErrUnsupported)
	}
	if cfg.Width > maxDimension || cfg.Height > maxDimension || cfg.Width*cfg.Height > maxPixels {
		return nil, fmt.Errorf("%w: %d×%d pixels", ErrTooLarge, cfg.Width, cfg.Height)
	}
	return &Image{data: data, format: format, w: cfg.Width, h: cfg.Height}, nil
}

// decodeBytes fully decodes validated image data with the decoder for its
// format (avoiding the global registry so no other format can sneak in).
func decodeBytes(data []byte, format string) (image.Image, error) {
	r := bytes.NewReader(data)
	switch format {
	case "png":
		return png.Decode(r)
	case "jpeg":
		return jpeg.Decode(r)
	case "gif":
		return gif.Decode(r)
	}
	return nil, ErrUnsupported
}
