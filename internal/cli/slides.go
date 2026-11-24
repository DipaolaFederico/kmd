package cli

import (
	"errors"
	"strings"

	"github.com/DipaolaFederico/kmd/internal/md"
	"github.com/DipaolaFederico/kmd/internal/render"
	"github.com/DipaolaFederico/kmd/internal/view"
)

// maxSlides bounds the slides rendered for a presentation.
const maxSlides = 1000

// slideWidth is the render width of slides unless --width or width is set.
const slideWidth = 80

// runSlides presents the document as slides in the viewer (--slides).
func runSlides(env Env, o options, s settings, watch bool) error {
	in := &stdinCache{}
	name := o.files[0]
	slides := func(cols, rows int) ([]*view.Page, error) {
		vs := viewerSettings(s, max(cols, 1))
		if s.fixedWidth == 0 {
			vs.opts.Width = min(vs.opts.Width, slideWidth) // centered on wide screens
		}
		doc, base, err := loadDoc(env, name, o.section, vs, in)
		if err != nil {
			return nil, err
		}
		vs.opts = vs.docOptions(name, base)
		// one budget for the whole deck: images, diagrams and formulas
		// are limited per document, not per slide
		vs.opts.Budget = &render.Budget{}
		vs.opts.Pauses = true
		parts := md.Slides(doc, maxSlides)
		pages := make([]*view.Page, 0, len(parts))
		for _, p := range parts {
			r := render.Render(p, vs.opts)
			res := output{text: r.Text, anchors: map[string]int{}}
			res.addNav(r, 0)
			page := pageOf(res)
			page.Steps = r.Pauses
			pages = append(pages, centered(page, cols, rows, vs.opts.Width))
		}
		return pages, nil
	}
	// render once before the screen is taken over, to report errors there;
	// the viewer's first call for the same size reuses that render
	cols, rows := max(env.Cols, 20), max(env.Rows, 3)
	first, err := slides(cols, rows)
	if err != nil {
		return err
	}
	fresh := slides
	slides = func(c, r int) ([]*view.Page, error) {
		if first != nil && c == cols && r == rows {
			p := first
			first = nil
			return p, nil
		}
		return fresh(c, r)
	}
	start := ""
	if name != "-" {
		start = name
	}
	sel, match, bar := render.ViewerStyles(s.opts.Theme, s.opts.Color, s.opts.TrueColor)
	opt := view.Options{Slides: slides, Start: start, Mouse: s.mouse, Watch: watch, Poll: env.Poll,
		Select: sel, Match: match, Bar: bar}
	if env.View != nil {
		return env.View(opt)
	}
	return view.Run(opt)
}

// centered places a slide in the middle of a cols × rows terminal: lines
// are indented to center the render width, and a slide shorter than the
// screen (minus the status bar) is moved down to its middle. Its steps
// stay where the whole slide puts them, so revealing one moves nothing.
func centered(p *view.Page, cols, rows, width int) *view.Page {
	left := max((cols-width)/2, 0)
	top := 0
	if n := len(p.Lines); n < rows-1 {
		top = (rows - 1 - n) / 2
	}
	if left == 0 && top == 0 {
		return p
	}
	pad := strings.Repeat(" ", left)
	lines := make([]string, 0, top+len(p.Lines))
	for range top {
		lines = append(lines, "")
	}
	for _, l := range p.Lines {
		if l != "" {
			l = pad + l
		}
		lines = append(lines, l)
	}
	p.Lines = lines
	for i := range p.Links {
		for j := range p.Links[i].Spans {
			p.Links[i].Spans[j].Line += top
			p.Links[i].Spans[j].Col += left
		}
	}
	for k, v := range p.Anchors {
		p.Anchors[k] = v + top
	}
	for i := range p.Headings {
		p.Headings[i].Line += top
	}
	for i := range p.Code {
		p.Code[i].Start += top
		p.Code[i].End += top
	}
	for i := range p.Steps {
		p.Steps[i] += top
	}
	return p
}

var errSlidesTTY = errors.New("--slides needs a terminal")
