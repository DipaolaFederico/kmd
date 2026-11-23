package view

import (
	"strconv"
	"strings"

	"github.com/DipaolaFederico/kmd/internal/styled"
)

// The outline is a list of the document's headings drawn over the page:
// t opens it at the section being read, Enter jumps to the chosen heading.

// section returns the index of the heading of the section shown at the top
// of the screen (-1 before the first heading).
func (m *model) section() int {
	cur := -1
	for i, h := range m.page.Headings {
		if h.Line > m.top+1 { // +1: a heading under the blank top line is on top
			break
		}
		cur = i
	}
	return cur
}

func (m *model) openOutline() {
	if len(m.page.Headings) == 0 {
		m.msg = "no headings in this document"
		return
	}
	m.outline = true
	m.ocur = max(m.section(), 0)
	m.otop = 0
}

// outlineRows is how many headings fit in the outline box.
func (m *model) outlineRows() int { return max(min(len(m.page.Headings), m.viewH()-2), 1) }

func (m *model) handleOutline(k key) {
	name := k.name
	if name == "char" {
		name = string(k.r)
	}
	n := len(m.page.Headings)
	switch name {
	case "esc", "t", "q", "ctrl-c", "ctrl-g":
		m.outline = false
	case "j", "down", "ctrl-n", "tab":
		m.ocur = min(m.ocur+1, n-1)
	case "k", "up", "ctrl-p", "btab":
		m.ocur = max(m.ocur-1, 0)
	case " ", "pgdn", "ctrl-f", "ctrl-d":
		m.ocur = min(m.ocur+m.outlineRows()-1, n-1)
	case "b", "pgup", "ctrl-b", "ctrl-u":
		m.ocur = max(m.ocur-m.outlineRows()+1, 0)
	case "g", "home":
		m.ocur = 0
	case "G", "end":
		m.ocur = n - 1
	case "enter", "l", "right":
		m.outline = false
		m.gotoHeading(m.ocur, true)
	case "mouse":
		m.outlineMouse(k)
	}
}

// outlineMouse: the wheel moves the cursor, a click on a heading jumps.
func (m *model) outlineMouse(k key) {
	switch {
	case k.btn == 64:
		m.ocur = max(m.ocur-1, 0)
	case k.btn == 65:
		m.ocur = min(m.ocur+1, len(m.page.Headings)-1)
	case k.btn == 0 && k.press:
		top, left, w := m.outlineBox()
		if i := m.otop + k.y - top - 1; k.y > top && k.y <= top+m.outlineRows() && k.x >= left && k.x < left+w &&
			i < len(m.page.Headings) {
			m.outline = false
			m.gotoHeading(i, true)
			return
		}
		m.outline = false
	}
}

// gotoHeading scrolls to heading i; record keeps the position for back.
func (m *model) gotoHeading(i int, record bool) {
	if i < 0 || i >= len(m.page.Headings) {
		return
	}
	if record {
		m.hist = append(m.hist, entry{m.path, m.top, m.sel, ""})
	}
	m.top, m.sel = m.page.Headings[i].Line, -1
	m.scroll(0)
}

// nextHeading moves to the next (dir 1) or previous (dir -1) heading.
func (m *model) nextHeading(dir int) {
	hs := m.page.Headings
	if len(hs) == 0 {
		m.msg = "no headings in this document"
		return
	}
	if dir > 0 {
		for i, h := range hs {
			if h.Line > m.top+1 {
				if h.Line > m.maxTop() && m.top == m.maxTop() {
					break // already at the bottom: it cannot move to the top
				}
				m.gotoHeading(i, false)
				return
			}
		}
		m.msg = "last section"
		return
	}
	for i := len(hs) - 1; i >= 0; i-- {
		if hs[i].Line < m.top {
			m.gotoHeading(i, false)
			return
		}
	}
	m.msg = "first section"
}

// outlineBox is the position of the outline box: top row, left column, width.
func (m *model) outlineBox() (top, left, w int) {
	w = min(max(m.cols*2/3, 30), m.cols)
	top = max((m.viewH()-m.outlineRows()-2)/2, 0)
	return top, max((m.cols-w)/2, 0), w
}

func (m *model) drawOutline(b *strings.Builder) {
	hs := m.page.Headings
	rows := m.outlineRows()
	if m.ocur < m.otop {
		m.otop = m.ocur
	}
	if m.ocur >= m.otop+rows {
		m.otop = m.ocur - rows + 1
	}
	top, left, w := m.outlineBox()
	minLevel := 6
	for _, h := range hs {
		minLevel = min(minLevel, h.Level)
	}
	line := func(row int, st, s string) {
		if row >= m.viewH() {
			return
		}
		moveTo(b, row, left)
		b.WriteString(st + s + reset)
	}
	title := " Contents " + strconv.Itoa(m.ocur+1) + "/" + strconv.Itoa(len(hs)) + " "
	if styled.Width(title) > w-4 {
		title = ""
	}
	line(top, m.opt.Bar, "╭"+title+strings.Repeat("─", max(w-2-styled.Width(title), 0))+"╮")
	for r := range rows {
		i := m.otop + r
		text := ""
		if i < len(hs) {
			indent := strings.Repeat("  ", min(max(hs[i].Level-minLevel, 0), 5))
			text = clip(indent+hs[i].Text, max(w-4, 1))
		}
		pad := strings.Repeat(" ", max(w-4-styled.Width(text), 0))
		st := m.opt.Bar
		if i == m.ocur {
			st = m.opt.Select
		}
		moveTo(b, top+1+r, left)
		if top+1+r < m.viewH() {
			b.WriteString(m.opt.Bar + "│ " + reset + st + text + pad + reset + m.opt.Bar + " │" + reset)
		}
	}
	line(top+1+rows, m.opt.Bar, "╰"+strings.Repeat("─", max(w-2, 0))+"╯")
}
