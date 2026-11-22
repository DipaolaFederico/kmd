package view

import (
	"os"
	"unicode"
)

// handle applies one input event to the model.
func (m *model) handle(k key) {
	if m.prompt {
		m.handlePrompt(k)
		return
	}
	if m.outline {
		m.msg = ""
		m.handleOutline(k)
		return
	}
	if m.help {
		m.help = false
		if k.name != "char" || k.r != 'q' {
			return
		}
	}
	m.msg = ""
	half := max(m.viewH()/2, 1)
	name := k.name
	if name == "char" {
		name = string(k.r)
	}
	switch name {
	case "q", "Q", "ctrl-c":
		m.quit = true
	case "j", "down", "ctrl-n", "ctrl-e":
		m.scroll(1)
	case "k", "up", "ctrl-p", "ctrl-y", "y":
		m.scroll(-1)
	case " ", "f", "pgdn", "ctrl-f", "ctrl-v":
		m.scroll(m.viewH() - 1)
	case "b", "pgup", "ctrl-b":
		m.scroll(-(m.viewH() - 1))
	case "d", "ctrl-d":
		m.scroll(half)
	case "u", "ctrl-u":
		m.scroll(-half)
	case "g", "<", "home":
		m.top = 0
	case "G", ">", "end":
		m.top = m.maxTop()
	case "tab":
		m.nextLink(1)
	case "btab":
		m.nextLink(-1)
	case "enter", "l", "right":
		if m.visible(m.sel) {
			m.follow(m.page.Links[m.sel].Href)
		} else if name == "enter" {
			m.scroll(1)
		} else {
			m.msg = "select a link with Tab first"
		}
	case "backspace", "h", "left":
		m.back()
	case "H":
		m.home()
	case "esc":
		m.sel, m.query, m.matches = -1, "", nil
	case "/":
		m.prompt, m.input = true, nil
	case "n":
		m.gotoMatch(m.cur + 1)
	case "N":
		m.gotoMatch(m.cur - 1)
	case "r", "ctrl-r":
		m.reload()
	case "ctrl-l":
		// redraw: every event redraws
	case "e":
		m.edit()
	case "?":
		m.help = true
	case "t":
		m.openOutline()
	case "]":
		m.nextHeading(1)
	case "[":
		m.nextHeading(-1)
	case "mouse":
		m.mouse(k)
	}
}

func (m *model) handlePrompt(k key) {
	switch k.name {
	case "enter":
		m.prompt, m.sel = false, -1
		m.search(string(m.input))
	case "esc", "ctrl-c", "ctrl-g":
		m.prompt = false
	case "backspace":
		if len(m.input) == 0 {
			m.prompt = false
		} else {
			m.input = m.input[:len(m.input)-1]
		}
	case "ctrl-u":
		m.input = nil
	case "char":
		if unicode.IsPrint(k.r) && len(m.input) < 200 {
			m.input = append(m.input, k.r)
		}
	}
}

func (m *model) mouse(k key) {
	switch {
	case k.btn == 64: // wheel up
		m.scroll(-3)
	case k.btn == 65:
		m.scroll(3)
	case k.btn == 0 && k.press:
		if k.y >= m.viewH() {
			return
		}
		if i := m.linkAt(k.x, k.y); i >= 0 {
			// Markdown and in-page links open at once; anything leaving kmd
			// (browser, mail, desktop apps) is selected first so its target
			// is shown, and opens on a second click or Enter
			if m.sel == i || m.internal(m.page.Links[i].Href) {
				m.sel = i
				m.follow(m.page.Links[i].Href)
				return
			}
			m.sel = i
		} else {
			m.sel = -1
		}
	}
}

func (m *model) edit() {
	switch {
	case m.path == "":
		m.msg = "only files can be edited (not stdin or several documents)"
	case isDir(m.path):
		m.msg = "a folder listing cannot be edited"
	case m.opt.Edit == nil:
		m.msg = "no editor available"
	default:
		if err := m.opt.Edit(m.path); err != nil {
			m.msg = errText(err)
		}
		m.reload()
	}
}

func isDir(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}
