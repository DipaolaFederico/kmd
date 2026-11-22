package diagram

import (
	"strings"
)

// parseFlowchart parses a Mermaid `graph`/`flowchart` diagram.
func parseFlowchart(header string, body []string) (*graph, bool) {
	g := newGraph()
	head, extra, _ := strings.Cut(header, ";")
	f := strings.Fields(head)
	if len(f) > 2 {
		return nil, false
	}
	if len(f) == 2 {
		if !setDirection(g, f[1]) {
			return nil, false
		}
	}
	p := &flowParser{g: g}
	if !p.lines(append([]string{extra}, body...)) {
		return nil, false
	}
	return g, !g.tooBig
}

// setDirection applies a Mermaid direction keyword.
func setDirection(g *graph, d string) bool {
	switch strings.ToUpper(d) {
	case "TB", "TD", "V":
		g.lr, g.flip = false, false
	case "BT", "^":
		g.lr, g.flip = false, true
	case "LR", ">":
		g.lr, g.flip = true, false
	case "RL", "<":
		g.lr, g.flip = true, true
	default:
		return false
	}
	return true
}

type flowParser struct {
	g *graph
	s string
	i int
}

func (p *flowParser) lines(lines []string) bool {
	for _, line := range lines {
		for _, st := range splitStatements(line) {
			if !p.statement(st) {
				return false
			}
		}
	}
	return true
}

// splitStatements splits a line at semicolons outside quotes and brackets.
func splitStatements(line string) []string {
	if !strings.Contains(line, ";") {
		return []string{line}
	}
	var out []string
	depth, quote, pipe, start := 0, false, false, 0
	for i := 0; i < len(line); i++ {
		switch c := line[i]; {
		case c == '"':
			quote = !quote
		case quote:
		case c == '|':
			pipe = !pipe
		case c == '[' || c == '(' || c == '{':
			depth++
		case c == ']' || c == ')' || c == '}':
			depth--
		case c == ';' && depth <= 0 && !pipe:
			out = append(out, line[start:i])
			start = i + 1
		}
	}
	return append(out, line[start:])
}

// statement parses one flowchart statement.
func (p *flowParser) statement(st string) bool {
	st = strings.TrimSpace(st)
	if st == "" || strings.HasPrefix(st, "%%") {
		return true
	}
	kw, rest := splitWord(st)
	switch kw {
	case "subgraph", "end", "classDef", "class", "style", "linkStyle", "click", "direction":
		// Subgraphs are flattened; styling and interaction are ignored.
		return true
	}
	if strings.HasPrefix(kw, "accTitle") || strings.HasPrefix(kw, "accDescr") {
		return true
	}
	_ = rest
	p.s, p.i = st, 0
	return p.chain()
}

// chain parses `group (edge group)*` where a group is `node (& node)*`.
func (p *flowParser) chain() bool {
	left, ok := p.group()
	if !ok {
		return false
	}
	for {
		p.skipSpace()
		if p.i >= len(p.s) {
			return true
		}
		e, hidden, ok := p.edgeOp()
		if !ok {
			return false
		}
		right, ok := p.group()
		if !ok {
			return false
		}
		if !hidden {
			for _, a := range left {
				for _, b := range right {
					e.from, e.to = a, b
					if !p.g.addEdge(e) {
						return false
					}
				}
			}
		}
		left = right
	}
}

func (p *flowParser) group() ([]int, bool) {
	var ids []int
	for {
		n, ok := p.nodeRef()
		if !ok {
			return nil, false
		}
		ids = append(ids, n)
		p.skipSpace()
		if p.i < len(p.s) && p.s[p.i] == '&' {
			p.i++
			continue
		}
		return ids, true
	}
}

func (p *flowParser) skipSpace() {
	for p.i < len(p.s) && (p.s[p.i] == ' ' || p.s[p.i] == '\t') {
		p.i++
	}
}

// edgeAhead reports whether an edge operator starts at position i.
func edgeAhead(s string, i int) bool {
	r := s[i:]
	return strings.HasPrefix(r, "--") || strings.HasPrefix(r, "-.") ||
		strings.HasPrefix(r, "==") || strings.HasPrefix(r, "~~")
}

// nodeRef parses `id[shape label][:::class]`.
func (p *flowParser) nodeRef() (int, bool) {
	p.skipSpace()
	start := p.i
	for p.i < len(p.s) {
		c := p.s[p.i]
		if c == ' ' || c == '\t' || strings.IndexByte("[](){}|&;\"<>:@", c) >= 0 {
			break
		}
		if (c == '-' || c == '=' || c == '~') && edgeAhead(p.s, p.i) {
			break
		}
		p.i++
	}
	id := p.s[start:p.i]
	if id == "" {
		return 0, false
	}
	n, ok := p.g.ensure(id, shapeRect)
	if !ok {
		return 0, false
	}
	if p.i < len(p.s) {
		sh, label, found, ok := p.shape()
		if !ok {
			return 0, false
		}
		if found {
			p.g.nodes[n].shape = sh
			p.g.nodes[n].label = cleanLabel(label)
		}
	}
	if strings.HasPrefix(p.s[p.i:], ":::") {
		p.i += 3
		for p.i < len(p.s) && p.s[p.i] != ' ' && p.s[p.i] != '&' && !edgeAhead(p.s, p.i) {
			p.i++
		}
	}
	return n, true
}

// opener returns the closing delimiters and shape for a shape opener at s.
func opener(s string) (open int, close, alt string, sh shape) {
	switch {
	case strings.HasPrefix(s, "((("):
		return 3, ")))", "", shapeCircle
	case strings.HasPrefix(s, "(("):
		return 2, "))", "", shapeCircle
	case strings.HasPrefix(s, "(["):
		return 2, "])", "", shapeStadium
	case strings.HasPrefix(s, "[["):
		return 2, "]]", "", shapeSubroutine
	case strings.HasPrefix(s, "[("):
		return 2, ")]", "", shapeDatabase
	case strings.HasPrefix(s, "[/"):
		return 2, "/]", "\\]", shapePara
	case strings.HasPrefix(s, "[\\"):
		return 2, "\\]", "/]", shapeParaAlt
	case strings.HasPrefix(s, "{{"):
		return 2, "}}", "", shapeHexagon
	case s[0] == '[':
		return 1, "]", "", shapeRect
	case s[0] == '(':
		return 1, ")", "", shapeRound
	case s[0] == '{':
		return 1, "}", "", shapeDecision
	case s[0] == '>':
		return 1, "]", "", shapeAsym
	}
	return 0, "", "", shapeRect
}

// shape parses a shape suffix; found is false when none is present.
func (p *flowParser) shape() (sh shape, label string, found, ok bool) {
	if strings.HasPrefix(p.s[p.i:], "@{") {
		return p.atShape()
	}
	n, closeTok, alt, sh := opener(p.s[p.i:])
	if n == 0 {
		return sh, "", false, true
	}
	rest := p.s[p.i+n:]
	end := -1
	if t := strings.TrimLeft(rest, " "); strings.HasPrefix(t, "\"") {
		q := strings.IndexByte(t[1:], '"')
		if q < 0 {
			return sh, "", true, false
		}
		label = t[1 : 1+q]
		after := t[q+2:]
		trimmed := strings.TrimLeft(after, " ")
		switch {
		case strings.HasPrefix(trimmed, closeTok):
			p.i = len(p.s) - len(trimmed) + len(closeTok)
		case alt != "" && strings.HasPrefix(trimmed, alt):
			p.i = len(p.s) - len(trimmed) + len(alt)
		default:
			return sh, "", true, false
		}
		return sh, label, true, true
	}
	if len(closeTok) == 1 && n == 1 && closeTok != "]" || (closeTok == "]" && sh == shapeRect) {
		end = matchClose(rest, p.s[p.i], closeTok[0])
	} else {
		end = strings.Index(rest, closeTok)
		if alt != "" {
			if j := strings.Index(rest, alt); j >= 0 && (end < 0 || j < end) {
				end = j
			}
		}
	}
	if end < 0 {
		return sh, "", true, false
	}
	label = strings.TrimSpace(rest[:end])
	p.i += n + end + len(closeTok)
	return sh, label, true, true
}

// matchClose finds the closing bracket matching an already consumed opener,
// honoring nested pairs of the same kind.
func matchClose(s string, open, close byte) int {
	depth := 1
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case open:
			depth++
		case close:
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// atShape parses the `@{ shape: x, label: "y" }` node metadata syntax.
func (p *flowParser) atShape() (shape, string, bool, bool) {
	end := strings.IndexByte(p.s[p.i:], '}')
	if end < 0 {
		return shapeRect, "", true, false
	}
	body := p.s[p.i+2 : p.i+end]
	p.i += end + 1
	sh, label, hasLabel := shapeRect, "", false
	for _, kv := range strings.Split(body, ",") {
		k, v, _ := strings.Cut(kv, ":")
		v = unquote(strings.TrimSpace(v))
		switch strings.TrimSpace(k) {
		case "shape":
			sh = namedShape(v)
		case "label":
			label, hasLabel = v, true
		}
	}
	if !hasLabel {
		return sh, "", false, true
	}
	return sh, label, true, true
}

// namedShape maps Mermaid's named shapes (v11 syntax) to node shapes.
func namedShape(name string) shape {
	switch name {
	case "rounded", "event":
		return shapeRound
	case "stadium", "terminal", "pill":
		return shapeStadium
	case "subproc", "subroutine", "fr-rect", "framed-rectangle":
		return shapeSubroutine
	case "cyl", "cylinder", "db", "database":
		return shapeDatabase
	case "circle", "circ", "dbl-circ", "double-circle":
		return shapeCircle
	case "diam", "diamond", "decision", "question":
		return shapeDecision
	case "hex", "hexagon", "prepare":
		return shapeHexagon
	case "lean-r", "lean-right", "in-out":
		return shapePara
	case "lean-l", "lean-left", "out-in":
		return shapeParaAlt
	case "sm-circ", "start", "small-circle":
		return shapeStart
	case "stop", "f-circ", "framed-circle":
		return shapeEnd
	case "fork", "join":
		return shapeBar
	}
	return shapeRect
}

func isWordByte(c byte) bool {
	return c == '_' || c >= 0x80 || (c|0x20) >= 'a' && (c|0x20) <= 'z' || c >= '0' && c <= '9'
}

// linkHead parses an optional edge head at s[i].
func linkHead(s string, i int) (int, endKind) {
	if i < len(s) {
		switch c := s[i]; {
		case c == '>':
			return i + 1, endArrow
		case (c == 'o' || c == 'x') && (i+1 == len(s) || !isWordByte(s[i+1])):
			if c == 'o' {
				return i + 1, endCircle
			}
			return i + 1, endCross
		}
	}
	return i, endNone
}

// runLink parses a solid (`-`) or thick (`=`) link starting at s[i], with an
// optional `-- text -->` style label.
func runLink(s string, i int, ch byte) (next int, head endKind, label string, ok bool) {
	j := i
	for j < len(s) && s[j] == ch {
		j++
	}
	if k, h := linkHead(s, j); h != endNone {
		return k, h, "", true
	}
	if j-i >= 3 {
		return j, endNone, "", true
	}
	closeAt := strings.Index(s[j:], string([]byte{ch, ch}))
	if closeAt < 0 {
		return 0, endNone, "", false
	}
	label = s[j : j+closeAt]
	k := j + closeAt
	m := k
	for m < len(s) && s[m] == ch {
		m++
	}
	if n, h := linkHead(s, m); h != endNone {
		return n, h, label, true
	}
	if m-k >= 3 {
		return m, endNone, label, true
	}
	return 0, endNone, "", false
}

// edgeOp parses an edge operator with optional tail, head and label.
func (p *flowParser) edgeOp() (e edge, hidden, ok bool) {
	p.skipSpace()
	s, i := p.s, p.i
	switch {
	case i < len(s) && s[i] == '<':
		e.tail = endArrow
		i++
	case i+2 < len(s) && (s[i] == 'o' || s[i] == 'x') && (strings.HasPrefix(s[i+1:], "--") || strings.HasPrefix(s[i+1:], "==") || strings.HasPrefix(s[i+1:], "-.")):
		e.tail = endCircle
		if s[i] == 'x' {
			e.tail = endCross
		}
		i++
	}
	if i+1 >= len(s) {
		return e, false, false
	}
	var label string
	switch {
	case strings.HasPrefix(s[i:], "~~~"):
		for i < len(s) && s[i] == '~' {
			i++
		}
		hidden = true
	case s[i] == '-' && s[i+1] == '.':
		e.style = styleDotted
		i += 2
		for i < len(s) && s[i] == '.' {
			i++
		}
		if i >= len(s) || s[i] != '-' {
			end := strings.Index(s[i:], ".-")
			if end < 0 {
				return e, false, false
			}
			label = s[i : i+end]
			i += end
			for i < len(s) && s[i] == '.' {
				i++
			}
		}
		i++ // the closing '-'
		i, e.head = linkHead(s, i)
	case s[i] == '-' && s[i+1] == '-':
		var ok bool
		i, e.head, label, ok = runLink(s, i, '-')
		if !ok {
			return e, false, false
		}
	case s[i] == '=' && s[i+1] == '=':
		e.style = styleThick
		var ok bool
		i, e.head, label, ok = runLink(s, i, '=')
		if !ok {
			return e, false, false
		}
	default:
		return e, false, false
	}
	p.i = i
	p.skipSpace()
	if p.i < len(s) && s[p.i] == '|' {
		from := p.i + 1
		if t := strings.TrimLeft(s[from:], " "); strings.HasPrefix(t, "\"") {
			if q := strings.IndexByte(t[1:], '"'); q >= 0 {
				from = len(s) - len(t) + q + 2
			}
		}
		end := strings.IndexByte(s[from:], '|')
		if end < 0 {
			return e, false, false
		}
		label = s[p.i+1 : from+end]
		p.i = from + end + 1
	}
	e.label = cleanLabel(unquote(strings.TrimSpace(label)))
	return e, hidden, true
}
