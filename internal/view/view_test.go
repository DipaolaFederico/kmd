package view

import (
	"errors"
	"github.com/DipaolaFederico/kmd/internal/testlimit"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/DipaolaFederico/kmd/internal/md"
	"github.com/DipaolaFederico/kmd/internal/render"
	"github.com/DipaolaFederico/kmd/internal/styled"
)

// parseKeys decodes one chunk with a fresh parser.
func parseKeys(b []byte) []key {
	var p keyParser
	return p.parse(b)
}

func TestParseKeys(t *testing.T) {
	names := func(b string) []string {
		var out []string
		for _, k := range parseKeys([]byte(b)) {
			if k.name == "char" {
				out = append(out, "char:"+string(k.r))
			} else {
				out = append(out, k.name)
			}
		}
		return out
	}
	for in, want := range map[string][]string{
		"jq":                     {"char:j", "char:q"},
		"ñ€":                     {"char:ñ", "char:€"},
		"\x1b[A\x1b[B\x1bOC":     {"up", "down", "right"},
		"\x1b[5~\x1b[6~\x1b[Z":   {"pgup", "pgdn", "btab"},
		"\x1b[1~\x1b[4~\x1b[H":   {"home", "end", "home"},
		"\x1b[1;5A":              {"up"},
		"\x1b[99~x":              {"char:x"},
		"\x1b":                   {"esc"},
		"\x1b[":                  {"esc", "char:["},
		"\x1b[12":                nil,
		"\r\n\t\x7f\x08\x03\x1a": {"enter", "enter", "tab", "backspace", "backspace", "ctrl-c", "ctrl-z"},
		"\x1b[<0;5;7M":           {"mouse"},
		// late answers to the background query are not keys ("e" would edit)
		"\x1b]11;rgb:eeee/eeee/eeee\x1b\\\x1b[?62;22cj": {"char:j"},
		"\x1b]11;rgb:1c1c/1c1c/1c1c\aq":                 {"char:q"},
		"\x1b]11;rgb:ec":                                nil,
		"\x1b]x":                                        {"esc", "char:]", "char:x"}, // Alt+] x
		"\x1b[<0;5M\x1b[<99999999999999999999;1;1M":     nil,
	} {
		if got := names(in); !reflect.DeepEqual(got, want) {
			t.Errorf("parseKeys(%q) = %v, want %v", in, got, want)
		}
	}
	k := parseKeys([]byte("\x1b[<65;10;3m"))[0]
	if k.btn != 65 || k.x != 9 || k.y != 2 || k.press {
		t.Errorf("mouse decode: %+v", k)
	}
}

// TestParseKeysSplitAnswer feeds a late answer to the background query in
// pieces, as a slow link delivers it: none of it becomes keys.
func TestParseKeysSplitAnswer(t *testing.T) {
	for _, chunks := range [][]string{
		{"\x1b]11;rgb:eeee/", "eeee/eeee\x1b\\\x1b[?62;22c", "j"},
		{"\x1b]11;rgb:eeee/eeee/eeee\x1b", "\\j"}, // split inside ESC \
		{"\x1b]11;rgb:ec", "ec/ecec/", "ecec\a", "j"},
	} {
		var p keyParser
		var got []key
		for _, c := range chunks {
			got = append(got, p.parse([]byte(c))...)
		}
		if len(got) != 1 || got[0].r != 'j' {
			t.Errorf("%q: %+v", chunks, got)
		}
	}
	// a lost terminator swallows at most maxOSC bytes
	var p keyParser
	p.parse([]byte("\x1b]11;rgb:"))
	if got := p.parse([]byte(strings.Repeat("x", maxOSC))); len(got) != 0 {
		t.Errorf("keys inside the string: %d", len(got))
	}
	if got := p.parse([]byte("j")); len(got) != 1 || got[0].r != 'j' {
		t.Errorf("after the bound: %+v", got)
	}
}

func TestStripEscapes(t *testing.T) {
	for in, want := range map[string]string{
		"plain":                      "plain",
		"\x1b[1;38;5;12mbold\x1b[0m": "bold",
		"\x1b]8;;https://x\x1b\\link\x1b]8;;\x1b\\": "link",
		"a\x1b]0;title\x07b":                        "ab",
		"\x1b_Gf=100;AAAA\x1b\\img":                 "img",
		"\x1bPq#0;2;0;0;0\x1b\\six":                 "six",
		"x\x1b":                                     "x\x1b",
		"x\x1b7y":                                   "xy",
		"\x1b[31":                                   "",
	} {
		if got := stripEscapes(in); got != want {
			t.Errorf("stripEscapes(%q) = %q, want %q", in, got, want)
		}
	}
}

// fixture writes a small documentation tree and returns its directory.
func fixture(t *testing.T) string {
	t.Helper()
	files := map[string]string{
		"README.md": "# Index\n\n[Ticket one](one.md) · [Two, section](sub/two.md#deep) · " +
			"[Here](#mapa) · [Web](https://example.com) · [Pic](pic.png) · [Script](run.sh) · " +
			"[Exe](tool.png) · [Missing](nope.md) · [Dir](sub/) · [Empty dir](empty/) · " +
			"[Mail](mailto:a@b.c) · [JS](javascript:alert(1))\n\n" +
			strings.Repeat("filler line\n\n", 40) + "## Mapa\n\nthe map, with a Needle and a needle\n\n" +
			strings.Repeat("more\n\n", 30) + "[last](one.md)\n",
		"one.md":        "# One\n\nBack to [index](README.md#mapa) or [self](one.md#one).\n",
		"sub/two.md":    "# Two\n\n" + strings.Repeat("x\n\n", 40) + "## Deep\n\ndeep text\n",
		"sub/README.md": "# Sub readme\n",
		"pic.png":       "png",
		"run.sh":        "#!/bin/sh\n",
		"tool.png":      "",
	}
	dir := tree(t, files)
	if err := os.Chmod(filepath.Join(dir, "tool.png"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "empty"), 0o750); err != nil {
		t.Fatal(err)
	}
	return dir
}

func loader(start string) Loader {
	return func(path string, cols int) (*Page, error) {
		if path == "" {
			path = start
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		res := render.Render(md.Parse(string(src), md.GFM), render.Options{
			Width: cols, Color: true, Navigate: true, BaseDir: filepath.Dir(path)})
		return &Page{Lines: strings.Split(strings.TrimSuffix(res.Text, "\n"), "\n"), Links: res.Links, Anchors: res.Anchors, Headings: res.Headings}, nil
	}
}

type harness struct {
	*model
	opened []string
	edited []string
}

func newHarness(t *testing.T, dir string) *harness {
	t.Helper()
	h := &harness{}
	opt := Options{
		Load: loader(filepath.Join(dir, "README.md")), Start: filepath.Join(dir, "README.md"),
		Select: "<S>", Match: "<M>", Bar: "<B>",
		Open: func(target string) error {
			if strings.Contains(target, "fail") {
				return errors.New("open failed")
			}
			h.opened = append(h.opened, target)
			return nil
		},
		Edit: func(path string) error { h.edited = append(h.edited, path); return nil },
	}
	m, err := newModel(opt, 80, 20)
	if err != nil {
		t.Fatal(err)
	}
	h.model = m
	return h
}

func (h *harness) keys(ks ...string) {
	for _, k := range ks {
		switch {
		case len([]rune(k)) == 1:
			h.handle(key{name: "char", r: []rune(k)[0]})
		default:
			h.handle(key{name: k})
		}
	}
}

func (h *harness) selHref() string {
	if h.sel < 0 {
		return ""
	}
	return h.page.Links[h.sel].Href
}

func (h *harness) screen() string {
	var b strings.Builder
	h.draw(&b)
	return b.String()
}

// bottom returns the text of the bottom line, without escape sequences.
func (h *harness) bottom() string {
	var b strings.Builder
	h.status(&b)
	return strings.TrimSpace(stripEscapes(b.String()))
}

func TestNewModelError(t *testing.T) {
	_, err := newModel(Options{Load: func(string, int) (*Page, error) { return nil, os.ErrNotExist }}, 80, 20)
	if err == nil {
		t.Fatal("load error not reported")
	}
}

func TestLinkSelection(t *testing.T) {
	h := newHarness(t, fixture(t))
	n := len(h.page.Links)
	if n != 13 {
		t.Fatalf("%d links", n)
	}
	h.keys("tab")
	if !strings.HasSuffix(h.selHref(), "/one.md") {
		t.Fatalf("first tab selected %q", h.selHref())
	}
	if s := h.screen(); !strings.Contains(s, "<S>Ticket one") || !strings.Contains(s, "→ one.md") {
		t.Errorf("selection not drawn: %q", s)
	}
	h.keys("btab") // wraps to the last link, scrolling to it
	if h.sel != n-1 || h.top == 0 {
		t.Errorf("btab wrap: sel=%d top=%d", h.sel, h.top)
	}
	h.keys("tab") // wraps to the first one
	if h.sel != 0 || h.top > h.linkLine(0) {
		t.Errorf("tab wrap: sel=%d top=%d", h.sel, h.top)
	}
	h.keys("G", "tab") // selection off screen: start from the screen
	if h.sel != n-1 {
		t.Errorf("tab from screen: %d", h.sel)
	}
	h.keys("g", "btab")
	if h.sel != n-2 { // the last link is far below the first screen
		t.Errorf("btab from screen: %d", h.sel)
	}
	h.keys("esc")
	if h.sel != -1 {
		t.Error("esc did not clear the selection")
	}
	h.keys("enter") // nothing selected: scroll like less
	if h.top != 1 {
		t.Errorf("enter scroll: %d", h.top)
	}
	h.keys("l")
	if h.msg == "" {
		t.Error("no hint for l without selection")
	}

	empty := &harness{model: &model{page: &Page{Lines: []string{"a"}}, rows: 10, cols: 40, sel: -1}}
	empty.keys("tab")
	if empty.msg != "no links in this document" {
		t.Errorf("no links: %q", empty.msg)
	}
}

// selectLink selects the link whose href ends with suffix.
func (h *harness) selectLink(t *testing.T, suffix string) {
	t.Helper()
	for range h.page.Links {
		h.keys("tab")
		if strings.HasSuffix(h.selHref(), suffix) {
			return
		}
	}
	t.Fatalf("no link %q", suffix)
}

func TestFollowAndBack(t *testing.T) {
	dir := fixture(t)
	h := newHarness(t, dir)

	h.selectLink(t, "/one.md")
	h.keys("enter")
	if h.path != filepath.Join(dir, "one.md") || len(h.hist) != 1 || h.top != 0 {
		t.Fatalf("follow: path=%q hist=%d", h.path, len(h.hist))
	}
	if s := h.screen(); !strings.Contains(s, "README.md › one.md") {
		t.Errorf("breadcrumb missing: %q", s)
	}
	h.selectLink(t, "/one.md#one") // link to the page itself: anchor jump
	h.keys("enter")
	if h.path != filepath.Join(dir, "one.md") || len(h.hist) != 2 {
		t.Errorf("self link: %q %d", h.path, len(h.hist))
	}
	h.keys("backspace", "tab") // back restores the selection (self link)
	if !strings.HasSuffix(h.selHref(), "README.md#mapa") {
		t.Fatalf("selected %q", h.selHref())
	}
	h.keys("right") // into README at #mapa
	if h.path != filepath.Join(dir, "README.md") || h.top == 0 {
		t.Errorf("anchor in other doc: %q top=%d", h.path, h.top)
	}
	if !strings.Contains(stripEscapes(h.page.Lines[h.top]), "Mapa") {
		t.Errorf("top line %q", stripEscapes(h.page.Lines[h.top]))
	}
	h.keys("h", "left") // back to one.md, back to README with Ticket one selected
	if h.path != filepath.Join(dir, "README.md") || !strings.HasSuffix(h.selHref(), "/one.md") {
		t.Errorf("back: %q sel=%q", h.path, h.selHref())
	}
	h.keys("backspace")
	if h.msg != "no previous document" {
		t.Errorf("empty history: %q", h.msg)
	}

	h.keys("g", "esc")
	h.selectLink(t, "/sub/two.md#deep")
	h.keys("enter")
	if l := h.page.Anchors["deep"]; h.path != filepath.Join(dir, "sub", "two.md") || l < h.top || l >= h.top+h.viewH() {
		t.Errorf("two.md#deep: %q top=%d", h.path, h.top)
	}
	h.keys("backspace", "esc", "g")
	h.selectLink(t, "#mapa")
	h.keys("enter")
	if h.top == 0 || len(h.hist) != 1 {
		t.Errorf("in-page anchor: top=%d hist=%d", h.top, len(h.hist))
	}
	h.keys("backspace")
	if h.top != 0 {
		t.Errorf("back after anchor: %d", h.top)
	}
	h.keys("esc")
	h.selectLink(t, "/sub")
	h.keys("enter")
	if h.path != filepath.Join(dir, "sub", "README.md") {
		t.Errorf("directory link: %q", h.path)
	}
	// history of a deleted file
	h.hist = append(h.hist, entry{path: filepath.Join(dir, "gone.md")})
	h.keys("backspace")
	if !strings.Contains(h.msg, "gone.md") {
		t.Errorf("back to a deleted file: %q", h.msg)
	}
}

// Wiki links are followed like relative links, headings by their text.
func TestFollowWikiLink(t *testing.T) {
	dir := tree(t, map[string]string{
		"README.md":   "# Index\n\nSee [[My Notes#Deep Dive|the notes]].\n",
		"My Notes.md": "# Notes\n\n" + strings.Repeat("x\n\n", 40) + "## Deep Dive\n\ndeep text\n",
	})
	h := newHarness(t, dir)
	h.selectLink(t, "/My%20Notes.md#Deep%20Dive")
	h.keys("enter")
	if l := h.page.Anchors["deep-dive"]; h.path != filepath.Join(dir, "My Notes.md") || l == 0 || l < h.top || l >= h.top+h.viewH() {
		t.Errorf("wiki link: path=%q top=%d msg=%q", h.path, h.top, h.msg)
	}
}

func TestFollowTargets(t *testing.T) {
	dir := fixture(t)
	h := newHarness(t, dir)
	for _, c := range []struct{ suffix, msg string }{
		{"https://example.com", "opened https://example.com"},
		{"/pic.png", "opened pic.png"},
		{"/run.sh", "not a Markdown file: run.sh"},
		{"/tool.png", "not a Markdown file: tool.png"},
		{"/nope.md", "not found: nope.md"},
		{"mailto:a@b.c", "opened mailto:a@b.c"},
		{"javascript:alert(1)", "unsupported link: javascript:alert(1)"},
	} {
		h.keys("esc", "g")
		h.selectLink(t, c.suffix)
		h.keys("enter")
		if h.msg != c.msg {
			t.Errorf("%s: msg %q, want %q", c.suffix, h.msg, c.msg)
		}
	}
	if want := []string{"https://example.com", filepath.Join(dir, "pic.png"), "mailto:a@b.c"}; !reflect.DeepEqual(h.opened, want) {
		t.Errorf("opened %v", h.opened)
	}
	h.follow("http://%zz")
	if h.msg != "invalid link" {
		t.Errorf("invalid: %q", h.msg)
	}
	h.follow("https://fail.example")
	if h.msg != "open failed" {
		t.Errorf("open error: %q", h.msg)
	}
	h.opt.Open = nil
	h.follow("https://x.y")
	if h.msg != "cannot open https://x.y" {
		t.Errorf("no opener: %q", h.msg)
	}
	h.follow("#no-such-heading")
	if h.msg != "no heading #no-such-heading" {
		t.Errorf("bad anchor: %q", h.msg)
	}
	h.follow("#Mapa") // not a slug: slugified
	if h.top == 0 {
		t.Error("anchor slugify")
	}
	// a page that fails to load keeps the current one
	h.opt.Load = func(string, int) (*Page, error) { return nil, errors.New("boom") }
	h.follow("file://" + filepath.ToSlash(filepath.Join(dir, "one.md")))
	if h.msg != "boom" || h.path != filepath.Join(dir, "README.md") {
		t.Errorf("load error: %q %q", h.msg, h.path)
	}
}

func TestMouse(t *testing.T) {
	dir := fixture(t)
	h := newHarness(t, dir)
	s := h.page.Links[0].Spans[0]
	h.handle(key{name: "mouse", btn: 0, press: true, x: s.Col + 1, y: s.Line - h.top})
	if h.path != filepath.Join(dir, "one.md") {
		t.Fatalf("click did not follow: %q", h.path)
	}
	h.keys("backspace")
	h.handle(key{name: "mouse", btn: 65})
	h.handle(key{name: "mouse", btn: 65})
	if h.top != 6 {
		t.Errorf("wheel down: %d", h.top)
	}
	h.handle(key{name: "mouse", btn: 64})
	if h.top != 3 {
		t.Errorf("wheel up: %d", h.top)
	}
	h.sel = 0
	h.handle(key{name: "mouse", btn: 0, press: true, x: 0, y: 0})
	if h.sel != -1 {
		t.Error("click on text kept the selection")
	}
	h.handle(key{name: "mouse", btn: 0, press: true, x: 0, y: h.rows - 1}) // status bar
	h.handle(key{name: "mouse", btn: 0, press: false})                     // release
}

func TestScrolling(t *testing.T) {
	h := newHarness(t, fixture(t))
	max := h.maxTop()
	for _, c := range []struct {
		keys []string
		top  int
	}{
		{[]string{"j", "down", "ctrl-n"}, 3},
		{[]string{"k", "up"}, 1},
		{[]string{" "}, 19},
		{[]string{"b"}, 1},
		{[]string{"d"}, 10},
		{[]string{"u", "u"}, 0},
		{[]string{"G"}, max},
		{[]string{"pgdn"}, max},
		{[]string{"home"}, 0},
		{[]string{"end", "g"}, 0},
		{[]string{"k"}, 0},
	} {
		h.keys(c.keys...)
		if h.top != c.top {
			t.Errorf("%v: top %d, want %d", c.keys, h.top, c.top)
		}
	}
	if p := h.percent(); p != "top" {
		t.Errorf("percent %q", p)
	}
	h.keys("j")
	if p := h.percent(); !strings.HasSuffix(p, "%") {
		t.Errorf("percent %q", p)
	}
	h.keys("G")
	if p := h.percent(); p != "end" {
		t.Errorf("percent %q", p)
	}
	h.keys("ctrl-l", "unknown")
	h.keys("q")
	if !h.quit {
		t.Error("q did not quit")
	}
	short := &model{page: &Page{Lines: []string{"a"}}, rows: 10}
	if short.percent() != "all" {
		t.Error("short page percent")
	}
}

func TestSearch(t *testing.T) {
	h := newHarness(t, fixture(t))
	h.keys("/", "n", "e", "x", "backspace", "e", "d", "l", "e")
	if s := h.screen(); !strings.Contains(s, "/needle") {
		t.Errorf("prompt not drawn: %q", stripEscapes(s))
	}
	h.keys("enter")
	if len(h.matches) != 2 || h.cur != 0 || h.top == 0 {
		t.Fatalf("case-insensitive search: %+v cur=%d top=%d", h.matches, h.cur, h.top)
	}
	if s := h.screen(); !strings.Contains(s, "<S>Needle") || !strings.Contains(s, "<M>needle") || !strings.Contains(s, "1/2") {
		t.Errorf("matches not drawn: %q", s)
	}
	h.keys("n")
	if h.cur != 1 {
		t.Error("n")
	}
	h.keys("n", "N", "N") // 1 → 0 → 1 → 0, wrapping both ways
	if h.cur != 0 {
		t.Errorf("N wrap: %d", h.cur)
	}
	h.keys("/", "N", "e", "e", "d", "l", "e", "enter") // uppercase: case-sensitive
	if len(h.matches) != 1 {
		t.Errorf("case-sensitive: %d", len(h.matches))
	}
	h.keys("/", "z", "z", "q", "enter")
	if h.msg != "not found: zzq" || h.quit {
		t.Errorf("not found: %q", h.msg)
	}
	h.keys("n")
	if h.msg != "not found: zzq" {
		t.Errorf("n without matches: %q", h.msg)
	}
	h.keys("/", "a", "ctrl-u", "b", "esc")
	if h.prompt || h.query != "zzq" {
		t.Error("esc in prompt")
	}
	h.keys("/", "backspace")
	if h.prompt {
		t.Error("backspace on empty prompt")
	}
	h.keys("/", "enter")
	if h.query != "" || h.matches != nil {
		t.Error("empty search")
	}
	h.handle(key{name: "char", r: '/'})
	h.handle(key{name: "char", r: '\x00'}) // not printable
	if len(h.input) != 0 {
		t.Error("control char in prompt")
	}
	h.keys("esc")
	// matches above the screen: wrap to the first
	h.keys("G", "/", "f", "i", "l", "l", "e", "r", "enter")
	if h.cur != 0 {
		t.Errorf("wrap to first match: %d", h.cur)
	}
	// reload keeps the search
	h.keys("r")
	if h.query != "filler" || len(h.matches) == 0 {
		t.Error("reload lost the search")
	}
}

func TestEditHelpReload(t *testing.T) {
	dir := fixture(t)
	h := newHarness(t, dir)
	h.keys("e")
	if !reflect.DeepEqual(h.edited, []string{filepath.Join(dir, "README.md")}) {
		t.Errorf("edited %v", h.edited)
	}
	h.opt.Edit = func(string) error { return errors.New("no vi") }
	h.keys("e")
	if h.msg != "no vi" {
		t.Errorf("edit error: %q", h.msg)
	}
	h.opt.Edit = nil
	h.keys("e")
	if h.msg != "no editor available" {
		t.Errorf("no editor: %q", h.msg)
	}
	h.path = ""
	h.keys("e")
	if !strings.HasPrefix(h.msg, "only files") {
		t.Errorf("stdin edit: %q", h.msg)
	}

	h.keys("?")
	if s := h.screen(); !strings.Contains(s, "kmd — keys") {
		t.Error("help not drawn")
	}
	h.keys("j") // any key closes help without acting
	if h.help || h.top != 0 {
		t.Error("help close")
	}
	h.keys("?", "q")
	if !h.quit {
		t.Error("q in help should quit")
	}

	// reload after a resize keeps the position in proportion
	h = newHarness(t, dir)
	h.keys("tab", "d")
	top, n := h.top, len(h.page.Lines)
	h.cols = 40
	h.reload()
	if len(h.page.Lines) <= n || h.top < top || h.sel != 0 {
		t.Errorf("reload: lines %d→%d top %d→%d sel %d", n, len(h.page.Lines), top, h.top, h.sel)
	}
	h.opt.Load = func(string, int) (*Page, error) { return nil, errors.New("gone") }
	h.reload()
	if h.msg != "gone" {
		t.Errorf("reload error: %q", h.msg)
	}
}

func TestHelpers(t *testing.T) {
	if pageName("") != "kmd" || pageName("/a/b/notes.md") != "notes.md" ||
		pageName("/a/etapa-5/README.md") != "etapa-5/README.md" || pageName("/x/docs/index.md") != "docs/index.md" {
		t.Error("pageName")
	}
	if clip("hello", 10) != "hello" || clip("hello world", 6) != "hello…" || clip("a\x1b[31mb", 10) != "a[31mb" {
		t.Errorf("clip: %q", clip("hello world", 6))
	}
	if got := errText(&os.PathError{Op: "open", Path: "/x/y.md", Err: os.ErrNotExist}); got != "file does not exist: y.md" {
		t.Errorf("errText %q", got)
	}
	m := &model{cols: 40, path: "/a/b/c/etapa-5/README.md"}
	for i := range 8 {
		m.hist = append(m.hist, entry{path: "/d/doc" + string(rune('a'+i)) + ".md"})
	}
	if b := m.breadcrumb(); !strings.HasPrefix(b, "… › ") || !strings.HasSuffix(b, "etapa-5/README.md") {
		t.Errorf("breadcrumb %q", b)
	}
	if m.target("#x", 40) != "#x" || m.target("https://a", 40) != "https://a" || m.target("file:///a/b/c/etapa-5/b.md#s", 40) != "b.md#s" {
		t.Errorf("target %q", m.target("file:///a/b/c/etapa-5/b.md#s", 40))
	}
	if m.rel("/elsewhere/very/far/x.md") != "/elsewhere/very/far/x.md" {
		t.Errorf("rel far: %q", m.rel("/elsewhere/very/far/x.md"))
	}
}

func TestPoll(t *testing.T) {
	dir := fixture(t)
	h := newHarness(t, dir)
	readme := filepath.Join(dir, "README.md")
	if h.poll() {
		t.Error("first poll only records the file")
	}
	if h.poll() {
		t.Error("unchanged file reloaded")
	}
	h.keys("d")
	top := h.top
	src, _ := os.ReadFile(readme)
	if err := os.WriteFile(readme, append(src, []byte("\nappended paragraph\n")...), 0o600); err != nil {
		t.Fatal(err)
	}
	if !h.poll() || h.top != top || !strings.Contains(stripEscapes(strings.Join(h.page.Lines, "\n")), "appended paragraph") {
		t.Errorf("change not reloaded at the same position: top %d→%d", top, h.top)
	}
	if err := os.Remove(readme); err != nil {
		t.Fatal(err)
	}
	if h.poll() {
		t.Error("a missing file (editor replacing it) must keep the page")
	}
	h.path = ""
	if h.poll() {
		t.Error("stdin is not watched")
	}
	// navigating watches the new page
	h = newHarness(t, fixture(t))
	h.poll()
	h.selectLink(t, "/one.md")
	h.keys("enter")
	if h.poll() || h.watched != h.path {
		t.Error("new page not watched")
	}
}

func TestFollowHardening(t *testing.T) {
	dir := fixture(t)
	secret := filepath.Join(t.TempDir(), "id_rsa")
	if err := os.WriteFile(secret, []byte("KEY"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "other.markdown"), []byte("# other"), 0o600); err != nil {
		t.Fatal(err)
	}
	links := map[string]string{
		"alias2.md": filepath.Join(dir, "other.markdown"), // Markdown to Markdown: fine
		"notes.md":  secret,                               // symlink to a non-Markdown file
		"alias.md":  filepath.Join(dir, "one.md"),         // symlink to Markdown: fine
		"doc.pdf":   filepath.Join(dir, "run.sh"),         // a "PDF" that is a script
		"pic2.png":  filepath.Join(dir, "README.md"),      // extension mismatch
		"loop.md":   filepath.Join(dir, "loop.md"),        // broken (self) link
	}
	for name, target := range links {
		if err := os.Symlink(target, filepath.Join(dir, name)); err != nil {
			t.Skip("no symlinks:", err)
		}
	}
	if err := syscallMkfifo(filepath.Join(dir, "pipe.md")); err != nil {
		t.Skip("no FIFOs:", err)
	}
	h := newHarness(t, dir)
	file := func(n string) string { return "file://" + filepath.ToSlash(filepath.Join(dir, n)) }
	for name, want := range map[string]string{
		"notes.md": "link points elsewhere: notes.md → ",
		"doc.pdf":  "link points elsewhere: doc.pdf → ",
		"pic2.png": "link points elsewhere: pic2.png → ",
		"pipe.md":  "not a regular file: pipe.md",
		"loop.md":  "not found: loop.md",
	} {
		h.follow(file(name))
		if !strings.HasPrefix(h.msg, want) {
			t.Errorf("%s: %q, want prefix %q", name, h.msg, want)
		}
	}
	if len(h.opened) != 0 {
		t.Errorf("handed to the desktop: %v", h.opened)
	}
	h.follow(file("alias2.md"))
	if h.path != filepath.Join(dir, "alias2.md") {
		t.Errorf("symlink to another Markdown extension not opened: %q", h.msg)
	}
	h.follow(file("alias.md"))
	if h.path != filepath.Join(dir, "alias.md") {
		t.Errorf("symlinked Markdown not opened: %q", h.msg)
	}

	h.follow("mailto:a@b.c?subject=Hi&attach=/home/u/.ssh/id_rsa&body=x")
	if len(h.opened) != 1 || h.opened[0] != "mailto:a@b.c?body=x&subject=Hi" {
		t.Errorf("mailto: %v", h.opened)
	}
	h.follow("https://exa\u200bmple.com")
	if h.msg != "invalid link" {
		t.Errorf("invisible characters: %q", h.msg)
	}
	// a user name poses as the host, also where Go sees no host but a
	// browser reads one (#72 SR-18); a file:// host is not this machine
	for href, want := range map[string]string{
		"https://github.com%2Flogin%2Foauth%2Fauthorize%3Fclient_id%3D0123456789abcdef%26scope%3Drepo@evil.example/x": "refused: hidden or missing host: https://…@evil.example",
		"https:github.com%2Flogin%2Foauth@evil.example/x":                                                             "refused: hidden or missing host: https:github.com",
		"https:/github.com%2Flogin%2Foauth@evil.example/x":                                                            "refused: hidden or missing host: https:/github.com",
		"https:///github.com%2Flogin@evil.example/x":                                                                  "refused: hidden or missing host: https:///github.com",
		"file://attacker.example/share/x.md":                                                                          "not a local file",
	} {
		h.follow(href)
		if !strings.HasPrefix(h.msg, want) {
			t.Errorf("%s: %q, want prefix %q", href, h.msg, want)
		}
	}
	if len(h.opened) != 1 || h.path != filepath.Join(dir, "alias.md") {
		t.Errorf("followed: %v, %q", h.opened, h.path)
	}
}

// The status bar shows the scheme and the whole host of a selected web
// link, whatever the width: a long path or user name must not push the
// real host out of view (SR-18).
func TestWebTarget(t *testing.T) {
	m := &model{}
	long := "https://docs.github.com." + strings.Repeat("a", 200) + ".evil.example/"
	for _, c := range []struct {
		href string
		n    int
		want string
	}{
		{"https://example.com/a/b?c=d#e", 40, "https://example.com/a/b?c=d#e"},
		{"https://example.com/" + strings.Repeat("p", 100), 30, "https://example.com/" + strings.Repeat("p", 9) + "…"},
		{"https://github.com%2Flogin%2Foauth%2Fauthorize%3Fclient_id%3D0123456789abcdef%26scope%3Drepo@evil.example/x", 72, "https://…@evil.example/x"},
		{long, 40, "https://…" + strings.Repeat("a", 18) + ".evil.example"},
		{long, 120, "https://…" + strings.Repeat("a", 98) + ".evil.example"},
		{"https://example.com/", 5, "http…"},
		{"https:github.com%2Flogin@evil.example/x", 60, "invalid web link: https:github.com%2Flogin@evil.example/x"},
		{"https:///github.com@evil.example/x", 30, "invalid web link: https:///gi…"},
	} {
		if got := m.target(c.href, c.n); got != c.want || styled.Width(got) > max(c.n, 5) {
			t.Errorf("target(%.40q, %d) = %q, want %q", c.href, c.n, got, c.want)
		}
	}
	// the status bar gives it the room the hints leave
	h := newHarness(t, fixture(t))
	h.cols = 80
	h.page.Links = append(h.page.Links, render.Link{Href: long, Spans: []render.Span{{Line: h.top, Width: 1}}})
	h.sel = len(h.page.Links) - 1
	var b strings.Builder
	h.status(&b)
	if !strings.Contains(b.String(), ".evil.example ") {
		t.Errorf("status bar hides the host: %q", b.String())
	}
}

func TestHomeAndBackHint(t *testing.T) {
	dir := fixture(t)
	h := newHarness(t, dir)
	if s := stripEscapes(h.screen()); strings.Contains(s, "⌫ back") {
		t.Error("back hint without history")
	}
	h.keys("H")
	if h.msg != "no previous document" {
		t.Errorf("H without history: %q", h.msg)
	}
	h.selectLink(t, "/one.md")
	top := h.top
	h.keys("enter") // README → one.md
	if s := stripEscapes(h.screen()); !strings.Contains(s, "⌫ back · Tab links") {
		t.Errorf("back hint missing: %q", s)
	}
	h.selectLink(t, "README.md#mapa")
	h.keys("enter") // → README#mapa
	h.selectLink(t, "/sub/two.md#deep")
	h.keys("enter") // → two.md
	if len(h.hist) != 3 {
		t.Fatalf("history %d", len(h.hist))
	}
	h.keys("H")
	if h.path != filepath.Join(dir, "README.md") || len(h.hist) != 0 || h.top != top || !strings.HasSuffix(h.selHref(), "/one.md") {
		t.Errorf("H: path=%q hist=%d top=%d/%d sel=%q", h.path, len(h.hist), h.top, top, h.selHref())
	}
}

func TestClipLongAndExternalClicks(t *testing.T) {
	long := strings.Repeat("a", 1<<20)
	start := time.Now()
	if got := clip(long, 40); styled.Width(got) != 40 || !strings.HasSuffix(got, "…") {
		t.Errorf("clip width %d", styled.Width(got))
	}
	// an emoji made wide by U+FE0F counts as two columns
	if got := clip(strings.Repeat("❤\ufe0f", 40), 20); styled.Width(got) > 20 {
		t.Errorf("emoji: %d columns", styled.Width(got))
	}
	if d := time.Since(start); d > testlimit.Scale()*time.Second {
		t.Errorf("clip of 1 MiB took %v", d)
	}
	testlimit.Linear(t, "clip", 1<<20, func(n int) func() {
		s := strings.Repeat("日a", n)
		return func() { _ = clip(s, n) }
	})
	if clip("12345", 5) != "12345" || clip("123456", 5) != "1234…" || clip("日本語", 6) != "日本語" || clip("日本語x", 6) != "日本…" {
		t.Errorf("clip edges: %q %q", clip("123456", 5), clip("日本語x", 6))
	}

	dir := fixture(t)
	h := newHarness(t, dir)
	h.selectLink(t, "https://example.com")
	s := h.page.Links[h.sel].Spans[0]
	h.keys("esc")
	click := func() { h.handle(key{name: "mouse", btn: 0, press: true, x: s.Col, y: s.Line - h.top}) }
	click()
	if len(h.opened) != 0 || !strings.HasPrefix(h.selHref(), "https://example.com") {
		t.Fatalf("first click on a web link must only select it: %v", h.opened)
	}
	if st := stripEscapes(h.screen()); !strings.Contains(st, "→ https://example.com") {
		t.Error("target not shown after the first click")
	}
	click()
	if len(h.opened) != 1 {
		t.Errorf("second click did not open: %v", h.opened)
	}
}

func TestFirstPage(t *testing.T) {
	first := &Page{Lines: []string{"prerendered"}}
	loads := 0
	opt := Options{First: first, FirstCols: 80, Load: func(string, int) (*Page, error) {
		loads++
		return &Page{Lines: []string{"loaded"}}, nil
	}}
	if m, _ := newModel(opt, 80, 20); m.page != first || loads != 0 {
		t.Error("first page not used")
	}
	if m, _ := newModel(opt, 100, 20); m.page == first || loads != 1 {
		t.Error("first page used at another width")
	}
}

func TestFollowFolderWithoutReadme(t *testing.T) {
	dir := fixture(t)
	h := newHarness(t, dir)
	base := h.opt.Load
	var loaded []string
	h.opt.Load = func(path string, cols int) (*Page, error) {
		loaded = append(loaded, path)
		if st, err := os.Stat(path); err == nil && st.IsDir() {
			return &Page{Lines: []string{"listing of " + path}}, nil
		}
		return base(path, cols)
	}
	h.selectLink(t, "/empty")
	h.keys("enter")
	empty := filepath.Join(dir, "empty")
	if h.path != empty || len(h.hist) != 1 || h.page.Lines[0] != "listing of "+empty {
		t.Fatalf("folder listing: path %q msg %q", h.path, h.msg)
	}
	if !h.picking || h.pick.root != empty {
		t.Fatalf("picker not opened on the folder: %+v", h.pick)
	}
	h.keys("esc")                                 // the plain listing stays
	h.follow("file://" + filepath.ToSlash(empty)) // already there: no reload
	if len(loaded) != 1 {
		t.Errorf("reloaded the listing: %v", loaded)
	}
	h.keys("esc", "e")
	if h.msg != "a folder listing cannot be edited" || len(h.edited) != 0 {
		t.Errorf("edit a folder: %q %v", h.msg, h.edited)
	}
	h.keys("backspace", "esc", "g")
	h.opt.Load = func(string, int) (*Page, error) { return nil, errors.New("unreadable") }
	h.selectLink(t, "/empty")
	h.keys("enter")
	if h.msg != "unreadable" {
		t.Errorf("listing error: %q", h.msg)
	}
}
