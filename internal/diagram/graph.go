package diagram

import (
	"slices"

	"github.com/DipaolaFederico/kmd/internal/styled"
)

// endKind is the decoration at one end of an edge.
type endKind uint8

const (
	endNone endKind = iota
	endArrow
	endCircle
	endCross
	endTriangle // class inheritance / realization
	endDiamond  // class composition
	endODiamond // class aggregation
)

// node is a graph vertex as parsed from the source.
type node struct {
	id    string
	label string
	shape shape
	// sections are the compartments below the label of a shapeClass box
	// (class attributes and methods, entity attributes), one row each.
	sections [][]string
}

// edge connects two nodes; head decorates the `to` end, tail the `from` end.
// fromCard and toCard are optional multiplicities shown next to each end.
type edge struct {
	from, to         int
	label            string
	style            lineStyle
	head, tail       endKind
	fromCard, toCard string
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
			e.fromCard, e.toCard = e.toCard, e.fromCard
		}
		g.flip = false
	}
	lines, ok, fatal := fitGraph(g, width)
	if !ok && !fatal {
		lines, ok = stackComponents(g, width)
	}
	if !ok {
		return nil, false
	}
	return withTitle(title, lines, width), true
}

// fitGraph tries the layout settings in order until one fits width; fatal
// reports that no other setting can help.
func fitGraph(g *graph, width int) (lines []styled.Line, ok, fatal bool) {
	try := func(o layoutOpts) ([]styled.Line, bool, bool) {
		c, fatal := layoutGraph(g, o, width)
		if c == nil {
			return nil, false, fatal
		}
		return c.lines(), true, false
	}
	if g.lr {
		for _, o := range lrSteps {
			if l, ok, fatal := try(o); ok || fatal {
				return l, ok, fatal
			}
		}
	}
	for _, o := range tdSteps {
		if l, ok, fatal := try(o); ok || fatal {
			return l, ok, fatal
		}
	}
	return nil, false, false
}

// stackComponents draws the connected components of a graph that is too
// wide side by side one below the other, each fitted on its own. The total
// output is held to the canvas cell budget.
func stackComponents(g *graph, width int) ([]styled.Line, bool) {
	n := len(g.nodes)
	parent := make([]int, n)
	for i := range parent {
		parent[i] = i
	}
	find := func(x int) int {
		for parent[x] != x {
			parent[x] = parent[parent[x]]
			x = parent[x]
		}
		return x
	}
	for _, e := range g.edges {
		parent[find(e.from)] = find(e.to)
	}
	// Components in order of their first node; local holds each node's
	// index inside its component.
	comp := make([]int, n)
	local := make([]int, n)
	var subs []*graph
	root := make(map[int]int)
	for i, nd := range g.nodes {
		r := find(i)
		ci, seen := root[r]
		if !seen {
			ci = len(subs)
			root[r] = ci
			subs = append(subs, &graph{lr: g.lr})
		}
		comp[i], local[i] = ci, len(subs[ci].nodes)
		subs[ci].nodes = append(subs[ci].nodes, nd)
	}
	// Each component takes at least two rows (itself and a spacer), and one
	// holding nearly every node would only fail again.
	if len(subs) < 2 || 2*len(subs)*width > maxCells || maxSize(subs)*10 >= 9*n {
		return nil, false
	}
	for _, e := range g.edges {
		s := subs[comp[e.from]]
		e.from, e.to = local[e.from], local[e.to]
		s.edges = append(s.edges, e)
	}
	// Largest first, so a component that cannot fit fails before the small
	// ones are paid for; the output keeps the source order.
	order := make([]int, len(subs))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int { return len(subs[b].nodes) - len(subs[a].nodes) })
	parts := make([][]styled.Line, len(subs))
	cells := 0
	for _, i := range order {
		lines, ok, _ := fitGraph(subs[i], width)
		if !ok {
			return nil, false
		}
		if cells += (len(lines) + 1) * width; cells > maxCells {
			return nil, false
		}
		parts[i] = lines
	}
	var out []styled.Line
	for i, lines := range parts {
		if i > 0 {
			out = append(out, nil)
		}
		out = append(out, lines...)
	}
	return out, true
}

// maxSize returns the node count of the largest graph.
func maxSize(gs []*graph) int {
	m := 0
	for _, g := range gs {
		m = max(m, len(g.nodes))
	}
	return m
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
