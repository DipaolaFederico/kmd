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

func TestExportSlides(t *testing.T) {
	deck := writeFile(t, "deck.md", "---\ntitle: Talk <b>\nauthor: Ana\n---\n# One\n\nfirst <script>alert(1)</script> [x](javascript:alert(1))\n\n"+
		"<!-- notes: secret notes -->\n<!-- pause -->\n\n![pic](pic.png)\n\n---\n\n## Two\n\nsecond\n")
	r := run(t, nil, "--slides", "--html", deck) // no terminal needed
	if r.code != 0 {
		t.Fatalf("export: %+v", r)
	}
	out := r.out
	if strings.Count(out, `<section class="slide">`) != 2 || !strings.Contains(out, "<span>1 / 2</span>") ||
		!strings.Contains(out, "<span>Talk &lt;b&gt; · Ana</span>") || !strings.Contains(out, "<title>Talk &lt;b&gt;</title>") {
		t.Errorf("slides, footer or title missing:\n%s", out)
	}
	// the safe defaults of --html: no raw HTML, no unsafe URLs, notes and
	// pauses left out, a CSP; one slide per printed page
	for _, bad := range []string{"<script>", "javascript:", "secret notes", "<!-- pause"} {
		if strings.Contains(out, bad) {
			t.Errorf("%q in the export", bad)
		}
	}
	if !strings.Contains(out, "Content-Security-Policy") || !strings.Contains(out, "break-after: page") || !strings.Contains(out, `<img src="pic.png"`) {
		t.Errorf("page:\n%s", out)
	}
	// --unsafe-html keeps raw HTML and URLs, never the presenter's notes
	if r := run(t, nil, "--slides", "--html", "--unsafe-html", deck); !strings.Contains(r.out, "javascript:") ||
		strings.Contains(r.out, "secret notes") || strings.Contains(r.out, "<!-- pause") {
		t.Errorf("--unsafe-html:\n%s", r.out)
	}
	hidden := writeFile(t, "hidden.md", "---\nfooter: off\n---\n# Only\n")
	if r := run(t, nil, "--slides", "--html", hidden); strings.Contains(r.out, "<footer>") || !strings.Contains(r.out, "<title>Only</title>") {
		t.Errorf("hidden footer:\n%s", r.out)
	}
	if r := run(t, nil, "--slides", "--html", "-s", "two", deck); strings.Count(r.out, `class="slide"`) != 1 || !strings.Contains(r.out, "<span>Talk &lt;b&gt; · Ana</span>") {
		t.Errorf("section: %+v", r)
	}
	if r := run(t, nil, "--slides", "--html", deck+".missing"); r.code != 1 {
		t.Errorf("missing: %+v", r)
	}
}
