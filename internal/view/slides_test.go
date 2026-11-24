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
	deck := func(cols, rows int) (*Deck, error) {
		calls++
		tall := make([]string, 50)
		return &Deck{Slides: []*Page{
			{Lines: []string{"one", "[web]", "[doc]"}, Links: []render.Link{
				{Href: "https://example.com", Spans: []render.Span{{Line: 1, Width: 5}}},
				{Href: "file:///tmp/other.md", Spans: []render.Span{{Line: 2, Width: 5}}},
			}},
			{Lines: tall},
			{Lines: []string{"three"}},
		}}, nil
	}
	var opened []string
	m, err := newModel(Options{Slides: deck, Start: "deck.md", Open: func(s string) error { opened = append(opened, s); return nil }}, 60, 10)
	if err != nil {
		t.Fatal(err)
	}
	h := &harness{model: m}
	if h.slide != 0 || h.page.Lines[0] != "one" || !strings.Contains(h.screen(), "1 / 3") {
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
	h.opt.Slides = func(int, int) (*Deck, error) { return nil, errors.New("gone") }
	h.reload()
	if h.msg != "gone" || h.slide != 1 {
		t.Errorf("reload error: %q", h.msg)
	}
	h.opt.Slides = func(int, int) (*Deck, error) { return nil, nil }
	h.reload()
	if len(h.slides) != 1 || h.slide != 0 {
		t.Errorf("no slides: %d", len(h.slides))
	}
	if _, err := newModel(Options{Slides: func(int, int) (*Deck, error) { return nil, errors.New("x") }}, 60, 10); err == nil {
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
	m, err := newModel(Options{Slides: func(int, int) (*Deck, error) { return &Deck{Slides: []*Page{{Lines: []string{"a"}}}}, nil }}, 60, 10)
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

func TestSlideSteps(t *testing.T) {
	tall := make([]string, 30)
	tall[29] = "end"
	deck := func(cols, rows int) (*Deck, error) {
		return &Deck{Slides: []*Page{
			{Lines: []string{"", "title", "", "[a]", "", "[b]"}, Steps: []int{2, 4},
				Links:    []render.Link{{Href: "#a", Spans: []render.Span{{Line: 3, Width: 3}}}, {Href: "#b", Spans: []render.Span{{Line: 5, Width: 3}}}},
				Anchors:  map[string]int{"title": 1, "b": 5},
				Headings: []render.Heading{{Text: "title", Line: 1}, {Text: "b", Line: 5}},
				Code:     []render.CodeBlock{{Start: 3, End: 3}, {Start: 5, End: 5}}},
			{Lines: tall, Steps: []int{2}},
		}, Footer: "Talk · Ana"}, nil
	}
	m, err := newModel(Options{Slides: deck}, 60, 10)
	if err != nil {
		t.Fatal(err)
	}
	h := &harness{model: m}
	cut := func(lines, links, anchors, headings, code int) bool {
		return len(h.page.Lines) == lines && len(h.page.Links) == links && len(h.page.Anchors) == anchors &&
			len(h.page.Headings) == headings && len(h.page.Code) == code
	}
	if !cut(2, 0, 1, 1, 0) {
		t.Fatalf("first step: %q", h.page.Lines)
	}
	h.keys("right")
	if h.step != 1 || !cut(4, 1, 1, 1, 1) {
		t.Errorf("second step: %d %q", h.step, h.page.Lines)
	}
	h.keys(" ")
	if h.step != 2 || !cut(6, 2, 2, 2, 2) {
		t.Errorf("last step: %d", h.step)
	}
	h.keys("right") // next slide, at its first step
	if h.slide != 1 || h.step != 0 || len(h.page.Lines) != 2 {
		t.Errorf("next slide: %d/%d", h.slide, h.step)
	}
	h.keys("enter") // nothing selected: the next step, brought into view
	if h.step != 1 || h.top == 0 || h.top > 2 {
		t.Errorf("reveal: step %d top %d", h.step, h.top)
	}
	h.keys("left") // back a step keeps the position, clamped
	if h.step != 0 || h.top != 0 {
		t.Errorf("back a step: %d top %d", h.step, h.top)
	}
	h.keys("left") // back to the previous slide, whole
	if h.slide != 0 || h.step != 2 {
		t.Errorf("previous slide: %d/%d", h.slide, h.step)
	}
	h.keys("G")
	if h.slide != 1 || h.step != 1 {
		t.Errorf("G: %d/%d", h.slide, h.step)
	}
	h.keys("right")
	if h.msg != "last slide" {
		t.Errorf("past the end: %q", h.msg)
	}
	h.keys("g")
	if h.slide != 0 || h.step != 0 {
		t.Errorf("g: %d/%d", h.slide, h.step)
	}
	h.keys("left")
	if h.msg != "first slide" {
		t.Errorf("before the first: %q", h.msg)
	}
	// g on the slide on screen goes back to its top
	h.keys("G")
	h.top = 5
	h.keys("G")
	if h.top != 0 {
		t.Errorf("G on the last slide keeps top %d", h.top)
	}
	// a reload (resize, edit) keeps the slide and the step
	h.keys("g", "right")
	h.reload()
	if h.slide != 0 || h.step != 1 || len(h.page.Lines) != 4 {
		t.Errorf("reload: %d/%d", h.slide, h.step)
	}
}

func TestSlideFooter(t *testing.T) {
	deck := &Deck{Slides: []*Page{{Lines: make([]string, 30)}, {Lines: []string{"b"}}}, Footer: "Talk · Ana"}
	m, err := newModel(Options{Slides: func(int, int) (*Deck, error) { return deck, nil }}, 60, 10)
	if err != nil {
		t.Fatal(err)
	}
	h := &harness{model: m}
	// for the audience: no key hints; the scroll position of a tall slide
	if b := h.bottom(); !strings.HasPrefix(b, "Talk · Ana") || !strings.HasSuffix(b, "1 / 2  top") || strings.Contains(b, "quit") {
		t.Errorf("footer: %q", b)
	}
	h.keys("right")
	if b := h.bottom(); !strings.HasSuffix(b, " 2 / 2") {
		t.Errorf("short slide: %q", b)
	}
	h.keys("right") // messages still show there
	if b := h.bottom(); b != "last slide" {
		t.Errorf("message: %q", b)
	}
	deck.NoFooter = true
	h.reload()
	h.msg = ""
	if b := h.bottom(); b != "" {
		t.Errorf("hidden footer: %q", b)
	}
	h.keys("/") // a prompt still shows
	if b := h.bottom(); b != "/" {
		t.Errorf("prompt without footer: %q", b)
	}
}
