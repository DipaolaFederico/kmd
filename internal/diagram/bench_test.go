package diagram

import (
	"fmt"
	"strings"
	"testing"
)

// genFlowchart builds a deterministic pseudo-random flowchart with n nodes
// and roughly 1.4 edges per node, including back edges and labels.
func genFlowchart(n int, dir string) string {
	var b strings.Builder
	b.WriteString("flowchart " + dir + "\n")
	seed := uint32(7)
	next := func(m int) int {
		seed = seed*1664525 + 1013904223
		return int(seed>>8) % m
	}
	shapes := []string{"[%s]", "(%s)", "{%s}", "([%s])", "[(%s)]", "((%s))"}
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "    n%d"+shapes[i%len(shapes)]+"\n", i, fmt.Sprintf("Node %d", i))
	}
	for i := 1; i < n; i++ {
		p := next(i)
		if i%5 == 0 {
			fmt.Fprintf(&b, "    n%d -->|step %d| n%d\n", p, i, i)
		} else {
			fmt.Fprintf(&b, "    n%d --> n%d\n", p, i)
		}
		if i%3 == 0 {
			fmt.Fprintf(&b, "    n%d -.-> n%d\n", i, next(n))
		}
	}
	return b.String()
}

func BenchmarkFlowchart30(b *testing.B) {
	src := genFlowchart(30, "TD")
	b.ReportAllocs()
	for b.Loop() {
		if _, ok := Mermaid(src, 200); !ok {
			b.Fatal("not rendered")
		}
	}
}

func BenchmarkFlowchart200(b *testing.B) {
	src := genFlowchart(200, "TD")
	b.ReportAllocs()
	for b.Loop() {
		Mermaid(src, 1000)
	}
}

func BenchmarkSequence(b *testing.B) {
	var sb strings.Builder
	sb.WriteString("sequenceDiagram\n")
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&sb, "  P%d->>P%d: message number %d\n", i%6, (i*7+1)%6, i)
	}
	src := sb.String()
	b.ReportAllocs()
	for b.Loop() {
		if _, ok := Mermaid(src, 160); !ok {
			b.Fatal("not rendered")
		}
	}
}

func BenchmarkClass(b *testing.B) {
	var sb strings.Builder
	sb.WriteString("classDiagram\n")
	for i := 1; i < 20; i++ {
		fmt.Fprintf(&sb, "  C%d <|-- C%d\n  C%d : +field%d int\n  C%d : +method%d() bool\n", (i-1)/3, i, i, i, i, i)
	}
	src := sb.String()
	b.ReportAllocs()
	for b.Loop() {
		if _, ok := Mermaid(src, 300); !ok {
			b.Fatal("not rendered")
		}
	}
}

func BenchmarkGantt(b *testing.B) {
	var sb strings.Builder
	sb.WriteString("gantt\n  dateFormat YYYY-MM-DD\n  section A\n  T0 :t0, 2024-01-01, 3d\n")
	for i := 1; i < 40; i++ {
		fmt.Fprintf(&sb, "  T%d :t%d, after t%d, %dd\n", i, i, i-1, i%5+1)
	}
	src := sb.String()
	b.ReportAllocs()
	for b.Loop() {
		if _, ok := Mermaid(src, 120); !ok {
			b.Fatal("not rendered")
		}
	}
}
