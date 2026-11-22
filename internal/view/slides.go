package view

import (
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/DipaolaFederico/kmd/internal/render"
)

// Presentation mode (kmd --slides): Options.Slides renders the document
// as one page per slide for the terminal size; the viewer shows one at a
// time and the arrow keys, Space and PgUp/PgDn move between them.

// loadSlides renders the slides for the current terminal size and shows
// slide i (clamped).
func (m *model) loadSlides(i int) error {
	slides, err := m.opt.Slides(m.cols, m.rows)
	if err != nil {
		return err
	}
	if len(slides) == 0 {
		slides = []*Page{{Lines: []string{""}}}
	}
	m.slides = slides
	m.showSlide(i)
	return nil
}

func (m *model) showSlide(i int) {
	m.slide = min(max(i, 0), len(m.slides)-1)
	m.setPage(m.slides[m.slide])
}

// handleSlide applies the keys that move between slides; false means the
// key is handled as in the viewer (scrolling a tall slide, search, copy…).
func (m *model) handleSlide(name string) bool {
	switch name {
	case "right", "l", "n", " ", "pgdn", "ctrl-f":
		if m.slide == len(m.slides)-1 {
			m.msg = "last slide"
			return true
		}
		m.showSlide(m.slide + 1)
	case "left", "h", "p", "b", "pgup", "ctrl-b", "backspace":
		if len(m.hist) > 0 && name == "backspace" { // back from an in-slide link
			return false
		}
		if m.slide == 0 {
			m.msg = "first slide"
			return true
		}
		m.showSlide(m.slide - 1)
	case "g", "home":
		m.showSlide(0)
	case "G", "end":
		m.showSlide(len(m.slides) - 1)
	case "enter":
		if m.visible(m.sel) {
			return false
		}
		m.handleSlide("right")
	case "H":
		m.showSlide(0)
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
	return "slide " + strconv.Itoa(m.slide+1) + "/" + strconv.Itoa(len(m.slides))
}
