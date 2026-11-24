package cli

import (
	"testing"

	"github.com/DipaolaFederico/kmd/internal/md"
)

func TestNotesPage(t *testing.T) {
	slides := md.Slides(md.Parse("intro\n\n---\n\n<!-- notes: *n* -->\n\n## Title `x`\n", md.GFM), 10)
	doc := notesPage(slides, 0, true, md.GFM)
	if s := md.PlainText(doc.LastChild); s != "Next: Title x" {
		t.Errorf("next: %q", s)
	}
	if s := md.PlainText(notesPage(slides, 5, true, md.GFM).FirstChild); s != "Slide 2 / 2" {
		t.Errorf("clamped: %q", s)
	}
	if s := slideTitle(md.Slides(md.Parse("x\n", md.GFM), 1)[0], 4); s != "slide 4" {
		t.Errorf("untitled: %q", s)
	}
}
