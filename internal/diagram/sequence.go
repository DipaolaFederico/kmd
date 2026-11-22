package diagram

import (
	"strconv"
	"strings"

	"github.com/DipaolaFederico/kmd/internal/styled"
)

// Sequence event kinds.
const (
	evMsg = iota
	evNote
	evOpen
	evElse
	evClose
)

// Message head decorations.
const (
	headNone = iota
	headFilled
	headCross
	headOpen
)

type seqPart struct {
	id, label string
	actor     bool
}

type seqEvent struct {
	kind     int
	from, to int // participants; notes span from..to
	text     string
	dotted   bool
	head     int
	bidir    bool
	notePos  byte // 'l', 'r' or 'o'
	block    string
	frame    bool
}

// seqDiagram is a parsed Mermaid sequence diagram.
type seqDiagram struct {
	title  string
	parts  []seqPart
	index  map[string]int
	events []seqEvent
}

// part returns the index of participant id, declaring it implicitly.
func (d *seqDiagram) part(id string) (int, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return 0, false
	}
	if i, ok := d.index[id]; ok {
		return i, true
	}
	if len(d.parts) >= maxParts {
		return 0, false
	}
	d.index[id] = len(d.parts)
	d.parts = append(d.parts, seqPart{id: id, label: cleanLabel(id)})
	return len(d.parts) - 1, true
}

func (d *seqDiagram) declare(rest string, actor bool) bool {
	id, label, hasAlias := strings.Cut(rest, " as ")
	id = unquote(strings.TrimSpace(id))
	i, ok := d.part(id)
	if !ok {
		return false
	}
	if hasAlias {
		d.parts[i].label = cleanLabel(unquote(strings.TrimSpace(label)))
	} else {
		d.parts[i].label = cleanLabel(id)
	}
	d.parts[i].actor = actor
	return true
}

func (d *seqDiagram) add(ev seqEvent) bool {
	if len(d.events) >= maxEvents {
		return false
	}
	d.events = append(d.events, ev)
	return true
}

// seqArrows lists message operators, longest first.
var seqArrows = [...]string{"<<-->>", "<<->>", "-->>", "->>", "--x", "-x", "--)", "-)", "-->", "->"}

// parseSequence parses a Mermaid sequence diagram body.
func parseSequence(body []string) (*seqDiagram, bool) {
	d := &seqDiagram{index: make(map[string]int)}
	var stack []int
	autonumber, num := false, 0
	for _, line := range body {
		line = strings.TrimSpace(strings.TrimSuffix(line, ";"))
		kw, rest := splitWord(line)
		if k, ok := strings.CutSuffix(kw, ":"); ok && (k == "title" || strings.HasPrefix(k, "acc")) {
			kw = k
		}
		switch kw {
		case "participant", "actor":
			if !d.declare(rest, kw == "actor") {
				return nil, false
			}
			continue
		case "create":
			k2, r2 := splitWord(rest)
			if (k2 != "participant" && k2 != "actor") || !d.declare(r2, k2 == "actor") {
				return nil, false
			}
			continue
		case "destroy", "activate", "deactivate", "links", "link", "properties", "details", "accTitle", "accDescr":
			continue
		case "autonumber":
			autonumber = true
			continue
		case "title":
			d.title = cleanLabel(strings.TrimSpace(strings.TrimPrefix(rest, ":")))
			continue
		case "loop", "alt", "opt", "par", "critical", "break", "rect", "box":
			stack = append(stack, len(d.events))
			frame := kw != "rect" && kw != "box"
			if len(stack) > maxDepth || !d.add(seqEvent{kind: evOpen, block: kw, text: cleanLabel(rest), frame: frame}) {
				return nil, false
			}
			continue
		case "else", "and", "option":
			if len(stack) == 0 || !d.add(seqEvent{kind: evElse, block: kw, text: cleanLabel(rest), frame: d.events[stack[len(stack)-1]].frame}) {
				return nil, false
			}
			continue
		case "end":
			if len(stack) == 0 {
				return nil, false
			}
			open := d.events[stack[len(stack)-1]]
			stack = stack[:len(stack)-1]
			if !d.add(seqEvent{kind: evClose, frame: open.frame}) {
				return nil, false
			}
			continue
		}
		if strings.EqualFold(kw, "note") {
			if !d.note(rest) {
				return nil, false
			}
			continue
		}
		ev, ok := d.message(line)
		if !ok {
			return nil, false
		}
		if autonumber {
			num++
			ev.text = strings.TrimSpace(strconv.Itoa(num) + ". " + ev.text)
		}
		if !d.add(ev) {
			return nil, false
		}
	}
	for len(stack) > 0 {
		stack = stack[:len(stack)-1]
		if !d.add(seqEvent{kind: evClose, frame: true}) {
			return nil, false
		}
	}
	return d, len(d.parts) > 0
}

// note parses `left of A: text`, `right of A: text` or `over A[,B]: text`.
func (d *seqDiagram) note(rest string) bool {
	where, text, ok := strings.Cut(rest, ":")
	if !ok {
		return false
	}
	ev := seqEvent{kind: evNote, text: cleanLabel(strings.TrimSpace(text))}
	low := strings.ToLower(strings.TrimSpace(where))
	var targets string
	switch {
	case strings.HasPrefix(low, "left of "):
		ev.notePos, targets = 'l', where[strings.Index(low, "of ")+3:]
	case strings.HasPrefix(low, "right of "):
		ev.notePos, targets = 'r', where[strings.Index(low, "of ")+3:]
	case strings.HasPrefix(low, "over "):
		ev.notePos, targets = 'o', where[strings.Index(low, "over ")+5:]
	default:
		return false
	}
	a, b, two := strings.Cut(targets, ",")
	i, ok := d.part(a)
	if !ok {
		return false
	}
	ev.from, ev.to = i, i
	if two && ev.notePos == 'o' {
		j, ok := d.part(b)
		if !ok {
			return false
		}
		ev.from, ev.to = min(i, j), max(i, j)
	}
	return d.add(ev)
}

// message parses `A->>B: text` style lines.
func (d *seqDiagram) message(line string) (seqEvent, bool) {
	for i := 1; i < len(line); i++ {
		if line[i] != '-' && line[i] != '<' {
			continue
		}
		op := ""
		for _, a := range seqArrows {
			if strings.HasPrefix(line[i:], a) {
				op = a
				break
			}
		}
		if op == "" {
			continue
		}
		rest := strings.TrimLeft(line[i+len(op):], " ")
		rest = strings.TrimLeft(rest, "+-")
		to, text, _ := strings.Cut(rest, ":")
		a, ok1 := d.part(line[:i])
		b, ok2 := d.part(to)
		if !ok1 || !ok2 {
			return seqEvent{}, false
		}
		ev := seqEvent{kind: evMsg, from: a, to: b, text: cleanLabel(strings.TrimSpace(text)), dotted: strings.Contains(op, "--")}
		switch {
		case strings.HasPrefix(op, "<<"):
			ev.head, ev.bidir = headFilled, true
		case strings.HasSuffix(op, ">>"):
			ev.head = headFilled
		case strings.HasSuffix(op, "x"):
			ev.head = headCross
		case strings.HasSuffix(op, ")"):
			ev.head = headOpen
		}
		return ev, true
	}
	return seqEvent{}, false
}

// seqLayout holds the geometry of one sequence rendering attempt.
type seqLayout struct {
	d      *seqDiagram
	lines  [][]string // participant label lines
	bw     []int      // participant box widths
	bh     int        // participant box height
	c      []int      // lifeline columns
	text   [][]string // wrapped event text
	left   []int      // frame extents for evOpen events (also used by else/close)
	right  []int
	owner  []int // for else/close: index of the opening event
	rows   []int // rows each event takes
	bottom bool  // repeat participant boxes at the bottom
	minX   int
	maxX   int
}

// renderSequence tries progressively narrower settings until it fits.
func renderSequence(d *seqDiagram, width int) ([]styled.Line, bool) {
	// {participant wrap, text wrap, gap}: message text is squeezed before
	// participant names, which are short and read badly when broken.
	for _, st := range [...][3]int{{28, 28, 3}, {20, 20, 2}, {16, 14, 1}, {14, 10, 1}, {12, 8, 1}, {8, 6, 1}, {6, 4, 1}} {
		if lines, ok := d.render(st[0], st[1], st[2], width); ok {
			return withTitle(d.title, lines, width), true
		}
	}
	return nil, false
}

// noteWidth is the outer width of a note box for the given text.
func noteWidth(lines []string) int { return maxWidth(lines) + 4 }

func (d *seqDiagram) render(partMax, labelMax, gap, width int) ([]styled.Line, bool) {
	L := &seqLayout{d: d}
	np := len(d.parts)
	L.lines = make([][]string, np)
	L.bw = make([]int, np)
	for i, p := range d.parts {
		L.lines[i] = wrap(p.label, partMax)
		L.bw[i] = maxWidth(L.lines[i]) + 4
		L.bh = max(L.bh, len(L.lines[i])+2)
	}
	L.text = make([][]string, len(d.events))
	type cons struct{ i, j, d int }
	var cs []cons
	leftNeed, rightNeed := 0, 0
	need := func(i, j, dist int) {
		switch {
		case i < 0:
			leftNeed = max(leftNeed, dist)
		case j >= np:
			rightNeed = max(rightNeed, dist)
		default:
			cs = append(cs, cons{i, j, dist})
		}
	}
	for k, ev := range d.events {
		switch ev.kind {
		case evMsg:
			L.text[k] = wrap(ev.text, labelMax)
			lw := maxWidth(L.text[k])
			if ev.from == ev.to {
				need(ev.from, ev.from+1, lw+7)
			} else {
				need(min(ev.from, ev.to), max(ev.from, ev.to), max(lw+4, 6))
			}
		case evNote:
			L.text[k] = wrap(ev.text, labelMax)
			nw := noteWidth(L.text[k])
			switch {
			case ev.notePos == 'r':
				need(ev.from, ev.from+1, nw+4)
			case ev.notePos == 'l':
				need(ev.from-1, ev.from, nw+4)
			case ev.from == ev.to:
				need(ev.from-1, ev.from, nw/2+3)
				need(ev.from, ev.from+1, nw-nw/2+2)
			default:
				need(ev.from, ev.to, nw-4)
			}
		case evOpen, evElse:
			L.text[k] = []string{truncate(oneLine(ev.text), labelMax)}
		}
	}
	L.c = make([]int, np)
	L.c[0] = max(L.bw[0]/2, leftNeed)
	for k := 1; k < np; k++ {
		L.c[k] = L.c[k-1] + (L.bw[k-1] - 1 - L.bw[k-1]/2) + gap + L.bw[k]/2 + 1
		for _, c := range cs {
			if c.j == k {
				L.c[k] = max(L.c[k], L.c[c.i]+c.d)
			}
		}
	}
	last := np - 1
	L.minX = 0
	L.maxX = max(L.c[last]+L.bw[last]-1-L.bw[last]/2, L.c[last]+rightNeed-1)
	L.frames()
	w := L.maxX - L.minX + 1
	if w > width || w*L.height() > maxCells {
		return nil, false
	}
	return L.draw(w), true
}

// extentOf returns the horizontal span an event draws over.
func (L *seqLayout) extentOf(k int) (int, int) {
	ev := L.d.events[k]
	switch ev.kind {
	case evMsg:
		if ev.from == ev.to {
			return L.c[ev.from], L.c[ev.from] + 5 + maxWidth(L.text[k])
		}
		return L.c[min(ev.from, ev.to)], L.c[max(ev.from, ev.to)]
	case evNote:
		x0, x1 := L.noteSpan(k)
		return x0, x1
	}
	return L.c[0], L.c[0]
}

// noteSpan returns the outer columns of a note box.
func (L *seqLayout) noteSpan(k int) (int, int) {
	ev := L.d.events[k]
	nw := noteWidth(L.text[k])
	c := L.c[ev.from]
	switch {
	case ev.notePos == 'r':
		return c + 2, c + 1 + nw
	case ev.notePos == 'l':
		return c - 1 - nw, c - 2
	case ev.from == ev.to:
		return c - nw/2, c - nw/2 + nw - 1
	}
	x0 := c - 2
	return x0, max(L.c[ev.to]+2, x0+nw-1)
}

// frames computes the extent of every block frame from its contents and
// the overall drawing extent.
func (L *seqLayout) frames() {
	n := len(L.d.events)
	L.left = make([]int, n)
	L.right = make([]int, n)
	L.owner = make([]int, n)
	type open struct {
		k      int
		lo, hi int
		any    bool
	}
	var stack []open
	grow := func(lo, hi int) {
		if len(stack) > 0 {
			t := &stack[len(stack)-1]
			if !t.any {
				t.lo, t.hi, t.any = lo, hi, true
			}
			t.lo, t.hi = min(t.lo, lo), max(t.hi, hi)
		}
		L.minX, L.maxX = min(L.minX, lo), max(L.maxX, hi)
	}
	for k, ev := range L.d.events {
		switch ev.kind {
		case evOpen:
			stack = append(stack, open{k: k})
		case evElse:
			if len(stack) > 0 {
				L.owner[k] = stack[len(stack)-1].k
			}
		case evClose:
			if len(stack) == 0 {
				continue
			}
			t := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			L.owner[k] = t.k
			if !t.any {
				t.lo, t.hi = L.c[0], L.c[len(L.c)-1]
			}
			lo, hi := t.lo, t.hi
			if L.d.events[t.k].frame {
				lo, hi = lo-2, hi+2
				need := 0
				for j := t.k; j <= k; j++ {
					if e := L.d.events[j]; (e.kind == evOpen || e.kind == evElse) && (j == t.k || L.owner[j] == t.k) {
						need = max(need, styled.Width(e.block)+styled.Width(L.text[j][0])+8)
					}
				}
				hi = max(hi, lo+need)
			}
			L.left[t.k], L.right[t.k] = lo, hi
			grow(lo, hi)
		default:
			grow(L.extentOf(k))
		}
	}
}

// height computes the rows every event takes and returns the total canvas
// height including participant boxes.
func (L *seqLayout) height() int {
	d := L.d
	body := 0
	L.rows = make([]int, len(d.events))
	for k, ev := range d.events {
		n := len(L.text[k])
		switch {
		case ev.kind == evMsg && ev.from == ev.to:
			L.rows[k] = max(2, n+1)
		case ev.kind == evMsg:
			L.rows[k] = n + 1
			if ev.text == "" {
				L.rows[k] = 1
			}
		case ev.kind == evNote:
			L.rows[k] = n + 2
		case ev.frame:
			L.rows[k] = 1
		}
		body += L.rows[k]
	}
	L.bottom = body > 12
	h := L.bh + body + 1
	if L.bottom {
		h += L.bh - 1
	}
	return h
}

// draw renders the laid-out diagram into styled lines.
func (L *seqLayout) draw(w int) []styled.Line {
	d := L.d
	h := L.height()
	bottom := L.bottom
	c := newCanvas(w, h)
	off := -L.minX
	for i := range d.parts {
		for y := L.bh; y < h; y++ {
			c.put(L.c[i]+off, y, '┆', styled.Border)
		}
		L.drawPart(c, i, off, 0, true)
		if bottom {
			L.drawPart(c, i, off, h-L.bh, false)
		}
	}
	y := L.bh
	starts := make([]int, len(d.events))
	for k := range d.events {
		starts[k] = y
		y += L.rows[k]
	}
	// Frames first so that messages drawn later stay on top.
	for k, ev := range d.events {
		if !ev.frame || ev.kind < evOpen {
			continue
		}
		o := k
		if ev.kind != evOpen {
			o = L.owner[k]
		}
		x0, x1 := L.left[o]+off, L.right[o]+off
		yy := starts[k]
		switch ev.kind {
		case evOpen:
			c.put(x0, yy, '┌', styled.Border)
			c.fill(x0+1, yy, x1-x0-1, '─', styled.Border)
			c.put(x1, yy, '┐', styled.Border)
			x := c.text(x0+2, yy, " "+ev.block+" ", styled.Title)
			if t := L.text[k][0]; t != "" {
				c.text(x, yy, "["+t+"] ", styled.Label)
			}
		case evElse:
			c.put(x0, yy, '├', styled.Border)
			c.fill(x0+1, yy, x1-x0-1, '┄', styled.Border)
			c.put(x1, yy, '┤', styled.Border)
			if t := L.text[k][0]; t != "" {
				c.text(x0+2, yy, " ["+t+"] ", styled.Label)
			}
		case evClose:
			c.put(x0, yy, '└', styled.Border)
			c.fill(x0+1, yy, x1-x0-1, '─', styled.Border)
			c.put(x1, yy, '┘', styled.Border)
			for r := starts[o] + 1; r < yy; r++ {
				if ch := c.rune(x0, r); ch != '├' {
					c.put(x0, r, '│', styled.Border)
				}
				if ch := c.rune(x1, r); ch != '┤' {
					c.put(x1, r, '│', styled.Border)
				}
			}
		}
	}
	for k, ev := range d.events {
		switch ev.kind {
		case evMsg:
			L.drawMsg(c, k, off, starts[k])
		case evNote:
			x0, x1 := L.noteSpan(k)
			x0, x1 = x0+off, x1+off
			yy := starts[k]
			bottomY := yy + L.rows[k] - 1
			for r := yy; r <= bottomY; r++ {
				lch, rch, fill := '┆', '┆', ' '
				switch r {
				case yy:
					lch, rch, fill = '╭', '╮', '┄'
				case bottomY:
					lch, rch, fill = '╰', '╯', '┄'
				}
				c.put(x0, r, lch, styled.Border)
				c.fill(x0+1, r, x1-x0-1, fill, styled.Border)
				c.put(x1, r, rch, styled.Border)
			}
			for i, l := range L.text[k] {
				c.text(x0+2, yy+1+i, l, styled.Plain)
			}
		}
	}
	return c.lines()
}

// drawPart draws a participant box centered on its lifeline.
func (L *seqLayout) drawPart(c *canvas, i, off, y int, top bool) {
	b := box{shape: shapeRect, lines: L.lines[i], w: L.bw[i], h: L.bh}
	if L.d.parts[i].actor {
		b.shape = shapeRound
	}
	x := L.c[i] + off - L.bw[i]/2
	b.draw(c, x, y)
	cy, side := y+L.bh-1, uint8(bitDown)
	if !top {
		cy, side = y, bitUp
	}
	c.put(L.c[i]+off, cy, portGlyph(c.rune(L.c[i]+off, cy), side, styleSolid), styled.Border)
}

// drawMsg draws a message (label above an arrow) or a self-message loop.
func (L *seqLayout) drawMsg(c *canvas, k, off, y int) {
	ev := L.d.events[k]
	hz, vt := '─', '│'
	if ev.dotted {
		hz, vt = '┄', '┆'
	}
	lines := L.text[k]
	if ev.from == ev.to {
		x := L.c[ev.from] + off
		last := y + L.rows[k] - 1
		c.put(x+1, y, hz, styled.Edge)
		c.put(x+2, y, hz, styled.Edge)
		c.put(x+3, y, '╮', styled.Edge)
		for r := y + 1; r < last; r++ {
			c.put(x+3, r, vt, styled.Edge)
		}
		c.put(x+1, last, headGlyph(ev.head, false, hz), styled.Arrow)
		c.put(x+2, last, hz, styled.Edge)
		c.put(x+3, last, '╯', styled.Edge)
		for i, l := range lines {
			c.text(x+5, y+i, l, styled.Label)
		}
		return
	}
	lo, hi := L.c[min(ev.from, ev.to)]+off, L.c[max(ev.from, ev.to)]+off
	ay := y + L.rows[k] - 1
	if ev.text != "" {
		mid := (lo + hi + 1) / 2
		for i, l := range lines {
			c.text(mid-styled.Width(l)/2, y+i, l, styled.Label)
		}
	}
	c.fill(lo+1, ay, hi-lo-1, hz, styled.Edge)
	right := ev.to > ev.from
	if right || ev.bidir {
		c.put(hi-1, ay, headGlyph(ev.head, true, hz), styled.Arrow)
	}
	if !right || ev.bidir {
		c.put(lo+1, ay, headGlyph(ev.head, false, hz), styled.Arrow)
	}
}

// headGlyph returns the arrow head for a message pointing right or left.
func headGlyph(h int, right bool, line rune) rune {
	switch h {
	case headFilled:
		if right {
			return '▶'
		}
		return '◀'
	case headCross:
		return '✕'
	case headOpen:
		if right {
			return '▷'
		}
		return '◁'
	}
	return line
}
