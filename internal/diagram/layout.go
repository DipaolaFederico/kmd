package diagram

import (
	"math"
	"slices"

	"github.com/DipaolaFederico/kmd/internal/styled"
)

// The layered ("Sugiyama-lite") layout works in two abstract axes: the
// primary axis p runs across layers (down for TD, right for LR) and the
// secondary axis s runs along a layer. Every phase is written in (p, s) and
// only drawing maps it to canvas columns and rows.

// layoutOpts are the knobs the width-fitting loop turns.
type layoutOpts struct {
	lr       bool
	labelMax int // wrap width for node and edge labels
	gap      int // spacing between neighboring nodes of a layer
}

// lnode is a layout vertex: a real node or a dummy on a long edge.
type lnode struct {
	real   int // graph node index, -1 for a dummy
	edge   int // owning ledge for dummies
	layer  int
	ord    int
	s      int // secondary coordinate of the slot start
	ssize  int // slot size along s
	psize  int // slot size along p
	anchor int // offset of the center (alignment point) within the slot
	boxOff int // offset of the box within the slot along s
	up     []int
	down   []int
}

// segment is the part of an edge between two adjacent layers.
type segment struct {
	upper, lower int // lnode indices
	edge         int
	a, b         int // port s-coordinates at the upper and lower end
	track        int
}

// ledge is an edge oriented from the upper to the lower layer.
type ledge struct {
	u, v        int
	style       lineStyle
	uEnd, vEnd  endKind
	lines       []string
	first, last int // segment indices
	reversed    bool
}

// loop is the merged self-loop of a node.
type loop struct {
	label string
	style lineStyle
	ok    bool
}

// labelSpot is a placed edge label inside a channel.
type labelSpot struct {
	b, x0, row, w int
	lines         []string
}

// channel is the routing space between two consecutive layers.
type channel struct {
	start                            int
	head, lead, tracks, labels, tail int
	segs                             []int
	spots                            []labelSpot
}

func (c *channel) size() int { return c.head + c.lead + c.tracks + c.labels + c.tail }

type layout struct {
	g      *graph
	o      layoutOpts
	boxes  []box
	loops  []loop
	edges  []ledge
	nodes  []lnode
	segs   []segment
	layers [][]int
	band   []int
	top    []int
	chans  []channel
	total  int
}

// pt is a point in layout space.
type pt struct{ p, s int }

// layoutGraph lays g out and draws it. It returns nil when the result would
// be wider than maxW; fatal reports that no other setting can help.
func layoutGraph(g *graph, o layoutOpts, maxW int) (c *canvas, fatal bool) {
	L := &layout{g: g, o: o}
	L.boxes = make([]box, len(g.nodes))
	for i, n := range g.nodes {
		L.boxes[i] = makeBox(n.shape, n.label, o.labelMax, o.lr)
	}
	L.buildEdges()
	L.assignLayers()
	if !L.addDummies() {
		return nil, true
	}
	L.order()
	L.prepareSizes()
	L.coordinates()
	L.assignPorts()
	L.routeChannels()
	L.primary()
	sMin, sMax := L.extent()
	w, h := sMax-sMin, L.total
	if o.lr {
		w, h = h, w
	}
	if w > maxW || w <= 0 || h <= 0 {
		return nil, false
	}
	if w*h > maxCells {
		return nil, true
	}
	c = newCanvas(w, h)
	L.draw(c, sMin)
	return c, false
}

// buildEdges merges self-loops, breaks cycles by reversing DFS back edges
// and orients every remaining edge from its upper to its lower node.
func (L *layout) buildEdges() {
	g := L.g
	n := len(g.nodes)
	L.loops = make([]loop, n)
	adj := make([][]int, n)
	for i, e := range g.edges {
		if e.from == e.to {
			lp := &L.loops[e.from]
			lab := truncate(oneLine(e.label), L.o.labelMax)
			switch {
			case !lp.ok:
				*lp = loop{label: lab, style: e.style, ok: true}
			case lab != "" && lp.label != "":
				lp.label = truncate(lp.label+", "+lab, L.o.labelMax)
			case lab != "":
				lp.label = lab
			}
			continue
		}
		adj[e.from] = append(adj[e.from], i)
	}
	reversed := make([]bool, len(g.edges))
	state := make([]uint8, n)
	type frame struct{ v, next int }
	var stack []frame
	for root := 0; root < n; root++ {
		if state[root] != 0 {
			continue
		}
		state[root] = 1
		stack = append(stack[:0], frame{root, 0})
		for len(stack) > 0 {
			f := &stack[len(stack)-1]
			if f.next == len(adj[f.v]) {
				state[f.v] = 2
				stack = stack[:len(stack)-1]
				continue
			}
			ei := adj[f.v][f.next]
			f.next++
			w := g.edges[ei].to
			switch state[w] {
			case 1:
				reversed[ei] = true
			case 0:
				state[w] = 1
				stack = append(stack, frame{w, 0})
			}
		}
	}
	L.edges = make([]ledge, 0, len(g.edges))
	for i, e := range g.edges {
		if e.from == e.to {
			continue
		}
		le := ledge{u: e.from, v: e.to, style: e.style, uEnd: e.tail, vEnd: e.head}
		if reversed[i] {
			le.u, le.v, le.uEnd, le.vEnd = e.to, e.from, e.head, e.tail
			le.reversed = true
		}
		if e.label != "" {
			if L.o.lr {
				le.lines = []string{truncate(oneLine(e.label), L.o.labelMax)}
			} else {
				le.lines = wrap(e.label, L.o.labelMax)
			}
		}
		L.edges = append(L.edges, le)
	}
}

// assignLayers uses longest-path layering, then pulls sources down next to
// their highest child so they do not dangle at the top with long edges.
func (L *layout) assignLayers() {
	n := len(L.g.nodes)
	indeg := make([]int, n)
	out := make([][]int, n)
	for i, e := range L.edges {
		indeg[e.v]++
		out[e.u] = append(out[e.u], i)
	}
	hasIn := make([]bool, n)
	queue := make([]int, 0, n)
	for v := 0; v < n; v++ {
		hasIn[v] = indeg[v] > 0
		if indeg[v] == 0 {
			queue = append(queue, v)
		}
	}
	layer := make([]int, n)
	for qi := 0; qi < len(queue); qi++ {
		u := queue[qi]
		for _, ei := range out[u] {
			v := L.edges[ei].v
			if layer[u]+1 > layer[v] {
				layer[v] = layer[u] + 1
			}
			indeg[v]--
			if indeg[v] == 0 {
				queue = append(queue, v)
			}
		}
	}
	for u := 0; u < n; u++ {
		if hasIn[u] || len(out[u]) == 0 {
			continue
		}
		m := math.MaxInt
		for _, ei := range out[u] {
			m = min(m, layer[L.edges[ei].v]-1)
		}
		if m > layer[u] {
			layer[u] = m
		}
	}
	L.nodes = make([]lnode, n, n+len(L.edges))
	for i := range L.nodes {
		L.nodes[i] = lnode{real: i, edge: -1, layer: layer[i]}
	}
}

// addDummies splits edges spanning several layers into unit segments.
func (L *layout) addDummies() bool {
	for ei := range L.edges {
		e := &L.edges[ei]
		prev := e.u
		lv := L.nodes[e.v].layer
		for l := L.nodes[e.u].layer + 1; l < lv; l++ {
			if len(L.nodes) >= maxLayoutNodes {
				return false
			}
			d := len(L.nodes)
			L.nodes = append(L.nodes, lnode{real: -1, edge: ei, layer: l})
			L.link(prev, d, ei)
			prev = d
		}
		L.link(prev, e.v, ei)
	}
	nl := 0
	for i := range L.nodes {
		nl = max(nl, L.nodes[i].layer+1)
	}
	L.layers = make([][]int, nl)
	for i := range L.nodes {
		l := L.nodes[i].layer
		L.nodes[i].ord = len(L.layers[l])
		L.layers[l] = append(L.layers[l], i)
	}
	return true
}

func (L *layout) link(u, v, ei int) {
	si := len(L.segs)
	L.segs = append(L.segs, segment{upper: u, lower: v, edge: ei})
	L.nodes[u].down = append(L.nodes[u].down, si)
	L.nodes[v].up = append(L.nodes[v].up, si)
	e := &L.edges[ei]
	if u == e.u {
		e.first = si
	}
	if v == e.v {
		e.last = si
	}
}

// order reduces crossings with alternating barycenter sweeps, keeping the
// best ordering seen.
func (L *layout) order() {
	nl := len(L.layers)
	if nl < 2 {
		return
	}
	best := L.snapshot(nil)
	bestC := L.crossings()
	keys := make([]float64, len(L.nodes))
	for it := 0; it < 8 && bestC > 0; it++ {
		if it%2 == 0 {
			for l := 1; l < nl; l++ {
				L.sortLayer(l, true, keys)
			}
		} else {
			for l := nl - 2; l >= 0; l-- {
				L.sortLayer(l, false, keys)
			}
		}
		if c := L.crossings(); c < bestC {
			bestC = c
			best = L.snapshot(best)
		}
	}
	for l, layer := range L.layers {
		copy(layer, best[l])
		for i, v := range layer {
			L.nodes[v].ord = i
		}
	}
}

func (L *layout) snapshot(dst [][]int) [][]int {
	if dst == nil {
		dst = make([][]int, len(L.layers))
	}
	for l, layer := range L.layers {
		dst[l] = append(dst[l][:0], layer...)
	}
	return dst
}

// sortLayer orders layer l by the mean position of its neighbors in the
// layer above (fromAbove) or below.
func (L *layout) sortLayer(l int, fromAbove bool, keys []float64) {
	layer := L.layers[l]
	for _, v := range layer {
		ln := &L.nodes[v]
		segs := ln.down
		if fromAbove {
			segs = ln.up
		}
		if len(segs) == 0 {
			keys[v] = float64(ln.ord)
			continue
		}
		sum := 0
		for _, si := range segs {
			if fromAbove {
				sum += L.nodes[L.segs[si].upper].ord
			} else {
				sum += L.nodes[L.segs[si].lower].ord
			}
		}
		keys[v] = float64(sum) / float64(len(segs))
	}
	slices.SortStableFunc(layer, func(a, b int) int {
		switch {
		case keys[a] < keys[b]:
			return -1
		case keys[a] > keys[b]:
			return 1
		}
		return 0
	})
	for i, v := range layer {
		L.nodes[v].ord = i
	}
}

// crossings counts segment crossings between all adjacent layer pairs using
// a Fenwick tree inversion count.
func (L *layout) crossings() int {
	nl := len(L.layers)
	pairs := make([][][2]int, nl)
	for _, s := range L.segs {
		u := L.nodes[s.upper]
		pairs[u.layer] = append(pairs[u.layer], [2]int{u.ord, L.nodes[s.lower].ord})
	}
	total := 0
	var tree []int
	for l := 0; l+1 < nl; l++ {
		ps := pairs[l]
		if len(ps) < 2 {
			continue
		}
		slices.SortFunc(ps, func(a, b [2]int) int {
			if a[0] != b[0] {
				return a[0] - b[0]
			}
			return a[1] - b[1]
		})
		n := len(L.layers[l+1])
		tree = append(tree[:0], make([]int, n+1)...)
		for i, p := range ps {
			// count earlier entries with a lower-end strictly greater than p[1]
			le := 0
			for k := p[1] + 1; k > 0; k -= k & -k {
				le += tree[k]
			}
			total += i - le
			for k := p[1] + 1; k <= n; k += k & -k {
				tree[k]++
			}
		}
	}
	return total
}

// groupKey groups segments that may share a port on one side of a node.
func (L *layout) groupKey(si int, upperSide bool) int {
	e := &L.edges[L.segs[si].edge]
	if upperSide {
		return int(e.uEnd)*4 + int(e.style)
	}
	if len(e.lines) > 0 {
		return -1 - si
	}
	return int(e.vEnd)*4 + int(e.style)
}

func (L *layout) groupCount(segs []int, upperSide bool) int {
	var keys []int
	for _, si := range segs {
		k := L.groupKey(si, upperSide)
		if !slices.Contains(keys, k) {
			keys = append(keys, k)
		}
	}
	return len(keys)
}

// prepareSizes grows boxes that need more ports than they have room for
// and computes every slot's size, anchor and self-loop reservation.
func (L *layout) prepareSizes() {
	for i := range L.nodes {
		ln := &L.nodes[i]
		if ln.real < 0 {
			ln.ssize, ln.psize, ln.anchor = 1, 0, 0
			continue
		}
		b := &L.boxes[ln.real]
		need := max(L.groupCount(ln.down, true), L.groupCount(ln.up, false))
		if L.o.lr {
			if lo, hi := b.lrRange(); b.h >= 3 && need > hi-lo+1 {
				b.h += need - (hi - lo + 1)
			}
		} else {
			lo1, hi1 := b.tbRange(false)
			lo2, hi2 := b.tbRange(true)
			if span := min(hi1-lo1, hi2-lo2) + 1; b.w >= 3 && need > span {
				b.w += need - span
			}
		}
		lp := L.loops[ln.real]
		extra := 0
		if lp.label != "" {
			extra = 1 + styled.Width(lp.label)
		}
		drawable := lp.ok && b.w >= 3 && b.h >= 3
		if L.o.lr {
			if drawable {
				if lo, hi := b.tbRange(false); hi-lo < 3 {
					b.w += 3 - (hi - lo)
				}
			}
			ln.ssize, ln.psize, ln.anchor = b.h, b.w, b.h/2
			if drawable {
				lo, _ := b.tbRange(false)
				ln.boxOff = 1
				ln.ssize++
				ln.anchor++
				ln.psize = max(ln.psize, lo+4+extra)
			}
		} else {
			ln.ssize, ln.psize, ln.anchor = b.w, b.h, b.w/2
			if drawable {
				ln.ssize = b.w + 2 + extra
			}
		}
	}
}

// gap is the minimum free space between two neighbors of a layer.
func (L *layout) gap(a, b int) int {
	ra, rb := L.nodes[a].real >= 0, L.nodes[b].real >= 0
	switch {
	case ra && rb:
		return L.o.gap
	case ra || rb:
		return min(L.o.gap, 2)
	}
	return 1
}

// coordinates assigns secondary coordinates: an initial left packing, then
// sweeps that move every node toward the mean of its neighbors with an
// order-preserving least-squares fit (pool adjacent violators).
func (L *layout) coordinates() {
	for _, layer := range L.layers {
		s := 0
		for i, v := range layer {
			L.nodes[v].s = s
			s += L.nodes[v].ssize
			if i+1 < len(layer) {
				s += L.gap(v, layer[i+1])
			}
		}
	}
	nl := len(L.layers)
	for it := 0; it < 7; it++ {
		if it%2 == 0 {
			for l := 1; l < nl; l++ {
				L.place(l, true)
			}
		} else {
			for l := nl - 2; l >= 0; l-- {
				L.place(l, false)
			}
		}
	}
	L.straighten()
	m := math.MaxInt
	for i := range L.nodes {
		m = min(m, L.nodes[i].s)
	}
	for i := range L.nodes {
		L.nodes[i].s -= m
	}
}

// straighten moves the dummy chain of each long edge onto one coordinate
// when that fits between the chain's layer neighbors, removing the small
// jogs left by the averaging sweeps.
func (L *layout) straighten() {
	var chain []int
	for ei := range L.edges {
		chain = chain[:0]
		for si := L.edges[ei].first; ; {
			d := L.segs[si].lower
			if L.nodes[d].real >= 0 {
				break
			}
			chain = append(chain, d)
			si = L.nodes[d].down[0]
		}
		if len(chain) < 2 {
			continue
		}
		fits := func(x int) bool {
			for _, d := range chain {
				ln := &L.nodes[d]
				layer := L.layers[ln.layer]
				if i := ln.ord - 1; i >= 0 {
					p := &L.nodes[layer[i]]
					if x < p.s+p.ssize+L.gap(layer[i], d) {
						return false
					}
				}
				if i := ln.ord + 1; i < len(layer) {
					if x+1+L.gap(d, layer[i]) > L.nodes[layer[i]].s {
						return false
					}
				}
			}
			return true
		}
		cands := []int{L.center(L.edges[ei].u), L.center(L.edges[ei].v), L.nodes[chain[len(chain)/2]].s}
		for _, d := range chain {
			cands = append(cands, L.nodes[d].s)
		}
		for _, x := range cands {
			if fits(x) {
				for _, d := range chain {
					L.nodes[d].s = x
				}
				break
			}
		}
	}
}

func (L *layout) center(v int) int { return L.nodes[v].s + L.nodes[v].anchor }

func (L *layout) place(l int, fromAbove bool) {
	layer := L.layers[l]
	n := len(layer)
	t := make([]float64, n)
	w := make([]float64, n)
	off := 0
	for i, v := range layer {
		ln := &L.nodes[v]
		segs := ln.down
		if fromAbove {
			segs = ln.up
		}
		desired, weight := float64(L.center(v)), 0.05
		if len(segs) > 0 {
			sum := 0
			for _, si := range segs {
				if fromAbove {
					sum += L.center(L.segs[si].upper)
				} else {
					sum += L.center(L.segs[si].lower)
				}
			}
			desired = float64(sum) / float64(len(segs))
			weight = 1
			if ln.real < 0 {
				weight = 0.5
			}
		}
		t[i] = desired - float64(ln.anchor) - float64(off)
		w[i] = weight
		off += ln.ssize
		if i+1 < n {
			off += L.gap(v, layer[i+1])
		}
	}
	pav(t, w)
	off = 0
	for i, v := range layer {
		ln := &L.nodes[v]
		ln.s = int(math.Round(t[i])) + off
		off += ln.ssize
		if i+1 < n {
			off += L.gap(v, layer[i+1])
		}
	}
}

// pav replaces y with its weighted non-decreasing least-squares fit.
func pav(y, w []float64) {
	type block struct {
		sum, wt float64
		n       int
	}
	bs := make([]block, 0, len(y))
	for i := range y {
		b := block{y[i] * w[i], w[i], 1}
		for len(bs) > 0 {
			p := bs[len(bs)-1]
			if p.sum/p.wt <= b.sum/b.wt {
				break
			}
			b.sum += p.sum
			b.wt += p.wt
			b.n += p.n
			bs = bs[:len(bs)-1]
		}
		bs = append(bs, b)
	}
	i := 0
	for _, b := range bs {
		m := b.sum / b.wt
		for k := 0; k < b.n; k++ {
			y[i] = m
			i++
		}
	}
}

// assignPorts picks where each segment attaches to its nodes. Upper sides
// are assigned first so lower sides can line up with the actual ports above,
// which keeps edges between slightly offset nodes perfectly straight.
func (L *layout) assignPorts() {
	for v := range L.nodes {
		ln := &L.nodes[v]
		if ln.real < 0 {
			for _, si := range ln.up {
				L.segs[si].b = ln.s
			}
			for _, si := range ln.down {
				L.segs[si].a = ln.s
			}
		}
	}
	for v := range L.nodes {
		if L.nodes[v].real >= 0 {
			L.ports(v, true)
		}
	}
	for v := range L.nodes {
		if L.nodes[v].real >= 0 {
			L.ports(v, false)
		}
	}
}

// ports assigns the ports of one side of a real node. Segments that may
// share a port are grouped; groups are ordered by the position of their far
// ends and placed as close to those positions as the side allows.
func (L *layout) ports(v int, upperSide bool) {
	ln := &L.nodes[v]
	segs := ln.up
	if upperSide {
		segs = ln.down
	}
	if len(segs) == 0 {
		return
	}
	type group struct {
		key    int
		pos    float64
		others []int
		segs   []int
	}
	var groups []group
	for _, si := range segs {
		k := L.groupKey(si, upperSide)
		other := L.segs[si].a
		if upperSide {
			other = L.center(L.segs[si].lower)
		}
		gi := slices.IndexFunc(groups, func(g group) bool { return g.key == k })
		if gi < 0 {
			groups = append(groups, group{key: k})
			gi = len(groups) - 1
		}
		groups[gi].pos += float64(other)
		groups[gi].others = append(groups[gi].others, other)
		groups[gi].segs = append(groups[gi].segs, si)
	}
	for i := range groups {
		groups[i].pos /= float64(len(groups[i].segs))
	}
	slices.SortStableFunc(groups, func(a, b group) int {
		switch {
		case a.pos < b.pos:
			return -1
		case a.pos > b.pos:
			return 1
		}
		return 0
	})
	b := &L.boxes[ln.real]
	var lo, hi int
	base := ln.s
	if L.o.lr {
		lo, hi = b.lrRange()
		base += ln.boxOff
	} else {
		lo, hi = b.tbRange(upperSide)
	}
	lo, hi = lo+base, hi+base
	anchor := min(max(ln.s+ln.anchor, lo), hi)
	k := len(groups)
	pos := make([]int, k)
	for gi, gr := range groups {
		// Prefer the far end closest to the anchor that the side can reach.
		best, bestD := anchor, math.MaxInt
		if k > 1 {
			best = min(max(int(math.Round(gr.pos)), lo), hi)
		}
		for _, o := range gr.others {
			if d := abs(o - anchor); o >= lo && o <= hi && d < bestD && (k == 1 || len(gr.others) == 1) {
				best, bestD = o, d
			}
		}
		pos[gi] = best
	}
	if k > 1 {
		sep := 1
		if hi-lo+1 >= 2*k-1 {
			sep = 2
		}
		for i := 1; i < k; i++ {
			pos[i] = max(pos[i], pos[i-1]+sep)
		}
		pos[k-1] = min(pos[k-1], hi)
		for i := k - 2; i >= 0; i-- {
			pos[i] = min(pos[i], pos[i+1]-sep)
		}
		if pos[0] < lo {
			span := hi - lo + 1
			for i := range pos {
				pos[i] = lo + ((2*i+1)*span)/(2*k)
			}
		}
	}
	for gi, gr := range groups {
		for _, si := range gr.segs {
			if upperSide {
				L.segs[si].a = pos[gi]
			} else {
				L.segs[si].b = pos[gi]
			}
		}
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// routeChannels sizes every channel: arrow-head rows, horizontal tracks for
// jogging segments and room for edge labels.
func (L *layout) routeChannels() {
	if len(L.layers) < 2 {
		return
	}
	L.chans = make([]channel, len(L.layers)-1)
	for si := range L.segs {
		l := L.nodes[L.segs[si].upper].layer
		L.chans[l].segs = append(L.chans[l].segs, si)
	}
	for ci := range L.chans {
		ch := &L.chans[ci]
		lowerArrow, fancy := false, false
		for _, si := range ch.segs {
			e := &L.edges[L.segs[si].edge]
			if e.first == si && e.uEnd != endNone {
				ch.head = 1
			}
			if e.last == si && e.vEnd != endNone || L.nodes[L.segs[si].lower].real < 0 {
				lowerArrow = true
			}
			if e.style != styleSolid {
				fancy = true
			}
		}
		ch.tracks = L.assignTracks(ch.segs)
		if L.o.lr {
			ch.lead, ch.tail = 1, 2
			L.lrLabels(ch)
		} else {
			L.tdLabels(ch)
			// Keep a visible stretch of line when it carries a label or a
			// dotted/thick style that a lone arrow head would hide.
			if (ch.labels > 0 || fancy) && ch.tracks == 0 {
				ch.lead = 1
			}
			// The arrow row is only needed for arrow heads, or when the
			// channel would otherwise be empty or end with a label.
			if lowerArrow || ch.head == 0 || ch.labels > 0 {
				ch.tail = 1
			}
		}
	}
}

// assignTracks gives every jogging segment of a channel a track so that
// horizontal runs never overlap unless they share a port, preferring an
// order that avoids needless crossings.
func (L *layout) assignTracks(segs []int) int {
	var jog []int
	for _, si := range segs {
		if s := &L.segs[si]; s.a != s.b {
			jog = append(jog, si)
		}
	}
	n := len(jog)
	if n == 0 {
		return 0
	}
	lo := func(i int) int { s := &L.segs[jog[i]]; return min(s.a, s.b) }
	hi := func(i int) int { s := &L.segs[jog[i]]; return max(s.a, s.b) }
	share := func(i, j int) bool {
		x, y := &L.segs[jog[i]], &L.segs[jog[j]]
		return x.a == y.a || x.b == y.b
	}
	overlap := func(i, j int) bool { return !(hi(i) < lo(j) || hi(j) < lo(i)) }
	in := func(x, i int) bool { return x >= lo(i) && x <= hi(i) }
	// before[i] counts segments that should sit on an earlier track than i.
	before := make([]int, n)
	var after [][]int
	if n <= 200 {
		after = make([][]int, n)
		for i := 0; i < n; i++ {
			for j := i + 1; j < n; j++ {
				if !overlap(i, j) || share(i, j) {
					continue
				}
				x, y := &L.segs[jog[i]], &L.segs[jog[j]]
				iFirst := in(x.a, j) || in(y.b, i)
				jFirst := in(y.a, i) || in(x.b, j)
				switch {
				case iFirst && !jFirst:
					after[i] = append(after[i], j)
					before[j]++
				case jFirst && !iFirst:
					after[j] = append(after[j], i)
					before[i]++
				}
			}
		}
	}
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int { return lo(a) - lo(b) })
	track := make([]int, n)
	for i := range track {
		track[i] = -1
	}
	left := n
	t := 0
	var on, cand []int
	for left > 0 {
		cand = cand[:0]
		for _, i := range order {
			if track[i] < 0 && before[i] == 0 {
				cand = append(cand, i)
			}
		}
		if len(cand) == 0 {
			for _, i := range order {
				if track[i] < 0 {
					cand = append(cand, i)
				}
			}
		}
		on = on[:0]
		for _, i := range cand {
			ok := true
			for _, j := range on {
				if overlap(i, j) && !share(i, j) {
					ok = false
					break
				}
			}
			if ok {
				on = append(on, i)
				track[i] = t
				left--
			}
		}
		for _, i := range on {
			if after != nil {
				for _, j := range after[i] {
					before[j]--
				}
			}
		}
		t++
	}
	for i, si := range jog {
		L.segs[si].track = track[i]
	}
	return t
}

// carrier returns the segment that holds an edge's label: the one next to
// the edge's source, so branch labels stay close to the decision they
// leave, except for reversed edges whose source is the lower node.
func (e *ledge) carrier() int {
	if e.reversed {
		return e.last
	}
	return e.first
}

// tdLabels packs edge labels into rows below the tracks, centered on their
// segment when possible and never covering another segment's line.
func (L *layout) tdLabels(ch *channel) {
	var verts []int
	for _, si := range ch.segs {
		if b := L.segs[si].b; !slices.Contains(verts, b) {
			verts = append(verts, b)
		}
	}
	for _, si := range ch.segs {
		e := &L.edges[L.segs[si].edge]
		if e.carrier() != si || len(e.lines) == 0 {
			continue
		}
		b := L.segs[si].b
		lw, nl := maxWidth(e.lines), len(e.lines)
		cands := [3]int{b - lw/2, b + 2, b - 1 - lw}
		x0 := cands[0]
		for _, c := range cands {
			free := true
			for _, v := range verts {
				if v != b && v >= c-1 && v <= c+lw {
					free = false
					break
				}
			}
			if free {
				x0 = c
				break
			}
		}
		row := 0
		for moved := true; moved; {
			moved = false
			for _, sp := range ch.spots {
				if x0-1 <= sp.x0+sp.w && sp.x0-1 <= x0+lw && row < sp.row+len(sp.lines) && sp.row < row+nl {
					row = sp.row + len(sp.lines)
					moved = true
				}
			}
		}
		ch.spots = append(ch.spots, labelSpot{b: b, x0: x0, row: row, w: lw, lines: e.lines})
		ch.labels = max(ch.labels, row+nl)
	}
}

// lrLabels reserves a zone after the tracks where labels sit on the
// horizontal run entering their target.
func (L *layout) lrLabels(ch *channel) {
	for _, si := range ch.segs {
		e := &L.edges[L.segs[si].edge]
		if e.carrier() != si || len(e.lines) == 0 {
			continue
		}
		b := L.segs[si].b
		text := e.lines[0]
		if i := slices.IndexFunc(ch.spots, func(sp labelSpot) bool { return sp.b == b }); i >= 0 {
			sp := &ch.spots[i]
			sp.lines[0] = truncate(sp.lines[0]+", "+text, 2*L.o.labelMax)
			sp.w = maxWidth(sp.lines)
			ch.labels = max(ch.labels, sp.w+2)
			continue
		}
		ch.spots = append(ch.spots, labelSpot{b: b, w: styled.Width(text), lines: []string{text}})
		ch.labels = max(ch.labels, styled.Width(text)+2)
	}
}

// primary assigns primary coordinates to layers and channels.
func (L *layout) primary() {
	nl := len(L.layers)
	L.band = make([]int, nl)
	L.top = make([]int, nl)
	for i := range L.nodes {
		ln := &L.nodes[i]
		L.band[ln.layer] = max(L.band[ln.layer], ln.psize)
	}
	p := 0
	for l := 0; l < nl; l++ {
		L.top[l] = p
		p += L.band[l]
		if l+1 < nl {
			L.chans[l].start = p
			p += L.chans[l].size()
		}
	}
	L.total = p
}

// extent returns the secondary range everything occupies.
func (L *layout) extent() (int, int) {
	lo, hi := 0, 0
	for i := range L.nodes {
		hi = max(hi, L.nodes[i].s+L.nodes[i].ssize)
	}
	for _, s := range L.segs {
		hi = max(hi, s.a+1, s.b+1)
		lo = min(lo, s.a, s.b)
	}
	if !L.o.lr {
		for _, ch := range L.chans {
			for _, sp := range ch.spots {
				lo = min(lo, sp.x0)
				hi = max(hi, sp.x0+sp.w)
			}
		}
	}
	return lo, hi
}

// boxPos returns the canvas position of a real node's box.
func (L *layout) boxPos(ln *lnode, sMin int) (int, int) {
	if L.o.lr {
		return L.top[ln.layer], ln.s + ln.boxOff - sMin
	}
	return ln.s - sMin, L.top[ln.layer]
}

// startP is the primary coordinate where a segment leaves its upper node.
func (L *layout) startP(v, a int) int {
	ln := &L.nodes[v]
	top := L.top[ln.layer]
	if ln.real < 0 {
		return top + L.band[ln.layer] - 1
	}
	b := &L.boxes[ln.real]
	if L.o.lr {
		_, rc := b.extent(a - ln.s - ln.boxOff)
		return top + rc
	}
	return top + b.h - 1
}

// endP is the primary coordinate where a segment reaches its lower node.
func (L *layout) endP(v, b int) int {
	ln := &L.nodes[v]
	top := L.top[ln.layer]
	if ln.real < 0 || !L.o.lr {
		return top
	}
	lc, _ := L.boxes[ln.real].extent(b - ln.s - ln.boxOff)
	return top + lc
}

func (L *layout) xy(q pt, sMin int) (int, int) {
	if L.o.lr {
		return q.p, q.s - sMin
	}
	return q.s - sMin, q.p
}

func (L *layout) line(c *canvas, sMin int, st lineStyle, pts ...pt) {
	for i := 1; i < len(pts); i++ {
		x0, y0 := L.xy(pts[i-1], sMin)
		x1, y1 := L.xy(pts[i], sMin)
		c.seg(x0, y0, x1, y1, st, styled.Edge)
	}
}

// arrow returns the glyph for an edge end pointing into a node; up selects
// the end at the upper node.
func (L *layout) arrow(k endKind, up bool) rune {
	switch k {
	case endCircle:
		return '○'
	case endCross:
		return '✕'
	}
	switch {
	case L.o.lr && up:
		return '◀'
	case L.o.lr:
		return '▶'
	case up:
		return '▲'
	}
	return '▼'
}

// draw paints nodes, edges, arrow heads, labels and self-loops.
func (L *layout) draw(c *canvas, sMin int) {
	for i := range L.nodes {
		ln := &L.nodes[i]
		if ln.real >= 0 {
			x, y := L.boxPos(ln, sMin)
			L.boxes[ln.real].draw(c, x, y)
		}
	}
	for si, sg := range L.segs {
		st := L.edges[sg.edge].style
		p0, p1 := L.startP(sg.upper, sg.a), L.endP(sg.lower, sg.b)
		if sg.a == sg.b {
			L.line(c, sMin, st, pt{p0, sg.a}, pt{p1, sg.b})
			continue
		}
		ch := &L.chans[L.nodes[sg.upper].layer]
		pt0 := ch.start + ch.head + ch.lead + L.segs[si].track
		L.line(c, sMin, st, pt{p0, sg.a}, pt{pt0, sg.a}, pt{pt0, sg.b}, pt{p1, sg.b})
	}
	for i := range L.nodes {
		ln := &L.nodes[i]
		if ln.real < 0 {
			l := ln.layer
			L.line(c, sMin, L.edges[ln.edge].style, pt{L.top[l] - 1, ln.s}, pt{L.top[l] + L.band[l], ln.s})
		}
	}
	upSide, downSide := bitDown, bitUp
	if L.o.lr {
		upSide, downSide = bitRight, bitLeft
	}
	for _, e := range L.edges {
		f := &L.segs[e.first]
		p := L.startP(f.upper, f.a)
		if e.uEnd != endNone {
			x, y := L.xy(pt{p + 1, f.a}, sMin)
			c.put(x, y, L.arrow(e.uEnd, true), styled.Arrow)
		} else {
			x, y := L.xy(pt{p, f.a}, sMin)
			c.put(x, y, portGlyph(c.rune(x, y), upSide, e.style), styled.Border)
		}
		s := &L.segs[e.last]
		p = L.endP(s.lower, s.b)
		if e.vEnd != endNone {
			x, y := L.xy(pt{p - 1, s.b}, sMin)
			c.put(x, y, L.arrow(e.vEnd, false), styled.Arrow)
		} else {
			x, y := L.xy(pt{p, s.b}, sMin)
			c.put(x, y, portGlyph(c.rune(x, y), downSide, e.style), styled.Border)
		}
	}
	for ci := range L.chans {
		ch := &L.chans[ci]
		for _, sp := range ch.spots {
			if L.o.lr {
				x, y := L.xy(pt{ch.start + ch.head + ch.lead + ch.tracks + 1, sp.b}, sMin)
				c.text(x, y, sp.lines[0], styled.Label)
				continue
			}
			for i, l := range sp.lines {
				x, y := L.xy(pt{ch.start + ch.head + ch.lead + ch.tracks + sp.row + i, sp.x0 + (sp.w-styled.Width(l))/2}, sMin)
				c.text(x, y, l, styled.Label)
			}
		}
	}
	for i := range L.nodes {
		ln := &L.nodes[i]
		if ln.real >= 0 && L.loops[ln.real].ok {
			L.drawLoop(c, ln, sMin)
		}
	}
}

// drawLoop draws a self-loop: on the right side for TD layouts and above
// the box for LR layouts, where the neighboring slots leave room for it.
func (L *layout) drawLoop(c *canvas, ln *lnode, sMin int) {
	b := &L.boxes[ln.real]
	if b.w < 3 || b.h < 3 {
		return
	}
	lp := L.loops[ln.real]
	x, y := L.boxPos(ln, sMin)
	if L.o.lr {
		lo, _ := b.tbRange(false)
		c1, c2 := x+lo, x+lo+3
		c.seg(c1, y, c1, y-1, lp.style, styled.Edge)
		c.seg(c1, y-1, c2, y-1, lp.style, styled.Edge)
		c.seg(c2, y-1, c2, y, lp.style, styled.Edge)
		c.put(c1, y, portGlyph(c.rune(c1, y), bitUp, lp.style), styled.Border)
		c.put(c2, y, '▼', styled.Arrow)
		c.text(c2+2, y-1, lp.label, styled.Label)
		return
	}
	r1, r2 := b.h/2, b.h-1
	_, re1 := b.extent(r1)
	_, re2 := b.extent(r2)
	xs, xe := x+re1, x+re2+1
	lc := max(xs+1, xe) + 1
	c.seg(xs, y+r1, lc, y+r1, lp.style, styled.Edge)
	c.seg(lc, y+r1, lc, y+r2, lp.style, styled.Edge)
	c.seg(lc, y+r2, xe, y+r2, lp.style, styled.Edge)
	c.put(xs, y+r1, portGlyph(c.rune(xs, y+r1), bitRight, lp.style), styled.Border)
	c.put(xe, y+r2, '◀', styled.Arrow)
	c.text(lc+2, y+r1, lp.label, styled.Label)
}
