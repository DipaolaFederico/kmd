// Package diagram renders Mermaid and Graphviz DOT diagrams as Unicode
// terminal art.
//
// Flowcharts, state diagrams and DOT graphs share a layered layout engine
// (cycle breaking, longest-path layering, dummy nodes, barycenter ordering,
// least-squares coordinate assignment and orthogonal channel routing on a
// junction-merging character canvas). Sequence diagrams and pie charts have
// dedicated renderers. Output is a list of styled lines whose spans carry
// semantic classes; the terminal renderer maps them to theme colors.
//
// Input is treated as untrusted: sizes are capped, nothing recurses without
// a bound, and any failure yields ok=false so the caller can fall back to
// showing the source.
package diagram

import (
	"strings"

	"github.com/DipaolaFederico/kmd/internal/styled"
)

// Hard limits for untrusted input.
const (
	maxSource      = 64 << 10 // bytes of diagram source
	maxNodes       = 500      // graph nodes
	maxEdges       = 2000     // graph edges
	maxLayoutNodes = 8000     // real plus dummy layout nodes
	maxCells       = 1 << 19  // canvas cells
	maxRenderWidth = 1000     // widths beyond this are clamped
	minRenderWidth = 8
	maxParts       = 100  // sequence participants
	maxEvents      = 2000 // sequence events
	maxSlices      = 200  // pie slices
	maxDepth       = 32   // DOT subgraph nesting
)

// Mermaid renders Mermaid source as terminal art no wider than width columns.
// ok is false when the diagram type is unsupported, the source is invalid,
// or it cannot fit; the caller then shows the source as a code block.
func Mermaid(src string, width int) (lines []styled.Line, ok bool) {
	defer func() {
		if recover() != nil {
			lines, ok = nil, false
		}
	}()
	return mermaid(src, width)
}

// Dot renders a Graphviz DOT subset (digraph/graph, node/edge statements,
// label attributes, rankdir=LR) using the same layout engine.
func Dot(src string, width int) (lines []styled.Line, ok bool) {
	defer func() {
		if recover() != nil {
			lines, ok = nil, false
		}
	}()
	return dot(src, width)
}

func clampWidth(width int) (int, bool) {
	if width < minRenderWidth {
		return 0, false
	}
	return min(width, maxRenderWidth), true
}

func mermaid(src string, width int) ([]styled.Line, bool) {
	width, ok := clampWidth(width)
	if !ok || len(src) > maxSource {
		return nil, false
	}
	title, lines := preprocess(src)
	if len(lines) == 0 {
		return nil, false
	}
	header, body := lines[0], lines[1:]
	kw, _ := splitWord(header)
	if i := strings.IndexByte(kw, ';'); i >= 0 {
		kw = kw[:i]
	}
	switch kw {
	case "graph", "flowchart", "flowchart-elk":
		g, ok := parseFlowchart(header, body)
		if !ok {
			return nil, false
		}
		return renderGraph(g, title, width)
	case "stateDiagram", "stateDiagram-v2":
		g, ok := parseState(body)
		if !ok {
			return nil, false
		}
		return renderGraph(g, title, width)
	case "sequenceDiagram":
		d, ok := parseSequence(body)
		if !ok {
			return nil, false
		}
		if d.title == "" {
			d.title = title
		}
		return renderSequence(d, width)
	case "pie":
		p, ok := parsePie(header, body)
		if !ok {
			return nil, false
		}
		if p.title == "" {
			p.title = title
		}
		return p.render(width)
	}
	return nil, false
}

func dot(src string, width int) ([]styled.Line, bool) {
	width, ok := clampWidth(width)
	if !ok || len(src) > maxSource {
		return nil, false
	}
	g, title, ok := parseDot(src)
	if !ok {
		return nil, false
	}
	return renderGraph(g, title, width)
}

// preprocess splits src into trimmed, non-empty lines, extracting a YAML
// front-matter title and dropping %% comments and %%{...}%% directives.
func preprocess(src string) (string, []string) {
	raw := strings.Split(src, "\n")
	var title string
	i := 0
	for i < len(raw) && strings.TrimSpace(raw[i]) == "" {
		i++
	}
	if i < len(raw) && strings.TrimSpace(raw[i]) == "---" {
		j := i + 1
		for j < len(raw) && strings.TrimSpace(raw[j]) != "---" {
			if k, v, ok := strings.Cut(strings.TrimSpace(raw[j]), ":"); ok && k == "title" {
				title = cleanLabel(unquote(strings.TrimSpace(v)))
			}
			j++
		}
		if j < len(raw) {
			i = j + 1
		}
	}
	var out []string
	inDirective := false
	for ; i < len(raw); i++ {
		l := strings.TrimSpace(raw[i])
		if inDirective {
			if strings.Contains(l, "}%%") {
				inDirective = false
			}
			continue
		}
		if strings.HasPrefix(l, "%%{") {
			inDirective = !strings.Contains(l, "}%%")
			continue
		}
		if l == "" || strings.HasPrefix(l, "%%") {
			continue
		}
		out = append(out, l)
	}
	return title, out
}
