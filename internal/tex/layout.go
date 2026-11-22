package tex

import (
	"strings"

	"github.com/DipaolaFederico/kmd/internal/styled"
)

// box is a rectangle of text: every row is exactly w columns wide and base
// is the index of the row that sits on the surrounding baseline.
type box struct {
	w, base int
	rows    []styled.Line
}

func (b box) h() int { return len(b.rows) }

const blanks = "                                                                "

func spaces(n int) string {
	if n <= len(blanks) {
		return blanks[:max(n, 0)]
	}
	return strings.Repeat(" ", n)
}

// single holds a one-span, one-row box in a single allocation.
type single struct {
	rows  [1]styled.Line
	spans [1]styled.Span
}

func textBox(s string, c styled.Class) box {
	o := &single{spans: [1]styled.Span{{Text: s, Class: c}}}
	o.rows[0] = o.spans[:]
	return box{w: styled.Width(s), rows: o.rows[:]}
}

func lineBox(l styled.Line) box {
	return box{w: styled.LineWidth(l), rows: []styled.Line{l}}
}

func blankBox(w, h int) box {
	if h == 1 && w > 0 {
		return textBox(spaces(w), styled.Plain)
	}
	rows := make([]styled.Line, h)
	if w > 0 {
		for i := range rows {
			rows[i] = styled.Line{{Text: spaces(w)}}
		}
	}
	return box{w: w, rows: rows}
}

// lineBuilder concatenates spans, coalescing runs of padding.
type lineBuilder struct {
	line styled.Line
	pad  int
}

func (lb *lineBuilder) space(n int) { lb.pad += n }

func (lb *lineBuilder) add(l styled.Line) {
	if len(l) == 0 {
		return
	}
	lb.flush()
	lb.line = append(lb.line, l...)
}

func (lb *lineBuilder) flush() {
	if lb.pad > 0 {
		lb.line = append(lb.line, styled.Span{Text: spaces(lb.pad)})
		lb.pad = 0
	}
}

func (lb *lineBuilder) done() styled.Line {
	lb.flush()
	l := lb.line
	lb.line = nil
	return l
}

// hjoin places parts side by side, part i starting at row tops[i]. Rows
// are shifted down so that no top is negative; the shift is returned.
func hjoin(parts []box, tops []int) (box, int) {
	lo := 0
	for _, t := range tops {
		lo = min(lo, t)
	}
	h, w := 0, 0
	for i, b := range parts {
		tops[i] -= lo
		h = max(h, tops[i]+b.h())
		w += b.w
	}
	rows := make([]styled.Line, h)
	var lb lineBuilder
	for r := range rows {
		lb.line = make(styled.Line, 0, len(parts))
		for i, b := range parts {
			if k := r - tops[i]; k >= 0 && k < b.h() {
				lb.add(b.rows[k])
			} else {
				lb.space(b.w)
			}
		}
		rows[r] = lb.done()
	}
	return box{w: w, rows: rows}, -lo
}

// hcat places parts side by side on a common baseline.
func hcat(parts ...box) box {
	top := 0
	for _, b := range parts {
		top = max(top, b.base)
	}
	tops := make([]int, len(parts))
	for i, b := range parts {
		tops[i] = top - b.base
	}
	r, _ := hjoin(parts, tops)
	r.base = top
	return r
}

const (
	alignLeft = iota
	alignCenter
	alignRight
)

// vstack stacks parts vertically, padded to the widest; base is 0.
func vstack(align int, parts ...box) box {
	w := 0
	for _, b := range parts {
		w = max(w, b.w)
	}
	var rows []styled.Line
	var lb lineBuilder
	for _, b := range parts {
		l := 0
		switch align {
		case alignCenter:
			l = (w - b.w) / 2
		case alignRight:
			l = w - b.w
		}
		for _, row := range b.rows {
			lb.space(l)
			lb.add(row)
			lb.space(w - b.w - l)
			rows = append(rows, lb.done())
		}
	}
	return box{w: w, rows: rows}
}

// maxRows is the tallest 2D layout attempted; taller formulas (or hostile
// input that would make layout expensive) are rendered linearly.
const maxRows = 100

// height returns an upper bound of the number of rows of layout(n),
// saturating above maxRows.
func height(n *node) int {
	h := 1
	switch n.kind {
	case kGroup:
		for _, k := range n.kids {
			h = max(h, height(k))
		}
	case kScript, kStack:
		h = height(n.a)
		for _, x := range [2]*node{n.b, n.c} {
			if x != nil {
				h += height(x)
			}
		}
	case kFrac:
		h = height(n.a) + height(n.b) + 1
	case kSqrt, kAccent, kBrace:
		h = height(n.a) + 1
	case kDelim, kPhantom:
		h = height(n.a)
	case kBoxed:
		h = height(n.a) + 2
	case kGrid:
		h = 0
		for _, row := range n.rows {
			rh := 1
			for _, c := range row {
				rh = max(rh, height(c))
			}
			h += rh + 1 // row and the gap below it
			if h > maxRows {
				break
			}
		}
	}
	return min(h, maxRows+1)
}

// layout renders n as a 2D box. Script is true inside scripts and limits,
// where TeX drops most automatic spacing.
func layout(n *node, script bool) box {
	switch n.kind {
	case kAtom, kText, kMiddle:
		return textBox(n.text, n.class)
	case kSpace:
		return blankBox(n.n, 1)
	case kGroup:
		return layoutRow(n.kids, script)
	case kScript:
		return layoutScript(n, script)
	case kFrac:
		return layoutFrac(n, script)
	case kSqrt:
		return layoutSqrt(n, script)
	case kDelim:
		return layoutDelim(n, script)
	case kGrid:
		return layoutGrid(n, script)
	case kAccent:
		return layoutAccent(n, script)
	case kBoxed:
		return layoutBoxed(n, script)
	case kStack:
		return layoutStack(n, script)
	case kBrace:
		return layoutBrace(n, script)
	case kPhantom:
		b := layout(n.a, script)
		r := blankBox(b.w, b.h())
		r.base = b.base
		return r
	}
	return blankBox(0, 1)
}

func layoutRow(kids []*node, script bool) box {
	if len(kids) == 0 {
		return blankBox(0, 1)
	}
	if len(kids) == 1 && kids[0].kind != kMiddle {
		return layout(kids[0], script)
	}
	var buf [16]atomType
	types := rowTypes(kids, false, buf[:0])
	parts := make([]box, 0, 2*len(kids))
	var mids []int
	for i, k := range kids {
		if i > 0 {
			if sp := spacing(types[i-1], types[i], script); sp > 0 {
				parts = append(parts, blankBox(sp, 1))
			}
		}
		if k.kind == kMiddle {
			mids = append(mids, len(parts))
			parts = append(parts, box{})
			continue
		}
		parts = append(parts, layout(k, script))
	}
	if len(mids) > 0 { // \middle grows to the height of the rest of the row
		above, below := 0, 0
		for _, b := range parts {
			if b.h() > 0 {
				above, below = max(above, b.base), max(below, b.h()-1-b.base)
			}
		}
		for j, m := range mids {
			parts[m] = delimBox(kids[midIndex(kids, j)].text, above+below+1, above)
		}
	}
	return hcat(parts...)
}

// midIndex returns the index in kids of the j-th \middle.
func midIndex(kids []*node, j int) int {
	for i, k := range kids {
		if k.kind == kMiddle {
			if j == 0 {
				return i
			}
			j--
		}
	}
	return 0
}

func layoutScript(n *node, script bool) box {
	base := layout(n.a, script)
	sup, sub := nonEmpty(n.b), nonEmpty(n.c)
	if n.a.limits && !script { // limits above and below
		var parts []box
		b := base.base
		if sup != nil {
			s := layout(sup, true)
			parts = append(parts, s)
			b += s.h()
		}
		parts = append(parts, base)
		if sub != nil {
			parts = append(parts, layout(sub, true))
		}
		r := vstack(alignCenter, parts...)
		r.base = b
		return r
	}
	if base.h() == 1 {
		if flat, ok := flatScripts(sub, sup); ok {
			return hcat(base, lineBox(flat))
		}
	}
	h := base.h()
	supTop, subTop := -1, 1 // relative to the top of the base
	if h > 1 {
		supTop, subTop = 0, h-1
	}
	// Raised and lowered scripts are written linearly (x^{1/2} is "1/2"
	// one row up), which keeps them compact.
	var col box
	top := supTop
	switch {
	case sup != nil && sub != nil:
		s, t := lineBox(renderLinear(sup, true)), lineBox(renderLinear(sub, true))
		col = vstack(alignLeft, s, blankBox(0, subTop-supTop-1), t)
	case sup != nil:
		col = lineBox(renderLinear(sup, true))
	default:
		col = lineBox(renderLinear(sub, true))
		top = subTop
	}
	r, shift := hjoin([]box{base, col}, []int{0, top})
	r.base = base.base + shift
	return r
}

// flatScripts renders scripts with Unicode script characters if possible.
func flatScripts(sub, sup *node) (styled.Line, bool) {
	var out styled.Line
	for i, n := range [2]*node{sub, sup} {
		if n == nil {
			continue
		}
		l, ok := convertScript(renderLinear(n, true), i == 1)
		if !ok {
			return nil, false
		}
		out = append(out, l...)
	}
	return out, true
}

func layoutFrac(n *node, script bool) box {
	num, den := layout(n.a, script), layout(n.b, script)
	if !n.bar {
		r := vstack(alignCenter, num, blankBox(0, 1), den)
		r.base = num.h()
		return hcat(delimBox("(", r.h(), r.base), r, delimBox(")", r.h(), r.base))
	}
	bar := textBox(strings.Repeat("─", max(num.w, den.w)+2), styled.Op)
	r := vstack(alignCenter, num, bar, den)
	r.base = num.h()
	return r
}

func layoutSqrt(n *node, script bool) box {
	body := layout(n.a, script)
	var idx styled.Line
	if x := nonEmpty(n.b); x != nil {
		idx = renderLinear(x, true)
	}
	sym := func(s string) styled.Line { return styled.Line{{Text: s, Class: styled.Symbol}} }
	var lb lineBuilder
	var r box
	if h := body.h(); h == 1 {
		sign := "√"
		switch plainText(idx) {
		case "2":
			idx = nil
		case "3":
			sign, idx = "∛", nil
		case "4":
			sign, idx = "∜", nil
		}
		lb.space(1)
		lb.add(sym(strings.Repeat("_", body.w)))
		top := lb.done()
		lb.add(sym(sign))
		lb.add(body.rows[0])
		r = box{w: body.w + 1, base: 1, rows: []styled.Line{top, lb.done()}}
	} else {
		rows := make([]styled.Line, 0, h+1)
		rows = append(rows, sym(" ┌"+strings.Repeat("─", body.w)))
		for i, row := range body.rows {
			pre := " │"
			if i == h-1 {
				pre = "╲│"
			}
			lb.add(sym(pre))
			lb.add(row)
			rows = append(rows, lb.done())
		}
		r = box{w: body.w + 2, base: body.base + 1, rows: rows}
	}
	if len(idx) == 0 {
		return r
	}
	out, _ := hjoin([]box{lineBox(idx), r}, []int{0, 0})
	out.base = r.base
	return out
}

func layoutDelim(n *node, script bool) box {
	body := layout(n.a, script)
	inner := n.a
	for inner.kind == kGroup && len(inner.kids) == 1 {
		inner = inner.kids[0]
	}
	if inner.kind == kGrid { // breathing room between a matrix and its fences
		var parts []box
		if n.left != "" {
			parts = append(parts, blankBox(1, 1))
		}
		parts = append(parts, body)
		if n.right != "" {
			parts = append(parts, blankBox(1, 1))
		}
		b := body.base
		body, _ = hjoin(parts, make([]int, len(parts)))
		body.base = b
	}
	h := body.h()
	out, _ := hjoin([]box{delimBox(n.left, h, body.base), body, delimBox(n.right, h, body.base)}, []int{0, 0, 0})
	out.base = body.base
	return out
}

// delimBox draws delimiter d h rows tall from Unicode bracket pieces.
func delimBox(d string, h, base int) box {
	if d == "" {
		b := blankBox(0, h)
		b.base = base
		return b
	}
	if h <= 1 {
		return textBox(d, styled.Symbol)
	}
	top, ext, bot, mid := d, d, d, ""
	switch d {
	case "(":
		top, ext, bot = "⎛", "⎜", "⎝"
	case ")":
		top, ext, bot = "⎞", "⎟", "⎠"
	case "[":
		top, ext, bot = "⎡", "⎢", "⎣"
	case "]":
		top, ext, bot = "⎤", "⎥", "⎦"
	case "⌊":
		top, ext, bot = "⎢", "⎢", "⎣"
	case "⌋":
		top, ext, bot = "⎥", "⎥", "⎦"
	case "⌈":
		top, ext, bot = "⎡", "⎢", "⎢"
	case "⌉":
		top, ext, bot = "⎤", "⎥", "⎥"
	case "{":
		top, ext, bot, mid = "⎧", "⎪", "⎩", "⎨"
	case "}":
		top, ext, bot, mid = "⎫", "⎪", "⎭", "⎬"
	case "|":
		top, ext, bot = "│", "│", "│"
	case "⟨":
		top, ext, bot, mid = "╱", "", "╲", "⟨"
	case "⟩":
		top, ext, bot, mid = "╲", "", "╱", "⟩"
	case "/":
		top, ext, bot = "╱", "╱", "╱"
	case "\\":
		top, ext, bot = "╲", "╲", "╲"
	}
	rows := make([]styled.Line, h)
	for i := range rows {
		s := ext
		switch {
		case ext == "": // angle brackets: upper half one way, lower half the other
			switch {
			case 2*i < h-1:
				s = top
			case 2*i > h-1:
				s = bot
			default:
				s = mid
			}
		case i == 0:
			s = top
		case i == h-1:
			s = bot
		case mid != "" && i == (h-1)/2:
			s = mid
		}
		rows[i] = styled.Line{{Text: s, Class: styled.Symbol}}
	}
	return box{w: styled.Width(top), base: base, rows: rows}
}

func gridAlign(g *node, j int) int {
	switch g.n {
	case gCases:
		return alignLeft
	case gAlign:
		if j%2 == 0 {
			return alignRight
		}
		return alignLeft
	case gArray:
		if j < len(g.text) {
			switch g.text[j] {
			case 'l':
				return alignLeft
			case 'r':
				return alignRight
			}
		}
	}
	return alignCenter
}

func gridGap(style, j int) int {
	switch style {
	case gAlign:
		if j%2 == 0 {
			return 0
		}
	case gSubstack:
		return 1
	}
	return 2
}

func layoutGrid(g *node, script bool) box {
	sub := script || g.n == gSubstack
	ncol := 0
	for _, row := range g.rows {
		ncol = max(ncol, len(row))
	}
	colW := make([]int, ncol)
	above, below := make([]int, len(g.rows)), make([]int, len(g.rows))
	cells := make([][]box, len(g.rows))
	tall := false
	for i, row := range g.rows {
		cells[i] = make([]box, len(row))
		for j, c := range row {
			b := layout(c, sub)
			cells[i][j] = b
			colW[j] = max(colW[j], b.w)
			above[i], below[i] = max(above[i], b.base), max(below[i], b.h()-1-b.base)
		}
		tall = tall || above[i]+below[i] > 0
	}
	var rowBoxes []box
	for i, row := range cells {
		if i > 0 && tall && g.n != gSubstack {
			rowBoxes = append(rowBoxes, blankBox(0, 1))
		}
		parts := make([]box, 0, 2*ncol)
		tops := make([]int, 0, 2*ncol)
		for j := range ncol {
			if j > 0 {
				parts = append(parts, blankBox(gridGap(g.n, j-1), 1))
				tops = append(tops, 0)
			}
			b := blankBox(0, 1)
			if j < len(row) {
				b = row[j]
			}
			parts = append(parts, vstack(gridAlign(g, j), b, box{w: colW[j]}))
			tops = append(tops, above[i]-b.base)
		}
		rb, _ := hjoin(parts, tops)
		rowBoxes = append(rowBoxes, rb)
	}
	r := vstack(alignLeft, rowBoxes...)
	if len(g.rows) == 1 {
		r.base = above[0]
	} else {
		r.base = (r.h() - 1) / 2
	}
	if r.h() == 0 {
		return blankBox(0, 1)
	}
	return r
}

func layoutAccent(n *node, script bool) box {
	body := layout(n.a, script)
	if body.w == 0 {
		return body
	}
	mark := n.mark
	st := accentStyle(mark)
	if st == accStrike || body.w == 1 && body.h() == 1 {
		rows := make([]styled.Line, body.h())
		for i, row := range body.rows {
			l := append(styled.Line(nil), row...)
			for k := range l {
				l[k].Text = combineAll(l[k].Text, mark)
			}
			rows[i] = l
		}
		return box{w: body.w, base: body.base, rows: rows}
	}
	g := textBox(accentGlyph(mark, body.w), styled.Symbol)
	if st == accUnder {
		r := vstack(alignCenter, body, g)
		r.base = body.base
		return r
	}
	r := vstack(alignCenter, g, body)
	r.base = body.base + 1
	return r
}

func layoutBoxed(n *node, script bool) box {
	body := layout(n.a, script)
	hor := strings.Repeat("─", body.w+2)
	border := func(s string) styled.Line { return styled.Line{{Text: s, Class: styled.Border}} }
	rows := make([]styled.Line, 0, body.h()+2)
	rows = append(rows, border("┌"+hor+"┐"))
	var lb lineBuilder
	for _, row := range body.rows {
		lb.add(border("│ "))
		lb.add(row)
		lb.add(border(" │"))
		rows = append(rows, lb.done())
	}
	rows = append(rows, border("└"+hor+"┘"))
	return box{w: body.w + 4, base: body.base + 1, rows: rows}
}

func layoutStack(n *node, script bool) box {
	base := layout(n.a, script)
	var parts []box
	b := base.base
	var over, under box
	if x := nonEmpty(n.b); x != nil {
		over = layout(x, true)
		parts = append(parts, over)
		b += over.h()
	}
	if x := nonEmpty(n.c); x != nil {
		under = layout(x, true)
	}
	if n.n == 1 { // extensible arrow
		base = textBox(stretchArrow(n.a.text, max(over.w, under.w)+2), styled.Op)
	}
	parts = append(parts, base)
	if under.h() > 0 {
		parts = append(parts, under)
	}
	r := vstack(alignCenter, parts...)
	r.base = b
	return r
}

func layoutBrace(n *node, script bool) box {
	body := layout(n.a, script)
	w := body.w
	g := strings.Repeat("─", max(w, 1))
	if w >= 3 {
		l := (w - 3) / 2
		if n.n == 1 {
			g = "╭" + strings.Repeat("─", l) + "┴" + strings.Repeat("─", w-3-l) + "╮"
		} else {
			g = "╰" + strings.Repeat("─", l) + "┬" + strings.Repeat("─", w-3-l) + "╯"
		}
	}
	brace := textBox(g, styled.Symbol)
	if n.n == 1 {
		r := vstack(alignCenter, brace, body)
		r.base = body.base + 1
		return r
	}
	r := vstack(alignCenter, body, brace)
	r.base = body.base
	return r
}
