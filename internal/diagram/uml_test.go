package diagram

import (
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/DipaolaFederico/kmd/internal/styled"
)

const (
	goldenClass = `┌──────────┐
│  Animal  │
├──────────┤
│ +int age │
└──────────┘
      △
 ┌────┴────┐
 │  Duck   │
 ├─────────┤
 │ +swim() │
 └─────────┘`
	goldenER = `┌──────────┐
│ CUSTOMER │
└─────┬────┘
      │
      1
   places
    0..*
      │
┌─────┴─────┐
│   ORDER   │
├───────────┤
│ int id PK │
└───────────┘`
	goldenGantt = `          2024-01-01   2024-01-03  2024-01-05   2024-01-07
          ┬────────────┬───────────┬────────────┬───────────
Plan
  Spec    ▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒
  Build                     ▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓
Ship
  Release                                                  ◆

▓ active  ▒ done  ◆ milestone`
)

func TestGoldenUML(t *testing.T) {
	for name, tc := range map[string]struct{ src, want string }{
		"class": {"classDiagram\n  Animal <|-- Duck\n  Animal : +int age\n  Duck : +swim()\n", goldenClass},
		"er":    {"erDiagram\n  CUSTOMER ||--o{ ORDER : places\n  ORDER {\n    int id PK\n  }\n", goldenER},
		"gantt": {"gantt\n  dateFormat YYYY-MM-DD\n  section Plan\n  Spec :done, s, 2024-01-01, 3d\n  Build :active, after s, 5d\n" +
			"  section Ship\n  Release :milestone, 2024-01-09, 0d\n", goldenGantt},
	} {
		if got := joined(render(t, tc.src, 60)); got != tc.want {
			t.Errorf("%s: got:\n%s\nwant:\n%s", name, got, tc.want)
		}
	}
}

func mustContainAll(t *testing.T, got string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q in\n%s", w, got)
		}
	}
}

func TestClassDiagram(t *testing.T) {
	src := `classDiagram-v2
  %% comment
  accTitle: classes
  namespace Shapes {
    class Shape {
      <<interface>>
      +area() double
    }
    class Circle["Round thing"]:::big {
      -double r
      +List~Point~ pts
    }
  }
  <<abstract>> Base
  class ` + "`Odd name`" + `
  Shape <|.. Circle : implements
  Circle "1" o-- "0..*" Point : center
  Point --> Point : next
  Base <|-- Shape
  Car *-- Wheel
  Wheel --* Hub
  A ..> B
  C -- D
  E .. F
  G ()-- H
  I --|> J
  note for A "ignored"
  click A call cb()
  style A fill:#f00
  K
`
	got := joined(render(t, src, 200))
	mustContainAll(t, got, "«interface»", "Shape", "+area() double", "Round thing", "-double r", "+List<Point> pts",
		"«abstract»", "Odd name", "implements", "center", "0..*", "next", "△", "◇", "◆", "▼", "┆", "○", "Hub", "K")

	// generics, also nested
	if g := generics("Map~K, List~V~~"); g != "Map<K, List<V>>" {
		t.Errorf("generics: %q", g)
	}
	// left-to-right layout keeps a one-line `card ── label ── card` label
	got = joined(render(t, "classDiagram\ndirection LR\nA \"1\" --> \"*\" B : has", 80))
	mustContainAll(t, got, "1 ── has ── *", "▶")
	// big compartments are elided and long members truncated
	var b strings.Builder
	b.WriteString("classDiagram\nclass Big {\n")
	for i := 0; i < 50; i++ {
		b.WriteString("  +field" + itoa(i) + " " + strings.Repeat("x", 60) + "\n")
	}
	b.WriteString("}\n")
	got = joined(render(t, b.String(), 120))
	mustContainAll(t, got, "+21 more", "…")
	if strings.Contains(got, "field40") {
		t.Errorf("rows beyond the cap shown:\n%s", got)
	}
	// an annotation on an existing class prepends a stereotype line; like
	// Mermaid, only one is shown
	got = joined(render(t, "classDiagram\nclass A\n<<service>> A\nA : <<x>>", 60))
	mustContainAll(t, got, "«service»")
	if strings.Contains(got, "«x»") {
		t.Errorf("second stereotype shown:\n%s", got)
	}

	for _, bad := range []string{
		"classDiagram\n",
		"classDiagram\n}",
		"classDiagram\nA <|-- ",
		"classDiagram\nA <|-- B C",
		"classDiagram\nA \"1 --> B",
		"classDiagram\nA --> \"1 B",
		"classDiagram\n<<x A",
		"classDiagram\nnamespace X",
		"classDiagram\ndirection XX",
		"classDiagram\nA B",
		"classDiagram\nclass A[",
		"classDiagram\nclass",
		"classDiagram\nA{ : x",
		"classDiagram\n~T~ : x",
		"classDiagram\n<<x>> A B",
	} {
		if _, ok := Mermaid(bad, 80); ok {
			t.Errorf("expected failure for %q", bad)
		}
	}
	// class block left open swallows the rest as members, without failing
	if _, ok := Mermaid("classDiagram\nclass A {\n+x", 80); !ok {
		t.Error("unterminated class block should still render")
	}
}

func TestERDiagram(t *testing.T) {
	src := `erDiagram
  direction TB
  CUSTOMER ||--o{ ORDER : places
  ORDER ||--|{ LINE-ITEM : "contains"
  CUSTOMER }|..|{ DELIVERY-ADDRESS : uses
  PERSON one or more optionally to zero or more CAR : drives
  CAR only one to only one ENGINE : has
  CAR |o--o| SPARE : ""
  p[Person]
  q["Quoted alias"] {
    string name
  }
  CUSTOMER {
    string name
    string custNumber PK, FK "the key"
    string sector "market sector"
  }
  ORDER {
    int orderNumber PK
    varchar(255) deliveryAddress FK
  }
  style CUSTOMER fill:#f00
  accDescr: x
`
	got := joined(render(t, src, 200))
	mustContainAll(t, got, "CUSTOMER", "places", "0..*", "1..*", "0..1", "contains", "drives", "┆",
		"string custNumber PK,FK the key", "Person", "Quoted alias", "varchar(255)", "LINE-ITEM", "SPARE")
	got = joined(render(t, "erDiagram\ndirection LR\nA ||--o{ B : has", 80))
	mustContainAll(t, got, "1 ── has ── 0..*")
	for _, bad := range []string{
		"erDiagram\n",
		"erDiagram\nA ||--x{ B : has",
		"erDiagram\nA }o--o} B : has",
		"erDiagram\nA ||-o{ B : has",
		"erDiagram\nA one to many B : x",
		"erDiagram\nA ||--o{ B C : x",
		"erDiagram\nA : x",
		"erDiagram\nA:B ||--o{ C : x",
		"erDiagram\nA {\n onlytype\n}",
		"erDiagram\nA {\n string x XX\n}",
		"erDiagram\nA {\n string x \"open\n}",
		"erDiagram\nA B",
		"erDiagram\ndirection XX",
		"erDiagram\nA zero or more to zero or more : x",
	} {
		if _, ok := Mermaid(bad, 80); ok {
			t.Errorf("expected failure for %q", bad)
		}
	}
}

func TestGantt(t *testing.T) {
	src := `---
title: Front title
---
gantt
  dateFormat YYYY-MM-DD
  axisFormat %b %d
  excludes weekends
  todayMarker off
  accTitle: x
  Kickoff :k, 2024-01-01, 1d
  section Design
  Spec :done, s, after k, 3d
  Review :crit, active, r, after s k, 2d
  section Build
  Code :c, 2024-01-08, until rel
  Rel :milestone, rel, 2024-01-20, 0d
  Docs :2w
`
	got := joined(render(t, src, 80))
	mustContainAll(t, got, "Front title", "Jan 01", "Design", "Build", "Kickoff", "▒", "▓", "█", "◆", "▓ active")
	lines, _ := Mermaid(src, 80)
	crit := false
	for _, l := range lines {
		for _, s := range l {
			crit = crit || s.Class == styled.Series6
		}
	}
	if !crit {
		t.Error("critical task not colored")
	}
	// hour durations, inclusive end dates, calendar-month and year ticks
	got = joined(render(t, "gantt\ndateFormat YYYY-MM-DD HH:mm\nA :a, 2024-05-01 08:00, 3h\nB :after a, 90m\nC : 30s\nD : 500ms", 60))
	mustContainAll(t, got, "05-01 08:00")
	got = joined(render(t, "gantt\ninclusiveEndDates\nA :2024-01-01, 2024-01-01\nB :2024-06-01, 2024-09-30", 60))
	mustContainAll(t, got, "2024-01", "2024-07")
	got = joined(render(t, "gantt\ntitle Ages\nA :0100-01-01, 9000-01-01", 40))
	mustContainAll(t, got, "Ages", "2500")
	got = joined(render(t, "gantt\ndateFormat DD/MM/YYYY\naxisFormat %Q\nA :01/02/2024, 3d", 40))
	mustContainAll(t, got, "02-01")
	for _, f := range []string{"MMMM D, YYYY", "MMM D YYYY h:mm A", "YY-M-D H:m:s Z"} {
		if _, ok := dayjsLayout(f); !ok {
			t.Errorf("dayjsLayout(%q) failed", f)
		}
	}
	if strftimeLayout("%Y-%m-%d %H:%M:%S %p %j %e %a %A %B %y %I") == "" || strftimeLayout("day %d") != "" {
		t.Error("strftimeLayout")
	}

	for _, bad := range []string{
		"gantt\n",
		"gantt\ntitle only",
		"gantt\nA : 3d",                                     // nothing to start from
		"gantt\nA :2024-01-01",                              // end without start
		"gantt\nA :a, 2024-13-01, 1d",                       // bad date
		"gantt\nA :after zz, 1d",                            // unknown id
		"gantt\nA :a, after b, 1d\nB :b, after a, 1d",       // cycle
		"gantt\nA :2024-01-05, 2024-01-01",                  // end before start
		"gantt\nA :a, 2024-01-01, 1d\nB :a, 2024-01-01, 1d", // duplicate id
		"gantt\nA :x, y, 2024-01-01, 1d",                    // too many fields
		"gantt\nA :done",                                    // only tags
		"gantt\nA :2024-01-01, ",                            // empty end
		"gantt\ndateFormat YYYY-MM-DDxx\nA :2024-01-01, 1d", // literal letters
		"gantt\ndateFormat X\nA :1, 1d",                     // unsupported token
		"gantt\nno colon here",
		"gantt\nA :2024-01-01, 1e400d",
		"gantt\nA :2024-01-01, 99999999999999999d",
		"gantt\nA :2024-01-01, until",
		"gantt\nA :a, 2024-01-01, 1d\nB :after , 1d",
		"gantt\ndateFormat YYYY\nA :0000, 1d",
	} {
		if _, ok := Mermaid(bad, 80); ok {
			t.Errorf("expected failure for %q", bad)
		}
	}
	if _, ok := Mermaid("gantt\nA :2024-01-01, 1d", 10); ok {
		t.Error("expected failure when too narrow")
	}
	// very long names are truncated to a third of the width
	got = joined(render(t, "gantt\n"+strings.Repeat("n", 100)+" :2024-01-01, 1d", 40))
	mustContainAll(t, got, "…")
}

func TestStackComponents(t *testing.T) {
	var b strings.Builder
	b.WriteString("graph TD\n")
	for i := 0; i < 6; i++ {
		b.WriteString("  a" + itoa(i) + "[Component " + itoa(i) + "] --> b" + itoa(i) + "\n")
	}
	got := render(t, b.String(), 30)
	if all := joined(got); !strings.Contains(all, "Component 0") || !strings.Contains(all, "Component 5") || strings.Count(all, "▼") != 6 {
		t.Errorf("components not stacked:\n%s", all)
	}
	// side by side when they fit
	if wide := render(t, b.String(), 200); len(wide) >= len(got) {
		t.Errorf("components stacked although they fit side by side")
	}
	// a single component that does not fit still fails
	if _, ok := Mermaid("graph LR\nA[a very long label here] --> B", 9); ok {
		t.Error("expected failure")
	}
	// one component that cannot fit makes the whole graph fail
	if _, ok := Mermaid("graph TD\nA --> B\nC[a very long label here]", 9); ok {
		t.Error("expected failure")
	}
	// the stacked output is held to the cell budget
	b.Reset()
	b.WriteString("graph TD\n")
	for i := 0; i < 300; i++ {
		b.WriteString("  n" + itoa(i) + "[" + strings.Repeat("w", 20) + "]\n")
	}
	if _, ok := Mermaid(b.String(), 1000); ok {
		t.Error("stacked output beyond the cell budget accepted")
	}
}

// TestUMLHostile checks that hostile inputs within the source cap are
// rejected or rendered quickly and within the width.
func TestUMLHostile(t *testing.T) {
	timed := func(name, src string, width int) {
		t.Helper()
		start := time.Now()
		lines, ok := Mermaid(src, width)
		if d := time.Since(start); d > raceFactor()*500*time.Millisecond {
			t.Errorf("%s took %v", name, d)
		}
		if ok {
			checkWidth(t, lines, width)
		}
	}
	var b strings.Builder
	build := func(head string, n int, line func(i int) string) string {
		b.Reset()
		b.WriteString(head + "\n")
		for i := 0; i < n && b.Len() < maxSource-200; i++ {
			b.WriteString(line(i) + "\n")
		}
		return b.String()
	}
	// a reversed `after` chain resolves through maxTasks levels of recursion
	chain := build("gantt", maxTasks-1, func(i int) string {
		return "T :t" + itoa(i) + ", after t" + itoa(i+1) + ", 1d"
	}) + "L :t" + itoa(maxTasks-1) + ", 2024-01-01, 1d"
	timed("gantt chain", chain, 200)
	if _, ok := Mermaid(chain, 200); !ok {
		t.Error("reversed after chain not resolved")
	}
	// every task waits for every earlier one: refs are resolved once each
	ids := ""
	timed("gantt fan-in", build("gantt", maxTasks, func(i int) string {
		if i == 0 {
			return "T :t0, 2024-01-01, 1d"
		}
		ids += " t" + itoa(i-1)
		if len(ids) > 400 {
			ids = " t0"
		}
		return "T :t" + itoa(i) + ", after" + ids + ", 1h"
	}), 1000)
	if _, ok := Mermaid(build("gantt", maxTasks+1, func(i int) string { return "T :2024-01-01, 1d" }), 200); ok {
		t.Error("too many tasks accepted")
	}
	timed("gantt huge span", "gantt\nA :0001-01-01, 9999-12-31\nB :5000-01-01, 1ms", 1000)
	timed("class members", build("classDiagram", 1<<20, func(i int) string { return "A : +m" + itoa(i) + "()" }), 200)
	timed("class relations", build("classDiagram", maxEdges, func(i int) string {
		return "C" + itoa(i%maxNodes) + " <|-- C" + itoa((i*7+1)%maxNodes)
	}), 1000)
	timed("class nesting", strings.Repeat("namespace A {\n", 200)+"classDiagram", 80)
	timed("class nesting2", "classDiagram\n"+strings.Repeat("namespace A {\n", 200), 80)
	timed("er attrs", build("erDiagram\nA {", 1<<20, func(i int) string { return "string a" + itoa(i) + " PK \"c\"" }), 200)
	// Regressions from the security and performance reviews: work must not
	// be quadratic in repeated stereotypes, one huge attribute field must
	// not pad every row, and long formats are rejected.
	allocs := func(name, src string, limit uint64) {
		t.Helper()
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		timed(name, src, 200)
		runtime.ReadMemStats(&after)
		if got := after.TotalAlloc - before.TotalAlloc; got > limit {
			t.Errorf("%s allocated %d bytes", name, got)
		}
	}
	allocs("stereotypes", build("classDiagram\nclass A {", 1<<20, func(int) string { return "<<a>>" }), 32<<20)
	allocs("stereotypes flat", build("classDiagram", 1<<20, func(int) string { return "<<a>> A" }), 32<<20)
	allocs("er wide field", build("erDiagram\nE {\n"+strings.Repeat("x", 30000)+" a", 1<<20, func(int) string { return "a b" }), 32<<20)
	if _, ok := Mermaid("gantt\ndateFormat YYYY"+strings.Repeat(" ", 100)+"MM\nA :2024 01, 1d", 80); ok {
		t.Error("long dateFormat accepted")
	}
	got := joined(render(t, "gantt\naxisFormat "+strings.Repeat("%Y", 40)+"\nA :2024-01-01, 1d", 80))
	mustContainAll(t, got, "01-01 00:00") // ignored: automatic labels
	got = joined(render(t, build("erDiagram\nE {", 40, func(i int) string { return "int a" + itoa(i) }), 80))
	mustContainAll(t, got, "+11 more")
	if _, ok := Mermaid("erDiagram\nE {\nint a PK FK UK PK\n}", 80); ok {
		t.Error("too many keys accepted")
	}
	timed("one big component plus a node", build("graph LR", maxEdges, func(i int) string {
		return "a" + itoa(i%31) + " --> b" + itoa(i/31%31)
	})+"zz\n", 80)

	timed("er relations", build("erDiagram", maxEdges+10, func(i int) string {
		return "E" + itoa(i%300) + " ||--o{ E" + itoa((i*13+5)%300) + " : r"
	}), 1000)
	if _, ok := Mermaid(build("erDiagram", maxNodes+1, func(i int) string { return "E" + itoa(i) }), 200); ok {
		t.Error("too many entities accepted")
	}
}
