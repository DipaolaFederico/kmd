package diagram

import (
	"strconv"
	"strings"

	"github.com/DipaolaFederico/kmd/internal/styled"
)

// maxAttrField bounds each attribute column (type, name, comment).
const maxAttrField = 32

// erAttr is one attribute row of an entity.
type erAttr struct {
	typ, name, keys, comment string
}

// parseER parses a Mermaid entity-relationship diagram into a graph of
// entity boxes joined by unadorned edges whose labels carry the
// relationship name and both cardinalities.
func parseER(body []string) (*graph, bool) {
	g := newGraph()
	attrs := map[int][]erAttr{}
	hidden := map[int]int{} // attribute rows beyond what a box shows
	block := -1
	for _, line := range body {
		line = strings.TrimSpace(line)
		if block >= 0 {
			if line == "}" {
				block = -1
				continue
			}
			a, ok := parseERAttr(line)
			if !ok {
				return nil, false
			}
			if len(attrs[block]) >= maxSectionRows-1 {
				hidden[block]++
				continue
			}
			attrs[block] = append(attrs[block], a)
			continue
		}
		kw, rest := splitWord(line)
		switch {
		case kw == "direction":
			if !setDirection(g, rest) {
				return nil, false
			}
			continue
		case kw == "style" || kw == "classDef" || kw == "class" ||
			strings.HasPrefix(kw, "accTitle") || strings.HasPrefix(kw, "accDescr"):
			continue
		}
		if found, ok := erRelation(g, line); found {
			if !ok {
				return nil, false
			}
			continue
		}
		open := strings.HasSuffix(line, "{")
		n, ok := erEntity(g, strings.TrimSpace(strings.TrimSuffix(line, "{")))
		if !ok {
			return nil, false
		}
		if open {
			block = n
		}
	}
	if g.tooBig || len(g.nodes) == 0 {
		return nil, false
	}
	for n, as := range attrs {
		rows := erRows(as)
		if k := hidden[n]; k > 0 {
			rows = append(rows, "… +"+strconv.Itoa(k)+" more")
		}
		g.nodes[n].sections = [][]string{rows}
	}
	return g, true
}

// erEntity resolves `NAME` or `NAME["Alias"]` / `NAME[Alias]`.
func erEntity(g *graph, s string) (int, bool) {
	var alias string
	if i := strings.IndexByte(s, '['); i > 0 && strings.HasSuffix(s, "]") {
		alias = cleanLabel(unquote(strings.TrimSpace(s[i+1 : len(s)-1])))
		s = strings.TrimSpace(s[:i])
	}
	if s == "" || strings.ContainsAny(s, " \t{}[]:\"|") {
		return 0, false
	}
	n, ok := g.ensure(s, shapeClass)
	if ok && alias != "" {
		g.nodes[n].label = alias
	}
	return n, ok
}

// erCards maps crow's-foot notation to readable cardinalities; the left
// and right spellings mirror each other.
var erCards = map[string]string{
	"|o": "0..1", "o|": "0..1",
	"||": "1",
	"}o": "0..*", "o{": "0..*",
	"}|": "1..*", "|{": "1..*",
}

// erAliases maps Mermaid's word cardinalities.
var erAliases = map[string]string{
	"one or zero": "0..1", "zero or one": "0..1",
	"one or more": "1..*", "one or many": "1..*", "many(1)": "1..*", "1+": "1..*",
	"zero or more": "0..*", "zero or many": "0..*", "many(0)": "0..*", "0+": "0..*",
	"only one": "1", "1": "1",
}

// erRelation parses `A ||--o{ B : label` (or the word form
// `A one or more to zero or more B : label`). found is false when the line
// is not a relationship.
func erRelation(g *graph, line string) (found, ok bool) {
	lhs, label, hasLabel := line, "", false
	if i := indexOutsideQuotes(line, ':'); i >= 0 {
		lhs, label, hasLabel = line[:i], line[i+1:], true
	}
	f := strings.Fields(lhs)
	if len(f) < 3 {
		return hasLabel, false
	}
	var aCard, bCard string
	st := styleSolid
	if len(f) == 3 {
		op := f[1]
		if len(op) != 6 || (op[2:4] != "--" && op[2:4] != "..") {
			return hasLabel, false
		}
		var ok1, ok2 bool
		aCard, ok1 = erCards[op[:2]]
		bCard, ok2 = erCards[op[4:]]
		if !ok1 || !ok2 || strings.ContainsAny(op[:2], "{") || strings.ContainsAny(op[4:], "}") {
			return true, false
		}
		if op[2] == '.' {
			st = styleDotted
		}
	} else {
		to := -1
		for i := 2; i < len(f)-2; i++ {
			if f[i] == "to" {
				to = i
				break
			}
		}
		if to < 0 {
			return hasLabel, false
		}
		left := f[1:to]
		if len(left) > 0 && left[len(left)-1] == "optionally" {
			left = left[:len(left)-1]
			st = styleDotted
		}
		var ok1, ok2 bool
		aCard, ok1 = erAliases[strings.Join(left, " ")]
		bCard, ok2 = erAliases[strings.Join(f[to+1:len(f)-1], " ")]
		if !ok1 || !ok2 {
			return true, false
		}
	}
	a, ok1 := erEntity(g, f[0])
	b, ok2 := erEntity(g, f[len(f)-1])
	if !ok1 || !ok2 {
		return true, false
	}
	return true, g.addEdge(edge{from: a, to: b, style: st, fromCard: aCard, toCard: bCard,
		label: oneLine(cleanLabel(unquote(strings.TrimSpace(label))))})
}

// parseERAttr parses `type name [PK|FK|UK[, ...]] ["comment"]`.
func parseERAttr(line string) (erAttr, bool) {
	var a erAttr
	if i := strings.IndexByte(line, '"'); i >= 0 {
		j := strings.LastIndexByte(line, '"')
		if j == i {
			return a, false
		}
		a.comment = truncate(oneLine(cleanLabel(line[i+1:j])), maxAttrField)
		line = line[:i]
	}
	f := strings.Fields(strings.ReplaceAll(line, ",", " "))
	if len(f) < 2 {
		return a, false
	}
	if len(f) > 5 {
		return a, false // at most PK, FK and UK
	}
	for _, k := range f[2:] {
		if k != "PK" && k != "FK" && k != "UK" {
			return a, false
		}
	}
	// Fields are cut before the columns are aligned, so one huge field
	// cannot pad every row of the entity to its width.
	a.typ = truncate(oneLine(cleanLabel(f[0])), maxAttrField)
	a.name = truncate(oneLine(cleanLabel(f[1])), maxAttrField)
	a.keys = strings.Join(f[2:], ",")
	return a, true
}

// erRows formats attributes as aligned columns: type, name, keys, comment.
func erRows(as []erAttr) []string {
	var tw, nw, kw int
	for _, a := range as {
		tw = max(tw, styled.Width(a.typ))
		nw = max(nw, styled.Width(a.name))
		kw = max(kw, len(a.keys))
	}
	rows := make([]string, 0, len(as))
	var b strings.Builder
	for _, a := range as {
		b.Reset()
		cols := [...]struct {
			s string
			w int
		}{{a.typ, tw}, {a.name, nw}, {a.keys, kw}, {a.comment, 0}}
		for i, c := range cols {
			if i > 0 && c.w+len(c.s) > 0 {
				b.WriteByte(' ')
			}
			b.WriteString(c.s)
			for pad := c.w - styled.Width(c.s); pad > 0; pad-- {
				b.WriteByte(' ')
			}
		}
		rows = append(rows, strings.TrimRight(b.String(), " "))
	}
	return rows
}
