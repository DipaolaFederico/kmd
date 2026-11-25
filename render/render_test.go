package render_test

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DipaolaFederico/kmd/internal/testlimit"
	"github.com/DipaolaFederico/kmd/render"
)

func mustRender(t *testing.T, src string, o render.Options) string {
	t.Helper()
	out, err := render.Render(src, o)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	return out
}

func TestZeroOptionsArePlainText(t *testing.T) {
	src := "# Title\n\nSome *text* with a [link](https://example.com) and `code`.\n\n" +
		strings.Repeat("word ", 60) + "\n\n- [x] done\n\n$x^2$\n\n```mermaid\ngraph LR\nA-->B\n```\n"
	out := mustRender(t, src, render.Options{})
	if strings.ContainsRune(out, '\x1b') {
		t.Errorf("zero Options emitted escape sequences:\n%q", out)
	}
	for _, want := range []string{"TITLE", "text", "link (https://example.com)", "code", "x²", "☑"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "┌") { // the diagram is drawn
		t.Errorf("mermaid diagram not drawn:\n%s", out)
	}
	for _, l := range strings.Split(out, "\n") {
		if n := len([]rune(l)); n > render.DefaultWidth {
			t.Errorf("line of %d columns exceeds the default width: %q", n, l)
		}
	}
}

func TestOptions(t *testing.T) {
	src := "# Title\n\n[link](https://example.com)\n\n$x^2$\n\n```mermaid\ngraph LR\nA-->B\n```\n\n~~gone~~\n"
	color := mustRender(t, src, render.Options{Color: true})
	if !strings.Contains(color, "\x1b[") || strings.Contains(color, "\x1b]8;") {
		t.Errorf("Color: want SGR and no hyperlinks:\n%q", color)
	}
	if out := mustRender(t, src, render.Options{Color: true, TrueColor: true}); !strings.Contains(out, "38;2;") {
		t.Errorf("TrueColor: no 24-bit colours:\n%q", out)
	}
	if out := mustRender(t, src, render.Options{Hyperlinks: true}); !strings.Contains(out, "\x1b]8;;https://example.com") {
		t.Errorf("Hyperlinks: no OSC 8:\n%q", out)
	}
	// a service must not disclose its working directory (SR-25)
	if out := mustRender(t, "[guide](guide.md)", render.Options{Hyperlinks: true}); strings.Contains(out, "\x1b]8;;f") {
		t.Errorf("Hyperlinks without BaseDir: relative link resolved:\n%q", out)
	}
	if out := mustRender(t, "[guide](guide.md)", render.Options{Hyperlinks: true, BaseDir: "/docs"}); !strings.Contains(out, "\x1b]8;;file:///docs/guide.md\x1b") {
		t.Errorf("Hyperlinks with BaseDir: %q", out)
	}
	if out := mustRender(t, src, render.Options{NoMath: true}); !strings.Contains(out, "$x^2$") {
		t.Errorf("NoMath: math was rendered:\n%s", out)
	}
	if out := mustRender(t, src, render.Options{NoDiagrams: true}); !strings.Contains(out, "A-->B") {
		t.Errorf("NoDiagrams: diagram was drawn:\n%s", out)
	}
	if out := mustRender(t, src, render.Options{CommonMark: true}); !strings.Contains(out, "~~gone~~") || !strings.Contains(out, "$x^2$") {
		t.Errorf("CommonMark: extensions were applied:\n%s", out)
	}
	narrow := mustRender(t, strings.Repeat("word ", 50), render.Options{Width: 30})
	for _, l := range strings.Split(narrow, "\n") {
		if len(l) > 30 {
			t.Errorf("Width 30: line %q", l)
		}
	}
	light := mustRender(t, src, render.Options{Color: true, TrueColor: true, Theme: render.DefaultTheme(false)})
	if light == mustRender(t, src, render.Options{Color: true, TrueColor: true}) {
		t.Error("the light theme renders like the dark one")
	}
	if mustRender(t, src, render.Options{Color: true, Theme: &render.Theme{}}) != color {
		t.Error("the zero Theme is not the default theme")
	}
}

func TestInvalidOptions(t *testing.T) {
	for _, o := range []render.Options{
		{Width: -1}, {Width: render.MaxWidth + 1}, {Images: render.ImagesSixel + 1},
		{ImageRows: -1}, {CellWidth: -1}, {CellHeight: -2},
		{ImageRows: 1001}, {CellWidth: 513}, {CellHeight: 513},
	} {
		if _, err := render.Render("x", o); err == nil {
			t.Errorf("Render accepted %+v", o)
		}
	}
	if _, err := render.Render("x", render.Options{Width: render.MaxWidth, ImageRows: 1000, CellWidth: 512, CellHeight: 512}); err != nil {
		t.Errorf("MaxWidth refused: %v", err)
	}
}

func TestTooLarge(t *testing.T) {
	big := strings.Repeat("a", 16<<20+1)
	if _, err := render.Render(big, render.Options{}); !errors.Is(err, render.ErrTooLarge) {
		t.Errorf("Render: %v, want ErrTooLarge", err)
	}
	if _, err := render.HTML(big, render.HTMLOptions{}); !errors.Is(err, render.ErrTooLarge) {
		t.Errorf("HTML: %v, want ErrTooLarge", err)
	}
}

// TestEscapeInjection checks that sequences written in the document never
// reach the terminal, whatever the options.
func TestEscapeInjection(t *testing.T) {
	evil := "\x1b]52;c;cHduZWQ=\x07" // OSC 52: write the clipboard
	clear := "\x1b[2J\x1b[H"         // clear the screen
	title := "\x1b]0;owned\x1b\\"    // set the window title
	csi8 := "\u009b31m"              // 8-bit CSI
	src := "# Head " + evil + "\n\nText " + clear + " and " + title + csi8 + "\n\n" +
		"[link" + evil + "](https://example.com/" + clear + ")\n\n" +
		"```go\n" + evil + clear + "\n```\n\n| a" + title + " |\n|---|\n| " + clear + " |\n\n" +
		"![alt" + evil + "](x.png)\n\n$" + evil + "$\n\n```mermaid\ngraph LR\nA" + clear + "-->B\n```\n"
	for _, o := range []render.Options{{}, {Color: true, TrueColor: true, Hyperlinks: true, Images: render.ImagesBlocks}} {
		out := mustRender(t, src, o)
		for _, bad := range []string{"\x1b]52", "\x1b[2J", "\x1b[H", "\x1b]0;", "\u009b", "\x07"} {
			if strings.Contains(out, bad) {
				t.Errorf("options %+v: output contains %q", o, bad)
			}
		}
	}
}

// TestLimits checks that the renderer's resource limits apply through the
// public API: hostile documents of a few hundred kilobytes render quickly.
func TestLimits(t *testing.T) {
	docs := map[string]string{
		"nesting":  strings.Repeat("> ", 50000) + "x\n" + strings.Repeat("- ", 50000) + "x\n",
		"brackets": strings.Repeat("[", 100000) + strings.Repeat("](", 50000),
		"emphasis": strings.Repeat("*a _b ", 50000),
		"table":    "|" + strings.Repeat(" a |", 5000) + "\n|" + strings.Repeat("-|", 5000) + "\n" + strings.Repeat("|x\n", 5000),
		"diagrams": strings.Repeat("```mermaid\ngraph LR\n"+strings.Repeat("A-->B\nB-->C\nC-->A\n", 50)+"```\n\n", 500),
		"math":     strings.Repeat("$$\n"+strings.Repeat(`\frac{1}{`, 200)+"x"+strings.Repeat("}", 200)+"\n$$\n\n", 300),
	}
	for name, src := range docs {
		start := time.Now()
		out, err := render.Render(src, render.Options{Color: true})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if d := time.Since(start); d > testlimit.Scale()*5*time.Second {
			t.Errorf("%s: rendering %d bytes took %v", name, len(src), d)
		}
		if len(out) > 200*len(src)+1<<20 {
			t.Errorf("%s: %d bytes of input produced %d bytes of output", name, len(src), len(out))
		}
	}
	// n repetitions of each pattern: the time grows linearly with n
	for _, c := range []struct{ name, head, unit string }{
		{"nesting", "", "> - "},
		{"brackets", "", "[[]("},
		{"emphasis", "", "*a _b "},
		{"table", "|a|b|\n|-|-|\n", "|x|y\n"},
		{"diagrams", "", "```mermaid\ngraph LR\nA-->B\nB-->C\nC-->A\n```\n\n"},
		{"math", "", "$$\n\\frac{1}{x^2}\n$$\n\n"},
	} {
		testlimit.Linear(t, c.name, 1<<20/len(c.unit), func(n int) func() {
			src := c.head + strings.Repeat(c.unit, n)
			return func() { _, _ = render.Render(src, render.Options{Color: true}) }
		})
	}
}

func pngBytes(t *testing.T) []byte {
	t.Helper()
	m := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for i := range m.Pix {
		m.Pix[i] = 0xc0
	}
	m.Set(0, 0, color.NRGBA{R: 255, A: 255})
	var b bytes.Buffer
	if err := png.Encode(&b, m); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// TestImagesConfinedToBaseDir checks that a document can only make the
// renderer read images inside BaseDir.
func TestImagesConfinedToBaseDir(t *testing.T) {
	outer := t.TempDir()
	dir := filepath.Join(outer, "doc")
	if err := os.MkdirAll(filepath.Join(dir, "img"), 0o700); err != nil {
		t.Fatal(err)
	}
	data := pngBytes(t)
	for _, p := range []string{filepath.Join(dir, "img", "in.png"), filepath.Join(outer, "secret.png")} {
		if err := os.WriteFile(p, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	secret := filepath.Join(outer, "secret.png")
	drawn := func(src, base string, mode render.ImageMode) bool {
		out := mustRender(t, "![alt]("+src+")\n", render.Options{Color: true, Images: mode, BaseDir: base})
		return strings.Contains(out, "▀")
	}
	if !drawn("img/in.png", dir, render.ImagesBlocks) {
		t.Error("an image inside BaseDir was not drawn")
	}
	if !drawn("data:image/png;base64,"+base64.StdEncoding.EncodeToString(data), "", render.ImagesBlocks) {
		t.Error("a data: URI image was not drawn")
	}
	for _, c := range []struct{ src, base string }{
		{"img/in.png", ""},        // no BaseDir: no file access
		{"../secret.png", dir},    // leaves BaseDir
		{secret, dir},             // absolute path
		{"file://" + secret, dir}, // file: URL
	} {
		if drawn(c.src, c.base, render.ImagesBlocks) {
			t.Errorf("image %q (BaseDir %q) was read", c.src, c.base)
		}
	}
	if drawn("img/in.png", dir, render.ImagesNone) {
		t.Error("ImagesNone drew an image")
	}
	if out := mustRender(t, "![alt](img/in.png)\n", render.Options{Images: render.ImagesBlocks, BaseDir: dir}); strings.Contains(out, "▀") {
		t.Error("an image was drawn without Color")
	}
}

func TestHTML(t *testing.T) {
	src := "# A & \"B\"\x1b]0;x\a\n\n<script>alert(1)</script>\n\n[x](javascript:alert(1)) $y$\n"
	frag, err := render.HTML(src, render.HTMLOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(frag+page(t, src), "\x1b\a") {
		t.Errorf("control characters in the HTML:\n%q", frag)
	}
	if strings.Contains(frag, "<script") || strings.Contains(frag, "javascript:") || strings.Contains(frag, "<html") {
		t.Errorf("unsafe or non-fragment output:\n%s", frag)
	}
	if !strings.Contains(frag, `class="math`) {
		t.Errorf("math not rendered:\n%s", frag)
	}
	unsafe, _ := render.HTML(src, render.HTMLOptions{Unsafe: true})
	if !strings.Contains(unsafe, "javascript:alert") {
		t.Errorf("Unsafe dropped the URL:\n%s", unsafe)
	}
	if nm, _ := render.HTML(src, render.HTMLOptions{NoMath: true, CommonMark: true}); strings.Contains(nm, `class="math`) {
		t.Errorf("NoMath rendered math:\n%s", nm)
	}
	page, _ := render.HTML(src, render.HTMLOptions{Standalone: true})
	for _, want := range []string{"<!DOCTYPE html>", "Content-Security-Policy", "<title>A &amp; \"B\"]0;x</title>", `content="kmd"`} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %q:\n%s", want, page)
		}
	}
	titled, _ := render.HTML(src, render.HTMLOptions{Standalone: true, Title: "Mine", Theme: render.DefaultTheme(false)})
	if !strings.Contains(titled, "<title>Mine</title>") || titled == page {
		t.Errorf("Title/Theme ignored:\n%s", titled)
	}
	if untitled, _ := render.HTML("text", render.HTMLOptions{Standalone: true}); !strings.Contains(untitled, "<title></title>") {
		t.Errorf("untitled page:\n%s", untitled)
	}
}

func TestImageModes(t *testing.T) {
	for m := render.ImagesNone; m <= render.ImagesSixel; m++ {
		got, err := render.ParseImageMode(strings.ToUpper(m.String()))
		if err != nil || got != m {
			t.Errorf("ParseImageMode(%q) = %v, %v", m, got, err)
		}
	}
	if m, err := render.ParseImageMode("iterm"); err != nil || m != render.ImagesITerm2 {
		t.Errorf("iterm alias: %v, %v", m, err)
	}
	if _, err := render.ParseImageMode("auto"); err == nil {
		t.Error("ParseImageMode accepted auto")
	}
	if s := render.ImageMode(42).String(); s != "ImageMode(42)" {
		t.Errorf("String() = %q", s)
	}
}

func TestThemes(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("mine.theme", "inherit = dracula\nlink = #ff0000\n")
	write("child.theme", "inherit = mine\n")
	write("nord.theme", "inherit = dracula\n") // shadows a bundled name

	names := render.Themes(dir, filepath.Join(dir, "missing"))
	if names[0] != "tokyo-night" || !contains(names, "mine") || !contains(names, "child") || count(names, "nord") != 1 {
		t.Errorf("Themes = %v", names)
	}
	if len(render.Themes()) < 10 {
		t.Errorf("only %d bundled themes", len(render.Themes()))
	}

	for _, c := range []struct {
		name string
		dark bool
	}{{"tokyo-night-day", false}, {"dracula", true}, {"mine", true}, {"child", true}, {filepath.Join(dir, "mine.theme"), true}} {
		th, err := render.LoadTheme(c.name, dir)
		if err != nil {
			t.Errorf("LoadTheme(%q): %v", c.name, err)
			continue
		}
		if th.Dark() != c.dark || th.Name() == "" {
			t.Errorf("LoadTheme(%q) = %q dark=%v", c.name, th.Name(), th.Dark())
		}
	}
	if _, err := render.LoadTheme("mine"); !errors.Is(err, render.ErrUnknownTheme) {
		t.Errorf("LoadTheme without dirs: %v", err)
	}
	if _, err := render.LoadTheme("no-such-theme", dir); !errors.Is(err, render.ErrUnknownTheme) {
		t.Errorf("unknown theme: %v", err)
	}

	th, err := render.ParseTheme("custom", strings.NewReader("# comment\ninherit = nord\nlink = #00ff00\n"))
	if err != nil || th.Name() != "custom" || !th.Dark() {
		t.Fatalf("ParseTheme: %v %v", th, err)
	}
	if out := mustRender(t, "[x](y)", render.Options{Color: true, TrueColor: true, Theme: th}); !strings.Contains(out, "38;2;0;255;0") {
		t.Errorf("parsed theme colour not used:\n%q", out)
	}
	for _, src := range []string{"inherit = mine\n", "link = nope\n", "garbage\n", strings.Repeat("#", 64<<10+1)} {
		if _, err := render.ParseTheme("bad", strings.NewReader(src)); err == nil {
			t.Errorf("ParseTheme accepted %.40q", src)
		}
	}
	if _, err := render.ParseTheme("bad", errReader{}); err == nil {
		t.Error("ParseTheme ignored a read error")
	}

	var nilTheme *render.Theme
	if nilTheme.Name() != "tokyo-night" || !nilTheme.Dark() || (&render.Theme{}).Name() != "tokyo-night" {
		t.Error("nil and zero themes are not the default")
	}
	if render.DefaultTheme(false).Dark() || render.DefaultTheme(false).Name() != "tokyo-night-day" {
		t.Error("DefaultTheme(false) is not the light theme")
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("boom") }

func contains(s []string, v string) bool { return count(s, v) > 0 }

func count(s []string, v string) int {
	n := 0
	for _, x := range s {
		if x == v {
			n++
		}
	}
	return n
}

// TestConcurrent renders from many goroutines sharing one theme; run with
// -race (make test) it proves the package keeps no unsynchronised state.
func TestConcurrent(t *testing.T) {
	th, err := render.LoadTheme("dracula")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.png"), pngBytes(t), 0o600); err != nil {
		t.Fatal(err)
	}
	src := "# T\n\n```go\nfunc main() {}\n```\n\n$x^2$\n\n![a](a.png)\n\n```mermaid\npie\n\"a\": 1\n\"b\": 2\n```\n\n| a | b |\n|---|---|\n| 1 | 2 |\n"
	o := render.Options{Color: true, TrueColor: true, Theme: th, Images: render.ImagesBlocks, BaseDir: dir}
	want := mustRender(t, src, o)
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for i := range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 10 {
				out, err := render.Render(src, o)
				if err != nil || out != want {
					errs <- fmt.Errorf("goroutine %d: different output (%v)", i, err)
					return
				}
				if _, err := render.HTML(src, render.HTMLOptions{Standalone: true, Theme: th}); err != nil {
					errs <- err
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func page(t *testing.T, src string) string {
	t.Helper()
	out, err := render.HTML(src, render.HTMLOptions{Standalone: true})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
