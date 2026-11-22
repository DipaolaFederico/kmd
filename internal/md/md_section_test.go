package md

import (
	"strings"
	"testing"
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
