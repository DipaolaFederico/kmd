package render

import (
	"slices"
	"strings"
	"testing"

	"github.com/DipaolaFederico/kmd/internal/md"
)

func TestPauses(t *testing.T) {
	src := "# Title\n\n<!-- pause -->\n\nfirst\n\n<!-- pause -->\n<!-- pause -->\n\n- a\n<!-- pause -->\n- b\n\n<!-- pause -->\n"
	res := Render(md.Parse(src, md.GFM), Options{Width: 40, Pauses: true})
	lines := strings.Split(res.Text, "\n")
	// a step ends before the blank line that separates the next block; two
	// pauses in a row make one step and a trailing one makes none
	if len(res.Pauses) != 3 {
		t.Fatalf("pauses: %v\n%s", res.Pauses, res.Text)
	}
	at := func(i int) string { return strings.TrimSpace(lines[res.Pauses[i]-1]) }
	if !strings.Contains(at(0), "═") || at(1) != "first" || at(2) != "• a" {
		t.Errorf("steps end at %q %q %q\n%s", at(0), at(1), at(2), res.Text)
	}
	// the list continued after the pause is not set apart
	if next := strings.TrimSpace(lines[res.Pauses[2]]); next != "• b" {
		t.Errorf("after the list pause: %q", next)
	}
	if !slices.IsSorted(res.Pauses) {
		t.Errorf("not increasing: %v", res.Pauses)
	}

	// without Pauses the comments are just hidden and nothing changes
	plain := Render(md.Parse(src, md.GFM), Options{Width: 40})
	if plain.Pauses != nil || !strings.Contains(plain.Text, "• a\n\n  • b") {
		t.Errorf("without Pauses: %v\n%s", plain.Pauses, plain.Text)
	}
	// lists that differ (loose, another marker) keep their blank line
	for _, src := range []string{"- a\n\n- b\n<!-- pause -->\n- c\n", "- a\n<!-- pause -->\n* b\n", "1. a\n<!-- pause -->\n- b\n"} {
		res := Render(md.Parse(src, md.GFM), Options{Width: 40, Pauses: true})
		lines := strings.Split(res.Text, "\n")
		if len(res.Pauses) != 1 || lines[res.Pauses[0]] != "" {
			t.Errorf("%q: %v\n%s", src, res.Pauses, res.Text)
		}
	}
	// an ordered list goes on with its own numbers
	ordered := Render(md.Parse("1. a\n<!-- pause -->\n2. b\n", md.GFM), Options{Width: 40, Pauses: true})
	if !strings.Contains(ordered.Text, "1. a\n  2. b") {
		t.Errorf("ordered list:\n%s", ordered.Text)
	}
	// a pause first: the slide starts empty
	if res := Render(md.Parse("<!-- pause -->\n\nx\n", md.GFM), Options{Width: 40, Pauses: true}); !slices.Equal(res.Pauses, []int{1}) {
		t.Errorf("leading pause: %v", res.Pauses)
	}
}
