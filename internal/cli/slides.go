package cli

import (
	"cmp"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/DipaolaFederico/kmd/internal/md"
	"github.com/DipaolaFederico/kmd/internal/render"
	"github.com/DipaolaFederico/kmd/internal/termimg"
	"github.com/DipaolaFederico/kmd/internal/view"
)

// maxSlides bounds the slides rendered for a presentation.
const maxSlides = 1000

// slideWidth is the render width of slides unless --width or width is set.
const slideWidth = 80

// runSlides presents the document as slides in the viewer (--slides).
func runSlides(env Env, o options, s settings) error {
	in := &stdinCache{}
	name := o.files[0]
	slides := func(cols, rows int) (*view.Deck, error) {
		vs := slideSettings(s, cols, rows)
		parts, front, base, err := readDeck(env, name, o.section, vs, in)
		if err != nil {
			return nil, err
		}
		deck := &view.Deck{Slides: make([]*view.Page, 0, len(parts))}
		deck.Footer, deck.NoFooter = deckFooter(front)
		vs.opts = vs.docOptions(name, base)
		// one budget for the whole deck: images, diagrams and formulas
		// are limited per document, not per slide
		vs.opts.Budget = &render.Budget{}
		vs.opts.Pauses = true
		for _, p := range parts {
			r := render.Render(p, vs.opts)
			page := renderedPage(r)
			page.Steps, page.Images = r.Pauses, r.Images
			deck.Slides = append(deck.Slides, centered(page, cols, rows, vs.opts.Width))
		}
		return deck, nil
	}
	// render once before the screen is taken over, to report errors there;
	// the viewer's first call for the same size reuses that render
	cols, rows := max(env.Cols, 20), max(env.Rows, 3)
	first, err := slides(cols, rows)
	if err != nil {
		return err
	}
	fresh := slides
	slides = func(c, r int) (*view.Deck, error) {
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
	opt := view.Options{Slides: slides, Start: start, Mouse: s.mouse, Watch: o.follow, Poll: env.Poll,
		Select: sel, Match: match, Bar: bar, ClearImages: termimg.Clear(slideImages(s))}
	defer publishSlides(env, name, &opt)()
	if env.View != nil {
		return env.View(opt)
	}
	return view.Run(opt)
}

// exportSlides writes the deck as one HTML page (--slides --html): one
// slide per page when printed, with the safe defaults of --html and none
// of the presenter's pauses and notes.
func exportSlides(env Env, o options, s settings) error {
	name := o.files[0]
	slides, front, base, err := readDeck(env, name, o.section, s, &stdinCache{})
	if err != nil {
		return err
	}
	hopt := htmlOptions(o, s.docOptions(name, base), render.SlideDiagramWidth)
	parts := make([]string, len(slides))
	for i, sl := range slides {
		md.StripDirectives(sl)
		parts[i] = md.HTML(sl, hopt)
	}
	footer, hide := deckFooter(front)
	title := docTitle(slides[0], name)
	if t, _ := frontValue(front, "title"); t != "" {
		title = t
	}
	_, err = io.WriteString(env.Stdout, render.SlidesPage(title, parts, footer, hide, "kmd "+version(), s.opts.Theme))
	return err
}

// slideSettings are the render settings of slides on a cols × rows
// terminal: the viewer's, at most slideWidth wide, and with terminal
// graphics drawn as such, since a slide does not scroll under them — the
// viewer draws them over the page (Placements). Sixel needs the real cell
// size to know the rows an image covers; without it, images are blocks.
func slideSettings(s settings, cols, rows int) settings {
	vs := viewerSettings(s, max(cols, 1))
	if s.fixedWidth == 0 {
		vs.opts.Width = min(vs.opts.Width, slideWidth) // centered on wide screens
	}
	vs.opts.Images = slideImages(s)
	vs.opts.Placements = true
	// an image fits on the screen: below the blank line slides start with
	// and above the footer
	vs.opts.ImageRows = min(cmp.Or(vs.opts.ImageRows, render.DefaultImageRows), max(rows-2, 1))
	return vs
}

// slideImages is how slides draw images: as the user chose, except sixel
// without the real cell size, as blocks.
func slideImages(s settings) termimg.Protocol {
	if s.opts.Images == termimg.Sixel && (s.opts.Cell.W <= 0 || s.opts.Cell.H <= 0) {
		return termimg.Blocks
	}
	return s.opts.Images
}

// readDeck reads the deck at name and splits it into slides, the same way
// for the presentation and its notes window so that their numbers agree.
// front is the front matter, which a section (sec) leaves out.
func readDeck(env Env, name, sec string, vs settings, in *stdinCache) (slides []*md.Node, front, base string, err error) {
	doc, base, err := loadDoc(env, name, "", vs, in)
	if err != nil {
		return nil, "", "", err
	}
	if c := doc.FirstChild; c != nil && c.Type == md.FrontMatter {
		front = c.Literal
	}
	if doc, err = sectionOf(doc, name, sec); err != nil {
		return nil, "", "", err
	}
	return md.Slides(doc, maxSlides), front, base, nil
}

// renderedPage is a viewer page of a render made with Navigate.
func renderedPage(r render.Result) *view.Page {
	res := output{text: r.Text, anchors: map[string]int{}}
	res.addNav(r, 0)
	return pageOf(res)
}

// deckFooter reads the footer of a presentation from the document's front
// matter: its footer key, or else its title and author; a footer of false,
// off or no hides it.
func deckFooter(front string) (text string, hide bool) {
	if footer, set := frontValue(front, "footer"); set {
		if strings.EqualFold(footer, "false") || strings.EqualFold(footer, "off") || strings.EqualFold(footer, "no") {
			return "", true
		}
		return footer, false
	}
	title, _ := frontValue(front, "title")
	author, _ := frontValue(front, "author")
	if title != "" && author != "" {
		return title + " · " + author, false
	}
	return title + author, false
}

// frontValue returns the value of key in the front matter, as text (see
// frontText); set is false without the key.
func frontValue(front, key string) (text string, set bool) {
	for line := range strings.Lines(front) {
		if k, v, ok := strings.Cut(line, ":"); ok && k == key {
			return frontText(v), true
		}
	}
	return "", false
}

// frontText returns a one-line YAML value as text: unquoted (no
// escapes), without a trailing comment, cleaned for the terminal and at
// most maxFrontValue runes. Block scalars, lists and nested keys are not
// read: a front matter is not worth a YAML parser here.
func frontText(v string) string {
	v = strings.TrimSpace(v)
	if q := v[:min(len(v), 1)]; q == `"` || q == "'" {
		if end := strings.Index(v[1:], q); end >= 0 {
			v = v[1 : end+1]
		}
	} else if i := strings.Index(v, " #"); i >= 0 {
		v = v[:i]
	}
	if n := 4 * maxFrontValue; len(v) > n { // cleaning a huge value costs memory
		for n > 0 && !utf8.RuneStart(v[n]) {
			n--
		}
		v = v[:n]
	}
	v = strings.Join(strings.Fields(render.Clean(v)), " ")
	if r := []rune(v); len(r) > maxFrontValue {
		v = string(r[:maxFrontValue])
	}
	return v
}

// maxFrontValue bounds each front matter value kept, shown in the footer
// (measured on every redraw of the viewer) and the exported page's title:
// a hostile one can be megabytes long.
const maxFrontValue = 200

// centered places a slide in the middle of a cols × rows terminal: lines
// are indented to center the render width, and a slide shorter than the
// screen (minus the footer line) is moved down to its middle. Its steps
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
	for i := range p.Images {
		p.Images[i].Line += top
		p.Images[i].Col += left
	}
	return p
}
