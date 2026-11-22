package tex

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/DipaolaFederico/kmd/internal/styled"
)

const (
	// maxSource is the largest formula rendered; longer input is shown verbatim.
	maxSource = 8 << 10
	// maxDepth bounds parser recursion (each brace level costs about two).
	maxDepth = 64
)

// atomType is the TeX math class of an item; it drives automatic spacing.
type atomType uint8

const (
	tOrd   atomType = iota // ordinary: variables, numbers, symbols
	tOp                    // named function (sin, lim)
	tBig                   // big operator (∑, ∫)
	tBin                   // binary operator (+, ×)
	tRel                   // relation (=, ≤, →)
	tOpen                  // opening delimiter
	tClose                 // closing delimiter
	tPunct                 // punctuation
	tInner                 // fraction-like construct
	tSpace                 // explicit space: no automatic spacing next to it
)

// kind is the shape of a node.
type kind uint8

const (
	kAtom    kind = iota // a symbol, a number, a function name
	kText                // \text{...} content (spaces preserved)
	kSpace               // explicit space of n columns
	kGroup               // a row of kids
	kScript              // a with superscript b and subscript c
	kFrac                // a over b (bar is false for binomials)
	kSqrt                // root of a with optional index b
	kDelim               // a between left and right delimiters
	kGrid                // environment: rows of cells
	kAccent              // a with the combining accent mark
	kBoxed               // a inside a frame
	kStack               // a with b above and c below (\overset, \xrightarrow)
	kBrace               // a with a horizontal brace, above when n == 1
	kPhantom             // blank space the size of a
	kMiddle              // \middle delimiter: stretches to the enclosing row
)

// Grid styles (node.n of a kGrid).
const (
	gMatrix   = iota // centered columns
	gCases           // left-aligned columns
	gAlign           // alternating right/left columns
	gGather          // centered, one formula per row
	gArray           // alignment from the column spec in node.text
	gSubstack        // compact centered rows (limits of big operators)
)

// node is an element of the parsed formula.
type node struct {
	kind    kind
	typ     atomType
	class   styled.Class
	limits  bool   // display mode puts scripts above and below
	bar     bool   // fraction bar (false for \binom)
	n       int    // space width, grid style, brace/stretchy flag
	mark    rune   // combining accent character (kAccent)
	text    string // atom text, array column spec
	left    string // delimiters (kDelim) or inline brackets (kGrid)
	right   string
	a, b, c *node
	kids    []*node
	rows    [][]*node
}

func atom(text string, typ atomType, c styled.Class) *node {
	return &node{kind: kAtom, text: text, typ: typ, class: c}
}

func spaceNode(n int) *node { return &node{kind: kSpace, typ: tSpace, n: n} }

// nonEmpty returns n unless it is nil or an empty group.
func nonEmpty(n *node) *node {
	if n == nil || n.kind == kGroup && len(n.kids) == 0 {
		return nil
	}
	return n
}

// Row terminators understood by parseRow.
const (
	stopBrace   = 1 << iota // '}' ends the row
	stopBracket             // ']' ends the row (optional arguments)
	stopCell                // '&', '\\' and \end end the row
	stopRight               // \right ends the row
)

// parser is a recursive-descent LaTeX math parser working on bytes. It never
// fails: malformed input degrades into something renderable. Only excessive
// nesting is reported (bad), so callers can show the source instead.
type parser struct {
	s     string
	i     int
	depth int
	bad   bool
	font  font
	tag   string
}

// parse returns the formula tree, the \tag text and false when the input is
// nested too deeply to be rendered.
func parse(src string) (*node, string, bool) {
	p := parser{s: src}
	rows := p.parseRows(0)
	for p.i < len(p.s) && !p.bad { // a stray \end ended the rows early
		at := p.i
		rows = append(rows, p.parseRows(0)...)
		if p.i == at {
			break
		}
	}
	if p.bad {
		return nil, "", false
	}
	if len(rows) > 1 { // drop rows left empty by stray \end or \\
		kept := rows[:0]
		for _, r := range rows {
			if len(r) > 1 || len(r[0].kids) > 0 {
				kept = append(kept, r)
			}
		}
		if len(kept) == 0 {
			kept = rows[:1]
		}
		rows = kept
	}
	if len(rows) == 1 && len(rows[0]) == 1 {
		return rows[0][0], p.tag, true
	}
	g := &node{kind: kGrid, n: gGather, rows: rows}
	for _, r := range rows {
		if len(r) > 1 {
			g.n = gAlign
			alignCells(rows)
			break
		}
	}
	return g, p.tag, true
}

func (p *parser) enter() bool {
	if p.bad {
		return false
	}
	if p.depth >= maxDepth {
		p.bad = true
		return false
	}
	p.depth++
	return true
}

func (p *parser) leave() { p.depth-- }

func isLetter(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }
func isDigit(c byte) bool  { return c >= '0' && c <= '9' }

// isControl reports runes that must never reach the terminal: C0/C1
// controls (escape sequences) and bidirectional overrides.
func isControl(r rune) bool {
	return r < 0x20 || r >= 0x7f && r <= 0x9f || r >= 0x200e && r <= 0x200f ||
		r >= 0x2028 && r <= 0x202e || r >= 0x2066 && r <= 0x2069 || r == 0x061c
}

// skipSpace skips whitespace, control characters and % comments.
func (p *parser) skipSpace() {
	for p.i < len(p.s) {
		c := p.s[p.i]
		switch {
		case c == '%':
			for p.i < len(p.s) && p.s[p.i] != '\n' {
				p.i++
			}
		case c <= ' ' || c == 0x7f:
			p.i++
		case c >= utf8.RuneSelf:
			r, size := utf8.DecodeRuneInString(p.s[p.i:])
			if !isControl(r) {
				return
			}
			p.i += size
		default:
			return
		}
	}
}

func (p *parser) peek(c byte) bool { return p.i < len(p.s) && p.s[p.i] == c }

// cmdAt returns the name of the control sequence starting at s[i] (a
// backslash) and the index after it.
func (p *parser) cmdAt(i int) (string, int) {
	j := i + 1
	if j >= len(p.s) {
		return "", j
	}
	if isLetter(p.s[j]) {
		k := j
		for k < len(p.s) && isLetter(p.s[k]) {
			k++
		}
		return p.s[j:k], k
	}
	r, size := utf8.DecodeRuneInString(p.s[j:])
	if isControl(r) {
		return "", j + size
	}
	return p.s[j : j+size], j + size
}

// star consumes an optional '*' after a command name.
func (p *parser) star() bool {
	if p.peek('*') {
		p.i++
		return true
	}
	return false
}

// parseRows parses '&'-separated cells and '\\'-separated rows up to \end
// (consumed), the end of input, or a terminator from extra (not consumed).
func (p *parser) parseRows(extra int) [][]*node {
	var rows [][]*node
	var cur []*node
	for {
		cur = append(cur, p.parseRow(stopCell|extra))
		if p.bad || p.i >= len(p.s) {
			break
		}
		if p.s[p.i] == '&' {
			p.i++
			continue
		}
		if p.s[p.i] != '\\' {
			break
		}
		name, next := p.cmdAt(p.i)
		if name == "end" {
			p.i = next
			p.rawArg()
			break
		}
		if name != "\\" && name != "cr" && name != "newline" {
			break
		}
		p.i = next
		p.skipDimen()
		rows = append(rows, cur)
		cur = nil
	}
	rows = append(rows, cur)
	if n := len(rows); n > 1 && len(rows[n-1]) == 1 && len(rows[n-1][0].kids) == 0 {
		rows = rows[:n-1] // trailing '\\'
	}
	return rows
}

// skipDimen skips the optional "[2pt]" after a row break.
func (p *parser) skipDimen() {
	if !p.peek('[') {
		return
	}
	for j := p.i + 1; j < len(p.s) && j < p.i+16; j++ {
		c := p.s[j]
		if c == ']' {
			p.i = j + 1
			return
		}
		if !(isDigit(c) || isLetter(c) || c == '.' || c == '-' || c == ' ') {
			return
		}
	}
}

// alignCells prepares aligned rows: a cell following an alignment point
// that starts with a relation gets an empty ordinary atom in front, so
// "x &= y" is spaced like "x = y".
func alignCells(rows [][]*node) {
	for _, row := range rows {
		for j := 1; j < len(row); j += 2 {
			c := row[j]
			if len(c.kids) == 0 {
				continue
			}
			if t := typeOf(c.kids[0], false); t == tRel || t == tBin {
				c.kids = append([]*node{atom("", tOrd, styled.Plain)}, c.kids...)
			}
		}
	}
}

// parseRow parses a sequence of items until a terminator selected by stop
// (not consumed) or the end of input. Stray terminators are skipped.
func (p *parser) parseRow(stop int) *node {
	g := &node{kind: kGroup}
	if !p.enter() {
		return g
	}
	defer p.leave()
	for !p.bad {
		p.skipSpace()
		if p.i >= len(p.s) {
			break
		}
		switch p.s[p.i] {
		case '}':
			if stop&stopBrace != 0 {
				return g
			}
			p.i++
			continue
		case ']':
			if stop&stopBracket != 0 {
				return g
			}
		case '&':
			if stop&stopCell != 0 {
				return g
			}
			p.i++
			continue
		case '\\':
			switch name, _ := p.cmdAt(p.i); name {
			case "\\", "cr", "newline", "end":
				if stop&stopCell != 0 {
					return g
				}
			case "right":
				if stop&stopRight != 0 {
					return g
				}
			}
		}
		at := p.i
		if n := p.scripts(p.parseAtom()); n != nil {
			g.kids = append(g.kids, n)
		}
		if p.i == at && !p.bad { // defensive: always make progress
			p.i++
		}
	}
	return g
}

// parseAtom parses one item without its scripts. It returns nil for items
// that render nothing and, without consuming, at '^', '_' and '\”.
func (p *parser) parseAtom() *node {
	if !p.enter() {
		return nil
	}
	defer p.leave()
	c := p.s[p.i]
	switch {
	case c == '{':
		return p.parseGroup()
	case c == '\\':
		name, next := p.cmdAt(p.i)
		p.i = next
		return p.command(name)
	case c == '^' || c == '_' || c == '\'':
		return nil
	case c == '~':
		p.i++
		return spaceNode(1)
	case isDigit(c) || c == '.' && p.i+1 < len(p.s) && isDigit(p.s[p.i+1]):
		start := p.i
		for p.i < len(p.s) && (isDigit(p.s[p.i]) || p.s[p.i] == '.' && p.i+1 < len(p.s) && isDigit(p.s[p.i+1])) {
			p.i++
		}
		return p.number(p.s[start:p.i])
	}
	r, size := utf8.DecodeRuneInString(p.s[p.i:])
	p.i += size
	return p.char(r)
}

func (p *parser) number(s string) *node {
	return atom(mapFont(p.font, s), tOrd, styled.Num)
}

// parseGroup parses a braced group; the opening brace is at p.i.
func (p *parser) parseGroup() *node {
	p.i++
	f := p.font
	g := p.parseRow(stopBrace)
	p.font = f
	if p.peek('}') {
		p.i++
	}
	return g
}

// parseArg parses a command argument: a braced group, a command with its
// own arguments, a single digit or a single character.
func (p *parser) parseArg() *node {
	p.skipSpace()
	if p.i >= len(p.s) {
		return &node{kind: kGroup}
	}
	switch c := p.s[p.i]; {
	case c == '{':
		return p.parseGroup()
	case c == '}' || c == '&' || c == '^' || c == '_':
		return &node{kind: kGroup}
	case isDigit(c):
		p.i++
		return p.number(p.s[p.i-1 : p.i])
	}
	if n := p.parseAtom(); n != nil {
		return n
	}
	return &node{kind: kGroup}
}

// scripts attaches any following superscripts, subscripts and primes to base.
// Repeated scripts are collected in one group so input like "x^^^^" cannot
// build deep trees.
func (p *parser) scripts(base *node) *node {
	var sn *node
	for !p.bad {
		p.skipSpace()
		if p.i >= len(p.s) {
			break
		}
		c := p.s[p.i]
		if c == '\\' {
			name, next := p.cmdAt(p.i)
			if name != "limits" && name != "nolimits" && name != "displaylimits" {
				break
			}
			p.i = next
			if base != nil && (base.typ == tOp || base.typ == tBig) {
				base.limits = name != "nolimits"
			}
			continue
		}
		if c != '^' && c != '_' && c != '\'' {
			break
		}
		if sn == nil {
			if base == nil {
				base = &node{kind: kGroup}
			}
			sn = &node{kind: kScript, a: base}
		}
		switch c {
		case '\'':
			k := 0
			for p.peek('\'') {
				p.i++
				k++
			}
			sn.b = appendTo(sn.b, atom(primes(k), tOrd, styled.Var))
		case '^':
			p.i++
			sn.b = appendTo(sn.b, p.parseArg())
		default:
			p.i++
			sn.c = appendTo(sn.c, p.parseArg())
		}
	}
	if sn == nil {
		return base
	}
	return sn
}

func appendTo(g, n *node) *node {
	if g == nil {
		return &node{kind: kGroup, kids: []*node{n}}
	}
	g.kids = append(g.kids, n)
	return g
}

func primes(k int) string {
	switch k {
	case 1:
		return "′"
	case 2:
		return "″"
	case 3:
		return "‴"
	case 4:
		return "⁗"
	}
	return strings.Repeat("′", k)
}

// char converts a literal character into an atom.
func (p *parser) char(r rune) *node {
	switch r {
	case '+':
		return atom("+", tBin, styled.Op)
	case '-':
		return atom("−", tBin, styled.Op)
	case '*':
		return atom("∗", tBin, styled.Op)
	case '=', '<', '>', ':':
		return atom(string(r), tRel, styled.Op)
	case ',', ';':
		return atom(string(r), tPunct, styled.Plain)
	case '(', '[':
		return atom(string(r), tOpen, styled.Symbol)
	case ')', ']':
		return atom(string(r), tClose, styled.Symbol)
	case '!', '?':
		return atom(string(r), tClose, styled.Plain)
	case '|':
		return atom("|", tOrd, styled.Symbol)
	case '/':
		return atom("/", tOrd, styled.Op)
	}
	if isControl(r) {
		return nil
	}
	if unicode.IsLetter(r) {
		return p.letter(r)
	}
	if unicode.IsDigit(r) {
		return atom(string(r), tOrd, styled.Num)
	}
	if typ, ok := unicodeType(r); ok {
		c := styled.Op
		if typ == tOrd || typ == tBig {
			c = styled.Symbol
		}
		return atom(string(r), typ, c)
	}
	return atom(string(r), tOrd, styled.Plain)
}

// letter returns a letter atom in the current font.
func (p *parser) letter(r rune) *node {
	m := fontRune(p.font, r)
	c := styled.Var
	switch p.font {
	case fRm, fBF, fSf, fTt:
		c = styled.Plain
	case fBB, fCal, fFrak:
		if m != r {
			c = styled.Symbol
		}
	}
	return atom(string(m), tOrd, c)
}

// rawArg returns the unparsed text of the next argument: the content of a
// braced group, or a single command or character.
func (p *parser) rawArg() string {
	p.skipSpace()
	if p.i >= len(p.s) || p.s[p.i] == '}' {
		return ""
	}
	start := p.i
	if p.s[p.i] != '{' {
		if p.s[p.i] == '\\' {
			_, p.i = p.cmdAt(p.i)
		} else {
			_, size := utf8.DecodeRuneInString(p.s[p.i:])
			p.i += size
		}
		return p.s[start:p.i]
	}
	depth := 0
	for j := p.i; j < len(p.s); j++ {
		switch p.s[j] {
		case '\\':
			j++
		case '{':
			depth++
		case '}':
			if depth--; depth == 0 {
				p.i = j + 1
				return p.s[start+1 : j]
			}
		}
	}
	p.i = len(p.s)
	return p.s[start+1:]
}

// rawText reads an argument in text mode.
func (p *parser) rawText() string { return textOf(p.rawArg()) }

// textOf converts text-mode LaTeX to plain text: escapes are resolved,
// symbol commands replaced, other commands and braces dropped.
func textOf(raw string) string {
	var b strings.Builder
	for i := 0; i < len(raw); {
		c := raw[i]
		switch {
		case c == '\\':
			j := i + 1
			if j >= len(raw) {
				i = j
				continue
			}
			if isLetter(raw[j]) {
				k := j
				for k < len(raw) && isLetter(raw[k]) {
					k++
				}
				if s, _, _, ok := symbol(raw[j:k]); ok {
					b.WriteString(s)
				}
				i = k
				continue
			}
			r, size := utf8.DecodeRuneInString(raw[j:])
			i = j + size
			switch r {
			case ',', ';', ':', ' ', '>', '\\':
				b.WriteByte(' ')
			case '!':
			default:
				if !isControl(r) {
					b.WriteRune(r)
				}
			}
		case c == '{' || c == '}' || c == '$':
			i++
		case c == '~' || c == '\t' || c == '\n' || c == '\r':
			b.WriteByte(' ')
			i++
		default:
			r, size := utf8.DecodeRuneInString(raw[i:])
			i += size
			if !isControl(r) {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}

// delim reads the delimiter after \left, \right, \big and friends.
func (p *parser) delim() string {
	p.skipSpace()
	if p.i >= len(p.s) {
		return ""
	}
	c := p.s[p.i]
	if c == '\\' {
		name, next := p.cmdAt(p.i)
		p.i = next
		return delimGlyph(name)
	}
	if c == '{' || c == '}' || c == '&' {
		return ""
	}
	r, size := utf8.DecodeRuneInString(p.s[p.i:])
	p.i += size
	switch r {
	case '(', ')', '[', ']', '|', '/':
		return string(r)
	case '<':
		return "⟨"
	case '>':
		return "⟩"
	}
	return ""
}

func (p *parser) leftRight() *node {
	l := p.delim()
	body := p.parseRow(stopRight | stopBrace | stopCell)
	r := ""
	if p.peek('\\') {
		if name, next := p.cmdAt(p.i); name == "right" {
			p.i = next
			r = p.delim()
		}
	}
	return &node{kind: kDelim, left: l, right: r, a: body}
}

// env parses \begin{name} ... \end{name}; \begin has been consumed.
func (p *parser) env() *node {
	name := strings.TrimSuffix(strings.TrimSpace(p.rawArg()), "*")
	g := &node{kind: kGrid}
	var left, right string
	switch name {
	case "matrix", "smallmatrix":
		g.left, g.right = "[", "]"
	case "pmatrix", "psmallmatrix":
		left, right = "(", ")"
	case "bmatrix", "bsmallmatrix":
		left, right = "[", "]"
	case "Bmatrix", "Bsmallmatrix":
		left, right = "{", "}"
	case "vmatrix", "vsmallmatrix":
		left, right = "|", "|"
	case "Vmatrix", "Vsmallmatrix":
		left, right = "‖", "‖"
	case "cases", "dcases":
		g.n, left = gCases, "{"
	case "rcases", "drcases":
		g.n, right = gCases, "}"
	case "aligned", "align", "split", "eqnarray", "flalign", "flaligned":
		g.n = gAlign
	case "alignat", "alignedat":
		g.n = gAlign
		p.rawArg()
	case "gather", "gathered", "equation", "multline", "displaymath":
		g.n = gGather
	case "array", "darray", "subarray":
		g.n = gArray
		p.skipSpace()
		if p.peek('[') { // vertical position: [t] [b] [c]
			for p.i < len(p.s) && p.s[p.i] != ']' {
				p.i++
			}
			p.i++
		}
		g.text = colSpec(p.rawArg())
	}
	f := p.font
	g.rows = p.parseRows(0)
	p.font = f
	if g.n == gAlign {
		alignCells(g.rows)
	}
	if left == "" && right == "" {
		return g
	}
	return &node{kind: kDelim, left: left, right: right, a: g}
}

// colSpec keeps the l/c/r alignment letters of an array column spec.
func colSpec(s string) string {
	var b strings.Builder
	depth := 0
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '{':
			depth++
		case c == '}':
			depth--
		case depth > 0:
		case c == 'l' || c == 'c' || c == 'r':
			b.WriteByte(c)
		case c == 'p' || c == 'm' || c == 'b' || c == 'X':
			b.WriteByte('l')
		}
	}
	return b.String()
}

// not handles \not: the following relation is negated.
func (p *parser) not() *node {
	n := p.parseArg()
	for n.kind == kGroup && len(n.kids) == 1 {
		n = n.kids[0]
	}
	if n.kind == kAtom {
		n.text, n.typ, n.class = negate(n.text), tRel, styled.Op
	}
	return n
}

// stackType is the class of an \overset or \underset: that of its base,
// looking into groups such as {\arg\min}.
func stackType(base *node) atomType {
	if base.kind == kGroup && len(base.kids) > 0 && base.typ == tOrd {
		return typeOf(base.kids[0], false)
	}
	return typeOf(base, false)
}

// typed wraps the next argument in a group of the given math class.
func (p *parser) typed(t atomType) *node {
	return &node{kind: kGroup, typ: t, kids: []*node{p.parseArg()}}
}

// command parses a control sequence; the name has been consumed.
func (p *parser) command(name string) *node {
	switch name {
	case "":
		return atom("\\", tOrd, styled.Muted)
	case "frac", "dfrac", "tfrac", "cfrac":
		num := p.parseArg()
		return &node{kind: kFrac, typ: tInner, bar: true, a: num, b: p.parseArg()}
	case "binom", "dbinom", "tbinom":
		num := p.parseArg()
		return &node{kind: kFrac, typ: tInner, a: num, b: p.parseArg()}
	case "sqrt":
		var idx *node
		p.skipSpace()
		if p.peek('[') {
			p.i++
			idx = p.parseRow(stopBracket | stopBrace)
			if p.peek(']') {
				p.i++
			}
		}
		return &node{kind: kSqrt, a: p.parseArg(), b: idx}
	case "left":
		return p.leftRight()
	case "right":
		p.delim()
		return nil
	case "middle":
		return &node{kind: kMiddle, typ: tRel, class: styled.Symbol, text: p.delim()}
	case "big", "Big", "bigg", "Bigg", "bigl", "Bigl", "biggl", "Biggl",
		"bigr", "Bigr", "biggr", "Biggr", "bigm", "Bigm", "biggm", "Biggm":
		d := p.delim()
		if d == "" {
			return nil
		}
		t := tOrd
		switch name[len(name)-1] {
		case 'l':
			t = tOpen
		case 'r':
			t = tClose
		case 'm':
			t = tRel
		default:
			if _, dt, _, ok := symbol(d); ok {
				t = dt
			} else if d == "(" || d == "[" {
				t = tOpen
			} else if d == ")" || d == "]" {
				t = tClose
			}
		}
		return atom(d, t, styled.Symbol)
	case "begin":
		return p.env()
	case "end", "label", "vphantom", "color":
		p.rawArg()
		return nil
	case "\\", "cr", "newline", "limits", "nolimits", "displaylimits",
		"displaystyle", "textstyle", "scriptstyle", "scriptscriptstyle",
		"nonumber", "notag", "hline", "hdashline", "!", "negthinspace",
		"negmedspace", "negthickspace", "relax", "allowbreak", "nobreak",
		"mathstrut", "strut", "protect", "centering":
		return nil
	case ",", ":", ";", ">", " ", "thinspace", "medspace", "thickspace",
		"enspace", "space", "nobreakspace", "enskip":
		return spaceNode(1)
	case "quad":
		return spaceNode(2)
	case "qquad":
		return spaceNode(4)
	case "hspace", "mspace":
		p.star()
		p.rawArg()
		return spaceNode(1)
	case "vspace":
		p.star()
		p.rawArg()
		return nil
	case "kern", "mkern", "hskip", "mskip":
		p.skipLength()
		return nil
	case "tag":
		p.star()
		p.tag = p.rawText()
		return nil
	case "text", "textrm", "textnormal", "textup", "textmd", "mbox", "hbox",
		"textsf", "texttt", "url", "ref":
		return &node{kind: kText, class: styled.Plain, text: p.rawText()}
	case "eqref":
		return &node{kind: kText, class: styled.Plain, text: "(" + p.rawText() + ")"}
	case "textit", "emph", "textsl":
		return &node{kind: kText, class: styled.Var, text: p.rawText()}
	case "textbf":
		return &node{kind: kText, class: styled.Plain, text: mapFont(fBF, p.rawText())}
	case "fbox", "framebox":
		return &node{kind: kBoxed, a: &node{kind: kText, class: styled.Plain, text: p.rawText()}}
	case "boxed":
		return &node{kind: kBoxed, a: p.parseArg()}
	case "textcolor", "href":
		p.rawArg()
		return p.parseArg()
	case "colorbox":
		p.rawArg()
		return &node{kind: kText, class: styled.Plain, text: p.rawText()}
	case "fcolorbox":
		p.rawArg()
		p.rawArg()
		return &node{kind: kText, class: styled.Plain, text: p.rawText()}
	case "operatorname", "operatornamewithlimits":
		lim := p.star() || name == "operatornamewithlimits"
		return &node{kind: kAtom, typ: tOp, class: styled.Func, text: p.rawText(), limits: lim}
	case "mathop":
		g := p.typed(tOp)
		g.limits = true
		return g
	case "mathbin":
		return p.typed(tBin)
	case "mathrel":
		return p.typed(tRel)
	case "mathord":
		return p.typed(tOrd)
	case "mathopen":
		return p.typed(tOpen)
	case "mathclose":
		return p.typed(tClose)
	case "mathpunct":
		return p.typed(tPunct)
	case "mathinner":
		return p.typed(tInner)
	case "not":
		return p.not()
	case "pmod", "pod":
		kids := []*node{spaceNode(1), atom("(", tOpen, styled.Symbol)}
		if name == "pmod" {
			kids = append(kids, atom("mod", tOp, styled.Func))
		}
		kids = append(kids, p.parseArg(), atom(")", tClose, styled.Symbol))
		return &node{kind: kGroup, kids: kids}
	case "bmod", "mod":
		return atom("mod", tBin, styled.Func)
	case "overbrace", "underbrace":
		n := &node{kind: kBrace, limits: true, a: p.parseArg()}
		if name == "overbrace" {
			n.n = 1
		}
		return n
	case "overset", "stackrel":
		over := p.parseArg()
		base := p.parseArg()
		t := stackType(base)
		if name == "stackrel" {
			t = tRel
		}
		return &node{kind: kStack, typ: t, a: base, b: over}
	case "underset":
		under := p.parseArg()
		base := p.parseArg()
		return &node{kind: kStack, typ: stackType(base), a: base, c: under}
	case "xrightarrow", "xleftarrow", "xRightarrow", "xLeftarrow", "xleftrightarrow",
		"xLeftrightarrow", "xmapsto", "xhookrightarrow", "xtwoheadrightarrow", "xlongequal":
		var under *node
		p.skipSpace()
		if p.peek('[') {
			p.i++
			under = p.parseRow(stopBracket | stopBrace)
			if p.peek(']') {
				p.i++
			}
		}
		arrow := atom(xArrow(name), tRel, styled.Op)
		return &node{kind: kStack, typ: tRel, n: 1, a: arrow, b: p.parseArg(), c: under}
	case "phantom", "hphantom":
		return &node{kind: kPhantom, a: p.parseArg()}
	case "smash":
		p.skipSpace()
		if p.peek('[') {
			p.i++
			p.rawArg()
			if p.peek(']') {
				p.i++
			}
		}
		return p.parseArg()
	case "mathclap", "clap", "llap", "rlap", "mathllap", "mathrlap":
		return p.parseArg()
	case "substack":
		p.skipSpace()
		if !p.peek('{') {
			return p.parseArg()
		}
		p.i++
		rows := p.parseRows(stopBrace)
		if p.peek('}') {
			p.i++
		}
		return &node{kind: kGrid, n: gSubstack, rows: rows}
	case "rm", "bf", "it", "sf", "tt", "cal", "frak", "mit":
		p.font = switchFont(name)
		return nil
	}
	if f, ok := fontCommand(name); ok {
		saved := p.font
		p.font = f
		g := p.parseArg()
		p.font = saved
		return g
	}
	if m := accentMark(name); m != 0 {
		return &node{kind: kAccent, mark: m, a: p.parseArg()}
	}
	if g, lim, ok := bigOp(name); ok {
		return &node{kind: kAtom, typ: tBig, class: styled.Symbol, text: g, limits: lim}
	}
	if f, lim, ok := function(name); ok {
		return &node{kind: kAtom, typ: tOp, class: styled.Func, text: f, limits: lim}
	}
	if s, t, c, ok := symbol(name); ok {
		return atom(s, t, c)
	}
	return p.unknown(name)
}

// unknown renders an unsupported command and its braced arguments verbatim
// in the Muted class, e.g. "\\foo{x}".
func (p *parser) unknown(name string) *node {
	g := &node{kind: kGroup, kids: []*node{atom("\\"+name, tOrd, styled.Muted)}}
	for p.peek('{') && !p.bad {
		g.kids = append(g.kids, atom("{", tOrd, styled.Muted), p.parseGroup(), atom("}", tOrd, styled.Muted))
	}
	if len(g.kids) == 1 {
		return g.kids[0]
	}
	return g
}

// skipLength skips a TeX length such as "-3mu" or "2.5pt plus 1fil".
func (p *parser) skipLength() {
	p.skipSpace()
	if p.peek('{') {
		p.rawArg()
		return
	}
	if p.peek('-') || p.peek('+') {
		p.i++
	}
	for p.i < len(p.s) && (isDigit(p.s[p.i]) || p.s[p.i] == '.') {
		p.i++
	}
	p.skipSpace()
	for k := 0; k < 2 && p.i < len(p.s) && isLetter(p.s[p.i]); k++ {
		p.i++
	}
}
