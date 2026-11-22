package diagram

import (
	"strconv"
	"strings"
)

// dtok is a DOT token. Kinds: 'i' bare identifier or number, 'q' quoted or
// HTML string, '>' directed edge op, '-' undirected edge op, or the
// punctuation byte itself.
type dtok struct {
	k byte
	s string
}

// dotLex tokenizes DOT source, skipping comments.
func dotLex(src string) ([]dtok, bool) {
	var toks []dtok
	lineStart := true
	for i := 0; i < len(src); {
		c := src[i]
		switch {
		case c == '\n':
			lineStart = true
			i++
			continue
		case c == ' ' || c == '\t' || c == '\r':
			i++
			continue
		case c == '#' && lineStart, c == '/' && i+1 < len(src) && src[i+1] == '/':
			for i < len(src) && src[i] != '\n' {
				i++
			}
			continue
		case c == '/' && i+1 < len(src) && src[i+1] == '*':
			end := strings.Index(src[i+2:], "*/")
			if end < 0 {
				return nil, false
			}
			i += end + 4
			continue
		}
		lineStart = false
		switch {
		case c == '"':
			var b strings.Builder
			j := i + 1
			for ; j < len(src) && src[j] != '"'; j++ {
				if src[j] == '\\' && j+1 < len(src) {
					j++
					switch src[j] {
					case 'n', 'l', 'r':
						b.WriteByte('\n')
					case '\n':
					default:
						b.WriteByte(src[j])
					}
					continue
				}
				b.WriteByte(src[j])
			}
			if j >= len(src) {
				return nil, false
			}
			toks = append(toks, dtok{'q', b.String()})
			i = j + 1
		case c == '<':
			depth, j := 0, i
			for ; j < len(src); j++ {
				if src[j] == '<' {
					depth++
				} else if src[j] == '>' {
					depth--
					if depth == 0 {
						break
					}
				}
			}
			if j >= len(src) {
				return nil, false
			}
			toks = append(toks, dtok{'q', stripTags(src[i+1 : j])})
			i = j + 1
		case c == '-' && i+1 < len(src) && src[i+1] == '>':
			toks = append(toks, dtok{'>', "->"})
			i += 2
		case c == '-' && i+1 < len(src) && src[i+1] == '-':
			toks = append(toks, dtok{'-', "--"})
			i += 2
		case strings.IndexByte("{}[];,=:+", c) >= 0:
			toks = append(toks, dtok{c, ""})
			i++
		case isWordByte(c) || c == '.' || c == '-':
			j := i + 1
			for j < len(src) && (isWordByte(src[j]) || src[j] == '.') {
				j++
			}
			toks = append(toks, dtok{'i', src[i:j]})
			i = j
		default:
			return nil, false
		}
	}
	return toks, true
}

// stripTags flattens an HTML-like label: <br/> becomes a newline and other
// tags are removed.
func stripTags(s string) string {
	s = replaceBreaks(s)
	var b strings.Builder
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '<':
			depth++
		case '>':
			if depth > 0 {
				depth--
			}
		default:
			if depth == 0 {
				b.WriteByte(s[i])
			}
		}
	}
	return b.String()
}

type dotParser struct {
	t        []dtok
	i        int
	g        *graph
	title    string
	directed bool
	defShape shape
	depth    int
	scopes   [][]int
}

// parseDot parses a DOT graph into the shared graph model.
func parseDot(src string) (*graph, string, bool) {
	toks, ok := dotLex(src)
	if !ok {
		return nil, "", false
	}
	p := &dotParser{t: toks, g: newGraph(), defShape: shapeRound}
	if p.peekWord("strict") {
		p.i++
	}
	switch {
	case p.peekWord("digraph"):
		p.directed = true
	case p.peekWord("graph"):
	default:
		return nil, "", false
	}
	p.i++
	if k := p.peek().k; k == 'i' || k == 'q' {
		p.i++
	}
	if !p.expect('{') || !p.stmts() || !p.expect('}') || p.i != len(p.t) || p.g.tooBig {
		return nil, "", false
	}
	return p.g, p.title, true
}

func (p *dotParser) peek() dtok {
	if p.i < len(p.t) {
		return p.t[p.i]
	}
	return dtok{}
}

func (p *dotParser) peekWord(w string) bool {
	t := p.peek()
	return t.k == 'i' && strings.EqualFold(t.s, w)
}

func (p *dotParser) expect(k byte) bool {
	if p.peek().k != k {
		return false
	}
	p.i++
	return true
}

func (p *dotParser) stmts() bool {
	for {
		switch p.peek().k {
		case 0:
			return false
		case '}':
			return true
		case ';', ',':
			p.i++
			continue
		}
		if !p.stmt() {
			return false
		}
	}
}

func (p *dotParser) stmt() bool {
	t := p.peek()
	if t.k == 'i' {
		switch strings.ToLower(t.s) {
		case "graph", "node", "edge":
			if p.i+1 < len(p.t) && p.t[p.i+1].k == '[' {
				p.i++
				attrs, ok := p.attrs()
				if !ok {
					return false
				}
				switch strings.ToLower(t.s) {
				case "graph":
					p.graphAttrs(attrs)
				case "node":
					if v, ok := attrs["shape"]; ok {
						p.defShape = dotShape(v)
					}
				}
				return true
			}
		}
	}
	if (t.k == 'i' || t.k == 'q') && p.i+1 < len(p.t) && p.t[p.i+1].k == '=' {
		if p.i+2 >= len(p.t) {
			return false
		}
		p.graphAttrs(map[string]string{strings.ToLower(t.s): p.t[p.i+2].s})
		p.i += 3
		return true
	}
	first, ok := p.operand()
	if !ok {
		return false
	}
	groups := [][]int{first}
	for p.peek().k == '>' || p.peek().k == '-' {
		p.i++
		next, ok := p.operand()
		if !ok {
			return false
		}
		groups = append(groups, next)
	}
	attrs, ok := p.attrs()
	if !ok {
		return false
	}
	if len(groups) == 1 {
		if len(first) == 1 && attrs != nil {
			n := &p.g.nodes[first[0]]
			if v, ok := attrs["label"]; ok && v != "\\N" {
				n.label = cleanLabel(v)
			}
			if v, ok := attrs["shape"]; ok {
				n.shape = dotShape(v)
			}
		}
		return true
	}
	e := p.edgeStyle(attrs)
	for gi := 1; gi < len(groups); gi++ {
		for _, a := range groups[gi-1] {
			for _, b := range groups[gi] {
				e.from, e.to = a, b
				if !p.g.addEdge(e) {
					return false
				}
			}
		}
	}
	return true
}

// edgeStyle converts edge attributes into an edge template.
func (p *dotParser) edgeStyle(attrs map[string]string) edge {
	var e edge
	if p.directed {
		e.head = endArrow
	}
	e.label = cleanLabel(attrs["label"])
	switch strings.ToLower(attrs["style"]) {
	case "dashed", "dotted":
		e.style = styleDotted
	case "bold":
		e.style = styleThick
	}
	if w, err := strconv.ParseFloat(attrs["penwidth"], 64); err == nil && w >= 2 {
		e.style = styleThick
	}
	switch strings.ToLower(attrs["dir"]) {
	case "both":
		e.head, e.tail = endArrow, endArrow
	case "back":
		e.head, e.tail = endNone, endArrow
	case "none":
		e.head, e.tail = endNone, endNone
	case "forward":
		e.head = endArrow
	}
	switch strings.ToLower(attrs["arrowhead"]) {
	case "none":
		e.head = endNone
	case "dot", "odot":
		e.head = endCircle
	case "tee", "box", "obox":
		e.head = endCross
	}
	return e
}

func (p *dotParser) graphAttrs(a map[string]string) {
	if p.depth > 0 {
		return // subgraph attributes (cluster labels, ranks) are ignored
	}
	if v, ok := a["rankdir"]; ok {
		setDirection(p.g, v)
	}
	if v, ok := a["label"]; ok {
		p.title = cleanLabel(v)
	}
}

// operand parses a node id (with optional port) or a subgraph and returns
// the nodes it denotes.
func (p *dotParser) operand() ([]int, bool) {
	t := p.peek()
	if t.k == '{' || t.k == 'i' && strings.EqualFold(t.s, "subgraph") {
		if t.k == 'i' {
			p.i++
			if k := p.peek().k; k == 'i' || k == 'q' {
				p.i++
			}
		}
		if p.depth >= maxDepth || !p.expect('{') {
			return nil, false
		}
		p.depth++
		p.scopes = append(p.scopes, nil)
		ok := p.stmts() && p.expect('}')
		inner := p.scopes[len(p.scopes)-1]
		p.scopes = p.scopes[:len(p.scopes)-1]
		p.depth--
		if !ok {
			return nil, false
		}
		p.note(inner...)
		return inner, true
	}
	if t.k != 'i' && t.k != 'q' {
		return nil, false
	}
	p.i++
	n, ok := p.g.ensure(t.s, p.defShape)
	if !ok {
		return nil, false
	}
	for k := 0; k < 2 && p.peek().k == ':'; k++ {
		p.i++
		if t := p.peek().k; t != 'i' && t != 'q' {
			return nil, false
		}
		p.i++
	}
	p.note(n)
	return []int{n}, true
}

// note records nodes in the innermost open subgraph scope.
func (p *dotParser) note(ns ...int) {
	if len(p.scopes) > 0 {
		s := &p.scopes[len(p.scopes)-1]
		*s = append(*s, ns...)
	}
}

// attrs parses zero or more `[k=v, ...]` lists.
func (p *dotParser) attrs() (map[string]string, bool) {
	var m map[string]string
	for p.peek().k == '[' {
		p.i++
		if m == nil {
			m = make(map[string]string)
		}
		for p.peek().k != ']' {
			k := p.peek()
			if k.k != 'i' && k.k != 'q' {
				return nil, false
			}
			p.i++
			v := ""
			if p.peek().k == '=' {
				p.i++
				vt := p.peek()
				if vt.k != 'i' && vt.k != 'q' {
					return nil, false
				}
				v = vt.s
				p.i++
			}
			m[strings.ToLower(k.s)] = v
			if c := p.peek().k; c == ',' || c == ';' {
				p.i++
			}
		}
		p.i++
	}
	return m, true
}

// dotShape maps Graphviz shape names to node shapes.
func dotShape(s string) shape {
	switch strings.ToLower(s) {
	case "box", "rect", "rectangle", "square", "record", "component", "note", "tab", "folder", "plaintext", "plain", "none", "underline":
		return shapeRect
	case "circle", "doublecircle", "mcircle":
		return shapeCircle
	case "point":
		return shapeStart
	case "diamond", "mdiamond":
		return shapeDecision
	case "hexagon", "octagon", "doubleoctagon", "tripleoctagon":
		return shapeHexagon
	case "cylinder":
		return shapeDatabase
	case "parallelogram":
		return shapePara
	case "cds", "larrow", "rarrow":
		return shapeAsym
	}
	return shapeRound
}
