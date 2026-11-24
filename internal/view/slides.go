package view

import (
	"math"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/DipaolaFederico/kmd/internal/render"
)

// Presentation mode (kmd --slides): Options.Slides renders the document
// as one page per slide for the terminal size; the viewer shows one at a
// time and the arrow keys, Space and PgUp/PgDn move between them, step by
// step through a slide with pauses.

// lastStep asks showSlide for a slide fully shown.
const lastStep = math.MaxInt

// loadSlides renders the slides for the current terminal size and shows
// slide i at step (both clamped).
func (m *model) loadSlides(i, step int) error {
	slides, err := m.opt.Slides(m.cols, m.rows)
	if err != nil {
		return err
	}
	if len(slides) == 0 {
		slides = []*Page{{Lines: []string{""}}}
	}
	m.slides = slides
	m.showSlide(i, step)
	return nil
}

// showSlide shows slide i at step (both clamped) from its top. Like any
// new page, it clears the search and the selected link.
func (m *model) showSlide(i, step int) {
	m.slide = min(max(i, 0), len(m.slides)-1)
	p := m.slides[m.slide]
	m.step = min(max(step, 0), len(p.Steps))
	if m.step < len(p.Steps) {
		p = p.upTo(p.Steps[m.step])
	}
	m.setPage(p)
}

// showStep shows another step of the slide on screen, keeping the scroll
// position.
func (m *model) showStep(step int) {
	top := m.top
	m.showSlide(m.slide, step)
	m.top = top
	m.scroll(0)
}

// upTo returns the page cut to its first n lines, with the links, anchors,
// headings and code blocks they hold. Steps end between top-level blocks,
// so nothing is cut in half.
func (p *Page) upTo(n int) *Page {
	n = min(n, len(p.Lines))
	q := &Page{Lines: p.Lines[:n:n], Anchors: map[string]int{}}
	for _, l := range p.Links {
		if len(l.Spans) > 0 && l.Spans[0].Line < n {
			q.Links = append(q.Links, l)
		}
	}
	for k, v := range p.Anchors {
		if v < n {
			q.Anchors[k] = v
		}
	}
	for _, h := range p.Headings {
		if h.Line < n {
			q.Headings = append(q.Headings, h)
		}
	}
	for _, c := range p.Code {
		if c.Start < n {
			q.Code = append(q.Code, c)
		}
	}
	return q
}

// forward reveals the next step of the slide, or shows the next slide.
func (m *model) forward() {
	switch {
	case m.step < len(m.slides[m.slide].Steps):
		shown := len(m.page.Lines)
		m.showStep(m.step + 1)
		// bring the new part into view, starting with its first line
		m.show(len(m.page.Lines) - 1)
		m.top = min(m.top, shown)
	case m.slide == len(m.slides)-1:
		m.msg = "last slide"
	default:
		m.showSlide(m.slide+1, 0)
	}
}

// backward hides the last step shown, or shows the previous slide whole.
func (m *model) backward() {
	switch {
	case m.step > 0:
		m.showStep(m.step - 1)
	case m.slide == 0:
		m.msg = "first slide"
	default:
		m.showSlide(m.slide-1, lastStep)
	}
}

// handleSlide applies the keys that move between slides; false means the
// key is handled as in the viewer (scrolling a tall slide, search, copy…).
func (m *model) handleSlide(name string) bool {
	switch name {
	case "right", "l", "n", " ", "pgdn", "ctrl-f":
		m.forward()
	case "left", "h", "p", "b", "pgup", "ctrl-b", "backspace":
		if len(m.hist) > 0 && name == "backspace" { // back from an in-slide link
			return false
		}
		m.backward()
	case "g", "home", "H":
		m.showSlide(0, 0)
	case "G", "end":
		m.showSlide(len(m.slides)-1, lastStep)
	case "enter":
		if m.visible(m.sel) {
			return false
		}
		m.forward()
	default:
		return false
	}
	return true
}

// slideLink reports whether a link may be followed during a presentation:
// in-slide anchors and anything handed to the desktop, not documents that
// would replace the slides.
func (m *model) slideLink(href string) bool {
	if strings.HasPrefix(href, "#") {
		return true
	}
	u, err := url.Parse(href)
	if err != nil || !strings.EqualFold(u.Scheme, "file") {
		return true
	}
	return !render.MarkdownExt[strings.ToLower(filepath.Ext(u.Path))] && filepath.Ext(u.Path) != ""
}

func (m *model) slideStatus() string {
	s := "slide " + strconv.Itoa(m.slide+1) + "/" + strconv.Itoa(len(m.slides))
	if n := len(m.slides[m.slide].Steps); n > 0 {
		s += " · step " + strconv.Itoa(m.step+1) + "/" + strconv.Itoa(n+1)
	}
	return s
}
