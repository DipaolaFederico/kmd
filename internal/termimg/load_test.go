package termimg

import (
	"bytes"
	"encoding/base64"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DipaolaFederico/kmd/internal/testlimit"
)

func TestLoadFiles(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	pngPath := writeFile(t, dir, "a.png", encodePNG(t, gradient(30, 20)))
	writeFile(t, dir, "b.jpg", encodeJPEG(t, gradient(31, 21)))
	writeFile(t, dir, "c.gif", encodeGIF(t, gradient(32, 22)))
	writeFile(t, dir, "my image.png", encodePNG(t, gradient(5, 5)))

	cases := []struct {
		src, base, format string
		w, h              int
	}{
		{"a.png", dir, "png", 30, 20},
		{"./b.jpg", dir, "jpeg", 31, 21},
		{"../c.gif", sub, "gif", 32, 22},
		{pngPath, "/nonexistent", "png", 30, 20},
		{"file://" + pngPath, "", "png", 30, 20},
		{"file://localhost" + pngPath, "", "png", 30, 20},
		{"my%20image.png", dir, "png", 5, 5},
		{"  a.png\n", dir, "png", 30, 20},
	}
	for _, c := range cases {
		img, err := new(Loader).Load(c.src, c.base)
		if err != nil {
			t.Errorf("Load(%q): %v", c.src, err)
			continue
		}
		if img.Format() != c.format || img.w != c.w || img.h != c.h {
			t.Errorf("Load(%q) = %s %d×%d", c.src, img.Format(), img.w, img.h)
		}
	}
}

func TestLoadRejects(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "text.png", []byte("not an image at all"))
	writeFile(t, dir, "huge.png", pngHeader(100000, 100000))
	writeFile(t, dir, "wide.png", pngHeader(9000, 10))
	writeFile(t, dir, "mega.png", pngHeader(7000, 7000))
	writeFile(t, dir, "zero.png", pngHeader(0, 10))
	writeFile(t, dir, "bmp.png", []byte("BM\x00\x00\x00\x00"))
	big := filepath.Join(dir, "big.png")
	f, err := os.Create(big)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(encodePNG(t, gradient(2, 2))); err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(maxFileBytes + 1); err != nil { // sparse file
		t.Fatal(err)
	}
	_ = f.Close()

	cases := []struct {
		src  string
		want error
	}{
		{"http://example.com/x.png", ErrRemote},
		{"HTTPS://example.com/x.png", ErrRemote},
		{"ftp://example.com/x.png", ErrRemote},
		{"//example.com/x.png", ErrRemote},
		{"file://example.com/x.png", ErrRemote},
		{"file://", ErrRemote},
		{"file://%zz", ErrRemote},
		{"mailto:someone", ErrRemote},
		{"missing.png", fs.ErrNotExist},
		{"miss%20ing.png", fs.ErrNotExist},
		{".", ErrNotRegular},
		{"text.png", ErrUnsupported},
		{"bmp.png", ErrUnsupported},
		{"huge.png", ErrTooLarge},
		{"wide.png", ErrTooLarge},
		{"mega.png", ErrTooLarge},
		{"zero.png", ErrUnsupported},
		{"big.png", ErrTooLarge},
		{"/dev/null", ErrNotRegular},
	}
	for _, c := range cases {
		_, err := new(Loader).Load(c.src, dir)
		if !errors.Is(err, c.want) {
			t.Errorf("Load(%q) = %v; want %v", c.src, err, c.want)
		}
	}
	if _, err := new(Loader).Load("", dir); err == nil {
		t.Error("empty source accepted")
	}
}

func TestDataURI(t *testing.T) {
	data := encodePNG(t, gradient(7, 3))
	b64 := base64.StdEncoding.EncodeToString(data)
	ok := []string{
		dataURI("image/png", data),
		"DATA:image/png;base64," + b64,
		"data:;base64," + b64,
		"data:image/jpeg;name=x;base64," + b64, // header sniffing decides
		"data:image/png;base64," + strings.TrimRight(b64, "="),
		"data:image/png;base64," + b64[:10] + "\n  " + b64[10:],
		"data:image/png;base64," + strings.ReplaceAll(b64, "+", "%2B"),
	}
	for _, s := range ok {
		img, err := new(Loader).Load(s, "")
		if err != nil || img.w != 7 || img.h != 3 {
			t.Errorf("Load(%.40q): %v", s, err)
		}
	}
	bad := []struct {
		src  string
		want error
	}{
		{"data:image/png;base64", ErrDataURI},
		{"data:image/png," + b64, ErrDataURI},
		{"data:text/plain;base64," + b64, ErrDataURI},
		{"data:image/png;base64,!!!!", ErrDataURI},
		{"data:image/png;base64,%zz", ErrDataURI},
		{"data:image/png;base64,QUJD", ErrUnsupported},
		{"data:image/png;base64," + base64.StdEncoding.EncodeToString(pngHeader(1e5, 1e5)), ErrTooLarge},
		{"data:image/png;base64," + strings.Repeat("A", base64.StdEncoding.EncodedLen(maxDataURIBytes)+8192), ErrTooLarge},
		{"data:image/png;base64," + strings.Repeat("AAAA", maxDataURIBytes/3+1), ErrTooLarge},
	}
	for _, c := range bad {
		if _, err := new(Loader).Load(c.src, ""); !errors.Is(err, c.want) {
			t.Errorf("Load(%.40q) = %v; want %v", c.src, err, c.want)
		}
	}
}

func TestURLScheme(t *testing.T) {
	cases := map[string]string{
		"http://x": "http", "a+b.c-d:x": "a+b.c-d", "c:/x": "", "img.png": "",
		":x": "", "1http:x": "", "dir/a:b": "", "data:": "data",
	}
	for in, want := range cases {
		if got := urlScheme(in); got != want {
			t.Errorf("urlScheme(%q) = %q; want %q", in, got, want)
		}
	}
}

func TestLoadInConfined(t *testing.T) {
	outer := t.TempDir()
	dir := filepath.Join(outer, "doc")
	if err := os.MkdirAll(filepath.Join(dir, "img"), 0o700); err != nil {
		t.Fatal(err)
	}
	secret := writeFile(t, outer, "secret.png", encodePNG(t, gradient(4, 4)))
	writeFile(t, dir, "img/a.png", encodePNG(t, gradient(6, 3)))
	writeFile(t, dir, "my image.png", encodePNG(t, gradient(5, 5)))
	symlinks := os.Symlink(secret, filepath.Join(dir, "link.png")) == nil &&
		os.Symlink(outer, filepath.Join(dir, "up")) == nil // not on every Windows

	for _, src := range []string{"img/a.png", "./img/../img/a.png", "my%20image.png", " img/a.png ",
		dataURI("image/png", encodePNG(t, gradient(2, 2)))} {
		if _, err := new(Loader).LoadIn(src, dir); err != nil {
			t.Errorf("LoadIn(%q): %v", src, err)
		}
	}
	if _, err := new(Loader).LoadIn(dataURI("image/png", encodePNG(t, gradient(2, 2))), ""); err != nil {
		t.Errorf("data: URI without a directory: %v", err)
	}

	cases := []struct {
		src, dir string
		want     error // nil: any error
	}{
		{secret, dir, ErrOutside},
		{"file://" + secret, dir, ErrOutside},
		{"/etc/passwd", dir, ErrOutside},
		{`\\host\share\x.png`, dir, ErrRemote},
		{"img/a.png", "", ErrOutside},
		{"../secret.png", dir, nil},
		{"img/../../secret.png", dir, nil},
		{"..%2Fsecret.png", dir, nil},
		{"https://example.com/x.png", dir, ErrRemote},
		{"//example.com/x.png", dir, ErrRemote},
		{"", dir, nil},
		{"missing.png", dir, fs.ErrNotExist},
		{"img/a.png", filepath.Join(outer, "nope"), fs.ErrNotExist},
		{"../secret.png", dir, ErrOutside},
		{"%2E%2E/secret.png", dir, ErrOutside},
		{".", dir, nil},
		{strings.Repeat("d/../", 1000) + "img/a.png", dir, ErrOutside}, // too long: refused before any syscall
	}
	if symlinks {
		cases = append(cases, struct {
			src, dir string
			want     error
		}{"link.png", dir, nil}, struct {
			src, dir string
			want     error
		}{"up/secret.png", dir, nil})
	}
	for _, c := range cases {
		img, err := new(Loader).LoadIn(c.src, c.dir)
		if err == nil {
			t.Errorf("LoadIn(%q, %q) = %v, want an error", c.src, c.dir, img)
			continue
		}
		if c.want != nil && !errors.Is(err, c.want) {
			t.Errorf("LoadIn(%q, %q) = %v, want %v", c.src, c.dir, err, c.want)
		}
	}
}

// TestNetworkPath: network and device paths are recognised with either
// separator on every platform (on Windows, os.Stat of one opens an SMB
// connection).
func TestNetworkPath(t *testing.T) {
	for _, p := range []string{`//host/share/a.png`, `\\host\share\a.png`, `/\host/a.png`, `\/host/a.png`,
		`\\?\UNC\host\share\a.png`, `\\.\pipe\x`, `\\?\C:\a.png`, `\??\UNC\host\a.png`, `/??/x`} {
		if !NetworkPath(p) {
			t.Errorf("NetworkPath(%q) = false", p)
		}
	}
	for _, p := range []string{"a.png", "/tmp/a.png", `C:\a.png`, "C:/a.png", "./a.png", `\a.png`, "?/x", "/?", ""} {
		if NetworkPath(p) {
			t.Errorf("NetworkPath(%q) = true", p)
		}
	}
}

// TestLoadNetworkPaths: every spelling of a network path is refused before
// any file system call, percent-encoded or inside a file: URL included.
func TestLoadNetworkPaths(t *testing.T) {
	dir := t.TempDir()
	for _, src := range []string{`file:////host/share/a.png`, `file://localhost//host/share/a.png`,
		`file:///%5C%5Chost/share/a.png`, `/\host/share/a.png`, `\\host\share\a.png`,
		`%5C%5Chost%5Cshare%5Ca.png`, `%2F%2Fhost/share/a.png`, `\\.\pipe\x`, `\\?\UNC\host\share\a.png`} {
		if _, err := new(Loader).Load(src, dir); !errors.Is(err, ErrRemote) {
			t.Errorf("Load(%q) = %v; want ErrRemote", src, err)
		}
		if _, err := new(Loader).LoadIn(src, dir); err == nil {
			t.Errorf("LoadIn(%q) succeeded", src)
		}
	}
}

// TestCheckPNG: a PNG must be well formed chunk by chunk, as kitty and
// iTerm2 get its bytes as they are; nothing may follow IEND.
func TestCheckPNG(t *testing.T) {
	good := encodePNG(t, gradient(4, 4))
	if _, err := validate(good); err != nil {
		t.Fatal(err)
	}
	badCRC := bytes.Clone(good)
	badCRC[len(badCRC)-20] ^= 1 // inside the IDAT data
	head := pngHeader(4, 4)
	bad := map[string][]byte{
		"no IEND":         head,
		"data after IEND": append(bytes.Clone(good), make([]byte, 1000)...),
		"bad checksum":    badCRC,
		"truncated":       good[:len(good)-5],
		"long chunk":      append(bytes.Clone(head), 0x7f, 0xff, 0xff, 0xff, 'I', 'D', 'A', 'T', 0, 0, 0, 0),
		"bad type":        pngChunk(bytes.Clone(head), "ID4T", nil),
		"second IHDR":     pngChunk(pngChunk(bytes.Clone(head), "IHDR", nil), "IEND", nil),
	}
	for name, data := range bad {
		if _, err := validate(data); !errors.Is(err, ErrUnsupported) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if err := checkPNG([]byte("GIF89a")); err == nil {
		t.Error("signature not checked")
	}
	if err := checkPNG(pngChunk([]byte("\x89PNG\r\n\x1a\n"), "IEND", nil)); err == nil {
		t.Error("PNG without IHDR accepted")
	}
}

// TestLoaderReadsFileOnce: every name of one file (./a.png, x/../a.png, an
// absolute path, a file: URL, links) gets the same image, read once.
func TestLoaderReadsFileOnce(t *testing.T) {
	dir := t.TempDir()
	data := encodePNG(t, gradient(8, 8))
	p := writeFile(t, dir, "a.png", data)
	relative := []string{"a.png", "./a.png", "././a.png", "x/../a.png"}
	if os.Symlink("a.png", filepath.Join(dir, "sym.png")) == nil {
		relative = append(relative, "sym.png")
	}
	if os.Link(p, filepath.Join(dir, "hard.png")) == nil {
		relative = append(relative, "hard.png")
	}
	var anywhere, confined Loader
	check := func(l *Loader, load func(string, string) (*Image, error), names []string) {
		t.Helper()
		var first *Image
		for _, name := range names {
			img, err := load(name, dir)
			if first == nil {
				first = img
			}
			if err != nil || img != first {
				t.Errorf("%q: %v, loaded again: %v", name, err, img != first)
			}
		}
		if l.read != len(data) {
			t.Errorf("read %d bytes, want %d", l.read, len(data))
		}
	}
	check(&anywhere, anywhere.Load, append(relative, p, "file://"+p))
	check(&confined, confined.LoadIn, relative)
}

// TestLoaderBudget: what a Loader reads is bounded per document, failed
// files included; a file already read is still found past the budget.
func TestLoaderBudget(t *testing.T) {
	dir := t.TempDir()
	data := encodePNG(t, gradient(8, 8))
	writeFile(t, dir, "a.png", data)
	writeFile(t, dir, "b.png", data)
	writeFile(t, dir, "text.png", []byte("not an image"))
	l := Loader{read: maxLoaderBytes - len(data) - 12}
	if _, err := l.Load("text.png", dir); !errors.Is(err, ErrUnsupported) || l.read != maxLoaderBytes-len(data) {
		t.Fatalf("failed file: %v, read %d", err, l.read)
	}
	a, err := l.Load("a.png", dir)
	if err != nil || l.read != maxLoaderBytes {
		t.Fatalf("a.png within the budget: %v, read %d", err, l.read)
	}
	if _, err := l.Load("b.png", dir); !errors.Is(err, ErrTooLarge) {
		t.Errorf("b.png past the budget: %v", err)
	}
	if _, err := l.Load(dataURI("image/png", data), dir); !errors.Is(err, ErrTooLarge) {
		t.Errorf("data: URI past the budget: %v", err)
	}
	if img, err := l.Load("./a.png", dir); err != nil || img != a {
		t.Errorf("a.png again: %v", err)
	}
	if l.read != maxLoaderBytes {
		t.Errorf("read %d bytes past the budget", l.read-maxLoaderBytes)
	}
}

// TestLoaderLookups: a document naming thousands of image files, empty or
// not images at all, costs the Loader linear time: it stops looking files
// up after maxLoaderFiles (before, each new file was compared with every
// one read before it).
func TestLoaderLookups(t *testing.T) {
	dir := t.TempDir()
	const files = 64 * maxLoaderFiles
	for i := range files {
		writeFile(t, dir, strconv.Itoa(i), nil)
	}
	var l Loader
	start := time.Now()
	for i := range files {
		want := ErrUnsupported // empty
		if i >= maxLoaderFiles {
			want = ErrTooLarge
		}
		if _, err := l.Load(strconv.Itoa(i), dir); !errors.Is(err, want) {
			t.Fatalf("file %d: %v, want %v", i, err, want)
		}
	}
	if d := time.Since(start); d > testlimit.Scale()*2*time.Second {
		t.Errorf("%d lookups took %v", files, d)
	}
	if _, err := l.Load("data:image/png;base64,"+base64.StdEncoding.EncodeToString(encodePNG(t, gradient(2, 2))), dir); err != nil {
		t.Errorf("data: URIs read no file and still load: %v", err)
	}
	testlimit.Linear(t, "image lookups", files, func(n int) func() {
		names := make([]string, n)
		for i := range names {
			names[i] = strconv.Itoa(i % files)
		}
		return func() {
			var l Loader
			for _, name := range names {
				_, _ = l.Load(name, dir)
			}
		}
	})
}
