package termimg

import (
	"encoding/base64"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
		img, err := Load(c.src, c.base)
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
		_, err := Load(c.src, dir)
		if !errors.Is(err, c.want) {
			t.Errorf("Load(%q) = %v; want %v", c.src, err, c.want)
		}
	}
	if _, err := Load("", dir); err == nil {
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
		img, err := Load(s, "")
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
		if _, err := Load(c.src, ""); !errors.Is(err, c.want) {
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
		if _, err := LoadIn(src, dir); err != nil {
			t.Errorf("LoadIn(%q): %v", src, err)
		}
	}
	if _, err := LoadIn(dataURI("image/png", encodePNG(t, gradient(2, 2))), ""); err != nil {
		t.Errorf("data: URI without a directory: %v", err)
	}

	cases := []struct {
		src, dir string
		want     error // nil: any error
	}{
		{secret, dir, ErrOutside},
		{"file://" + secret, dir, ErrOutside},
		{"/etc/passwd", dir, ErrOutside},
		{`\\host\share\x.png`, dir, ErrOutside},
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
		img, err := LoadIn(c.src, c.dir)
		if err == nil {
			t.Errorf("LoadIn(%q, %q) = %v, want an error", c.src, c.dir, img)
			continue
		}
		if c.want != nil && !errors.Is(err, c.want) {
			t.Errorf("LoadIn(%q, %q) = %v, want %v", c.src, c.dir, err, c.want)
		}
	}
}
