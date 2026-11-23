package view

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/DipaolaFederico/kmd/internal/render"
	"github.com/DipaolaFederico/kmd/internal/styled"
)

// The picker is a full-screen list of every Markdown document under a
// folder, filtered as you type (o, or a folder without a README); Tab
// switches it to a text search across those documents (S). Enter opens
// the document, Backspace in the viewer comes back to the list.

// picker is the state of the list for one root folder; it survives while
// documents are read, so going back finds it as it was.
type picker struct {
	root string // absolute
	docs []doc  // sorted by name
	walk *task  // listing in progress
	note string // limits reached by the walk

	query []rune
	shown []int // indexes into docs, best match first
	cur   int   // selected row
	top   int   // first visible row

	text     bool // text search mode
	tquery   []rune
	searched string // query of hits
	hits     []hit
	search   *task // search in progress
	done     int   // documents searched so far
	tnote    string
}

// pickRoot is the folder the picker lists: the folder on screen, or the
// one kmd was started in.
func (m *model) pickRoot() string {
	switch {
	case m.path != "" && isDir(m.path):
		return m.abs(m.path)
	case m.opt.Root != "":
		return m.abs(m.opt.Root)
	}
	return ""
}

// openPicker shows the picker for the current root, listing it if needed;
// text opens it in text search mode.
func (m *model) openPicker(text bool) {
	if m.slides != nil {
		m.msg = "documents do not open during a presentation"
		return
	}
	root := m.pickRoot()
	if root == "" {
		m.msg = "no folder to list"
		return
	}
	m.showPicker(root)
	m.pick.text = text
}

// showPicker makes the picker of root visible (a new one if root changed).
func (m *model) showPicker(root string) {
	if m.pick == nil || m.pick.root != root {
		m.stopPicker()
		p := &picker{root: root}
		m.pick = p
		t := newTask()
		p.walk = t
		bg := m.bg
		go func() {
			note := walkTree(root, t.stopped, func(b []doc) bool {
				return t.send(bg, func(m *model) {
					if m.pick == p && p.walk == t {
						p.add(b)
					}
				})
			})
			t.send(bg, func(m *model) {
				if m.pick == p && p.walk == t {
					p.walk, p.note = nil, note
				}
			})
		}()
	}
	m.picking = true
}

// stopPicker cancels the picker's background work.
func (m *model) stopPicker() {
	if m.pick != nil {
		m.pick.walk.cancel()
		m.pick.search.cancel()
	}
}

// busy reports whether background work is pending.
func (m *model) busy() bool {
	return m.pick != nil && (m.pick.walk != nil || m.pick.search != nil)
}

// add merges a batch of documents found by the walk, keeping them sorted.
func (p *picker) add(b []doc) {
	sel := -1
	if p.cur < len(p.shown) {
		sel = p.shown[p.cur]
	}
	var keep string
	if sel >= 0 {
		keep = p.docs[sel].name
	}
	p.docs = append(p.docs, b...)
	sortDocs(p.docs)
	p.filter()
	if keep != "" { // stay on the selected document as the list grows
		for i, d := range p.shown {
			if p.docs[d].name == keep {
				p.cur = i
				break
			}
		}
	}
}

func sortDocs(ds []doc) {
	// insertion of a batch into a sorted list: a full sort is simplest and
	// a 10,000-name sort is ~1 ms, done at most once per batch of 256
	sort.Slice(ds, func(i, j int) bool { return ds[i].name < ds[j].name })
}

func (p *picker) filter() {
	p.shown = filterDocs(p.docs, newQuery(string(p.query)), p.shown)
	p.cur = min(p.cur, max(len(p.shown)-1, 0))
}

// rows is the number of entries in the current mode.
func (p *picker) rows() int {
	if p.text {
		return len(p.hits)
	}
	return len(p.shown)
}

// listH is how many rows of the list fit under its header.
func (m *model) listH() int { return max(m.viewH()-1, 1) }

func (m *model) handlePicker(k key) {
	p := m.pick
	name := k.name
	input := &p.query
	if p.text {
		input = &p.tquery
	}
	edited := false
	switch name {
	case "esc", "ctrl-g", "ctrl-c":
		switch {
		case p.search != nil:
			p.search.cancel()
			p.search, p.tnote = nil, "search cancelled"
		case len(*input) > 0 && name == "esc":
			*input, edited = nil, true
		default:
			m.picking = false
		}
	case "tab", "btab":
		p.text = !p.text
	case "up", "ctrl-p":
		p.cur--
	case "down", "ctrl-n":
		p.cur++
	case "pgup":
		p.cur -= m.listH() - 1
	case "pgdn":
		p.cur += m.listH() - 1
	case "home":
		p.cur = 0
	case "end":
		p.cur = p.rows() - 1
	case "enter":
		if p.text && (string(p.tquery) != p.searched || len(p.hits) == 0 && p.search == nil) {
			m.startSearch()
			return
		}
		m.pickOpen(p.cur)
	case "backspace":
		if len(*input) > 0 {
			*input, edited = (*input)[:len(*input)-1], true
		} else if !isDir(m.path) {
			m.picking = false // opened over a document: back to it
		}
	case "ctrl-u":
		*input, edited = nil, true
	case "char":
		if unicode.IsPrint(k.r) && len(*input) < 200 {
			*input, edited = append(*input, k.r), true
		}
	case "mouse":
		switch {
		case k.btn == 64:
			p.cur--
		case k.btn == 65:
			p.cur++
		case k.btn == 0 && k.press && k.y >= 1 && k.y <= m.listH():
			if i := p.top + k.y - 1; i < p.rows() {
				m.pickOpen(i)
				return
			}
		}
	}
	if edited && !p.text {
		p.cur, p.top = 0, 0
		p.filter()
	}
	p.cur = min(max(p.cur, 0), max(p.rows()-1, 0))
}

// startSearch searches the text of the listed documents in the background.
func (m *model) startSearch() {
	p := m.pick
	p.search.cancel()
	p.hits, p.cur, p.top, p.done, p.tnote = nil, 0, 0, 0, ""
	p.searched = string(p.tquery)
	if p.searched == "" {
		p.search = nil
		return
	}
	t := newTask()
	p.search = t
	docs, q, bg := p.docs, p.searched, m.bg
	go func() {
		note := searchDocs(docs, q, t.stopped, func(hs []hit, done int) bool {
			return t.send(bg, func(m *model) {
				if m.pick == p && p.search == t {
					p.hits = append(p.hits, hs...)
					p.done = done
				}
			})
		})
		t.send(bg, func(m *model) {
			if m.pick == p && p.search == t {
				p.search, p.tnote = nil, note
			}
		})
	}()
}

// pickOpen opens entry i of the list; a text match opens at its section
// with the query highlighted.
func (m *model) pickOpen(i int) {
	p := m.pick
	if i < 0 || i >= p.rows() {
		return
	}
	var h *hit
	path := ""
	if p.text {
		h = &p.hits[i]
		path = h.doc.path
	} else {
		path = p.docs[p.shown[i]].path
	}
	p.cur = i
	m.msg = ""
	m.followFile(path, "")
	if m.msg != "" { // could not open it: stay in the list
		return
	}
	m.picking = false
	if h == nil {
		return
	}
	if h.heading != "" {
		if line, ok := m.page.Anchors[render.Slug(h.heading)]; ok {
			m.top = line
			m.scroll(0)
		}
	}
	m.search(p.searched)
}

func (m *model) drawPicker(b *strings.Builder) {
	p := m.pick
	h := m.listH()
	n := p.rows()
	if p.cur < p.top {
		p.top = p.cur
	}
	if p.cur >= p.top+h {
		p.top = p.cur - h + 1
	}
	p.top = min(p.top, max(n-h, 0))

	var parts []string
	if p.text {
		switch {
		case p.searched == "":
			parts = append(parts, "type text and press Enter")
		default:
			parts = append(parts, plural(len(p.hits), "match", "matches"))
			if p.search != nil {
				parts = append(parts, "searching "+strconv.Itoa(p.done)+"/"+strconv.Itoa(len(p.docs))+"…")
			}
		}
		if p.tnote != "" {
			parts = append(parts, p.tnote)
		}
	} else {
		c := plural(len(p.docs), "document", "documents")
		if len(p.query) > 0 {
			c = strconv.Itoa(len(p.shown)) + "/" + c
		}
		parts = append(parts, c)
		if p.walk != nil {
			parts = append(parts, "listing…")
		}
	}
	if p.note != "" {
		parts = append(parts, p.note)
	}
	where := "in "
	if p.text {
		where = "search in "
	}
	head := " " + strings.Join(append(parts, where+rootName(p.root)), " · ")
	head = clip(head, m.cols)
	moveTo(b, 0, 0)
	b.WriteString(m.opt.Bar + head + strings.Repeat(" ", max(m.cols-styled.Width(head), 0)) + reset)

	q := newQuery(string(p.query))
	var pos []int
	for r := range h {
		moveTo(b, r+1, 0)
		i := p.top + r
		switch {
		case i < n && p.text:
			hi := p.hits[i]
			label := hi.doc.name + ":" + strconv.Itoa(hi.line)
			rest := "  "
			if hi.heading != "" {
				rest += hi.heading + " › "
			}
			rest += hi.text
			line := clip(" "+label+rest, m.cols)
			if i == p.cur {
				b.WriteString(m.opt.Select + line + strings.Repeat(" ", max(m.cols-styled.Width(line), 0)) + reset)
			} else {
				b.WriteString(line)
			}
		case i < n:
			name := p.docs[p.shown[i]].name
			_, pos, _ = fuzzy(name, q, pos[:0])
			b.WriteString(m.paintName(name, pos, i == p.cur))
		case i == 0 && r == 0:
			switch {
			case p.text && p.searched != "" && p.search == nil:
				b.WriteString(" no matches")
			case !p.text && p.walk == nil && len(p.query) > 0:
				b.WriteString(" no document matches")
			case !p.text && p.walk == nil:
				b.WriteString(" no Markdown documents here")
			}
		}
		b.WriteString(reset + "\x1b[K")
	}
}

// paintName draws a document name with its matched runes (byte offsets
// pos) highlighted; a name too wide keeps its end, the file name.
func (m *model) paintName(name string, pos []int, selected bool) string {
	w := m.cols - 2
	skip := 0
	if styled.Width(name) > w {
		over := styled.Width(name) - (w - 1)
		for i, r := range name {
			if over <= 0 {
				skip = i
				break
			}
			over -= styled.RuneWidth(r)
		}
	}
	base, hl := "", m.opt.Match
	if selected {
		base, hl = m.opt.Select, m.opt.Select+"\x1b[4m"
	}
	var b strings.Builder
	b.WriteString(base + " ")
	if skip > 0 {
		b.WriteString("…")
	}
	k := 0
	width := 1
	if skip > 0 {
		width++
	}
	for i, r := range name {
		for k < len(pos) && pos[k] < i {
			k++
		}
		if i < skip {
			continue
		}
		if k < len(pos) && pos[k] == i {
			b.WriteString(hl + string(r) + reset + base)
		} else {
			b.WriteRune(r)
		}
		width += styled.RuneWidth(r)
	}
	if selected {
		b.WriteString(strings.Repeat(" ", max(m.cols-width, 0)))
	}
	b.WriteString(reset)
	return b.String()
}

// rootName shows the root relative to the working directory when it is
// under it ("docs"), else in full.
func rootName(root string) string {
	if wd, err := os.Getwd(); err == nil {
		if r, err := filepath.Rel(wd, root); err == nil && !strings.HasPrefix(r, "..") {
			if r == "." {
				return "./"
			}
			return r + "/"
		}
	}
	return root
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}
