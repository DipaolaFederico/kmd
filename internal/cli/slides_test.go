package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DipaolaFederico/kmd/internal/render"
	"github.com/DipaolaFederico/kmd/internal/termimg"
	"github.com/DipaolaFederico/kmd/internal/view"
)

func TestSlideSettings(t *testing.T) {
	s := settings{}
	s.opts.Images, s.opts.ImageRows = termimg.Kitty, 40
	vs := slideSettings(s, 100, 24)
	if vs.opts.Images != termimg.Kitty || !vs.opts.Placements || vs.opts.ImageRows != 22 {
		t.Errorf("kitty: %v placements %v rows %d", vs.opts.Images, vs.opts.Placements, vs.opts.ImageRows)
	}
	s.opts.Images, s.opts.ImageRows = termimg.Sixel, 0
	if vs := slideSettings(s, 100, 24); vs.opts.Images != termimg.Blocks || vs.opts.ImageRows != render.DefaultImageRows {
		t.Errorf("sixel without a cell size: %v rows %d", vs.opts.Images, vs.opts.ImageRows)
	}
	s.opts.Cell = termimg.Cell{W: 8, H: 16}
	if vs := slideSettings(s, 100, 24); vs.opts.Images != termimg.Sixel {
		t.Errorf("sixel: %v", vs.opts.Images)
	}
}

func TestSlideImagesCentered(t *testing.T) {
	deck := writeFile(t, "deck.md", "# Pictures\n\n![chart](gradient.png)\n")
	if err := os.WriteFile(filepath.Join(filepath.Dir(deck), "gradient.png"), gradientPNG(t), 0o600); err != nil {
		t.Fatal(err)
	}
	var got view.Options
	run(t, func(e *Env) {
		e.StdoutTTY, e.Cols, e.Rows, e.CellW, e.CellH = true, 120, 40, 8, 16
		e.Getenv = envMap(map[string]string{"HOME": t.TempDir(), "XDG_RUNTIME_DIR": t.TempDir(), "TERM": "xterm-kitty", "COLORTERM": "truecolor"})
		e.View = func(o view.Options) error { got = o; return nil }
	}, "--slides", "-i", "kitty", deck)
	if got.Slides == nil || got.ClearImages == "" {
		t.Fatalf("slides: %+v", got)
	}
	d, err := got.Slides(120, 40)
	if err != nil || len(d.Slides[0].Images) != 1 {
		t.Fatalf("images: %+v %v", d, err)
	}
	page, img := d.Slides[0], d.Slides[0].Images[0]
	if strings.Contains(strings.Join(page.Lines, ""), "\x1b_G") {
		t.Error("the image is in the text")
	}
	// moved with the centered slide, like its heading: compare with a
	// terminal as wide as the slide and too short to move it down
	narrow, err := got.Slides(slideWidth, 3)
	if err != nil || len(narrow.Slides[0].Images) != 1 {
		t.Fatalf("narrow: %v", err)
	}
	base, head := narrow.Slides[0].Images[0], narrow.Slides[0].Headings[0].Line
	if dl, dc := img.Line-base.Line, img.Col-base.Col; dl != page.Headings[0].Line-head || dc != (120-slideWidth)/2 || dl == 0 {
		t.Errorf("placement %+v, uncentered %+v", img, base)
	}
}
