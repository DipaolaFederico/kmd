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
