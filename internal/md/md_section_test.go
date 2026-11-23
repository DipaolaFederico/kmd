package md

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestSection(t *testing.T) {
	src := "# Title\n\nintro\n\n## Install\n\nstep[^a]\n\n### Fedora\n\ndnf\n\n## Usage\n\nrun[^b]\n\n[^a]: note a\n[^b]: note b\n"
	byText := func(s string) func(*Node) bool {
		return func(h *Node) bool { return PlainText(h) == s }
	}
	doc := Parse(src, GFM)
	sec := Section(doc, byText("Install"))
	if sec == nil {
		t.Fatal("no section")
	}
	got := HTML(sec, HTMLOptions{})
	for _, want := range []string{"<h2>Install</h2>", "step", "<h3>Fedora</h3>", "dnf", "note a"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
	for _, bad := range []string{"intro", "Usage", "run", "note b"} {
		if strings.Contains(got, bad) {
			t.Errorf("unexpected %q in %s", bad, got)
		}
	}
	// a level-1 section runs to the end; the last section too
	if s := HTML(Section(Parse(src, GFM), byText("Title")), HTMLOptions{}); !strings.Contains(s, "Usage") {
		t.Errorf("whole document expected: %s", s)
	}
	if s := HTML(Section(Parse(src, GFM), byText("Usage")), HTMLOptions{}); !strings.Contains(s, "run") || strings.Contains(s, "step") {
		t.Errorf("last section: %s", s)
	}
	doc = Parse(src, GFM)
	if Section(doc, byText("Nope")) != nil || !strings.Contains(HTML(doc, HTMLOptions{}), "intro") {
		t.Error("a missing section must leave the document untouched")
	}
}

func TestSlides(t *testing.T) {
	texts := func(slides []*Node) []string {
		var out []string
		for _, s := range slides {
			out = append(out, strings.TrimSpace(HTML(s, HTMLOptions{})))
		}
		return out
	}
	// thematic breaks win; front matter is dropped; empty slides vanish
	got := texts(Slides(Parse("---\ntitle: x\n---\n# One\n\na\n\n---\n\n## Two\n\nb[^n]\n\n---\n\n---\n\nc\n\n[^n]: note\n", GFM), 100))
	if len(got) != 3 || !strings.Contains(got[0], "One") || strings.Contains(got[0], "title") ||
		!strings.Contains(got[1], "Two") || !strings.Contains(got[1], "note") || got[2] != "<p>c</p>" {
		t.Errorf("breaks: %q", got)
	}
	// otherwise level 1 and 2 headings start slides, level 3 does not
	got = texts(Slides(Parse("intro\n\n# A\n\na\n\n### A.1\n\nx\n\n## B\n\nb\n", GFM), 100))
	if len(got) != 3 || got[0] != "<p>intro</p>" || !strings.Contains(got[1], "A.1") || !strings.Contains(got[2], "B") {
		t.Errorf("headings: %q", got)
	}
	if s := Slides(Parse("", GFM), 100); len(s) != 1 || s[0].FirstChild != nil {
		t.Errorf("empty document: %d slides", len(s))
	}
}

func TestSlidesLimits(t *testing.T) {
	var b strings.Builder
	for i := range 5 {
		b.WriteString("s" + string(rune('a'+i)) + "\n\n---\n\n")
	}
	got := Slides(Parse(b.String(), GFM), 3)
	if len(got) != 3 || !strings.Contains(HTML(got[2], HTMLOptions{}), "se") {
		t.Errorf("max: %d slides", len(got))
	}
	// many slides and footnotes stay linear
	b.Reset()
	const n = 20000
	for i := range n {
		b.WriteString("x[^" + strconv.Itoa(i) + "]\n\n---\n\n")
	}
	for i := range n {
		b.WriteString("[^" + strconv.Itoa(i) + "]: d\n")
	}
	doc := Parse(b.String(), GFM)
	start := time.Now()
	got = Slides(doc, n+1)
	if d := time.Since(start); d > raceFactor()*time.Second {
		t.Errorf("%d slides with footnotes took %v", n, d)
	}
	if len(got) != n || !strings.Contains(HTML(got[n-1], HTMLOptions{}), "d") {
		t.Errorf("footnotes: %d slides", len(got))
	}
}
