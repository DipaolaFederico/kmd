package render

import "testing"

func TestSameKind(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"notes.md", "/x/notes.md", true},
		{"notes.md", "notes.Markdown", true},
		{"doc.PDF", "doc.pdf", true},
		{"notes.md", "id_rsa", false},
		{"notes.md", "notes.txt", false},
		{"doc.pdf", "run.sh", false},
		{"pic.png", "README.md", false},
	} {
		if got := SameKind(c.a, c.b); got != c.want {
			t.Errorf("SameKind(%q, %q) = %v", c.a, c.b, got)
		}
	}
}
