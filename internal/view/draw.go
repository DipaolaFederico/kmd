package view

import (
	"net/url"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/DipaolaFederico/kmd/internal/render"
	"github.com/DipaolaFederico/kmd/internal/styled"
)

const reset = "\x1b[0m"

func moveTo(b *strings.Builder, row, col int) {
	b.WriteString("\x1b[" + strconv.Itoa(row+1) + ";" + strconv.Itoa(col+1) + "H")
}

// draw paints the whole screen. Synchronized output (mode 2026) keeps
// terminals that support it from showing half-drawn frames.
func (m *model) draw(b *strings.Builder) {
	b.WriteString("\x1b[?2026h")
	if m.picking {
		m.drawPicker(b)
	} else {
		m.drawPage(b)
	}
	m.status(b)
	if m.outline {
		m.drawOutline(b)
	}
	if m.help {
		m.drawHelp(b)
	}
	b.WriteString("\x1b[?2026l")
}

// drawPage paints the document: its lines, search matches and the selected
// link.
func (m *model) drawPage(b *strings.Builder) {
	h := m.viewH()
	now := drawnImages{top: m.top, cols: m.cols, rows: m.rows, imgs: m.shownImages()}
	keep := len(now.imgs) > 0 && now.equal(m.drawn) // the images on screen are right
	imgs := now.imgs
	for y := range h {
		i := m.top + y
		if keep && covered(imgs, i) {
			continue // clearing the row would erase a sixel or iTerm2 image
		}
		moveTo(b, y, 0)
		if i < len(m.page.Lines) {
			b.WriteString(m.page.Lines[i])
		}
		b.WriteString(reset + "\x1b[K")
	}
	if keep {
		m.drawn = now // the same strings from now on: compared without reading them
	} else {
		if len(m.drawn.imgs) > 0 {
			b.WriteString(m.opt.ClearImages)
		}
		for _, p := range imgs {
			moveTo(b, p.Line-m.top, p.Col)
			b.WriteString(p.Seq)
		}
		m.drawn = now
	}
	for i, mt := range m.matches {
		if mt.line >= m.top && mt.line < m.top+h {
			st := m.opt.Match
			if i == m.cur {
				st = m.opt.Select
			}
			m.overlay(b, mt.line-m.top, mt.col, mt.width, st)
		}
	}
	if m.visible(m.sel) {
		for _, s := range m.page.Links[m.sel].Spans {
			if s.Line >= m.top && s.Line < m.top+h {
				moveTo(b, s.Line-m.top, s.Col)
				b.WriteString(m.opt.Select + s.Text + reset)
			}
		}
	}
}

// drawnImages are the graphics images drawn on a screen: while the next
// screen would show the same ones at the same place, they are left there,
// not sent again (they can be megabytes).
type drawnImages struct {
	top, cols, rows int
	imgs            []render.Placement
}

func (d drawnImages) equal(o drawnImages) bool {
	return d.top == o.top && d.cols == o.cols && d.rows == o.rows && slices.Equal(d.imgs, o.imgs)
}

// shownImages returns the graphics images of the page that fit on screen
// whole (a picture cannot be cut), none while a panel covers the page.
// Search matches and the selected link are never on their rows, which
// hold no text but a list or quote marker.
func (m *model) shownImages() []render.Placement {
	if m.help || m.outline {
		return nil
	}
	var out []render.Placement
	for _, p := range m.page.Images {
		if p.Line >= m.top && p.Line+p.Rows <= m.top+m.viewH() {
			out = append(out, p)
		}
	}
	return out
}

// covered reports whether line i lies under one of imgs.
func covered(imgs []render.Placement, i int) bool {
	for _, p := range imgs {
		if i >= p.Line && i < p.Line+p.Rows {
			return true
		}
	}
	return false
}

// overlay repaints columns [col, col+w) of a screen row with style st.
func (m *model) overlay(b *strings.Builder, row, col, w int, st string) {
	plain := m.plainLine(m.top + row)
	start, end, c := -1, len(plain), 0
	for i, r := range plain {
		if c >= col+w {
			end = i
			break
		}
		if c >= col && start < 0 {
			start = i
		}
		c += styled.RuneWidth(r)
	}
	if start < 0 {
		return
	}
	moveTo(b, row, col)
	b.WriteString(st + plain[start:end] + reset)
}

func (m *model) plainLine(i int) string {
	if m.plain != nil {
		return m.plain[i]
	}
	return stripEscapes(m.page.Lines[i])
}

// status draws the bottom line: prompt, message, link target or position.
func (m *model) status(b *strings.Builder) {
	moveTo(b, m.rows-1, 0)
	left, right := "", ""
	switch {
	case m.prompt:
		left = "/" + string(m.input)
	case m.picking && m.msg != "":
		left = m.msg
	case m.picking:
		left, right = "Find: "+string(m.pick.query), "Enter open · Tab text search · Esc close"
		if m.pick.text {
			left, right = "Search: "+string(m.pick.tquery), "Enter search/open · Tab files · Esc close"
		}
		if styled.Width(left)+styled.Width(right)+6 > m.cols {
			right = "" // the query comes first
		}
	case m.outline:
		left = "↑↓ choose · Enter jump · Esc close"
	case m.msg != "":
		left = m.msg
	case m.visible(m.sel):
		right = "Enter open · Esc clear"
		// the columns left of right, its padding and the percentage
		left = "→ " + m.target(m.page.Links[m.sel].Href, m.cols-styled.Width(right+m.percent())-8)
	case m.slides != nil && m.deck.NoFooter:
		b.WriteString("\x1b[2K\x1b[?25l")
		return
	case m.slides != nil:
		left, right = m.footer()
	default:
		left = m.breadcrumb()
		if len(m.matches) > 0 && m.cur >= 0 {
			right = strconv.Itoa(m.cur+1) + "/" + strconv.Itoa(len(m.matches)) + " · n/N · "
		}
		if len(m.hist) > 0 {
			right += "⌫ back · "
		}
		right += "Tab links · ? help"
	}
	if right != "" && !m.picking && m.slides == nil {
		right += "  " + m.percent()
	}
	right = " " + right + " "
	left = " " + clip(left, max(m.cols-styled.Width(right)-2, 4))
	pad := max(m.cols-styled.Width(left)-styled.Width(right), 0)
	b.WriteString(m.opt.Bar + left + strings.Repeat(" ", pad) + right + reset)
	if m.prompt || m.picking && m.msg == "" {
		moveTo(b, m.rows-1, styled.Width(left))
		b.WriteString("\x1b[?25h")
	} else {
		b.WriteString("\x1b[?25l")
	}
}

func (m *model) percent() string {
	n := len(m.page.Lines)
	if n <= m.viewH() {
		return "all"
	}
	switch m.top {
	case 0:
		return "top"
	case m.maxTop():
		return "end"
	}
	return strconv.Itoa((m.top+m.viewH())*100/n) + "%"
}

// target describes a link destination for the status bar, in n columns.
func (m *model) target(href string, n int) string {
	if strings.HasPrefix(href, "#") {
		return href
	}
	u, err := url.Parse(href)
	switch {
	case err != nil:
	case u.Scheme == "file":
		s := m.rel(u.Path)
		if u.Fragment != "" {
			s += "#" + u.Fragment
		}
		return s
	case isWeb(u) && u.Host != "":
		return webTarget(u, n) // a user name shows as "…@"
	case isWeb(u):
		return clip("invalid web link: "+href, n)
	}
	return href
}

// isWeb reports whether u is an http or https URL.
func isWeb(u *url.URL) bool {
	s := strings.ToLower(u.Scheme)
	return s == "http" || s == "https"
}

// webURL reports whether the web URL u names its host plainly: with "//",
// a host and no user name. Go parses "https:github.com%2F…@evil.example/"
// and "https:/…@evil.example" without a host, but browsers read a user
// name and the host evil.example in them; a user name hides the real host
// behind one that looks trusted ("https://github.com%2Flogin@evil.example/").
func webURL(u *url.URL) bool {
	return u.Opaque == "" && u.Host != "" && u.User == nil
}

// webTarget shows a web URL in n columns, keeping what tells where it
// leads, the scheme and the whole host: selecting a link to see its
// target before opening it is the defence against phishing. The path is
// shortened first; a host too long to fit loses its left side, so the
// registrable domain stays visible ("https://…aaaa.evil.example"), and a
// user name, which can pose as a host, shows as "…@".
func webTarget(u *url.URL, n int) string {
	head := u.Scheme + "://"
	if u.User != nil {
		head += "…@"
	}
	host := render.Clean(u.Host)
	rest := u.EscapedPath()
	if u.RawQuery != "" {
		rest += "?" + u.RawQuery
	}
	if u.Fragment != "" {
		rest += "#" + u.EscapedFragment()
	}
	if w := styled.Width(head + host); w < n {
		return head + host + clip(rest, n-w)
	}
	if k := n - styled.Width(head); k > 1 {
		return head + styled.ElideLeft(host, k)
	}
	return clip(head+host, n)
}

// breadcrumb shows the documents visited to get here.
func (m *model) breadcrumb() string {
	var parts []string
	for _, e := range m.hist {
		if len(parts) == 0 || parts[len(parts)-1] != pageName(e.path) {
			parts = append(parts, pageName(e.path))
		}
	}
	if n := pageName(m.path); len(parts) == 0 || parts[len(parts)-1] != n {
		parts = append(parts, n)
	}
	s := strings.Join(parts, " › ")
	if w := m.cols / 2; styled.Width(s) > w {
		for len(parts) > 1 && styled.Width("… › "+strings.Join(parts, " › ")) > w {
			parts = parts[1:]
		}
		s = "… › " + strings.Join(parts, " › ")
	}
	return s
}

// pageName is a short label for a page: "etapa-5/README.md" for READMEs
// (which are all called the same), the file name otherwise.
func pageName(p string) string {
	if p == "" {
		return "kmd"
	}
	base := filepath.Base(p)
	if strings.EqualFold(strings.TrimSuffix(base, filepath.Ext(base)), "readme") || strings.EqualFold(base, "index.md") {
		if a, err := filepath.Abs(p); err == nil {
			return filepath.Base(filepath.Dir(a)) + "/" + base
		}
	}
	return base
}

var helpText = []string{
	"kmd — keys",
	"",
	"j k ↓ ↑          scroll a line",
	"Space b          scroll a page (also PgDn PgUp)",
	"d u              scroll half a page",
	"g G              top / bottom (also Home End)",
	"",
	"Tab Shift-Tab    select next / previous link",
	"Enter l →        open the selected link",
	"click            open a link (web: click twice)",
	"Backspace h ←    back",
	"H                back to the first document",
	"",
	"t                contents: jump to a heading",
	"o                find a document (starting folder)",
	"S                search the text of its documents",
	"[ ]              previous / next section",
	"/ n N            search, next / previous match",
	"c                copy a code block on screen",
	"Esc              clear selection and search",
	"e                edit the file ($VISUAL / $EDITOR)",
	"r                reload",
	"q                quit",
	"",
	"Presentation (--slides): → Space PgDn next,",
	"← PgUp back (step by step at pauses),",
	"g G first / last slide.",
	"",
	"Markdown links open here; web links and",
	"images open in the desktop's default app.",
	"A folder without a README lists all of its",
	"documents (o): type to filter, Tab to search",
	"their text, Enter to open, Backspace returns.",
}

func (m *model) drawHelp(b *strings.Builder) {
	w := 0
	for _, l := range helpText {
		w = max(w, styled.Width(l))
	}
	w += 4
	top := max((m.viewH()-len(helpText)-2)/2, 0)
	left := max((m.cols-w)/2, 0)
	line := func(row int, s string) {
		if row >= m.viewH() {
			return
		}
		moveTo(b, row, left)
		b.WriteString(m.opt.Bar + s + reset)
	}
	line(top, "╭"+strings.Repeat("─", w-2)+"╮")
	for i, l := range helpText {
		line(top+1+i, "│ "+l+strings.Repeat(" ", w-4-styled.Width(l))+" │")
	}
	line(top+1+len(helpText), "╰"+strings.Repeat("─", w-2)+"╯")
}
