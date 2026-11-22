package view

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DipaolaFederico/kmd/internal/render"
)

func TestSlides(t *testing.T) {
	calls := 0
	deck := func(cols, rows int) ([]*Page, error) {
		calls++
		tall := make([]string, 50)
		return []*Page{
			{Lines: []string{"one", "[web]", "[doc]"}, Links: []render.Link{
				{Href: "https://example.com", Spans: []render.Span{{Line: 1, Width: 5}}},
				{Href: "file:///tmp/other.md", Spans: []render.Span{{Line: 2, Width: 5}}},
			}},
			{Lines: tall},
			{Lines: []string{"three"}},
		}, nil
	}
	var opened []string
	m, err := newModel(Options{Slides: deck, Start: "deck.md", Open: func(s string) error { opened = append(opened, s); return nil }}, 60, 10)
	if err != nil {
		t.Fatal(err)
	}
	h := &harness{model: m}
	if h.slide != 0 || h.page.Lines[0] != "one" || !strings.Contains(h.screen(), "slide 1/3") {
		t.Fatalf("first slide: %d", h.slide)
	}
	h.keys("left")
	if h.msg != "first slide" {
		t.Errorf("before the first: %q", h.msg)
	}
	h.keys("tab", "enter") // a web link opens; Enter does not change slide
	if len(opened) != 1 || h.slide != 0 {
		t.Errorf("web link: %v slide %d", opened, h.slide)
	}
	h.keys("tab", "enter")
	if h.msg != "documents do not open during a presentation" || h.slide != 0 {
		t.Errorf("document link: %q", h.msg)
	}
	h.keys("esc", "enter") // nothing selected: next slide
	if h.slide != 1 {
		t.Fatalf("enter: slide %d", h.slide)
	}
	h.keys("j", "j") // a tall slide scrolls
	if h.top != 2 || h.slide != 1 {
		t.Errorf("scroll in a slide: top %d", h.top)
	}
	h.keys("right", "right")
	if h.slide != 2 || h.msg != "last slide" {
		t.Errorf("last: %d %q", h.slide, h.msg)
	}
	h.keys("g")
	if h.slide != 0 {
		t.Errorf("g: %d", h.slide)
	}
	h.keys("G", "backspace", "p", "H")
	if h.slide != 0 {
		t.Errorf("back keys: %d", h.slide)
	}
	h.keys("n", " ", "pgup")
	if h.slide != 1 {
		t.Errorf("n Space PgUp: %d", h.slide)
	}
	// reload (resize, file changed) keeps the slide and the position
	h.top = 3
	h.reload()
	if calls != 2 || h.slide != 1 || h.top != 3 {
		t.Errorf("reload: calls %d slide %d top %d", calls, h.slide, h.top)
	}
	h.opt.Slides = func(int, int) ([]*Page, error) { return nil, errors.New("gone") }
	h.reload()
	if h.msg != "gone" || h.slide != 1 {
		t.Errorf("reload error: %q", h.msg)
	}
	h.opt.Slides = func(int, int) ([]*Page, error) { return nil, nil }
	h.reload()
	if len(h.slides) != 1 || h.slide != 0 {
		t.Errorf("no slides: %d", len(h.slides))
	}
	if _, err := newModel(Options{Slides: func(int, int) ([]*Page, error) { return nil, errors.New("x") }}, 60, 10); err == nil {
		t.Error("first render error not reported")
	}
}

func TestSlideLinks(t *testing.T) {
	m := &model{slides: []*Page{{}}}
	for href, ok := range map[string]bool{
		"#anchor": true, "https://x.y": true, "mailto:a@b.c": true, "file:///a/pic.png": true,
		"file:///a/doc.md": false, "file:///a/folder": false, "%zz": true,
	} {
		if got := m.slideLink(href); got != ok {
			t.Errorf("%s: %v", href, got)
		}
	}
}

func TestSlidesRefuseFolders(t *testing.T) {
	dir := t.TempDir()
	for _, d := range []string{"assets.d", "pics.png"} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "pics.png", "README.md"), []byte("# x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m, err := newModel(Options{Slides: func(int, int) ([]*Page, error) { return []*Page{{Lines: []string{"a"}}}, nil }}, 60, 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"assets.d", "pics.png"} {
		m.follow("file://" + filepath.ToSlash(filepath.Join(dir, d)))
		if m.msg != "documents do not open during a presentation" {
			t.Errorf("%s: %q", d, m.msg)
		}
	}
	if err := m.load(filepath.Join(dir, "x.md")); err == nil {
		t.Error("load without a loader")
	}
}
