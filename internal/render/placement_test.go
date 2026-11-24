package render

import (
	"strings"
	"testing"

	"github.com/DipaolaFederico/kmd/internal/md"
	"github.com/DipaolaFederico/kmd/internal/termimg"
)

func TestPlacements(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, dir)
	src := "text\n\n- ![pic](pic.png)\n\nafter\n"
	opt := Options{Width: 60, Color: true, Images: termimg.Kitty, Cell: termimg.Cell{W: 2, H: 4}, BaseDir: dir, Navigate: true, Placements: true}
	res := Render(md.Parse(src, md.GFM), opt)
	if len(res.Images) != 1 {
		t.Fatalf("images: %+v", res.Images)
	}
	img := res.Images[0]
	lines := strings.Split(res.Text, "\n")
	// left out of the text: blank rows under it, the list's bullet before it
	if strings.Contains(res.Text, "\x1b_G") || img.Rows != 2 || !strings.HasPrefix(img.Seq, "\x1b_G") {
		t.Errorf("placement: %+v", img)
	}
	if !strings.Contains(lines[img.Line], "•") || img.Col != len(margin)+2 || strings.TrimSpace(lines[img.Line+1]) != "" {
		t.Errorf("rows: %q at col %d", lines[img.Line:img.Line+2], img.Col)
	}
	if !strings.Contains(lines[img.Line+img.Rows], "pic") { // the alt text follows
		t.Errorf("after the image: %q", lines[img.Line+img.Rows])
	}
	// blocks stay in the text; so does everything without Placements
	for _, o := range []Options{{Images: termimg.Blocks, Placements: true}, {Images: termimg.Kitty}} {
		o.Width, o.Color, o.BaseDir, o.Navigate, o.Cell = 60, true, dir, true, opt.Cell
		if res := Render(md.Parse(src, md.GFM), o); len(res.Images) != 0 {
			t.Errorf("%v placed %d images", o.Images, len(res.Images))
		}
	}
}

func TestPlacementBudget(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, dir)
	opt := Options{Width: 60, Color: true, Images: termimg.Kitty, Cell: termimg.Cell{W: 2, H: 4}, BaseDir: dir,
		Navigate: true, Placements: true, Budget: &Budget{}}
	// a logo on every slide of a deck: placed, its data never copied, so
	// it counts once against the document's image bytes
	for i := range 100 {
		if res := Render(md.Parse("![logo](pic.png)\n", md.GFM), opt); len(res.Images) != 1 {
			t.Fatalf("slide %d: %d images", i, len(res.Images))
		}
	}
	// what one slide sends to the terminal is bounded: past it, alt text
	one := len(Render(md.Parse("![logo](pic.png)\n", md.GFM), opt).Images[0].Seq)
	src := strings.Repeat("![logo](pic.png)\n\n", maxPlacedBytes/one+2)
	res := Render(md.Parse(src, md.GFM), opt)
	if n := len(res.Images); n == 0 || n*one > maxPlacedBytes || !strings.Contains(res.Text, "▣") {
		t.Errorf("%d images of %d bytes placed", n, one)
	}
}
