package render

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/DipaolaFederico/kmd/internal/md"
	"github.com/DipaolaFederico/kmd/internal/termimg"
)

func TestFootnotesAndRefs(t *testing.T) {
	out := render("Text[^a] and more[^b].\n\n[^a]: First note\n[^b]: Second", plain)
	mustContain(t, out, "Text¹ and more².", "¹ First note", "² Second")
	if superscript(10) != "¹⁰" {
		t.Error("superscript")
	}
}

func TestMathRendering(t *testing.T) {
	o := Options{Width: 60, Math: true}
	out := render("Inline $x^2$ here.\n\n$$\n\\frac{a}{b}\n$$\n\n```math\n\\alpha\n```", o)
	mustContain(t, out, "x²", "α", "─")
	if strings.Contains(out, "$x^2$") || strings.Contains(out, "\\frac") {
		t.Errorf("math not rendered:\n%s", out)
	}
	// display math alone in a paragraph is centered on its own lines
	out = render("$$x$$", o)
	if !strings.Contains(out, strings.Repeat(" ", 10)+"x") {
		t.Errorf("display math not centered: %q", out)
	}
	// disabled: source is shown
	off := render("Inline $x^2$.\n\n$$y$$", Options{Width: 60})
	mustContain(t, off, "$x^2$", "math")
}

func TestDiagramRendering(t *testing.T) {
	src := "```mermaid\ngraph LR\n  A[Start] --> B[End]\n```"
	on := render(src, Options{Width: 80, Diagrams: true})
	mustContain(t, on, "Start", "End")
	if strings.Contains(on, "graph LR") {
		t.Errorf("diagram not rendered:\n%s", on)
	}
	off := render(src, Options{Width: 80})
	mustContain(t, off, "graph LR", "mermaid")
	// unsupported diagram types fall back to the source
	mustContain(t, render("```mermaid\njourney\n  title Day\n```", Options{Width: 80, Diagrams: true}), "journey")
	mustContain(t, render("```dot\ndigraph { a -> b }\n```", Options{Width: 80, Diagrams: true}), "a", "b")
}

func writePNG(t *testing.T, dir string) string {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.Set(x, y, color.NRGBA{R: uint8(x * 30), G: uint8(y * 30), B: 200, A: 255})
		}
	}
	p := filepath.Join(dir, "pic.png")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestImages(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, dir)
	doc := md.Parse("![A picture](pic.png)\n\n[![badge](pic.png)](https://x.y)\n\n![remote](https://x.y/a.png)\n\nText ![inline](pic.png) here", md.GFM)
	res := Render(doc, Options{Width: 40, Color: true, TrueColor: true, Images: termimg.Blocks, BaseDir: dir})
	if !strings.Contains(res.Text, "▀") || !strings.Contains(res.Text, "A picture") {
		t.Errorf("block image missing:\n%q", res.Text)
	}
	if res.Graphics {
		t.Error("blocks must not count as graphics")
	}
	mustContain(t, res.Text, "▣\u00a0remote", "▣\u00a0inline")
	res = Render(doc, Options{Width: 40, Color: true, Images: termimg.Kitty, BaseDir: dir})
	if !res.Graphics || !strings.Contains(res.Text, "\x1b_G") {
		t.Error("kitty image not emitted")
	}
	// images disabled → alt text
	res = Render(doc, Options{Width: 40, Color: true, BaseDir: dir})
	mustContain(t, res.Text, "▣\u00a0A picture")
}

func TestHTMLInline(t *testing.T) {
	out := render("<p align=\"center\"><img src=\"logo.png\" alt=\"Logo &amp; co\" width=\"10\"></p>\n\n<!-- hidden -->\n\nPress <kbd>Ctrl</kbd>+<kbd>C</kbd><br>done", plain)
	mustContain(t, out, "▣\u00a0Logo & co", "Press Ctrl+C\n  done")
	if strings.Contains(out, "hidden") || strings.Contains(out, "<kbd>") {
		t.Errorf("html not stripped:\n%s", out)
	}
	if attr(`img alt="a"`, "src") != "" || attr(`img xalt="a"`, "alt") != "" || attr(`img alt='b'`, "alt") != "b" {
		t.Error("attr")
	}
}

func TestTOC(t *testing.T) {
	doc := md.Parse("# A\n## B\n### C\n## D\n# E", md.GFM)
	out := TOC(doc, Options{Width: 40})
	for _, want := range []string{"1  A", "1.1  B", "1.1.1  C", "1.2  D", "2  E"} {
		if !strings.Contains(out, want) {
			t.Errorf("TOC missing %q:\n%s", want, out)
		}
	}
	if !strings.Contains(TOC(doc, Options{Width: 40, Color: true}), "\x1b[") {
		t.Error("colored TOC")
	}
}

func TestSanitizeOutput(t *testing.T) {
	out := render("evil \x1b]0;pwned\x07 text &#27;[31m and\ttab", Options{Width: 80, Color: true})
	if strings.Contains(out, "\x07") || strings.Contains(out, "\x1b]0") || strings.Contains(out, "\x1b[31m") {
		t.Errorf("control sequences leaked: %q", out)
	}
	if clean("a\u0085b\tc") != "ab c" {
		t.Errorf("clean = %q", clean("a\u0085b\tc"))
	}
	var b strings.Builder
	p := &painter{color: true, links: true}
	p.line(&b, rline{{text: "a", st: Style{URL: "u", Attr: Bold}}, {text: "IMG", raw: true, w: 3}, {text: "b", st: Style{Attr: Bold}}}, 0)
	if got := b.String(); strings.Count(got, "\x1b[0;1m") != 2 || !strings.Contains(got, "IMG") {
		t.Errorf("raw run handling: %q", got)
	}
	if lineWidth(rline{{text: "xx"}, {text: "ESC", raw: true, w: 5}}) != 7 {
		t.Error("raw width")
	}
}

func TestThemes(t *testing.T) {
	dirs := []string{t.TempDir()}
	for _, n := range append(BuiltinThemes(), "dark", "light") {
		th, err := LoadTheme(n, dirs)
		if err != nil || th.H1Bg == 0 || th.Series[7] == 0 {
			t.Fatalf("theme %s: %v", n, err)
		}
	}
	if _, err := LoadTheme("nope", dirs); err == nil {
		t.Fatal("unknown theme accepted")
	}
	custom := "# mine\ninherit = nord\ndark = false\nblue = #112233 # accent\nheading2 = #abc\n"
	if err := os.WriteFile(filepath.Join(dirs[0], "mine.theme"), []byte(custom), 0o600); err != nil {
		t.Fatal(err)
	}
	child := "inherit = mine\nsyntax-keyword = #010203\n"
	if err := os.WriteFile(filepath.Join(dirs[0], "child.theme"), []byte(child), 0o600); err != nil {
		t.Fatal(err)
	}
	th, err := LoadTheme("mine", dirs)
	if err != nil {
		t.Fatal(err)
	}
	if th.Link != RGB(0x112233) || th.H2 != RGB(0xaabbcc) || th.Dark || th.Name != "mine" {
		t.Errorf("custom theme = %+v", th)
	}
	c, err := LoadTheme("child", dirs)
	if err != nil || c.Keyword != RGB(0x010203) || c.Link != RGB(0x112233) {
		t.Errorf("inherited user theme: %v %+v", err, c)
	}
	if names := UserThemes(dirs); len(names) != 2 {
		t.Errorf("UserThemes = %v", names)
	}
	// names a shell or the terminal would interpret are never listed
	for _, hostile := range []string{"$(touch PWNED).theme", "`id`.theme", "a b.theme", "esc\x1b]0;T\a.theme", "-rf.theme", ".theme", "x:y.theme"} {
		if err := os.WriteFile(filepath.Join(dirs[0], hostile), []byte(custom), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dirs[0], "my-theme_2.1.theme"), []byte(custom), 0o600); err != nil {
		t.Fatal(err)
	}
	if names := strings.Join(UserThemes(dirs), " "); names != "child mine my-theme_2.1" {
		t.Errorf("UserThemes with hostile names = %q", names)
	}
	// explicit path
	if _, err := LoadTheme(filepath.Join(dirs[0], "mine.theme"), nil); err != nil {
		t.Error(err)
	}
	bad := map[string]string{
		"syntax":      "nope",
		"color":       "blue = #12",
		"key":         "unknown-key = #123456",
		"role color":  "heading2 = zzz",
		"dark":        "dark = maybe",
		"self":        "inherit = self",
		"user parent": "inherit = mine\nblue = #123456",
		"parent":      "inherit = missing",
	}
	for name, src := range bad {
		p := filepath.Join(dirs[0], "self.theme")
		if err := os.WriteFile(p, []byte(src), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadTheme("self", dirs); err == nil {
			t.Errorf("bad theme %q accepted", name)
		}
	}
	tpl, err := ThemeTemplate("dracula")
	if err != nil || !strings.Contains(tpl, "inherit = dracula") || !strings.Contains(tpl, "# syntax-keyword") {
		t.Errorf("template: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dirs[0], "tpl.theme"), []byte(tpl), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadTheme("tpl", dirs); err != nil {
		t.Errorf("template does not round-trip: %v", err)
	}
	if _, err := ThemeTemplate("nope"); err == nil {
		t.Error("template for unknown theme")
	}
	if c, err := ParseColor("#ABCDEF"); err != nil || c.Hex() != "#abcdef" {
		t.Error("ParseColor")
	}
	if d := ThemeDirs(func(k string) string { return map[string]string{"XDG_CONFIG_HOME": "/x"}[k] }); d[0] != "/x/kmd/themes" {
		t.Errorf("ThemeDirs = %v", d)
	}
	if d := ThemeDirs(func(k string) string { return map[string]string{"HOME": "/h"}[k] }); d[0] != "/h/.config/kmd/themes" {
		t.Errorf("ThemeDirs = %v", d)
	}
	// relative values would read themes from the current folder: ignored
	rel := map[string]string{"XDG_CONFIG_HOME": "~/.config", "HOME": "/h"}
	if d := ThemeDirs(func(k string) string { return rel[k] }); d[0] != "/h/.config/kmd/themes" {
		t.Errorf("relative XDG_CONFIG_HOME: ThemeDirs = %v", d)
	}
	rel["HOME"] = "~"
	if d := ThemeDirs(func(k string) string { return rel[k] }); len(d) != 1 || d[0] != "/usr/share/kmd/themes" {
		t.Errorf("relative HOME: ThemeDirs = %v", d)
	}
}

// Regression tests for issues found in the performance/quality review.
func TestReviewRegressions(t *testing.T) {
	// attr must not panic when lowercasing changes byte lengths
	out := render("x <img title=\"ȺȺȺȺȺȺȺȺȺȺȺȺȺȺȺȺȺȺȺȺ\" alt=\"İ ok\" src=\"s.png\"> y", plain)
	mustContain(t, out, "İ ok")

	// tabs and control characters measure the same width they paint
	out = render("| a |\n|---|\n| x\ty |\n| é\ty |\n| \x1b[31mz |", Options{Width: 40})
	widths := map[int]bool{}
	for _, l := range nonEmpty(out) {
		widths[strWidth(clean(l))] = true
	}
	if len(widths) != 1 {
		t.Errorf("misaligned table:\n%s", out)
	}

	// tables never exceed the width: shrink, or stack when hopeless
	var hdr, sep, row strings.Builder
	for i := 0; i < 30; i++ {
		hdr.WriteString("| col" + strconv.Itoa(i) + " ")
		sep.WriteString("|---")
		row.WriteString("| value" + strconv.Itoa(i) + " ")
	}
	wide := hdr.String() + "|\n" + sep.String() + "|\n" + row.String() + "|\n" + row.String() + "|"
	out = render(wide, Options{Width: 80})
	mustContain(t, out, "col0: value0", "col29: value29", "┄")
	for _, l := range nonEmpty(out) {
		if strWidth(l) > 80 {
			t.Fatalf("stacked table too wide: %q", l)
		}
	}
	mid := "| aaaaaaaaaa | bbbbbbbbbb | cccccccccc | dddddddddd |\n|---|---|---|---|\n| 1 | 2 | 3 | 4 |"
	for _, l := range nonEmpty(render(mid, Options{Width: 40})) {
		if strWidth(l) > 40 {
			t.Errorf("table wider than 40: %q", l)
		}
	}

	// deep nesting keeps a usable width instead of one char per line
	deep := strings.Repeat("> ", 60) + "hello world"
	out = render(deep, Options{Width: 100})
	if !strings.Contains(out, "hello world") || len(nonEmpty(out)) > 3 {
		t.Errorf("deep quote collapsed:\n%s", out)
	}
	deepList := ""
	for i := 0; i < 60; i++ {
		deepList += strings.Repeat("  ", i) + "- item\n"
	}
	for _, l := range nonEmpty(render(deepList, Options{Width: 100})) {
		if strWidth(l) > 100 {
			t.Fatalf("deep list too wide: %q", l)
		}
	}

	// OSC 8 never carries C1 controls or invalid UTF-8
	if safeURL("http://a/\u009b31m") != "" || safeURL("http://a/\xff") != "" || safeURL("http://ü.de/") == "" {
		t.Error("safeURL C1/UTF-8")
	}

	// TOC depth follows the outline, not absolute levels
	toc := TOC(md.Parse("## A\n# B\n### C\n#### D", md.GFM), Options{Width: 40})
	for _, want := range []string{"1  A", "2  B", "2.1  C", "2.1.1  D"} {
		if !strings.Contains(toc, want) {
			t.Errorf("TOC missing %q:\n%s", want, toc)
		}
	}
	if expandTabs("a\tb\n\tc") != "a   b\n    c" {
		t.Errorf("expandTabs = %q", expandTabs("a\tb\n\tc"))
	}
}

func TestGraphicsFlagOnPartialFailure(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, dir)
	doc := md.Parse("![ok](pic.png) ![missing](nope.png)", md.GFM)
	res := Render(doc, Options{Width: 40, Color: true, Images: termimg.Kitty, BaseDir: dir})
	if res.Graphics || strings.Contains(res.Text, "\x1b_G") {
		t.Error("graphics flagged although the paragraph fell back to text")
	}
}

// Regressions from the security review.
func TestSecurityHardening(t *testing.T) {
	// bidi overrides (Trojan Source) are stripped; invisible chars kill URLs
	out := render("```go\nif x \u202e} else {\u2066 y\n```\n\n<img src=\"u\u202e\" alt=\"a\">", Options{Width: 60, Color: true, Hyperlinks: true})
	if strings.ContainsAny(out, "\u202e\u2066") {
		t.Errorf("bidi control leaked: %q", out)
	}
	if safeURL("http://a/\u200b") != "" || safeURL("http://a/\ufeff") != "" {
		t.Error("invisible characters accepted in URL")
	}
	if Clean("a\u202eb\u2069c\x1bd") != "abcd" {
		t.Errorf("Clean = %q", Clean("a\u202eb\u2069c\x1bd"))
	}
	// zero-width and tag characters would make two commands look the same
	out = render("```sh\ncurl https://example.com/install\u200b.sh\U000E0041 | sh\n```\n\nword\u2060joiner &#xfeff;", Options{Width: 60})
	if strings.ContainsAny(out, "\u200b\U000E0041\u2060\ufeff") || !strings.Contains(out, "install.sh | sh") {
		t.Errorf("invisible character leaked: %q", out)
	}
	if out := render("\U0001F468\u200d\U0001F469\u200d\U0001F467 \u2764\ufe0f \u0645\u06cc\u200c\u062e\u0648\u0627\u0647\u0645", Options{Width: 60}); !strings.Contains(out, "\U0001F468\u200d\U0001F469\u200d\U0001F467 \u2764\ufe0f \u0645\u06cc\u200c\u062e\u0648\u0627\u0647\u0645") {
		t.Errorf("joiners and variation selectors must stay: %q", out)
	}

	// repeated images are decoded once and drawn from the cache
	dir := t.TempDir()
	writePNG(t, dir)
	r := &renderer{Budget: &Budget{}, th: &Dark, opt: Options{Width: 40, Color: true, Images: termimg.Blocks, BaseDir: dir}}
	doc := md.Parse(strings.Repeat("![a](pic.png)\n\n", 200), md.GFM)
	lines := r.blocks(doc, 36, false)
	if len(r.images) != 1 || len(lines) == 0 {
		t.Errorf("image cache: %d entries", len(r.images))
	}
	// distinct images beyond the budget fall back to alt text
	var many strings.Builder
	for i := 0; i < maxImages+5; i++ {
		many.WriteString("![alt" + strconv.Itoa(i) + "](pic.png?" + strconv.Itoa(i) + ")\n\n")
	}
	_ = Render(md.Parse(many.String(), md.GFM), Options{Width: 40, Color: true, Images: termimg.Blocks, BaseDir: dir})

	// diagrams and display math beyond the budget show their source
	var specials strings.Builder
	for i := 0; i < maxSpecials+3; i++ {
		specials.WriteString("```mermaid\ngraph LR\nA-->B\n```\n\n$$x" + strconv.Itoa(i) + "$$\n\n")
	}
	out = render(specials.String(), Options{Width: 60, Math: true, Diagrams: true})
	if !strings.Contains(out, "graph LR") {
		t.Error("special-block budget not applied")
	}
	var inl strings.Builder
	for i := 0; i < maxInlineMath+3; i++ {
		inl.WriteString("$x$ ")
	}
	if out := render(inl.String(), Options{Width: 60, Math: true}); !strings.Contains(out, "$x$") {
		t.Error("inline math budget not applied")
	}
}

// TestImageLoadedOnce: an image drawn at many sizes (nesting depths) is
// loaded once, and the source pixels scaled per document are bounded.
func TestImageLoadedOnce(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, dir)
	var src strings.Builder
	for i := range 30 {
		src.WriteString(strings.Repeat("> ", i) + "![a](pic.png)\n\n")
	}
	doc := md.Parse(src.String(), md.GFM)
	b := &Budget{}
	res := Render(doc, Options{Width: 100, Color: true, Images: termimg.Blocks, BaseDir: dir, Budget: b})
	if len(b.loaded) != 1 || !strings.Contains(res.Text, "▀") {
		t.Errorf("image loaded %d times", len(b.loaded))
	}
	spent := &Budget{imgPixels: maxImagePixels}
	if res := Render(doc, Options{Width: 100, Color: true, Images: termimg.Blocks, BaseDir: dir, Budget: spent}); strings.Contains(res.Text, "▀") {
		t.Error("image drawn past the pixel budget")
	}
}

// TestImageNamesShareOneImage: the names of one file (pic.png, ./pic.png,
// ././pic.png…) are one image with one frame per size, not one each.
func TestImageNamesShareOneImage(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, dir)
	var src strings.Builder
	for i := range 60 {
		src.WriteString("![a](" + strings.Repeat("./", i) + "pic.png)\n\n")
	}
	b := &Budget{}
	res := Render(md.Parse(src.String(), md.GFM), Options{Width: 40, Color: true, Images: termimg.Kitty, BaseDir: dir, Budget: b})
	if len(b.images) != 1 || strings.Count(res.Text, "\x1b_G") != 60 {
		t.Errorf("%d frames for one file, %d drawn", len(b.images), strings.Count(res.Text, "\x1b_G"))
	}
}

// TestImageFramesWithinBudget: a frame that cannot be shown because the
// output budget is spent is not kept either.
func TestImageFramesWithinBudget(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, dir)
	b := &Budget{imgBytes: maxImageBytes - 10}
	res := Render(md.Parse("![a](pic.png)", md.GFM), Options{Width: 40, Color: true, Images: termimg.Kitty, BaseDir: dir, Budget: b})
	if strings.Contains(res.Text, "\x1b_G") {
		t.Error("image drawn past the budget")
	}
	for _, f := range b.images {
		if f.ok || len(f.frame.Lines) > 0 {
			t.Errorf("frame kept past the budget: %d lines", len(f.frame.Lines))
		}
	}
}

// TestThemeFileKinds: theme paths must name regular files; a device or a
// FIFO would be read forever or block.
func TestThemeFileKinds(t *testing.T) {
	for _, p := range []string{"/dev/zero", t.TempDir() + "/"} {
		if _, err := os.Stat(p); err != nil {
			continue
		}
		if _, err := LoadTheme(p, nil); err == nil {
			t.Errorf("LoadTheme(%q) succeeded", p)
		}
	}
	big := filepath.Join(t.TempDir(), "big.theme")
	if err := os.WriteFile(big, []byte(strings.Repeat("#\n", maxThemeBytes)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadTheme(big, nil); err == nil || !strings.Contains(err.Error(), "too large") {
		t.Errorf("large theme file: %v", err)
	}
}
