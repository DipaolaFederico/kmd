package termimg

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func TestParseProtocol(t *testing.T) {
	cases := map[string]Protocol{
		"none": None, "blocks": Blocks, "kitty": Kitty, "iterm2": ITerm2,
		"iterm": ITerm2, "sixel": Sixel, " Kitty ": Kitty, "SIXEL": Sixel,
	}
	for in, want := range cases {
		got, err := ParseProtocol(in)
		if err != nil || got != want {
			t.Errorf("ParseProtocol(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "auto", "png", "kittyx"} {
		if _, err := ParseProtocol(bad); err == nil {
			t.Errorf("ParseProtocol(%q) succeeded", bad)
		}
	}
}

func TestProtocolString(t *testing.T) {
	for _, p := range []Protocol{None, Blocks, Kitty, ITerm2, Sixel} {
		back, err := ParseProtocol(p.String())
		if err != nil || back != p {
			t.Errorf("round trip %v: %v %v", p, back, err)
		}
	}
	if s := Protocol(42).String(); s != "Protocol(42)" {
		t.Errorf("unknown protocol string %q", s)
	}
}

func TestDetect(t *testing.T) {
	cases := []struct {
		env       map[string]string
		truecolor bool
		want      Protocol
	}{
		{map[string]string{"TERM": "xterm-kitty"}, true, Kitty},
		{map[string]string{"TERM": "xterm-256color", "KITTY_WINDOW_ID": "1"}, false, Kitty},
		{map[string]string{"TERM_PROGRAM": "ghostty"}, true, Kitty},
		{map[string]string{"TERM": "xterm-ghostty"}, true, Kitty},
		{map[string]string{"TERM_PROGRAM": "iTerm.app"}, true, ITerm2},
		{map[string]string{"LC_TERMINAL": "iTerm2"}, true, ITerm2},
		{map[string]string{"TERM_PROGRAM": "WezTerm"}, true, ITerm2},
		{map[string]string{"WEZTERM_PANE": "0"}, true, ITerm2},
		{map[string]string{"TERM_PROGRAM": "vscode"}, true, ITerm2},
		{map[string]string{"TERM_PROGRAM": "mintty"}, true, ITerm2},
		{map[string]string{"TERM": "mintty"}, true, ITerm2},
		{map[string]string{"KONSOLE_VERSION": "230804"}, true, Kitty},
		{map[string]string{"TERM": "foot"}, true, Sixel},
		{map[string]string{"TERM": "foot-extra"}, true, Sixel},
		{map[string]string{"TERM": "mlterm"}, false, Sixel},
		{map[string]string{"WT_SESSION": "x"}, true, Sixel},
		{map[string]string{"TERM": "xterm-kitty", "TMUX": "/tmp/tmux"}, true, Blocks},
		{map[string]string{"TERM": "screen-256color", "TERM_PROGRAM": "iTerm.app"}, false, Blocks},
		{map[string]string{"TERM": "tmux-256color"}, true, Blocks},
		{map[string]string{"STY": "123.pts"}, false, Blocks},
		{map[string]string{"TERM": "dumb"}, true, None},
		{map[string]string{"TERM": "xterm-256color"}, true, Blocks},
		{map[string]string{"TERM": "xterm-256color"}, false, None},
		{map[string]string{}, false, None},
	}
	for _, c := range cases {
		got := Detect(func(k string) string { return c.env[k] }, c.truecolor)
		if got != c.want {
			t.Errorf("Detect(%v, %v) = %v; want %v", c.env, c.truecolor, got, c.want)
		}
	}
}

func TestFit(t *testing.T) {
	cases := []struct {
		name               string
		w, h, mc, mr       int
		cell               Cell
		cols, rows, pw, ph int
	}{
		{"natural", 80, 160, 100, 100, Cell{8, 16}, 10, 10, 80, 160},
		{"natural ceil", 81, 161, 100, 100, Cell{8, 16}, 11, 11, 81, 161},
		{"width bound", 1024, 768, 80, 100, Cell{8, 16}, 80, 30, 640, 480},
		{"height bound", 1024, 768, 200, 10, Cell{8, 16}, 27, 10, 213, 160},
		{"unknown cell", 1024, 768, 80, 0, Cell{}, 80, 30, 640, 480},
		{"half unknown cell", 16, 16, 80, 24, Cell{W: 10}, 2, 1, 16, 16},
		{"real cell", 1000, 1000, 50, 50, Cell{10, 20}, 50, 25, 500, 500},
		{"unlimited", 100, 100, 0, 0, Cell{10, 20}, 10, 5, 100, 100},
		{"tiny", 1, 1, 80, 24, Cell{8, 16}, 1, 1, 1, 1},
		{"extreme aspect", 8000, 1, 10, 10, Cell{8, 16}, 10, 1, 80, 1},
	}
	for _, c := range cases {
		l := fit(c.w, c.h, c.mc, c.mr, c.cell)
		if l.cols != c.cols || l.rows != c.rows || l.pxW != c.pw || l.pxH != c.ph {
			t.Errorf("%s: fit = %d×%d cells %d×%d px; want %d×%d cells %d×%d px",
				c.name, l.cols, l.rows, l.pxW, l.pxH, c.cols, c.rows, c.pw, c.ph)
		}
	}
}

func TestBlocksGeometry(t *testing.T) {
	// 1024×768 at 80 columns, 8×16 cells: 80 grid px wide, 480/8 = 60 tall.
	if w, h := blocksGeometry(1024, 768, 80, 0, Cell{}); w != 80 || h != 60 {
		t.Fatalf("got %d×%d", w, h)
	}
	if w, h := blocksGeometry(1024, 768, 80, 10, Cell{}); w != 27 || h != 20 {
		t.Fatalf("row-limited got %d×%d", w, h)
	}
}

var (
	red  = color.NRGBA{255, 0, 0, 255}
	blue = color.NRGBA{0, 0, 255, 255}
)

func TestEncodeBlocksExact(t *testing.T) {
	m := image.NewNRGBA(image.Rect(0, 0, 4, 3))
	// Row 0/1 pair: [red/blue] [red/transparent] [transparent/blue] [blue/blue]
	m.SetNRGBA(0, 0, red)
	m.SetNRGBA(0, 1, blue)
	m.SetNRGBA(1, 0, red)
	m.SetNRGBA(2, 1, blue)
	m.SetNRGBA(3, 0, blue)
	m.SetNRGBA(3, 1, blue)
	// Row 2 (odd height): [red] [transparent] [half alpha red → drawn] [alpha 100 → transparent]
	m.SetNRGBA(0, 2, red)
	m.SetNRGBA(2, 2, color.NRGBA{255, 0, 0, 200})
	m.SetNRGBA(3, 2, color.NRGBA{255, 0, 0, 100})
	f := encodeBlocks(m)
	want := []string{
		"\x1b[38;2;255;0;0m\x1b[48;2;0;0;255m▀" +
			"\x1b[49m▀" +
			"\x1b[38;2;0;0;255m▄" +
			"\x1b[48;2;0;0;255m▀\x1b[0m",
		"\x1b[38;2;255;0;0m▀ ▀ \x1b[0m",
	}
	if f.Cols != 4 || len(f.Lines) != 2 {
		t.Fatalf("frame %d cols %d lines", f.Cols, len(f.Lines))
	}
	for i := range want {
		if f.Lines[i] != want[i] {
			t.Errorf("line %d:\n got %q\nwant %q", i, f.Lines[i], want[i])
		}
	}
}

func TestEncodeBlocksCoalesceSolid(t *testing.T) {
	m := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	for i := 0; i < len(m.Pix); i += 4 {
		copy(m.Pix[i:], []uint8{1, 2, 3, 255})
	}
	f := encodeBlocks(m)
	want := "\x1b[38;2;1;2;3m\x1b[48;2;1;2;3m▀  \x1b[0m"
	if f.Lines[0] != want {
		t.Fatalf("got %q want %q", f.Lines[0], want)
	}
}

func TestRenderBlocks(t *testing.T) {
	img := mustLoadBytes(t, encodePNG(t, gradient(1024, 768)))
	f, err := Render(img, Blocks, 80, 0, Cell{})
	if err != nil {
		t.Fatal(err)
	}
	if f.Cols != 80 || len(f.Lines) != 30 {
		t.Fatalf("got %d cols %d lines", f.Cols, len(f.Lines))
	}
	for i, l := range f.Lines {
		if !strings.HasSuffix(l, "\x1b[0m") {
			t.Fatalf("line %d lacks reset", i)
		}
		vis := stripANSI(l)
		if n := len([]rune(vis)); n != 80 {
			t.Fatalf("line %d has %d visible cells", i, n)
		}
	}
}

func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b {
			for i < len(s) && s[i] != 'm' {
				i++
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// parseKitty splits a kitty frame line into control strings and payloads.
func parseKitty(t *testing.T, s string) (ctrls []string, chunks []string) {
	t.Helper()
	for _, part := range strings.Split(s, "\x1b\\") {
		if part == "" {
			continue
		}
		if !strings.HasPrefix(part, "\x1b_G") {
			t.Fatalf("bad kitty escape %q", part[:min(len(part), 20)])
		}
		ctrl, payload, ok := strings.Cut(part[3:], ";")
		if !ok {
			t.Fatal("kitty escape without payload")
		}
		ctrls = append(ctrls, ctrl)
		chunks = append(chunks, payload)
	}
	return ctrls, chunks
}

func checkKittyChunks(t *testing.T, ctrls, chunks []string, firstPrefix string) []byte {
	t.Helper()
	if !strings.HasPrefix(ctrls[0], firstPrefix) {
		t.Fatalf("first control %q, want prefix %q", ctrls[0], firstPrefix)
	}
	var all strings.Builder
	for i, c := range chunks {
		last := i == len(chunks)-1
		m := "m=1"
		if last {
			m = "m=0"
		}
		if i == 0 {
			if !strings.HasSuffix(ctrls[0], ","+m) {
				t.Fatalf("first control %q lacks %s", ctrls[0], m)
			}
		} else if ctrls[i] != m {
			t.Fatalf("chunk %d control %q want %q", i, ctrls[i], m)
		}
		if len(c) > kittyChunk || (!last && len(c) != kittyChunk) {
			t.Fatalf("chunk %d length %d", i, len(c))
		}
		all.WriteString(c)
	}
	data, err := base64.StdEncoding.DecodeString(all.String())
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestRenderKittyPNG(t *testing.T) {
	data := encodePNG(t, noise(64, 64, 1)) // ≈12 KiB → several chunks
	img := mustLoadBytes(t, data)
	f, err := Render(img, Kitty, 80, 24, Cell{W: 8, H: 16})
	if err != nil {
		t.Fatal(err)
	}
	if f.Cols != 8 || len(f.Lines) != 4 {
		t.Fatalf("got %d cols, %d lines", f.Cols, len(f.Lines))
	}
	for _, l := range f.Lines[1:] {
		if l != "" {
			t.Fatalf("placeholder line %q", l)
		}
	}
	ctrls, chunks := parseKitty(t, f.Lines[0])
	if len(chunks) < 3 {
		t.Fatalf("only %d chunks", len(chunks))
	}
	got := checkKittyChunks(t, ctrls, chunks, "a=T,f=100,t=d,C=1,c=8,r=4,q=2,")
	if string(got) != string(data) {
		t.Fatal("payload differs from original PNG")
	}
}

func TestRenderKittySingleChunk(t *testing.T) {
	img := mustLoadBytes(t, encodePNG(t, gradient(4, 4)))
	f, err := Render(img, Kitty, 80, 24, Cell{})
	if err != nil {
		t.Fatal(err)
	}
	ctrls, chunks := parseKitty(t, f.Lines[0])
	if len(chunks) != 1 || ctrls[0] != "a=T,f=100,t=d,C=1,c=1,r=1,q=2,m=0" {
		t.Fatalf("got %q", ctrls)
	}
}

func TestRenderKittyJPEG(t *testing.T) {
	img := mustLoadBytes(t, encodeJPEG(t, gradient(200, 100)))
	f, err := Render(img, Kitty, 10, 24, Cell{W: 10, H: 20})
	if err != nil {
		t.Fatal(err)
	}
	ctrls, chunks := parseKitty(t, f.Lines[0])
	got := checkKittyChunks(t, ctrls, chunks, "a=T,f=24,s=100,v=50,t=d,C=1,c=10,r=3,q=2,")
	if len(got) != 100*50*3 {
		t.Fatalf("payload %d bytes", len(got))
	}
	if len(f.Lines) != 3 || f.Cols != 10 {
		t.Fatalf("frame %d lines %d cols", len(f.Lines), f.Cols)
	}
}

func TestRenderKittyTransparentGIF(t *testing.T) {
	pal := image.NewPaletted(image.Rect(0, 0, 6, 4), color.Palette{color.Transparent, red})
	pal.SetColorIndex(1, 1, 1)
	img := mustLoadBytes(t, encodeGIF(t, pal))
	f, err := Render(img, Kitty, 80, 24, Cell{W: 1, H: 1})
	if err != nil {
		t.Fatal(err)
	}
	ctrls, chunks := parseKitty(t, f.Lines[0])
	got := checkKittyChunks(t, ctrls, chunks, "a=T,f=32,s=6,v=4,t=d,C=1,c=6,r=4,q=2,")
	if len(got) != 6*4*4 || got[(1*6+1)*4] != 255 || got[3] != 0 {
		t.Fatalf("unexpected RGBA payload % x", got[:8])
	}
}

// iTerm2Payload renders img with iTerm2 in a 20-column box of 10×20 cells
// and returns the file the sequence carries.
func iTerm2Payload(t *testing.T, img *Image) []byte {
	t.Helper()
	f, err := Render(img, ITerm2, 20, 24, Cell{W: 10, H: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Lines) != 1 || f.Cols != 20 {
		t.Fatalf("frame %d lines %d cols", len(f.Lines), f.Cols)
	}
	l := f.Lines[0]
	head, body, ok := strings.Cut(l, ";preserveAspectRatio=1:")
	if !ok || !strings.HasPrefix(head, "\x1b]1337;File=inline=1;size=") || !strings.HasSuffix(head, ";width=20;height=7") ||
		!strings.HasSuffix(body, "\a") {
		t.Fatalf("bad sequence %q", l[:min(80, len(l))])
	}
	got, err := base64.StdEncoding.DecodeString(body[:len(body)-1])
	if err != nil || !strings.Contains(head, "size="+itoa(len(got))+";") {
		t.Fatalf("payload: %v", err)
	}
	return got
}

// TestRenderITerm2: a PNG is passed through; a JPEG or GIF is never
// handed to the terminal's decoder: kmd decodes it and sends its first
// frame, scaled, as a PNG.
// TestRenderITerm2: PNG, JPEG and GIF files go to the terminal as they
// are, once Load has checked their structure; bytes after the end of a
// JPEG or GIF are left out.
func TestRenderITerm2(t *testing.T) {
	for _, data := range [][]byte{
		encodePNG(t, gradient(300, 200)),
		encodeJPEG(t, gradient(300, 200)),
		encodeGIF(t, gradient(300, 200)),
	} {
		if got := iTerm2Payload(t, mustLoadBytes(t, data)); !bytes.Equal(got, data) {
			t.Errorf("%.4q… not passed through", data)
		}
		if format := mustLoadBytes(t, data).Format(); format != "png" {
			trailer := append(bytes.Clone(data), "motion photo"...)
			if got := iTerm2Payload(t, mustLoadBytes(t, trailer)); !bytes.Equal(got, data) {
				t.Errorf("%s: bytes after the image sent", format)
			}
		}
	}
}

func itoa(n int) string { return strconv.Itoa(n) }

func TestEncodeSixelExact(t *testing.T) {
	// 5×1 pure red (cube level 5,0,0 = index 210) then 2 transparent pixels
	// and 1 blue (index 5); exact cube colors are unaffected by dithering.
	m := image.NewNRGBA(image.Rect(0, 0, 8, 1))
	for x := 0; x < 5; x++ {
		m.SetNRGBA(x, 0, red)
	}
	m.SetNRGBA(7, 0, blue)
	got := string(encodeSixel(m))
	want := "\x1bP0;1;0q\"1;1;8;1#5;2;0;0;100#210;2;100;0;0#210!5@$#5!7?@\x1b\\"
	if got != want {
		t.Fatalf("\n got %q\nwant %q", got, want)
	}
}

func TestEncodeSixelRoundTrip(t *testing.T) {
	m := gradient(37, 20) // 4 bands, the last one partial
	m.SetNRGBA(3, 3, color.NRGBA{})
	m.SetNRGBA(36, 19, color.NRGBA{9, 9, 9, 10})
	checkSixel(t, m)
	checkSixel(t, noise(13, 7, 3))
	checkSixel(t, image.NewNRGBA(image.Rect(0, 0, 5, 13))) // fully transparent
}

func TestRenderSixel(t *testing.T) {
	img := mustLoadBytes(t, encodePNG(t, gradient(1024, 768)))
	f, err := Render(img, Sixel, 80, 0, Cell{W: 10, H: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Lines) != 1 || f.Cols != 80 {
		t.Fatalf("frame %d lines %d cols", len(f.Lines), f.Cols)
	}
	d, err := decodeSixel(f.Lines[0])
	if err != nil {
		t.Fatal(err)
	}
	if d.w != 800 || d.h != 600 {
		t.Fatalf("sixel %d×%d", d.w, d.h)
	}
	for i, c := range d.pix {
		if c < 0 {
			t.Fatalf("opaque image left pixel %d unset", i)
		}
	}
}

func TestRenderMisc(t *testing.T) {
	if _, err := Render(nil, Blocks, 80, 24, Cell{}); !errors.Is(err, ErrNilImage) {
		t.Fatalf("nil image: %v", err)
	}
	img := mustLoadBytes(t, encodePNG(t, gradient(4, 4)))
	if f, err := Render(img, None, 80, 24, Cell{}); err != nil || len(f.Lines) != 0 {
		t.Fatalf("None: %v %v", f, err)
	}
	if _, err := Render(img, Protocol(99), 80, 24, Cell{}); err == nil {
		t.Fatal("unknown protocol accepted")
	}
	if img.Format() != "png" {
		t.Fatal(img.Format())
	}
	if w, h := img.Size(); w != 4 || h != 4 {
		t.Fatal(w, h)
	}
}

// TestRenderDecodeError: well-formed files whose content does not decode
// (a broken compressed stream, a bad Huffman table) fail where kmd decodes.
func TestRenderDecodeError(t *testing.T) {
	badDeflate := pngChunk(pngChunk(pngHeader(10, 10), "IDAT", []byte("not deflate")), "IEND", nil)
	badHuffman := encodeJPEG(t, noise(64, 64, 2))
	dht := bytes.Index(badHuffman, []byte{0xff, 0xc4})
	badHuffman[dht+5] = 0xfe // 254 one-bit codes
	for _, data := range [][]byte{badDeflate, badHuffman} {
		img := mustLoadBytes(t, data)
		for _, p := range []Protocol{Blocks, Sixel} {
			if _, err := Render(img, p, 80, 24, Cell{}); err == nil {
				t.Errorf("%v: truncated %s rendered", p, img.Format())
			}
		}
		_, err := Render(img, Kitty, 80, 24, Cell{})
		if (img.Format() == "png") != (err == nil) {
			t.Errorf("kitty %s: err=%v", img.Format(), err)
		}
	}
}

func TestRenderConcurrent(t *testing.T) {
	img := mustLoadBytes(t, encodeGIF(t, gradient(50, 40)))
	var wg sync.WaitGroup
	for _, p := range []Protocol{Blocks, Kitty, ITerm2, Sixel, Blocks, Sixel} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := Render(img, p, 20, 10, Cell{}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
}

func TestFrameRowsAndClear(t *testing.T) {
	img := mustLoadBytes(t, encodePNG(t, gradient(64, 64)))
	cell := Cell{W: 8, H: 16}
	for _, p := range []Protocol{Blocks, Kitty, ITerm2, Sixel} {
		f, err := Render(img, p, 10, 4, cell)
		if err != nil || f.Rows < 1 || f.Rows > 5 || (p == Blocks || p == Kitty) && f.Rows != len(f.Lines) {
			t.Errorf("%v: rows %d lines %d %v", p, f.Rows, len(f.Lines), err)
		}
	}
	if Clear(Kitty) == "" || Clear(Sixel) != "" || Clear(ITerm2) != "" {
		t.Error("clear sequences")
	}
}
