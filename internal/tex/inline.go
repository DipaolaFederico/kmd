package tex

import (
	"strings"
	"unicode/utf8"

	"github.com/DipaolaFederico/kmd/internal/styled"
)

// typeOf returns the spacing class of n. Inline rendering writes digit
// fractions as ½ or ¹²⁄₃₅, which then behave like ordinary atoms.
func typeOf(n *node, inline bool) atomType {
	switch n.kind {
	case kSpace:
		return tSpace
	case kScript:
		if nonEmpty(n.a) == nil {
			return tOrd
		}
		return typeOf(n.a, inline)
	case kFrac:
		if inline && digitFrac(n) {
			return tOrd
		}
		return tInner
	case kDelim, kGrid, kSqrt, kAccent, kBoxed, kBrace, kText:
		return tOrd
	case kPhantom:
		return typeOf(n.a, inline)
	case kMiddle:
		return tRel
	}
	return n.typ
}

// rowTypes returns the spacing classes of a row, turning binary operators
// that have no left operand (or no right operand) into ordinary atoms, as
// TeX does for a unary minus.
func rowTypes(kids []*node, inline bool, buf []atomType) []atomType {
	t := append(buf[:0], make([]atomType, len(kids))...)
	last := -1
	for i, k := range kids {
		t[i] = typeOf(k, inline)
		if t[i] == tSpace {
			continue
		}
		if t[i] == tBin {
			if last < 0 {
				t[i] = tOrd
			} else {
				switch t[last] {
				case tBin, tOp, tBig, tRel, tOpen, tPunct:
					t[i] = tOrd
				}
			}
		}
		if last >= 0 && t[last] == tBin {
			switch t[i] {
			case tRel, tClose, tPunct:
				t[last] = tOrd
			}
		}
		last = i
	}
	if last >= 0 && t[last] == tBin {
		t[last] = tOrd
	}
	return t
}

// spacing returns the number of columns between adjacent items of classes
// l and r (a terminal rendition of TeX's inter-atom spacing table).
func spacing(l, r atomType, script bool) int {
	if l == tSpace || r == tSpace {
		return 0
	}
	if script {
		if (l == tOp || l == tBig) && (r == tOrd || r == tOp || r == tBig) ||
			l == tOrd && (r == tOp || r == tBig) {
			return 1
		}
		return 0
	}
	switch l {
	case tOrd:
		switch r {
		case tOp, tBig, tBin, tRel, tInner:
			return 1
		}
	case tOp:
		switch r {
		case tOrd, tOp, tBig, tRel, tInner:
			return 1
		}
	case tBig:
		switch r {
		case tClose, tPunct:
			return 0
		}
		return 1
	case tBin, tRel:
		switch r {
		case tOrd, tOp, tBig, tOpen, tInner:
			return 1
		}
	case tClose:
		switch r {
		case tOp, tBig, tBin, tRel, tInner:
			return 1
		}
	case tPunct:
		switch r {
		case tRel, tClose, tPunct:
			return 0
		}
		return 1
	case tInner:
		switch r {
		case tClose, tPunct:
			return 0
		}
		return 1
	}
	return 0
}

// linear renders a formula on one line.
type linear struct {
	out    styled.Line
	script bool
}

func renderLinear(n *node, script bool) styled.Line {
	r := linear{script: script}
	r.node(n)
	return r.out
}

func (r *linear) emit(s string, c styled.Class) {
	if s != "" {
		r.out = append(r.out, styled.Span{Text: s, Class: c})
	}
}

func (r *linear) add(l styled.Line) { r.out = append(r.out, l...) }

// sub renders n in place in the given script mode and returns where its
// output starts in r.out.
func (r *linear) sub(n *node, script bool) int {
	start, saved := len(r.out), r.script
	r.script = script
	r.node(n)
	r.script = saved
	return start
}

func (r *linear) paren(l styled.Line) {
	r.emit("(", styled.Symbol)
	r.add(l)
	r.emit(")", styled.Symbol)
}

func (r *linear) node(n *node) {
	switch n.kind {
	case kAtom, kText, kMiddle:
		r.emit(n.text, n.class)
	case kSpace:
		r.emit(spaces(n.n), styled.Plain)
	case kGroup:
		r.row(n.kids)
	case kScript, kStack:
		if n.a.kind == kBrace { // annotation of \overbrace or \underbrace
			r.part(n.a.a, true)
			r.marked(n.c, false)
			r.marked(n.b, true)
			return
		}
		r.node(n.a)
		r.script1(n.c, false)
		r.script1(n.b, true)
	case kBrace, kBoxed:
		r.node(n.a)
	case kFrac:
		r.frac(n)
	case kSqrt:
		r.sqrt(n)
	case kDelim:
		r.emit(n.left, styled.Symbol)
		r.node(n.a)
		r.emit(n.right, styled.Symbol)
	case kGrid:
		r.grid(n)
	case kAccent:
		r.accent(n)
	case kPhantom:
		start := r.sub(n.a, r.script)
		w := styled.LineWidth(r.out[start:])
		r.out = r.out[:start]
		r.emit(spaces(w), styled.Plain)
	}
}

func (r *linear) row(kids []*node) {
	var buf [16]atomType
	types := rowTypes(kids, true, buf[:0])
	for i, k := range kids {
		if i > 0 {
			r.emit(spaces(spacing(types[i-1], types[i], r.script)), styled.Plain)
		}
		r.node(k)
	}
}

// script1 writes a superscript or subscript: Unicode script characters when
// every character has one, else ^x or ^(…).
func (r *linear) script1(n *node, sup bool) {
	if n = nonEmpty(n); n == nil {
		return
	}
	start := r.sub(n, true)
	if convertInPlace(r.out[start:], sup) {
		return
	}
	l := append(styled.Line(nil), r.out[start:]...)
	r.out = r.out[:start]
	r.mark(l, sup)
}

// marked writes a script with an explicit ^ or _ marker.
func (r *linear) marked(n *node, sup bool) {
	if n = nonEmpty(n); n != nil {
		r.mark(renderLinear(n, true), sup)
	}
}

func (r *linear) mark(l styled.Line, sup bool) {
	if sup {
		r.emit("^", styled.Op)
	} else {
		r.emit("_", styled.Op)
	}
	if clusters(l) == 1 {
		r.add(l)
		return
	}
	r.paren(l)
}

// convertScript maps every character of l to its superscript (or
// subscript) form; ok is false if some character has none.
func convertScript(l styled.Line, sup bool) (styled.Line, bool) {
	out := make(styled.Line, 0, len(l))
	var b strings.Builder
	for _, s := range l {
		b.Reset()
		for _, c := range s.Text {
			m := subRune(c)
			if sup {
				m = supRune(c)
			}
			if m == 0 {
				return nil, false
			}
			b.WriteRune(m)
		}
		out = append(out, styled.Span{Text: b.String(), Class: s.Class})
	}
	return out, true
}

// convertInPlace is convertScript overwriting l; l is unchanged on failure.
func convertInPlace(l styled.Line, sup bool) bool {
	for _, s := range l {
		for _, c := range s.Text {
			if sup && supRune(c) == 0 || !sup && subRune(c) == 0 {
				return false
			}
		}
	}
	c, _ := convertScript(l, sup)
	copy(l, c)
	return true
}

// clusters counts the visible characters of l.
func clusters(l styled.Line) int {
	k := 0
	for _, s := range l {
		for _, c := range s.Text {
			if styled.RuneWidth(c) > 0 {
				k++
			}
		}
	}
	return k
}

// digitsOf returns the digits of a plain integer node, or "".
func digitsOf(n *node) string {
	for n.kind == kGroup && len(n.kids) == 1 {
		n = n.kids[0]
	}
	if n.kind != kAtom || n.class != styled.Num || n.text == "" {
		return ""
	}
	for i := 0; i < len(n.text); i++ {
		if !isDigit(n.text[i]) {
			return ""
		}
	}
	return n.text
}

func digitFrac(n *node) bool { return n.bar && digitsOf(n.a) != "" && digitsOf(n.b) != "" }

// simple reports whether n reads unambiguously next to a slash or a root
// sign without parentheses. Non-strict mode also accepts products such as
// n(n+1) or 2a.
func simple(n *node, strict bool) bool {
	for n.kind == kGroup && len(n.kids) == 1 {
		n = n.kids[0]
	}
	switch n.kind {
	case kAtom, kText:
		return n.typ != tBin && n.typ != tRel && n.typ != tBig && !strings.Contains(n.text, " ")
	case kScript:
		return simple(n.a, true)
	case kSqrt, kDelim, kGrid, kAccent, kBoxed, kPhantom, kBrace:
		return true
	case kFrac:
		return digitFrac(n)
	case kGroup:
		if strict || len(n.kids) == 0 {
			return len(n.kids) == 0
		}
		depth := 0 // anything goes inside parentheses: n(n+1)
		var buf [16]atomType
		for i, t := range rowTypes(n.kids, true, buf[:0]) {
			switch {
			case t == tOpen:
				depth++
			case t == tClose:
				depth--
			case depth > 0:
			case t != tOrd || n.kids[i].kind == kFrac && !digitFrac(n.kids[i]):
				return false
			}
		}
		return depth == 0
	}
	return false
}

func (r *linear) frac(n *node) {
	if !n.bar { // binomial
		a, b := renderLinear(n.a, true), renderLinear(n.b, true)
		if ca, ok := convertScript(a, true); ok {
			if cb, ok := convertScript(b, false); ok {
				r.paren(append(ca, cb...))
				return
			}
		}
		r.emit("C", styled.Func)
		r.emit("(", styled.Symbol)
		r.add(a)
		r.emit(", ", styled.Plain)
		r.add(b)
		r.emit(")", styled.Symbol)
		return
	}
	if digitFrac(n) {
		ns, ds := digitsOf(n.a), digitsOf(n.b)
		if v := vulgar(ns, ds); v != "" {
			r.emit(v, styled.Num)
			return
		}
		var b strings.Builder
		for _, c := range ns {
			b.WriteRune(supRune(c))
		}
		b.WriteString("⁄")
		for _, c := range ds {
			b.WriteRune(subRune(c))
		}
		r.emit(b.String(), styled.Num)
		return
	}
	r.part(n.a, false)
	r.emit("/", styled.Op)
	r.part(n.b, true)
}

// part writes n, parenthesized unless simple.
func (r *linear) part(n *node, strict bool) {
	if simple(n, strict) {
		r.node(n)
		return
	}
	r.emit("(", styled.Symbol)
	r.node(n)
	r.emit(")", styled.Symbol)
}

func (r *linear) sqrt(n *node) {
	sign := "√"
	if x := nonEmpty(n.b); x != nil {
		l := renderLinear(x, true)
		switch plainText(l) {
		case "2":
		case "3":
			sign = "∛"
		case "4":
			sign = "∜"
		default:
			if c, ok := convertScript(l, true); ok {
				r.add(c)
			} else {
				r.paren(l)
			}
		}
	}
	r.emit(sign, styled.Symbol)
	r.part(n.a, true)
}

func plainText(l styled.Line) string {
	if len(l) == 1 {
		return l[0].Text
	}
	var b strings.Builder
	for _, s := range l {
		b.WriteString(s.Text)
	}
	return b.String()
}

func (r *linear) grid(g *node) {
	colSep, rowSep := " ", "; "
	switch g.n {
	case gCases:
		colSep = ", "
	case gAlign, gGather:
		colSep = ""
	case gSubstack:
		rowSep = ", "
	}
	r.emit(g.left, styled.Symbol)
	for i, row := range g.rows {
		if i > 0 {
			r.emit(rowSep, styled.Plain)
		}
		for j, c := range row {
			if j > 0 {
				r.emit(colSep, styled.Plain)
			}
			r.node(c)
		}
	}
	r.emit(g.right, styled.Symbol)
}

func (r *linear) accent(n *node) {
	l := r.out[r.sub(n.a, r.script):]
	mark := n.mark
	switch st := accentStyle(mark); {
	case clusters(l) > 1 && (st == accLine || st == accUnder || st == accStrike):
		for i := range l {
			l[i].Text = combineAll(l[i].Text, mark)
		}
	default:
		for i := len(l) - 1; i >= 0; i-- {
			if l[i].Text != "" {
				l[i].Text += string(mark)
				break
			}
		}
	}
}

// combineAll puts the combining mark after every visible character of s.
func combineAll(s string, mark rune) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		c, size := utf8.DecodeRuneInString(s[i:])
		i += size
		b.WriteRune(c)
		for i < len(s) { // keep existing marks attached to their base
			d, dsize := utf8.DecodeRuneInString(s[i:])
			if styled.RuneWidth(d) > 0 {
				break
			}
			b.WriteRune(d)
			i += dsize
		}
		if styled.RuneWidth(c) > 0 {
			b.WriteRune(mark)
		}
	}
	return b.String()
}
