package render

import (
	"github.com/DipaolaFederico/kmd/internal/testlimit"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DipaolaFederico/kmd/internal/md"
)

func TestHasScheme(t *testing.T) {
	for s, want := range map[string]bool{
		"https://x": true, "mailto:a@b": true, "git+ssh://x": true, "file:///a": true,
		"C:\\x": false, "a.md": false, "../a.md": false, "#x": false, "": false, "1x:y": false, "a b:c": false,
	} {
		if hasScheme(s) != want {
			t.Errorf("hasScheme(%q) = %v", s, !want)
		}
	}
}

func TestHref(t *testing.T) {
	dir := t.TempDir()
	r := &renderer{Budget: &Budget{}, opt: Options{BaseDir: dir}}
	for dest, want := range map[string]string{
		"":                  "",
		"#sec":              "#sec",
		"https://x.y/a":     "https://x.y/a",
		"//cdn.x/a":         "//cdn.x/a",
		"a.md":              "file://" + filepath.ToSlash(filepath.Join(dir, "a.md")),
		"../b.md#mapa":      "file://" + filepath.ToSlash(filepath.Join(filepath.Dir(dir), "b.md")) + "#mapa",
		"sub/c%20d.md?x=1":  "file://" + filepath.ToSlash(filepath.Join(dir, "sub", "c%20d.md")), // re-escaped
		"/abs/e.md#%C3%A1":  "file:///abs/e.md#%C3%A1",
		"dir/":              "file://" + filepath.ToSlash(filepath.Join(dir, "dir")),
		"bad%zz.md#bad%zz2": "file://" + filepath.ToSlash(filepath.Join(dir, "bad%25zz.md")) + "#bad%25zz2",
	} {
		if got, _ := r.href(dest); got != want { // undecodable escapes are kept literally
			t.Errorf("href(%q) = %q, want %q", dest, got, want)
		}
	}
	// without BaseDir, paths resolve against the working directory
	wd, _ := filepath.Abs(".")
	if got, local := (&renderer{}).href("x.md"); !local || got != "file://"+filepath.ToSlash(filepath.Join(wd, "x.md")) {
		t.Errorf("no BaseDir: %q", got)
	}
}

func TestSlug(t *testing.T) {
	for in, want := range map[string]string{
		"Mapa de la etapa":             "mapa-de-la-etapa",
		"  Estructura del repo  ":      "estructura-del-repo",
		"E5-01: Prometheus (lab)!":     "e5-01-prometheus-lab",
		"snake_case & C++":             "snake_case--c",
		"Configuración — avanzada":     "configuración--avanzada",
		"`code` *emph*":                "code-emph",
		"1.2.3 Release":                "123-release",
		"Ünïcödé ñandú":                "ünïcödé-ñandú",
		"":                             "",
		"Émojis 🚀 are dropped":         "émojis--are-dropped",
		"Tabs\tand\u00a0nbsp collapse": "tabsandnbsp-collapse",
	} {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNavigateLinksAndAnchors(t *testing.T) {
	src := "# Title\n\nSee [one](a.md) and [two](https://x.y \"t\").\n\n## Mapa\n\n" +
		"A [long link text that will certainly wrap across lines](b.md#mapa) here.\n\n## Mapa\n\n" +
		"| a | b |\n|---|---|\n| [c](c.md) | ![img](i.png) |\n"
	dir := t.TempDir()
	for _, color := range []bool{true, false} {
		res := Render(md.Parse(src, md.GFM), Options{Width: 30, Color: color, Navigate: true, BaseDir: dir})
		lines := strings.Split(res.Text, "\n")
		if len(res.Links) != 4 {
			t.Fatalf("color=%v: %d links: %+v", color, len(res.Links), res.Links)
		}
		for _, l := range res.Links {
			for _, s := range l.Spans {
				plain := stripSGR(lines[s.Line])
				if got := cols(plain, s.Col, s.Width); got != s.Text {
					t.Errorf("color=%v span %+v: screen has %q", color, s, got)
				}
			}
		}
		if res.Links[1].Href != "https://x.y" || !strings.HasSuffix(res.Links[0].Href, "/a.md") {
			t.Errorf("hrefs: %q %q", res.Links[0].Href, res.Links[1].Href)
		}
		if l := res.Links[2]; len(l.Spans) < 2 || !strings.HasSuffix(l.Href, "/b.md#mapa") {
			t.Errorf("wrapped link: %+v", l)
		}
		for k, want := range map[string]string{"title": "TITLE", "mapa": "Mapa", "mapa-1": "Mapa"} {
			line, ok := res.Anchors[k]
			if !ok || !strings.Contains(strings.ToUpper(stripSGR(lines[line])), strings.ToUpper(want)) {
				t.Errorf("color=%v anchor %q -> %d %q", color, k, line, stripSGR(lines[line]))
			}
		}
	}
	// without Navigate nothing is collected and relative OSC 8 targets are resolved
	res := Render(md.Parse("[a](a.md)", md.GFM), Options{Width: 40, Color: true, Hyperlinks: true, BaseDir: dir})
	if res.Links != nil || res.Anchors != nil {
		t.Error("navigation data without Navigate")
	}
	if want := "\x1b]8;;file://" + filepath.ToSlash(filepath.Join(dir, "a.md")); !strings.Contains(res.Text, want) {
		t.Errorf("OSC 8 target not absolute: %q", res.Text)
	}
}

// Wiki links and emoji shortcodes reach the terminal like ordinary relative
// links and text: resolved OSC 8 targets, navigable links, emoji widths.
func TestWikiLinksAndEmoji(t *testing.T) {
	dir := t.TempDir()
	src := "Go to [[My Notes#Deep Dive|notes]] :rocket: [[sub/Other]] `[[x]]`\n"
	res := Render(md.Parse(src, md.GFM), Options{Width: 80, Color: true, Hyperlinks: true, Navigate: true, BaseDir: dir})
	base := "file://" + filepath.ToSlash(dir)
	if len(res.Links) != 2 || res.Links[0].Href != base+"/My%20Notes.md#Deep%20Dive" || res.Links[1].Href != base+"/sub/Other.md" {
		t.Fatalf("links: %+v", res.Links)
	}
	if !strings.Contains(res.Text, "\x1b]8;;"+base+"/My%20Notes.md#Deep%20Dive\x1b\\") {
		t.Errorf("OSC 8 target: %q", res.Text)
	}
	plain := stripSGR(res.Text)
	if !strings.Contains(plain, "Go to notes 🚀 sub/Other \u00a0[[x]]\u00a0") {
		t.Errorf("text: %q", plain)
	}
	if s := res.Links[1].Spans[0]; s.Text != "sub/Other" || s.Col != strWidth("  Go to notes 🚀 ") {
		t.Errorf("span after an emoji: %+v", s)
	}
	cm := Render(md.Parse(src, md.Options{}), Options{Width: 80})
	if !strings.Contains(cm.Text, "[[My Notes#Deep Dive|notes]] :rocket:") {
		t.Errorf("CommonMark: %q", cm.Text)
	}
}

// stripSGR removes SGR and OSC 8 sequences.
func stripSGR(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			for i < len(s) && s[i] != 'm' {
				i++
			}
			continue
		}
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == ']' {
			end := strings.Index(s[i:], "\x1b\\")
			i += end + 1
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// cols returns the text in display columns [col, col+w).
func cols(s string, col, w int) string {
	var b strings.Builder
	c := 0
	for _, r := range s {
		if c >= col && c < col+w {
			b.WriteRune(r)
		}
		c += runeWidth(r)
	}
	return b.String()
}

func TestViewerStyles(t *testing.T) {
	sel, match, bar := ViewerStyles(nil, true, true)
	for _, s := range []string{sel, match, bar} {
		if !strings.HasPrefix(s, "\x1b[0") || !strings.HasSuffix(s, "m") || !strings.Contains(s, "48;2;") {
			t.Errorf("color style %q", s)
		}
	}
	if sel == match {
		t.Error("selection and matches look the same")
	}
	sel, _, _ = ViewerStyles(&Light, false, false)
	if sel != "\x1b[0;7m" {
		t.Errorf("no-color selection %q", sel)
	}
}

func TestRepeatedHeadingsAreLinear(t *testing.T) {
	// GitHub's suffixes, including a heading that collides with a suffix
	res := Render(md.Parse("# a\n\n# a\n\n# a-1\n\n# a\n", md.GFM), Options{Width: 40, Navigate: true})
	for _, k := range []string{"a", "a-1", "a-1-1", "a-2"} {
		if _, ok := res.Anchors[k]; !ok {
			t.Errorf("anchor %q missing: %v", k, res.Anchors)
		}
	}
	// was quadratic: 40,000 identical headings took ~44 s
	src := strings.Repeat("# a\n", 40000)
	start := time.Now()
	res = Render(md.Parse(src, md.GFM), Options{Width: 40, Navigate: true})
	if len(res.Anchors) != 40000 {
		t.Errorf("%d anchors", len(res.Anchors))
	}
	if d := time.Since(start); d > testlimit.Scale()*5*time.Second {
		t.Errorf("40,000 repeated headings took %v", d)
	}
	testlimit.Linear(t, "repeated headings", 1<<17, func(n int) func() {
		doc := md.Parse(strings.Repeat("# a\n", n), md.GFM)
		return func() { _ = Render(doc, Options{Width: 40, Navigate: true}) }
	})
}

func TestOSC8OnlyForOpenableFiles(t *testing.T) {
	dir := t.TempDir()
	src := "[a](doc.md) [b](run.sh) [c](evil.desktop) [d](sub/) [e](pic.png#x)"
	res := Render(md.Parse(src, md.GFM), Options{Width: 80, Color: true, Hyperlinks: true, Navigate: true, BaseDir: dir})
	abs := "file://" + filepath.ToSlash(dir)
	for _, want := range []string{"\x1b]8;;" + abs + "/doc.md\x1b", "\x1b]8;;run.sh\x1b", "\x1b]8;;evil.desktop\x1b", "\x1b]8;;sub/\x1b", "\x1b]8;;" + abs + "/pic.png#x\x1b"} {
		if !strings.Contains(res.Text, want) {
			t.Errorf("missing OSC 8 %q", want)
		}
	}
	// the viewer still gets resolved targets (it applies its own checks)
	if !strings.HasSuffix(res.Links[1].Href, "/run.sh") || !strings.HasPrefix(res.Links[1].Href, "file://") {
		t.Errorf("viewer href %q", res.Links[1].Href)
	}
	if !SafeURL("https://x") || SafeURL("https://x\u200b") || SafeURL("") {
		t.Error("SafeURL")
	}
}

func TestHrefHostileInputs(t *testing.T) {
	// a huge reference destination used many times: was quadratic (13 s)
	dest := strings.Repeat("a/", 50000) + "x.md"
	src := "[r]: <" + dest + ">\n\n" + strings.Repeat("[a][r] ", 4000)
	start := time.Now()
	res := Render(md.Parse(src, md.GFM), Options{Width: 80, Color: true, Hyperlinks: true, Navigate: true})
	if d := time.Since(start); d > testlimit.Scale()*5*time.Second {
		t.Errorf("long reference destination took %v", d)
	}
	// both the destination (always over maxURL) and the number of links
	// grow with n: linear, not their product
	testlimit.Linear(t, "long reference destination", 1<<10, func(n int) func() {
		doc := md.Parse("[r]: <"+strings.Repeat("a/", maxURL/2*n)+"x.md>\n\n"+strings.Repeat("[a][r] ", 64*n), md.GFM)
		return func() { _ = Render(doc, Options{Width: 80, Color: true, Hyperlinks: true, Navigate: true}) }
	})
	if strings.Contains(res.Text, "\x1b]8;;"+dest[:10]) || res.Links[0].Href != dest {
		t.Error("over-long target must be kept as written and never emitted")
	}
	// explicit file:// links: only openable files become OSC 8 targets
	res = Render(md.Parse("[a](file:///x/run.desktop) [b](file:///x/doc.md)", md.GFM), Options{Width: 80, Color: true, Hyperlinks: true})
	if strings.Contains(res.Text, "run.desktop\x1b") || !strings.Contains(res.Text, "\x1b]8;;file:///x/doc.md\x1b") {
		t.Errorf("file:// OSC 8 filtering: %q", res.Text)
	}
}

func TestCodeBlockPositions(t *testing.T) {
	src := "# T\n\n```go\nfmt.Println(1)\n```\n\n- item\n\n  ```\n  nested\x1b\n  two\n  ```\n\n> ```sh\n> quoted\n> ```\n\ntext\n"
	res := Render(md.Parse(src, md.GFM), Options{Width: 60, Navigate: true, Color: true})
	lines := strings.Split(res.Text, "\n")
	if len(res.Code) != 3 {
		t.Fatalf("%d code blocks", len(res.Code))
	}
	want := []struct{ lang, text string }{{"go", "fmt.Println(1)"}, {"", "nested\x1b\ntwo"}, {"sh", "quoted"}}
	for i, c := range res.Code {
		if c.Lang != want[i].lang || c.Text != want[i].text {
			t.Errorf("block %d: %q %q", i, c.Lang, c.Text)
		}
		top, bottom := stripSGR(lines[c.Start]), stripSGR(lines[c.End])
		if !strings.Contains(top, "╭") || !strings.Contains(bottom, "╰") || c.End-c.Start < 2 {
			t.Errorf("block %d at %d-%d: %q … %q", i, c.Start, c.End, top, bottom)
		}
	}
	// without navigation nothing is recorded
	if res := Render(md.Parse(src, md.GFM), Options{Width: 60}); res.Code != nil {
		t.Error("code blocks recorded without Navigate")
	}
}

func TestSharedBudget(t *testing.T) {
	src := strings.Repeat("```mermaid\ngraph LR\n  A --> B\n```\n\n", 60)
	diagrams := func(res Result) int { return strings.Count(res.Text, "┌") }
	b := &Budget{}
	o := Options{Width: 60, Diagrams: true, Budget: b}
	first := Render(md.Parse(src, md.GFM), o)
	second := Render(md.Parse(src, md.GFM), o)
	if d1, d2 := diagrams(first), diagrams(second); d1 == 0 || d2 >= d1 {
		t.Errorf("a shared budget must carry over: %d then %d boxes", d1, d2)
	}
	o.Budget = nil
	if d := diagrams(Render(md.Parse(src, md.GFM), o)); d != diagrams(first) {
		t.Errorf("without a shared budget each document has its own: %d", d)
	}
}

// With a BaseURL (a piped document), relative links become web links in
// OSC 8 targets, navigation and plain output; anchors, schemes and images
// are left alone, and file: links are still not made clickable.
func TestBaseURL(t *testing.T) {
	base, _ := url.Parse("https://example.com/docs/README.md")
	src := "[g](guide.md#Setup) [a](#top) [w](https://a.b/) [f](file:///bin/sh) [o](//other.org/x) ![i](i.png)\n"
	res := Render(md.Parse(src, md.GFM), Options{Width: 120, Color: true, Hyperlinks: true, Navigate: true, BaseURL: base})
	var hrefs []string
	for _, l := range res.Links {
		hrefs = append(hrefs, l.Href)
	}
	want := []string{"https://example.com/docs/guide.md#Setup", "#top", "https://a.b/", "file:///bin/sh", "https://other.org/x"}
	if strings.Join(hrefs, " ") != strings.Join(want, " ") {
		t.Errorf("links: %q", hrefs)
	}
	for _, s := range []string{"\x1b]8;;https://example.com/docs/guide.md#Setup\x1b\\", "\x1b]8;;#top\x1b\\", "\x1b]8;;i.png\x1b\\"} {
		if !strings.Contains(res.Text, s) {
			t.Errorf("missing OSC 8 %q in %q", s, res.Text)
		}
	}
	if strings.Contains(res.Text, "]8;;file:") || strings.Contains(res.Text, "example.com/docs/i.png") {
		t.Errorf("file: link clickable or image resolved: %q", res.Text)
	}
	plain := Render(md.Parse(src+"[guide.md](guide.md)", md.GFM), Options{Width: 120, BaseURL: base}).Text
	if !strings.Contains(plain, "g (https://example.com/docs/guide.md#Setup)") ||
		!strings.Contains(plain, "guide.md (https://example.com/docs/guide.md)") {
		t.Errorf("plain output: %q", plain)
	}
}
