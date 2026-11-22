package view

import (
	"github.com/DipaolaFederico/kmd/internal/render"
	"strings"
	"testing"
)

func outlinePage() *Page {
	lines := make([]string, 200)
	for i := range lines {
		lines[i] = "text"
	}
	return &Page{Lines: lines, Headings: []render.Heading{
		{Level: 1, Text: "Title", Line: 1},
		{Level: 2, Text: "Install", Line: 40},
		{Level: 3, Text: "Fedora\x1b[2J", Line: 60},
		{Level: 2, Text: "Usage", Line: 120},
		{Level: 2, Text: "End", Line: 195},
	}}
}

func TestOutline(t *testing.T) {
	h := &harness{model: &model{page: outlinePage(), rows: 12, cols: 60, sel: -1, opt: Options{Select: "<S>", Bar: "<B>"}}}
	h.keys("]")
	if h.top != 40 {
		t.Fatalf("] to Install: top %d", h.top)
	}
	h.keys("]", "]")
	if h.top != 120 {
		t.Fatalf("] ] to Usage: top %d", h.top)
	}
	h.keys("]") // End is on the last screen: scrolls to the bottom
	if h.top != h.maxTop() {
		t.Fatalf("] to End: top %d", h.top)
	}
	h.keys("]")
	if h.msg != "last section" {
		t.Errorf("past the last section: %q", h.msg)
	}
	h.keys("[")
	if h.top != 120 {
		t.Errorf("[ back to Usage: %d", h.top)
	}
	h.keys("g", "[")
	if h.msg != "first section" {
		t.Errorf("before the first section: %q", h.msg)
	}

	h.top = 70 // reading the Fedora section
	h.keys("t")
	if !h.outline || h.ocur != 2 {
		t.Fatalf("outline opens at the current section: %v %d", h.outline, h.ocur)
	}
	s := h.screen()
	if !strings.Contains(s, "Contents 3/5") || !strings.Contains(s, "<S>    Fedora[2J") || strings.Contains(s, "\x1b[2J") {
		t.Errorf("outline not drawn (or not cleaned): %q", s)
	}
	if !strings.Contains(s, "Enter jump") {
		t.Error("no outline hint in the status bar")
	}
	h.keys("k", "k", "k", "j") // clamps at the first heading
	if h.ocur != 1 {
		t.Errorf("cursor: %d", h.ocur)
	}
	h.keys("enter")
	if h.outline || h.top != 40 || len(h.hist) != 1 {
		t.Errorf("jump: outline=%v top=%d hist=%d", h.outline, h.top, len(h.hist))
	}
	h.keys("backspace")
	if h.top != 70 {
		t.Errorf("back after the jump: %d", h.top)
	}
	h.keys("t", "G")
	if h.ocur != 4 {
		t.Errorf("G: %d", h.ocur)
	}
	h.keys("g", " ", "b", "tab", "btab")
	if h.ocur != 0 {
		t.Errorf("paging: %d", h.ocur)
	}
	h.keys("esc")
	if h.outline {
		t.Error("esc does not close")
	}
	// mouse: wheel moves, a click on a heading jumps, elsewhere closes
	h.keys("t")
	h.handle(key{name: "mouse", btn: 65})
	h.handle(key{name: "mouse", btn: 64})
	h.handle(key{name: "mouse", btn: 65})
	if h.ocur != 3 {
		t.Errorf("wheel: %d", h.ocur)
	}
	top, left, _ := h.outlineBox()
	h.handle(key{name: "mouse", btn: 0, press: true, x: left + 2, y: top + 2}) // second row: Install
	if h.outline || h.top != 40 {
		t.Errorf("click: outline=%v top=%d", h.outline, h.top)
	}
	h.keys("t")
	h.handle(key{name: "mouse", btn: 0, press: true, x: 0, y: 0})
	if h.outline {
		t.Error("a click outside does not close")
	}

	// a small screen scrolls the list with the cursor
	small := &harness{model: &model{page: outlinePage(), rows: 5, cols: 40, sel: -1}}
	small.keys("t", "G")
	if s := small.screen(); small.otop == 0 || !strings.Contains(s, "End") {
		t.Errorf("outline does not scroll: otop=%d", small.otop)
	}

	none := &harness{model: &model{page: &Page{Lines: []string{"a"}}, rows: 10, cols: 40, sel: -1}}
	for _, k := range []string{"t", "]", "["} {
		none.keys(k)
		if none.msg != "no headings in this document" || none.outline {
			t.Errorf("%s without headings: %q", k, none.msg)
		}
	}
}
