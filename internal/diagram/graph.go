package diagram

import "github.com/DipaolaFederico/kmd/internal/styled"

// endKind is the decoration at one end of an edge.
type endKind uint8

const (
	endNone endKind = iota
	endArrow
	endCircle
	endCross
)

// node is a graph vertex as parsed from the source.
type node struct {
	id    string
	label string
	shape shape
}

// edge connects two nodes; head decorates the `to` end, tail the `from` end.
type edge struct {
	from, to   int
	label      string
	style      lineStyle
	head, tail endKind
}

// graph is the diagram-independent model the layered layout consumes.
type graph struct {
	lr     bool // left-to-right instead of top-down
	flip   bool // BT / RL: edges are reversed before layout
	nodes  []node
	index  map[string]int
	edges  []edge
	tooBig bool
}

func newGraph() *graph {
	return &graph{index: make(map[string]int)}
}

// ensure returns the index of node id, creating it with shape sh if needed.
func (g *graph) ensure(id string, sh shape) (int, bool) {
	if i, ok := g.index[id]; ok {
		return i, true
	}
	if len(g.nodes) >= maxNodes {
		g.tooBig = true
		return 0, false
	}
	g.index[id] = len(g.nodes)
	g.nodes = append(g.nodes, node{id: id, label: cleanLabel(id), shape: sh})
	return len(g.nodes) - 1, true
}

// addEdge appends an edge, enforcing the edge limit.
func (g *graph) addEdge(e edge) bool {
	if len(g.edges) >= maxEdges {
		g.tooBig = true
		return false
	}
	g.edges = append(g.edges, e)
	return true
}

// graphSteps are the layout attempts tried in order until one fits: first
// roomy, then with narrower labels and tighter spacing.
var (
	tdSteps = [...]layoutOpts{{labelMax: 24, gap: 3}, {labelMax: 18, gap: 2}, {labelMax: 14, gap: 1}, {labelMax: 10, gap: 1}, {labelMax: 6, gap: 1}}
	lrSteps = [...]layoutOpts{{lr: true, labelMax: 24, gap: 1}, {lr: true, labelMax: 16, gap: 1}, {lr: true, labelMax: 10, gap: 1}}
)

// renderGraph lays out g within width columns, trying progressively more
// compact settings and falling back from LR to TD.
func renderGraph(g *graph, title string, width int) ([]styled.Line, bool) {
	if len(g.nodes) == 0 || g.tooBig {
		return nil, false
	}
	if g.flip {
		for i := range g.edges {
			e := &g.edges[i]
			e.from, e.to = e.to, e.from
			e.head, e.tail = e.tail, e.head
		}
		g.flip = false
	}
	try := func(o layoutOpts) ([]styled.Line, bool, bool) {
		c, fatal := layoutGraph(g, o, width)
		if c == nil {
			return nil, false, fatal
		}
		return withTitle(title, c.lines(), width), true, false
	}
	if g.lr {
		for _, o := range lrSteps {
			if l, ok, fatal := try(o); ok || fatal {
				return l, ok
			}
		}
	}
	for _, o := range tdSteps {
		if l, ok, fatal := try(o); ok || fatal {
			return l, ok
		}
	}
	return nil, false
}

// withTitle prepends a title line (and a spacer) when title is set.
func withTitle(title string, lines []styled.Line, width int) []styled.Line {
	title = oneLine(title)
	if title == "" {
		return lines
	}
	out := make([]styled.Line, 0, len(lines)+2)
	out = append(out, styled.Line{{Text: truncate(title, width), Class: styled.Title}}, nil)
	return append(out, lines...)
}
