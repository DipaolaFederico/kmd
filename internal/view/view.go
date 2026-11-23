// Package view is kmd's interactive document viewer: a pager that knows
// where the links are, so they can be selected with the keyboard or clicked,
// and followed — Markdown files open in the viewer itself, with history.
package view

import (
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/DipaolaFederico/kmd/internal/render"
	"github.com/DipaolaFederico/kmd/internal/styled"
)

// Page is a rendered document.
type Page struct {
	Lines    []string // painted lines (escape sequences included)
	Links    []render.Link
	Anchors  map[string]int
	Headings []render.Heading // the outline, in document order
	Code     []render.CodeBlock
}

// Loader renders the document at path for a terminal cols columns wide.
// The empty path stands for the documents kmd was started with (which may
// be several files or stdin).
type Loader func(path string, cols int) (*Page, error)

// Options configures the viewer.
type Options struct {
	Load  Loader
	Start string // path of the first page ("" = documents kmd started with)
	Mouse bool   // report clicks and the wheel (Shift+drag still selects text)

	// Watch reloads the page on screen whenever its file changes (live
	// preview, kmd -f), checking every Poll (0 = 250ms).
	Watch bool
	Poll  time.Duration

	// First, when set, is the first page already rendered for a terminal
	// FirstCols wide; it is used instead of loading Start again if the
	// viewer's terminal has that width.
	First     *Page
	FirstCols int

	// SGR sequences for the selected link, search matches and the status bar.
	Select, Match, Bar string

	// Open hands a URL or a local file to the desktop (browser, image
	// viewer…); Edit opens a file in the user's editor. Both are replaced
	// in tests; nil disables the action.
	Open func(target string) error
	Edit func(path string) error
	// Copy puts text in the clipboard (OSC 52 in the terminal).
	Copy func(text string) error

	// Slides, when set, turns the viewer into a presentation: it renders
	// the slides for a terminal cols × rows (Load is then only used to
	// check that Start can be read).
	Slides func(cols, rows int) ([]*Page, error)

	// Root is the folder the file picker (o, S) lists when the page on
	// screen is a document ("" = none); a folder page lists itself.
	Root string
}

// entry is a history position.
type entry struct {
	path     string
	top, sel int
	pick     string // root of the picker that was open ("" = none)
}

// match is a search hit.
type match struct{ line, col, width int }

type model struct {
	opt        Options
	cols, rows int

	path  string
	page  *Page
	plain []string // Lines without escape sequences (search), lazily built
	top   int
	sel   int // selected link, -1 = none
	hist  []entry

	msg     string // one-shot status message
	help    bool
	prompt  bool
	input   []rune
	query   string
	matches []match
	cur     int // current match

	quit bool

	copied int // code block copied last (c), -1 = none

	slides []*Page // presentation mode
	slide  int

	outline    bool // the outline (t) is open
	ocur, otop int  // its selected heading and first visible row

	pick    *picker           // file picker (o, S), kept while documents are read
	picking bool              // the picker is on screen
	bg      chan func(*model) // results of background work, applied by the loop

	// watched file and its last seen state (Watch)
	watched string
	stamp   stamp
}

// stamp identifies a version of a file.
type stamp struct {
	mod  time.Time
	size int64
}

func statStamp(path string) (stamp, bool) {
	st, err := os.Stat(path)
	if err != nil {
		return stamp{}, false
	}
	return stamp{st.ModTime(), st.Size()}, true
}

// poll reloads the page if its file changed since the last call, keeping
// the exact position (an edit rarely moves what is being read). A file that
// cannot be stat'ed is usually an editor replacing it: keep the page.
func (m *model) poll() bool {
	if m.path == "" {
		return false
	}
	s, ok := statStamp(m.path)
	if !ok {
		return false
	}
	if m.path != m.watched {
		m.watched, m.stamp = m.path, s
		return false
	}
	if s == m.stamp {
		return false
	}
	m.stamp = s
	top := m.top
	m.reload()
	m.top = top
	m.scroll(0)
	return true
}

// maxMatches bounds search work and memory on huge documents.
const maxMatches = 10000

func newModel(opt Options, cols, rows int) (*model, error) {
	m := &model{opt: opt, cols: cols, rows: rows, sel: -1, path: opt.Start, bg: make(chan func(*model), 16)}
	if opt.Slides != nil {
		return m, m.loadSlides(0)
	}
	p := opt.First
	if p == nil || opt.FirstCols != cols {
		var err error
		if p, err = opt.Load(opt.Start, cols); err != nil {
			return nil, err
		}
	}
	m.setPage(p)
	if opt.Start != "" && isDir(opt.Start) {
		m.openPicker(false)
	}
	return m, nil
}

func (m *model) setPage(p *Page) {
	m.page, m.plain = p, nil
	m.sel, m.matches, m.query = -1, nil, ""
	m.top, m.outline, m.copied = 0, false, -1
}

func (m *model) viewH() int { return max(m.rows-1, 1) }

func (m *model) maxTop() int { return max(len(m.page.Lines)-m.viewH(), 0) }

func (m *model) scroll(d int) { m.top = min(max(m.top+d, 0), m.maxTop()) }

// show scrolls the least needed so line is on screen, with some context.
func (m *model) show(line int) {
	switch h := m.viewH(); {
	case line < m.top:
		m.top = max(line-min(2, h/4), 0)
	case line >= m.top+h:
		m.top = line - h + 1 + min(2, h/4)
	}
	m.scroll(0)
}

func (m *model) linkLine(i int) int { return m.page.Links[i].Spans[0].Line }

func (m *model) visible(i int) bool {
	if i < 0 || i >= len(m.page.Links) {
		return false
	}
	l := m.linkLine(i)
	return l >= m.top && l < m.top+m.viewH()
}

// nextLink selects the next (dir 1) or previous (dir -1) link: relative to
// the selection when it is on screen, else from the edge of the screen.
func (m *model) nextLink(dir int) {
	n := len(m.page.Links)
	if n == 0 {
		m.msg = "no links in this document"
		return
	}
	c := -1
	switch {
	case m.sel >= 0 && m.visible(m.sel):
		c = m.sel + dir
	case dir > 0:
		for i := range n {
			if m.linkLine(i) >= m.top {
				c = i
				break
			}
		}
	default:
		for i := n - 1; i >= 0; i-- {
			if m.linkLine(i) < m.top+m.viewH() {
				c = i
				break
			}
		}
	}
	switch { // wrap around, like a browser
	case c >= n || c < 0 && dir > 0:
		c = 0
	case c < 0:
		c = n - 1
	}
	m.sel = c
	m.show(m.linkLine(c))
}

// linkAt returns the link drawn at screen cell (x, y), or -1.
func (m *model) linkAt(x, y int) int {
	line := m.top + y
	for i, l := range m.page.Links {
		for _, s := range l.Spans {
			if s.Line == line && x >= s.Col && x < s.Col+s.Width {
				return i
			}
		}
	}
	return -1
}

// load replaces the page with the document at path, keeping history.
func (m *model) load(path string) error {
	if m.opt.Load == nil {
		return errors.New("cannot open documents here")
	}
	p, err := m.opt.Load(path, m.cols)
	if err != nil {
		return err
	}
	m.hist = append(m.hist, entry{m.path, m.top, m.sel, m.pickedRoot()})
	m.path = path
	m.setPage(p)
	return nil
}

func (m *model) back() { m.goBack(len(m.hist) - 1) }

// home returns to the first document, where kmd started, as if Backspace
// were pressed until the history is empty.
func (m *model) home() { m.goBack(0) }

// goBack returns to history entry i, dropping it and every later one.
func (m *model) goBack(i int) {
	if len(m.hist) == 0 {
		m.msg = "no previous document"
		return
	}
	e := m.hist[i]
	if e.path != m.path {
		p, err := m.opt.Load(e.path, m.cols)
		if err != nil {
			m.msg = errText(err)
			return
		}
		m.setPage(p)
		m.path = e.path
	}
	m.hist = m.hist[:i]
	m.picking = false
	if e.pick != "" {
		m.showPicker(e.pick)
	}
	m.top, m.sel = e.top, e.sel
	m.scroll(0)
	if m.sel >= len(m.page.Links) {
		m.sel = -1
	}
}

// reload re-renders the current page (file changed, terminal resized),
// keeping the reading position in proportion.
func (m *model) reload() {
	if m.slides != nil {
		top := m.top
		if err := m.loadSlides(m.slide); err != nil {
			m.msg = errText(err)
			return
		}
		m.top = top
		m.scroll(0)
		return
	}
	p, err := m.opt.Load(m.path, m.cols)
	if err != nil {
		m.msg = errText(err)
		return
	}
	old := max(len(m.page.Lines), 1)
	top, sel, q := m.top, m.sel, m.query
	m.setPage(p)
	m.top = top * len(p.Lines) / old
	m.scroll(0)
	if sel < len(p.Links) {
		m.sel = sel
	}
	if q != "" {
		m.search(q)
	}
}

// jump moves to the heading with the given anchor.
func (m *model) jump(frag string) bool {
	line, ok := m.page.Anchors[frag]
	if !ok {
		line, ok = m.page.Anchors[render.Slug(frag)]
	}
	if !ok {
		m.msg = "no heading #" + clip(frag, 40)
		return false
	}
	m.top = line
	m.scroll(0)
	m.sel = -1
	return true
}

// follow opens a link target.
func (m *model) follow(href string) {
	if strings.HasPrefix(href, "#") {
		m.anchorJump(href[1:])
		return
	}
	u, err := url.Parse(href)
	if err != nil || !render.SafeURL(href) {
		m.msg = "invalid link"
		return
	}
	if m.slides != nil && !m.slideLink(href) {
		m.msg = "documents do not open during a presentation"
		return
	}
	switch strings.ToLower(u.Scheme) {
	case "file":
		m.followFile(u.Path, u.Fragment)
	case "http", "https":
		m.external(href, clip(href, 60))
	case "mailto":
		// only the address, subject and body: some mail clients honour
		// parameters such as attach=/path/to/secret
		q := url.Values{}
		for _, k := range []string{"subject", "body"} {
			if v := u.Query().Get(k); v != "" {
				q.Set(k, v)
			}
		}
		u.RawQuery = q.Encode()
		m.external(u.String(), clip(u.String(), 60))
	default:
		m.msg = "unsupported link: " + clip(href, 50)
	}
}

// internal reports whether href stays inside the viewer (an anchor or a
// local Markdown file).
func (m *model) internal(href string) bool {
	if strings.HasPrefix(href, "#") {
		return true
	}
	u, err := url.Parse(href)
	return err == nil && strings.EqualFold(u.Scheme, "file") && render.MarkdownExt[strings.ToLower(filepath.Ext(u.Path))]
}

// anchorJump jumps within the page, recording the position for back.
func (m *model) anchorJump(frag string) {
	top, sel := m.top, m.sel
	if m.jump(frag) {
		m.hist = append(m.hist, entry{m.path, top, sel, ""})
	}
}

// followFile opens a local link target. Symlinks are resolved first and
// the checks apply to the real file, which must be a regular file: a link
// named notes.md may point at ~/.ssh/id_rsa, a FIFO would block forever.
func (m *model) followFile(path, frag string) {
	st, err := os.Stat(path)
	if err != nil {
		m.msg = "not found: " + m.rel(path)
		return
	}
	if st.IsDir() {
		if m.slides != nil {
			m.msg = "documents do not open during a presentation"
			return
		}
		found := false
		for _, n := range render.IndexNames {
			if s, err := os.Stat(filepath.Join(path, n)); err == nil && !s.IsDir() {
				path, found = filepath.Join(path, n), true
				break
			}
		}
		if !found { // list its documents
			if path != m.path {
				if err := m.load(path); err != nil {
					m.msg = errText(err)
					return
				}
			}
			m.openPicker(false)
			return
		}
	}
	shown := m.rel(path)
	real, err := filepath.EvalSymlinks(path)
	if err == nil {
		st, err = os.Stat(real)
	}
	if err != nil || !st.Mode().IsRegular() {
		m.msg = "not a regular file: " + shown
		return
	}
	ext := strings.ToLower(filepath.Ext(path))
	if rext := strings.ToLower(filepath.Ext(real)); rext != ext {
		m.msg = "link points elsewhere: " + shown + " → " + m.rel(real)
		return
	}
	switch {
	case render.MarkdownExt[ext]:
		if path == m.abs(m.path) {
			if frag != "" {
				m.anchorJump(frag)
			}
			return
		}
		if err := m.load(path); err != nil {
			m.msg = errText(err)
			return
		}
		if frag != "" {
			m.jump(frag)
		}
	case render.DocumentExt[ext] && st.Mode()&0o111 == 0:
		m.external(real, shown)
	default:
		m.msg = "not a Markdown file: " + shown
	}
}

func (m *model) external(target, label string) {
	if m.opt.Open == nil {
		m.msg = "cannot open " + label
		return
	}
	if err := m.opt.Open(target); err != nil {
		m.msg = errText(err)
		return
	}
	m.msg = "opened " + label
}

// pickedRoot is the root of the picker on screen, if any.
func (m *model) pickedRoot() string {
	if m.picking {
		return m.pick.root
	}
	return ""
}

// abs returns the absolute form of a page path ("" stays "").
func (m *model) abs(p string) string {
	if p == "" {
		return ""
	}
	if a, err := filepath.Abs(p); err == nil {
		return a
	}
	return p
}

// rel shortens path for display, relative to the current page's directory.
func (m *model) rel(path string) string {
	dir := "."
	if m.path != "" {
		dir = filepath.Dir(m.abs(m.path))
	}
	if d, err := filepath.Abs(dir); err == nil {
		if r, err := filepath.Rel(d, path); err == nil && !strings.HasPrefix(r, "../../..") {
			path = r
		}
	}
	return clip(path, 60)
}

// search finds every occurrence of q (case-insensitive unless q has an
// uppercase letter) and moves to the first one from the top of the screen.
func (m *model) search(q string) {
	m.query, m.matches, m.cur = q, nil, -1
	if q == "" {
		return
	}
	fold := strings.ToLower(q) == q
	needle := []rune(q)
	if m.plain == nil {
		m.plain = make([]string, len(m.page.Lines))
		for i, l := range m.page.Lines {
			m.plain[i] = stripEscapes(l)
		}
	}
	for i, l := range m.plain {
		if len(m.matches) >= maxMatches {
			break
		}
		// cheap pre-filter; columns are only computed on lines with a hit
		// (strings.ToLower folds rune by rune, like hasAt)
		if fold && !strings.Contains(strings.ToLower(l), q) || !fold && !strings.Contains(l, q) {
			continue
		}
		rs := []rune(l)
		col := 0
		for j := 0; j+len(needle) <= len(rs) && len(m.matches) < maxMatches; j++ {
			if hasAt(rs, j, needle, fold) {
				w := 0
				for _, r := range rs[j : j+len(needle)] {
					w += styled.RuneWidth(r)
				}
				m.matches = append(m.matches, match{i, col, w})
			}
			col += styled.RuneWidth(rs[j])
		}
	}
	if len(m.matches) == 0 {
		m.msg = "not found: " + clip(q, 40)
		return
	}
	for i, mt := range m.matches {
		if mt.line >= m.top {
			m.gotoMatch(i)
			return
		}
	}
	m.gotoMatch(0)
}

func hasAt(rs []rune, j int, needle []rune, fold bool) bool {
	for k, n := range needle {
		r := rs[j+k]
		if fold {
			r = unicode.ToLower(r)
		}
		if r != n {
			return false
		}
	}
	return true
}

func (m *model) gotoMatch(i int) {
	n := len(m.matches)
	if n == 0 {
		if m.query != "" {
			m.msg = "not found: " + clip(m.query, 40)
		}
		return
	}
	m.cur = (i%n + n) % n
	m.show(m.matches[m.cur].line)
}

// stripEscapes removes CSI, OSC, DCS and APC sequences from a painted line.
func stripEscapes(s string) string {
	if strings.IndexByte(s, 0x1b) < 0 {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != 0x1b || i+1 >= len(s) {
			b.WriteByte(s[i])
			continue
		}
		switch s[i+1] {
		case '[':
			j := i + 2
			for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
				j++
			}
			i = j
		case ']', 'P', '_':
			j := i + 2
			for j < len(s) && s[j] != 0x07 && (s[j] != 0x1b || j+1 >= len(s) || s[j+1] != '\\') {
				j++
			}
			if j < len(s) && s[j] == 0x1b {
				j++
			}
			i = j
		default:
			i++
		}
	}
	return b.String()
}

// clip cleans s and shortens it to n columns with an ellipsis, in one pass
// (link targets can be megabytes long).
func clip(s string, n int) string {
	s = render.Clean(s)
	keep, w := len(s), 0 // keep: where to cut so the ellipsis fits
	for i, r := range s {
		rw := styled.RuneWidth(r)
		if w+rw > n-1 && keep == len(s) {
			keep = i
		}
		if w += rw; w > n {
			return s[:keep] + "…"
		}
	}
	return s
}

func errText(err error) string {
	var pe *os.PathError
	if errors.As(err, &pe) {
		return clip(pe.Err.Error()+": "+filepath.Base(pe.Path), 70)
	}
	return clip(err.Error(), 70)
}
