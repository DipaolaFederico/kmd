package diagram

import "strings"

// parseState parses a Mermaid state diagram into a graph. Composite states
// are flattened (each keeps its own start and end markers); notes are
// ignored.
func parseState(body []string) (*graph, bool) {
	g := newGraph()
	scopes := []string{""}
	inNote := false
	ref := func(name string, source bool) (int, bool) {
		name = strings.TrimSpace(name)
		if i := strings.Index(name, ":::"); i >= 0 {
			name = strings.TrimSpace(name[:i])
		}
		if name == "" {
			return 0, false
		}
		if name == "[*]" {
			scope := scopes[len(scopes)-1]
			if source {
				return g.ensure("\x00start\x00"+scope, shapeStart)
			}
			return g.ensure("\x00end\x00"+scope, shapeEnd)
		}
		if strings.ContainsAny(name, " \t") {
			return 0, false
		}
		return g.ensure(name, shapeRound)
	}
	for _, line := range body {
		line = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(line), ";"))
		lowerLine := strings.ToLower(line)
		if inNote {
			if lowerLine == "end note" {
				inNote = false
			}
			continue
		}
		kw, rest := splitWord(line)
		switch {
		case line == "" || strings.HasPrefix(line, "%%") || line == "--":
			continue
		case line == "}":
			if len(scopes) > 1 {
				scopes = scopes[:len(scopes)-1]
			}
			continue
		case strings.HasPrefix(lowerLine, "note "):
			inNote = !strings.Contains(line, ":")
			continue
		case kw == "direction":
			if !setDirection(g, rest) {
				return nil, false
			}
			continue
		case kw == "classDef" || kw == "class" || kw == "style" || kw == "scale" || kw == "hide" ||
			strings.HasPrefix(kw, "accTitle") || strings.HasPrefix(kw, "accDescr"):
			continue
		case kw == "state":
			if !stateDecl(g, rest, &scopes) {
				return nil, false
			}
			continue
		}
		if i := strings.Index(line, "-->"); i >= 0 {
			target, label, _ := strings.Cut(line[i+3:], ":")
			a, ok1 := ref(line[:i], true)
			b, ok2 := ref(target, false)
			if !ok1 || !ok2 {
				return nil, false
			}
			if !g.addEdge(edge{from: a, to: b, head: endArrow, label: cleanLabel(strings.TrimSpace(label))}) {
				return nil, false
			}
			continue
		}
		if name, desc, ok := strings.Cut(line, ":"); ok {
			n, ok := ref(name, true)
			if !ok {
				return nil, false
			}
			desc = cleanLabel(strings.TrimSpace(desc))
			nd := &g.nodes[n]
			if nd.label == nd.id || nd.label == "" {
				nd.label = desc
			} else if desc != "" {
				nd.label += "\n" + desc
			}
			continue
		}
		if _, ok := ref(line, true); !ok {
			return nil, false
		}
	}
	if len(g.nodes) == 0 || g.tooBig {
		return nil, false
	}
	return g, true
}

// stateDecl handles `state "Desc" as Id`, `state Id <<choice>>` and
// `state Id {` declarations.
func stateDecl(g *graph, rest string, scopes *[]string) bool {
	open := strings.HasSuffix(rest, "{")
	rest = strings.TrimSpace(strings.TrimSuffix(rest, "{"))
	var id, label string
	sh := shapeRound
	if strings.HasPrefix(rest, "\"") {
		end := strings.IndexByte(rest[1:], '"')
		if end < 0 {
			return false
		}
		label = rest[1 : 1+end]
		after := strings.TrimSpace(rest[end+2:])
		kw, name := splitWord(after)
		if kw != "as" || name == "" {
			return false
		}
		id = name
	} else {
		var tail string
		id, tail = splitWord(rest)
		switch {
		case strings.HasPrefix(tail, "as "):
			label = unquote(strings.TrimSpace(tail[3:]))
		case tail == "<<choice>>":
			sh, label = shapeDecision, " "
		case tail == "<<fork>>" || tail == "<<join>>":
			sh = shapeBar
		case tail != "":
			return false
		}
	}
	if id == "" || strings.ContainsAny(id, " \t") {
		return false
	}
	n, ok := g.ensure(id, sh)
	if !ok {
		return false
	}
	if sh != shapeRound {
		g.nodes[n].shape = sh
	}
	if label != "" {
		g.nodes[n].label = cleanLabel(label)
	}
	if open && len(*scopes) < maxDepth {
		*scopes = append(*scopes, id)
	}
	return true
}
