package termimg

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
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
	maxPathBytes    = 4096       // longest confined image path (PATH_MAX)
	// maxLoaderBytes bounds what one Loader (one document) reads from
	// image files and data: URIs: those bytes stay in memory as long as
	// their images can be drawn.
	maxLoaderBytes = 64 << 20
	// maxLoaderFiles bounds the image paths one Loader looks up, failed
	// ones included: each costs system calls and a comparison with every
	// file read before, so thousands of empty files named by one document
	// would cost time quadratic in their number.
	maxLoaderFiles = 1000
)

// errBudget: the images of the document have read all a Loader allows.
var errBudget = fmt.Errorf("%w: the document's images exceed %d MiB", ErrTooLarge, maxLoaderBytes>>20)

// errTooMany: the document named more image files than a Loader looks up.
var errTooMany = fmt.Errorf("%w: the document names more than %d image files", ErrTooLarge, maxLoaderFiles)

// Loader loads the images of one document. Each file is read at most once,
// however the document names it (a.png, ./a.png, a link to it): every name
// gets the same *Image, found by the identity of the open file. The bytes
// read from files and data: URIs, kept with their images, are at most
// 64 MiB per Loader, and at most 1,000 file paths are looked up; a source
// past either fails with ErrTooLarge. The zero value is ready. A Loader is
// not safe for concurrent use.
type Loader struct {
	read    int          // bytes read so far
	lookups int          // file paths looked up so far
	files   []loadedFile // files read, compared with os.SameFile
}

type loadedFile struct {
	fi  fs.FileInfo // from the open descriptor
	img *Image
	err error
}

// Load resolves src and validates it as a PNG, JPEG or GIF image. src is a
// path relative to baseDir, an absolute path, a file: URL or a base64 data:
// URI. Any other URL (http:, https:, ftp:, //host, …) and network or device
// paths (see NetworkPath) return ErrRemote: images are never fetched over
// the network.
//
// The file's structure is checked here (see validate) without decoding its
// pixels; Render decodes them on demand.
func (l *Loader) Load(src, baseDir string) (*Image, error) {
	src = strings.TrimSpace(src)
	if src == "" {
		return nil, fmt.Errorf("termimg: empty image source")
	}
	if NetworkPath(src) {
		return nil, ErrRemote
	}
	switch scheme := strings.ToLower(urlScheme(src)); scheme {
	case "":
		if err := l.lookup(); err != nil {
			return nil, err
		}
		return l.loadPath(src, baseDir)
	case "data":
		return l.loadData(src)
	case "file":
		u, err := url.Parse(src)
		if err != nil || !LocalHost(u.Host) || u.Path == "" || NetworkPath(u.Path) {
			return nil, ErrRemote
		}
		if err := l.lookup(); err != nil {
			return nil, err
		}
		return l.loadFrom(osFS{}, u.Path)
	default:
		return nil, ErrRemote
	}
}

// NetworkPath reports whether p is, on some platform, a network or device
// path rather than a local file (images and OSC 8 targets name neither): two leading separators in any mix of /
// and \ (//host/share, \\host\share, /\host, \\?\UNC\…, \\.\pipe\…), the
// NT prefix \??\, or a Windows volume that is not a drive letter. On
// Windows, a mere os.Stat of \\host\share opens an SMB connection to the
// host (and offers it the user's NTLM hash). The rule does not depend on
// the platform, but for the volume check, so it is tested everywhere.
func NetworkPath(p string) bool {
	sep := func(i int) bool { return i < len(p) && (p[i] == '/' || p[i] == '\\') }
	if sep(0) && (sep(1) || strings.HasPrefix(p[1:], "??") && sep(3)) {
		return true
	}
	v := filepath.VolumeName(p)
	return v != "" && (len(v) != 2 || v[1] != ':')
}

// LocalHost reports whether host, the host of a file: URL, names this
// machine: empty or localhost (any case). Another host is a network share.
func LocalHost(host string) bool {
	return host == "" || strings.EqualFold(host, "localhost")
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

// LoadIn is Load confined to the directory dir: src must be a data: URI or
// a relative path that stays inside dir. Absolute paths, file: URLs, ".."
// and symbolic links leading outside dir are refused: symbolic links by
// os.Root, the others with ErrOutside. With dir == "" only data: URIs load.
func (l *Loader) LoadIn(src, dir string) (*Image, error) {
	src = strings.TrimSpace(src)
	if src == "" {
		return nil, fmt.Errorf("termimg: empty image source")
	}
	if NetworkPath(src) {
		return nil, ErrRemote
	}
	switch scheme := strings.ToLower(urlScheme(src)); scheme {
	case "":
	case "data":
		return l.loadData(src)
	case "file":
		return nil, ErrOutside
	default:
		return nil, ErrRemote
	}
	if dir == "" || len(src) > maxPathBytes {
		return nil, ErrOutside
	}
	// Refuse what cannot stay inside dir before any system call (os.Root
	// walks a long "a/../a/../…" one component at a time); os.Root still
	// guards against symbolic links.
	p, ok := localPath(src)
	if !ok {
		return nil, ErrOutside
	}
	if err := l.lookup(); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	img, err := l.loadFrom(root, p)
	if errors.Is(err, fs.ErrNotExist) && strings.Contains(src, "%") {
		if u, uerr := url.PathUnescape(src); uerr == nil && u != src {
			if p, ok = localPath(u); !ok {
				return nil, ErrOutside
			}
			img, err = l.loadFrom(root, p)
		}
	}
	return img, err
}

// lookup counts a file path about to be looked up, or fails once the
// Loader has looked up maxLoaderFiles.
func (l *Loader) lookup() error {
	if l.lookups >= maxLoaderFiles {
		return errTooMany
	}
	l.lookups++
	return nil
}

// localPath cleans a slash-separated relative path and reports whether it
// stays below its starting directory (no "..", absolute path, volume name
// or leading separator).
func localPath(p string) (string, bool) {
	if strings.HasPrefix(p, "/") || strings.HasPrefix(p, `\`) {
		return "", false
	}
	p = filepath.Clean(filepath.FromSlash(p))
	return p, filepath.IsLocal(p)
}

// loadPath resolves a (possibly relative, possibly percent-encoded) path.
func (l *Loader) loadPath(p, baseDir string) (*Image, error) {
	img, err := l.loadFrom(osFS{}, joinBase(p, baseDir))
	if errors.Is(err, fs.ErrNotExist) && strings.Contains(p, "%") {
		if u, uerr := url.PathUnescape(p); uerr == nil && u != p {
			if NetworkPath(u) { // %5C%5Chost: checked like the source
				return nil, ErrRemote
			}
			return l.loadFrom(osFS{}, joinBase(u, baseDir))
		}
	}
	return img, err
}

func joinBase(p, baseDir string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(baseDir, p)
}

// loadData decodes a data: URI, counting its bytes in the Loader's budget.
func (l *Loader) loadData(src string) (*Image, error) {
	data, err := parseDataURI(src)
	if err != nil {
		return nil, err
	}
	if len(data) > maxLoaderBytes-l.read {
		return nil, errBudget
	}
	l.read += len(data)
	return validate(data)
}

// fileSystem is where image files are read from: the whole file system
// (osFS) or an *os.Root confining every access to one directory.
type fileSystem interface {
	Stat(name string) (fs.FileInfo, error)
	OpenFile(name string, flag int, perm fs.FileMode) (*os.File, error)
}

type osFS struct{}

func (osFS) Stat(name string) (fs.FileInfo, error) { return os.Stat(name) }

func (osFS) OpenFile(name string, flag int, perm fs.FileMode) (*os.File, error) {
	// #nosec G304 -- see loadFrom: the path is checked, opened non-blocking,
	// re-checked on the descriptor, size-limited and sniffed.
	return os.OpenFile(name, flag, perm)
}

// loadFrom reads and validates an image file. Only regular files are opened:
// a FIFO or device could block forever or produce unbounded data. A file
// the Loader has read before, under any name, is not read again.
func (l *Loader) loadFrom(fsys fileSystem, path string) (*Image, error) {
	fi, err := fsys.Stat(path)
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: %s", ErrNotRegular, path)
	}
	if fi.Size() > maxFileBytes {
		return nil, fmt.Errorf("%w: %s is %d bytes", ErrTooLarge, path, fi.Size())
	}
	// The path comes from the (untrusted) document: it was checked to be a
	// regular file above, is opened non-blocking so a FIFO swapped in
	// meanwhile cannot hang us, is re-checked on the descriptor,
	// size-limited, and sniffed as PNG/JPEG/GIF before use.
	f, err := fsys.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
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
	for _, seen := range l.files {
		if os.SameFile(seen.fi, fi) {
			return seen.img, seen.err
		}
	}
	data, err := l.readFile(f, fi.Size(), path)
	if errors.Is(err, errBudget) {
		return nil, err // nothing was read: not remembered
	}
	var img *Image
	if err == nil {
		img, err = validate(data)
	}
	l.files = append(l.files, loadedFile{fi: fi, img: img, err: err})
	return img, err
}

// readFile reads an open image file of the given size within the file
// limit and what is left of the Loader's budget. Bytes read for a file
// that then fails count too: the budget bounds reading, not only memory.
func (l *Loader) readFile(f io.Reader, size int64, path string) ([]byte, error) {
	limit := min(maxFileBytes, maxLoaderBytes-l.read)
	if size > int64(limit) {
		return nil, errBudget
	}
	data, err := io.ReadAll(io.LimitReader(f, int64(limit)+1))
	l.read += len(data)
	if err != nil {
		return nil, err
	}
	if len(data) > limit { // the file grew after it was checked
		return nil, fmt.Errorf("%w: %s", ErrTooLarge, path)
	}
	return data, nil
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
	if format == "png" {
		if err := checkPNG(data); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrUnsupported, err)
		}
	}
	return &Image{data: data, format: format, w: cfg.Width, h: cfg.Height}, nil
}

// checkPNG walks the chunks of a PNG without inflating them: IHDR first,
// every length within the file, every type four letters, every CRC right,
// and IEND last with nothing after it. kitty and iTerm2 receive PNG files
// as they are, so a terminal's decoder only ever sees a well-formed one;
// the walk costs a CRC over the file, far less than decoding it.
func checkPNG(data []byte) error {
	rest, ok := bytes.CutPrefix(data, []byte("\x89PNG\r\n\x1a\n"))
	if !ok {
		return errors.New("png: bad signature")
	}
	for first := true; len(rest) >= 12; first = false {
		n := binary.BigEndian.Uint32(rest)
		if int64(n) > int64(len(rest)-12) {
			return errors.New("png: truncated chunk")
		}
		typ, end := string(rest[4:8]), 8+int(n)
		if !chunkType(typ) || first != (typ == "IHDR") {
			return fmt.Errorf("png: unexpected chunk %q", typ)
		}
		if crc32.ChecksumIEEE(rest[4:end]) != binary.BigEndian.Uint32(rest[end:]) {
			return fmt.Errorf("png: bad checksum in %s", typ)
		}
		rest = rest[end+4:]
		if typ == "IEND" {
			if len(rest) > 0 {
				return errors.New("png: data after IEND")
			}
			return nil
		}
	}
	return errors.New("png: no IEND chunk")
}

// chunkType reports whether t is a PNG chunk type: four ASCII letters.
func chunkType(t string) bool {
	for i := 0; i < len(t); i++ {
		if c := t[i] | 0x20; c < 'a' || c > 'z' {
			return false
		}
	}
	return true
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
