package diagram

import (
	"strings"
	"testing"

	"github.com/DipaolaFederico/kmd/internal/styled"
)

const (
	goldenTD = `┌───────┐
│ Start │
└───┬───┘
    ▼
 ╭─────╮
 │ End │
 ╰─────╯`
	goldenLR = `┌───┐        ┌───┐
│ A ├───────▶│ B ├┄╮  ┌───┐
│   ┝━┓      └───┘ ╰┄▶│ C │
└───┘ ┃            ┏━▶│   │
      ┗━go━━━━━━━━━┛  └───┘`
	goldenSeq = `┌───────┐   ┌─────┐
│ Alice │   │ Bob │
└───┬───┘   └──┬──┘
    ┆    Hi    ┆
    ┆─────────▶┆
    ┆   Hello  ┆
    ┆◀┄┄┄┄┄┄┄┄┄┆
    ┆          ┆`
	goldenPie = `Pets

Dogs  ███████████████████████████████████████████████  75.0%
Cats  ███████████████▋                                 25.0%`
	goldenState = `   ●
   ▼
╭────╮
│ On │
╰──┬─╯
   │
 flip
   ▼
╭─────╮
│ Off │
╰──┬──╯
   ▼
   ◉`
	goldenDot = `╭───╮
│ a │
╰─┬─╯
  │
  x
  ▼
╭───╮
│ b │
╰───╯`
)

// render renders src (Mermaid, or DOT when dot is set) and returns plain
// lines, failing the test when the result is not ok or exceeds width.
func render(t *testing.T, src string, width int) []string {
	t.Helper()
	lines, ok := Mermaid(src, width)
	if !ok {
		t.Fatalf("Mermaid(%q, %d) not ok", src, width)
	}
	checkWidth(t, lines, width)
	return plainLines(lines)
}

func checkWidth(t *testing.T, lines []styled.Line, width int) {
	t.Helper()
	for i, l := range lines {
		if w := styled.LineWidth(l); w > width {
			t.Fatalf("line %d is %d columns wide, limit %d: %q", i, w, width, plainLines(lines)[i])
		}
		for _, s := range l {
			if s.Text == "" {
				t.Fatalf("line %d has an empty span", i)
			}
		}
		if n := len(l); n > 0 && strings.HasSuffix(l[n-1].Text, " ") {
			t.Fatalf("line %d has trailing spaces: %q", i, l[n-1].Text)
		}
	}
}

func joined(lines []string) string { return strings.Join(lines, "\n") }

func TestGolden(t *testing.T) {
	tests := []struct {
		name, src string
		dot       bool
		want      string
	}{
		{"td", "graph TD\n  A[Start] --> B(End)\n", false, goldenTD},
		{"lr", "graph LR\n  A --> B -.-> C\n  A ==>|go| C\n", false, goldenLR},
		{"sequence", "sequenceDiagram\n  Alice->>Bob: Hi\n  Bob-->>Alice: Hello\n", false, goldenSeq},
		{"pie", "pie title Pets\n  \"Dogs\" : 3\n  \"Cats\" : 1\n", false, goldenPie},
		{"state", "stateDiagram-v2\n  [*] --> On\n  On --> Off : flip\n  Off --> [*]\n", false, goldenState},
		{"dot", "digraph { a -> b [label=\"x\"]; }\n", true, goldenDot},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var lines []styled.Line
			var ok bool
			if tt.dot {
				lines, ok = Dot(tt.src, 60)
			} else {
				lines, ok = Mermaid(tt.src, 60)
			}
			if !ok {
				t.Fatal("not ok")
			}
			checkWidth(t, lines, 60)
			if got := joined(plainLines(lines)); got != tt.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

func TestClasses(t *testing.T) {
	lines, ok := Mermaid("---\ntitle: Demo\n---\ngraph TD\n  A[x] -->|lbl| B", 40)
	if !ok {
		t.Fatal("not ok")
	}
	seen := map[styled.Class]string{}
	for _, l := range lines {
		for _, s := range l {
			seen[s.Class] += s.Text
		}
	}
	for cls, want := range map[styled.Class]string{
		styled.Title: "Demo", styled.Border: "┌", styled.Node: "x", styled.Arrow: "▼", styled.Label: "lbl", styled.Edge: "│",
	} {
		if !strings.Contains(seen[cls], want) {
			t.Errorf("class %d: %q does not contain %q", cls, seen[cls], want)
		}
	}
	pie, ok := Mermaid("pie\n\"a\":1\n\"b\":2\n\"c\":3", 40)
	if !ok {
		t.Fatal("pie not ok")
	}
	for i, l := range pie {
		found := false
		for _, s := range l {
			if s.Class == styled.Series(i) && strings.Contains(s.Text, "█") {
				found = true
			}
		}
		if !found {
			t.Errorf("pie row %d has no Series(%d) bar: %#v", i, i, l)
		}
	}
}

func TestShapes(t *testing.T) {
	tests := []struct{ src, want string }{
		{"A[rect]", "┌──────┐\n│ rect │\n└──────┘"},
		{"A(round)", "╭───────╮\n│ round │\n╰───────╯"},
		{"A([stadium])", "╭─────────╮\n( stadium )\n╰─────────╯"},
		{"A[[sub]]", "╓─────╖\n║ sub ║\n╙─────╜"},
		{"A[(db)]", "╭────╮\n├────┤\n│ db │\n╰────╯"},
		{"A((c))", " ╭─╮\n╭╯ ╰╮\n│ c │\n╰╮ ╭╯\n ╰─╯"},
		{"A{ok?}", "╱─────╲\n< ok? >\n╲─────╱"},
		{"A{{hex}}", " ─────\n< hex >\n ─────"},
		{"A>asym]", "╲──────┐\n> asym │\n╱──────┘"},
		{"A[/in/]", "  ╱────╱\n ╱ in ╱\n╱────╱"},
		{"A[\\out\\]", "╲─────╲\n ╲ out ╲\n  ╲─────╲"},
		{"A[/trap\\]", "  ╱──────╱\n ╱ trap ╱\n╱──────╱"},
		{"A(((dbl)))", " ╭───╮\n╭╯   ╰╮\n│ dbl │\n╰╮   ╭╯\n ╰───╯"},
		{"A[\"quoted (parens)\"]", "┌─────────────────┐\n│ quoted (parens) │\n└─────────────────┘"},
		{"A", "┌───┐\n│ A │\n└───┘"},
		{"A[a [nested] b]", "┌──────────────┐\n│ a [nested] b │\n└──────────────┘"},
		{"A@{ shape: cyl, label: \"Store\" }", "╭───────╮\n├───────┤\n│ Store │\n╰───────╯"},
		{"A[\"`**bold** md`\"]", "┌─────────┐\n│ bold md │\n└─────────┘"},
	}
	for _, tt := range tests {
		got := joined(render(t, "graph TD\n"+tt.src, 40))
		if got != tt.want {
			t.Errorf("%s:\ngot:\n%s\nwant:\n%s", tt.src, got, tt.want)
		}
	}
}

func TestEdges(t *testing.T) {
	tests := []struct {
		src       string
		want, not []string
	}{
		{"A --> B", []string{"▼", "│"}, []string{"▲"}},
		{"A --- B", []string{"┴", "┬"}, []string{"▼"}},
		{"A ---- B", []string{"┴"}, []string{"▼"}},
		{"A -.-> B", []string{"┆", "▼"}, nil},
		{"A -.- B", []string{"┆"}, []string{"▼"}},
		{"A -..-> B", []string{"┆", "▼"}, nil},
		{"A ==> B", []string{"┃", "┰", "▼"}, nil},
		{"A === B", []string{"┰", "┸"}, []string{"▼"}},
		{"A --o B", []string{"○"}, []string{"▼"}},
		{"A --x B", []string{"✕"}, []string{"▼"}},
		{"A o--o B", []string{"○"}, []string{"▼"}},
		{"A x--x B", []string{"✕"}, nil},
		{"A <--> B", []string{"▲", "▼"}, nil},
		{"A -->|yes| B", []string{"yes", "▼"}, nil},
		{"A -- maybe --> B", []string{"maybe", "▼"}, nil},
		{"A -- open text --- B", []string{"open text"}, []string{"▼"}},
		{"A -. dotted .-> B", []string{"dotted", "┆"}, nil},
		{"A == thick ==> B", []string{"thick", "┃"}, nil},
		{"A == thick === B", []string{"thick", "┃"}, []string{"▼"}},
		{"A --> B --> C", []string{"│ A │", "│ B │", "│ C │"}, nil},
		{"A & B --> C & D", []string{"│ A │", "│ D │", "├"}, nil},
		{"A-->B;B-->C", []string{"│ C │"}, nil},
		{"A ~~~ B", []string{"│ A │", "│ B │"}, []string{"▼"}},
		{"A -->|\"q|x\"| B", []string{"q|x"}, nil},
		{"A:::cls --> B:::other", []string{"│ A │", "│ B │"}, []string{"cls"}},
		{"my-node --> other.node", []string{"my-node", "other.node"}, nil},
		{"A --> A", []string{"◀", "╮"}, nil},
		{"A -->|again| A", []string{"again", "◀"}, nil},
		{"A --> B\nB --> A", []string{"▲", "▼"}, nil},
		{"A --o B\nB --x A", []string{"○", "✕"}, nil},
	}
	for _, tt := range tests {
		got := joined(render(t, "graph TD\n"+tt.src, 60))
		for _, w := range tt.want {
			if !strings.Contains(got, w) {
				t.Errorf("%q: missing %q in\n%s", tt.src, w, got)
			}
		}
		for _, w := range tt.not {
			if strings.Contains(got, w) {
				t.Errorf("%q: unexpected %q in\n%s", tt.src, w, got)
			}
		}
	}
}

func TestDirections(t *testing.T) {
	src := func(d string) string { return "graph " + d + "\n  A[top] --> B[bottom]" }
	td := render(t, src("TD"), 40)
	if !strings.Contains(td[0], "┌") || !strings.Contains(td[1], "top") {
		t.Errorf("TD: A should be first:\n%s", joined(td))
	}
	for _, d := range []string{"TB", "v"} {
		if got := render(t, src(d), 40); joined(got) != joined(td) {
			t.Errorf("%s differs from TD:\n%s", d, joined(got))
		}
	}
	for _, d := range []string{"BT", "^"} {
		bt := render(t, src(d), 40)
		if !strings.Contains(bt[1], "bottom") || !strings.Contains(joined(bt), "▲") {
			t.Errorf("%s: B should be first with an upward arrow:\n%s", d, joined(bt))
		}
	}
	for _, d := range []string{"LR", ">"} {
		lr := render(t, src(d), 40)
		if len(lr) != 3 || !strings.Contains(lr[1], "top │───▶│ bottom") && !strings.Contains(lr[1], "top ├──▶│ bottom") {
			t.Errorf("%s: expected one row of boxes:\n%s", d, joined(lr))
		}
	}
	for _, d := range []string{"RL", "<"} {
		rl := render(t, src(d), 40)
		if len(rl) != 3 || strings.Index(rl[1], "bottom") > strings.Index(rl[1], "top") {
			t.Errorf("%s: B should be left of A:\n%s", d, joined(rl))
		}
	}
	if _, ok := Mermaid(src("XY"), 40); ok {
		t.Error("unknown direction accepted")
	}
	if got := render(t, "flowchart\nA-->B", 40); joined(got) != joined(td[:0]) && !strings.Contains(joined(got), "▼") {
		t.Errorf("default direction should be TD:\n%s", joined(got))
	}
	if got := render(t, "graph LR;A-->B;B-->C", 40); len(got) != 3 {
		t.Errorf("inline statements:\n%s", joined(got))
	}
}

func TestLRFallsBackToTD(t *testing.T) {
	src := "graph LR\n A[a fairly long label] --> B[another long label] --> C[and a third long label]"
	lr := render(t, src, 200)
	if len(lr) > 5 {
		t.Errorf("LR at 200 should be one row:\n%s", joined(lr))
	}
	td := render(t, src, 30)
	if len(td) < 9 {
		t.Errorf("LR at 30 should fall back to TD:\n%s", joined(td))
	}
}

func TestWrapWhenNarrow(t *testing.T) {
	src := "graph TD\n A[one two three four five six seven eight nine ten] --> B[x] & C[y] & D[z]"
	wide := render(t, src, 100)
	narrow := render(t, src, 24)
	if len(narrow) <= len(wide) {
		t.Errorf("narrow layout should wrap into more rows:\n%s\n---\n%s", joined(wide), joined(narrow))
	}
	if _, ok := Mermaid(src, 9); ok {
		t.Error("expected failure at width 9")
	}
}

func TestFlowchartFeatures(t *testing.T) {
	src := `---
title: "My title"
---
%%{init: {"theme": "dark",
  "flowchart": {}}}%%
flowchart TD
  %% a comment
  subgraph one [First group]
    direction LR
    a1 --> a2
  end
  subgraph two
    b1 --> b2
  end
  a2 --> b1
  classDef red fill:#f00
  class a1 red
  style b2 fill:#0f0
  linkStyle 0 stroke:#f00
  click a1 callback
  accTitle: hidden
  accDescr: hidden too
`
	got := render(t, src, 60)
	if got[0] != "My title" || got[1] != "" {
		t.Errorf("title missing:\n%s", joined(got))
	}
	all := joined(got)
	for _, w := range []string{"a1", "a2", "b1", "b2"} {
		if !strings.Contains(all, w) {
			t.Errorf("missing %s in\n%s", w, all)
		}
	}
	for _, w := range []string{"red", "callback", "hidden", "theme"} {
		if strings.Contains(all, w) {
			t.Errorf("unexpected %s in\n%s", w, all)
		}
	}
}

func TestLabels(t *testing.T) {
	got := joined(render(t, "graph TD\nA[line one<br/>line two<BR>three] -->|a<br>b| B[&quot;q&quot; #quot;r#quot; &lt;&gt; &amp;]", 60))
	for _, w := range []string{"line one", "line two", "three", `"q" "r" <> &`} {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q in\n%s", w, got)
		}
	}
	got = joined(render(t, "graph TD\nA[漢字の ラベル 🎉] --> B[tab\there]", 60))
	if !strings.Contains(got, "漢字の ラベル 🎉") || !strings.Contains(got, "tab here") {
		t.Errorf("unicode labels:\n%s", got)
	}
}

func TestBackEdgesAndLongEdges(t *testing.T) {
	src := "graph TD\nA --> B --> C --> D --> E\nE --> A\nA --> E\nB -.-> D"
	got := render(t, src, 60)
	all := joined(got)
	if !strings.Contains(all, "▲") {
		t.Errorf("back edge should point up:\n%s", all)
	}
	if n := strings.Count(all, "▼"); n < 5 {
		t.Errorf("expected at least 5 down arrows, got %d:\n%s", n, all)
	}
	lr := joined(render(t, strings.Replace(src, "TD", "LR", 1), 120))
	if !strings.Contains(lr, "◀") || !strings.Contains(lr, "▶") {
		t.Errorf("LR back edge:\n%s", lr)
	}
}

func TestLRLabelsAndLoops(t *testing.T) {
	src := "graph LR\nA -->|one| C\nB -->|two| C\nC -->|self| C\nC --> C\nD -->|x| E\nD -->|y| E"
	got := joined(render(t, src, 100))
	for _, w := range []string{"one", "two", "self", "x", "y", "▼"} {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q in\n%s", w, got)
		}
	}
	td := joined(render(t, strings.Replace(src, "LR", "TD", 1), 100))
	for _, w := range []string{"one", "two", "self", "◀"} {
		if !strings.Contains(td, w) {
			t.Errorf("TD missing %q in\n%s", w, td)
		}
	}
}

func TestManyLabelsShareChannel(t *testing.T) {
	src := "graph TD\nA --> |alpha| X\nB -->|beta| X\nC -->|gamma| X\nD -->|delta| X\nA -->|epsilon| Y\nB -->|zeta| Y"
	got := joined(render(t, src, 80))
	for _, w := range []string{"alpha", "beta", "gamma", "delta", "epsilon", "zeta"} {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q in\n%s", w, got)
		}
	}
	src = "graph LR\nA -->|alpha| X\nA -->|beta| X\nA -->|gamma| X\nB -->|delta| X\nC -->|eps| X\nD -->|zeta| X"
	got = joined(render(t, src, 120))
	for _, w := range []string{"alpha", "beta", "gamma"} {
		if !strings.Contains(got, w) {
			t.Errorf("LR missing %q in\n%s", w, got)
		}
	}
}

func TestStateDiagram(t *testing.T) {
	src := `stateDiagram-v2
    direction TB
    accTitle: x
    [*] --> First
    state "Second state" as Second
    state Third {
        [*] --> Inner
        Inner --> [*]
    }
    state choice <<choice>>
    state fork <<fork>>
    First --> Second : go
    Second --> choice
    choice --> Third : yes
    choice --> fork
    fork --> Done
    Done : All done
    Done : really
    Third --> [*]
    note right of First : a note
    note left of Second
        multi line
    end note
    classDef bad fill:red
    class Done bad
    Done:::bad --> [*]
    hide empty description
    --
    Lonely
`
	got := joined(render(t, src, 80))
	for _, w := range []string{"●", "◉", "First", "Second state", "Inner", "go", "yes", "━━━", "All done", "really", "Lonely", "< ", " >"} {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q in\n%s", w, got)
		}
	}
	for _, w := range []string{"a note", "multi line", "bad"} {
		if strings.Contains(got, w) {
			t.Errorf("unexpected %q in\n%s", w, got)
		}
	}
	lr := joined(render(t, "stateDiagram\ndirection LR\n[*] --> A\nA --> B\nstate f <<join>>\nB --> f", 80))
	if !strings.Contains(lr, "▶") || !strings.Contains(lr, "┃") {
		t.Errorf("LR state:\n%s", lr)
	}
	for _, bad := range []string{
		"stateDiagram\nfoo bar baz",
		"stateDiagram\nstate \"unterminated as X",
		"stateDiagram\nstate \"x\" is Y",
		"stateDiagram\nstate X <<weird>>",
		"stateDiagram\nA --> ",
		"stateDiagram\ndirection XX",
		"stateDiagram\n : desc",
		"stateDiagram\n",
		"stateDiagram\nstate \"x\" as",
	} {
		if _, ok := Mermaid(bad, 80); ok {
			t.Errorf("expected failure for %q", bad)
		}
	}
}

func TestSequence(t *testing.T) {
	src := `sequenceDiagram
    title: Checkout
    autonumber
    actor U as User
    participant W as Web<br/>Server
    create participant DB
    U->>+W: Login
    activate W
    W->>W: validate
    W-->>DB: query
    DB--)W: rows
    W-xU: denied
    W--xU: dotted cross
    W->U: plain
    W-->U: dotted plain
    U<<->>W: both
    U<<-->>W: both dotted
    deactivate W
    loop Every minute
        U-)W: ping
    end
    alt success
        W->>U: ok
    else failure
        W->>U: error
    end
    opt maybe
    end
    par one
        U->>W: a
    and two
        U->>DB: b
    end
    critical crit
        W->>DB: c
    option alt path
        W->>DB: d
    end
    break stop
        W->>U: e
    end
    rect rgb(0,0,0)
        U->>W: in rect
    end
    box Group
    end
    Note left of U: left note
    Note right of DB: right note
    Note over W: over one
    Note over U,DB: spanning note
    destroy DB
    links W: {}
`
	got := render(t, src, 120)
	all := joined(got)
	if got[0] != "Checkout" {
		t.Errorf("title: %q", got[0])
	}
	for _, w := range []string{"╭──────╮", "│ User │", "Web", "Server", "1. Login", "▶", "◀", "✕", "▷", "◁", "┄", "╮", "╯",
		"loop", "[Every minute]", "alt", "[failure]", "opt", "par", "[two]", "critical", "[alt path]", "break",
		"left note", "right note", "over one", "spanning note", "in rect", "┴"} {
		if !strings.Contains(all, w) {
			t.Errorf("missing %q in\n%s", w, all)
		}
	}
	if strings.Contains(all, "rgb") || strings.Contains(all, "Group") {
		t.Errorf("rect/box should not draw frames:\n%s", all)
	}
	narrow := render(t, src, 70)
	if len(narrow) <= len(got) {
		t.Errorf("narrow sequence should wrap")
	}
	if _, ok := Mermaid(src, 20); ok {
		t.Error("expected failure at width 20")
	}
	for _, bad := range []string{
		"sequenceDiagram\nA->>",
		"sequenceDiagram\nNote left of A text",
		"sequenceDiagram\nNote above A: text",
		"sequenceDiagram\nNote over : text",
		"sequenceDiagram\nNote over A, : text",
		"sequenceDiagram\nend",
		"sequenceDiagram\nelse x",
		"sequenceDiagram\ngarbage line",
		"sequenceDiagram\ncreate thing X",
		"sequenceDiagram\nparticipant",
		"sequenceDiagram\n",
	} {
		if _, ok := Mermaid(bad, 80); ok {
			t.Errorf("expected failure for %q", bad)
		}
	}
	// Unclosed blocks are closed implicitly.
	if lines := render(t, "sequenceDiagram\nloop forever\nA->>B: x", 60); !strings.Contains(joined(lines), "└") {
		t.Errorf("implicit end:\n%s", joined(lines))
	}
	// Self message on the last participant and a note left of the first.
	lines := joined(render(t, "sequenceDiagram\nNote left of A: far left\nA->>A: self talk", 60))
	if !strings.Contains(lines, "far left") || !strings.Contains(lines, "self talk") {
		t.Errorf("edges:\n%s", lines)
	}
}

func TestPie(t *testing.T) {
	got := render(t, "pie showData title Fruits\n  \"Apples\" : 10\n  \"Bananas\" : 2.5\n  \"Zero\" : 0\n", 50)
	all := joined(got)
	for _, w := range []string{"Fruits", "Apples", "80.0%", "20.0%", "0.0%", "10", "2.5", "▎"} {
		if !strings.Contains(all, w) {
			t.Errorf("missing %q in\n%s", w, all)
		}
	}
	got = render(t, "---\ntitle: Front\n---\npie\nshowData\n\"a\" : 0\naccTitle: x\n", 30)
	if got[0] != "Front" || !strings.Contains(joined(got), "0.0%") {
		t.Errorf("front matter / zero total:\n%s", joined(got))
	}
	got = render(t, "pie\ntitle T\n\"A very long label that will be truncated\" : 1234567.123456789\n", 30)
	if !strings.Contains(joined(got), "…") {
		t.Errorf("long label should be truncated:\n%s", joined(got))
	}
	for _, bad := range []string{
		"pie\n",
		"pie\n\"a\" 1",
		"pie\n\"a : 1",
		"pie\n\"a\" : -1",
		"pie\n\"a\" : NaN",
		"pie\n\"a\" : x",
		"pie\nfoo",
		"pie whatever\n\"a\":1",
	} {
		if _, ok := Mermaid(bad, 40); ok {
			t.Errorf("expected failure for %q", bad)
		}
	}
	if _, ok := Mermaid("pie showData\n\"a label\" : 123456789", 12); ok {
		t.Error("expected failure when too narrow")
	}
}

func TestDot(t *testing.T) {
	src := `/* leading comment */
strict digraph "Build" {
  // line comment
  # preprocessor-like comment
  graph [rankdir=LR, label="Pipeline"];
  node [shape=box, color=red];
  edge [color=gray];
  label = "Overridden title"
  start [label="Start\nhere", shape=ellipse];
  db [shape=cylinder];
  dec [shape=diamond label=<Is <b>ok</b>?<br/>really>];
  p [shape=point]; c [shape=circle]; h [shape=hexagon]; pg [shape=parallelogram]; a [shape=cds]; n [shape=whatever];
  start -> db -> dec [label="next"];
  dec -> start [style=dashed, label="retry"];
  dec -> p [penwidth=3];
  dec -> c [dir=both]; c -> h [dir=back]; h -> pg [dir=none]; pg -> a [arrowhead=odot]; a -> n [arrowhead=tee];
  n -> start [arrowhead=none, style=bold];
  subgraph cluster_x { label="ignored"; x1; x2 -> x3; }
  x1 -> { y1 y2 };
  subgraph { z1 }
  "quoted id":port:n -> 3.14;
  -1 -> n;
}`
	lines, ok := Dot(src, 200)
	if !ok {
		t.Fatal("not ok")
	}
	checkWidth(t, lines, 200)
	all := joined(plainLines(lines))
	for _, w := range []string{"Overridden title", "Start", "here", "├────", "Is ok?", "really", "next", "retry", "●", "x3", "y2", "z1", "quoted id", "3.14", "-1", "◀", "○", "✕"} {
		if !strings.Contains(all, w) {
			t.Errorf("missing %q in\n%s", w, all)
		}
	}
	und := joined(plainLines(mustDot(t, "graph { a -- b; b -- c [dir=forward] }", 40)))
	if strings.Count(und, "▼") != 1 {
		t.Errorf("undirected graph should have exactly one arrow:\n%s", und)
	}
	for _, bad := range []string{
		"",
		"digraph {",
		"digraph { a -> }",
		"digraph { a [label=] }",
		"digraph { a [=x] }",
		"digraph { \"unterminated }",
		"digraph { <unterminated }",
		"digraph { a } extra",
		"digraph { /* unterminated }",
		"tree { a }",
		"digraph { a -> b; @ }",
		"digraph { a:: }",
		"digraph { x = }",
		"digraph { subgraph { a }",
		"digraph { subgraph s a }",
		"digraph { a -> { b }",
		strings.Repeat("digraph { subgraph { ", 40) + strings.Repeat("} ", 41),
	} {
		if _, ok := Dot(bad, 80); ok {
			t.Errorf("expected failure for %q", bad)
		}
	}
	if _, ok := Dot("digraph { a }", 3); ok {
		t.Error("expected failure for tiny width")
	}
}

func mustDot(t *testing.T, src string, width int) []styled.Line {
	t.Helper()
	lines, ok := Dot(src, width)
	if !ok {
		t.Fatalf("Dot(%q) not ok", src)
	}
	checkWidth(t, lines, width)
	return lines
}

func TestUnsupportedAndInvalid(t *testing.T) {
	for _, src := range []string{
		"",
		"   \n\n",
		"%% only a comment",
		"classDiagram\nA <|-- ",
		"erDiagram\nA ||--x{ B : has",
		"gantt\ntitle x",
		"journey\ntitle x",
		"mindmap\nroot",
		"timeline\ntitle x",
		"gitGraph\ncommit",
		"graph TD\nA[unterminated",
		"graph TD\nA((unterminated)",
		"graph TD\nA[\"unterminated]",
		"graph TD\nA[\"x\" junk]",
		"graph TD\nA -->",
		"graph TD\nA -- text",
		"graph TD\nA -- text -- B",
		"graph TD\nA -. text B",
		"graph TD\nA -->|unterminated B",
		"graph TD\nA @ B",
		"graph TD\nA -> B",
		"graph TD\n--> B",
		"graph TD\nA@{ shape: rect",
		"graph TD extra words",
		"graph TD\n",
	} {
		if _, ok := Mermaid(src, 80); ok {
			t.Errorf("expected failure for %q", src)
		}
	}
	if _, ok := Mermaid("graph TD\nA-->B", 4); ok {
		t.Error("expected failure for tiny width")
	}
	if _, ok := Mermaid("graph TD\nA-->B"+strings.Repeat(" ", maxSource), 80); ok {
		t.Error("expected failure for oversized source")
	}
}

func TestLimits(t *testing.T) {
	var b strings.Builder
	b.WriteString("graph TD\n")
	for i := 0; i <= maxNodes; i++ {
		b.WriteString("n" + itoa(i) + "\n")
	}
	if _, ok := Mermaid(b.String(), 200); ok {
		t.Error("too many nodes accepted")
	}
	b.Reset()
	b.WriteString("graph TD\n")
	for i := 0; i <= maxEdges/4; i++ {
		b.WriteString("A & B --> C & D\n")
	}
	if _, ok := Mermaid(b.String(), 200); ok {
		t.Error("too many edges accepted")
	}
	b.Reset()
	b.WriteString("digraph {\n")
	for i := 0; i <= maxEdges; i++ {
		b.WriteString("a -> b;\n")
	}
	b.WriteString("}")
	if _, ok := Dot(b.String(), 200); ok {
		t.Error("too many DOT edges accepted")
	}
	b.Reset()
	b.WriteString("sequenceDiagram\n")
	for i := 0; i <= maxParts; i++ {
		b.WriteString("participant p" + itoa(i) + "\n")
	}
	if _, ok := Mermaid(b.String(), 200); ok {
		t.Error("too many participants accepted")
	}
	b.Reset()
	b.WriteString("sequenceDiagram\n")
	for i := 0; i <= maxEvents; i++ {
		b.WriteString("A->>B: x\n")
	}
	if _, ok := Mermaid(b.String(), 200); ok {
		t.Error("too many events accepted")
	}
	b.Reset()
	b.WriteString("sequenceDiagram\n")
	for i := 0; i <= maxDepth; i++ {
		b.WriteString("loop x\n")
	}
	if _, ok := Mermaid(b.String(), 200); ok {
		t.Error("too deep nesting accepted")
	}
	b.Reset()
	b.WriteString("pie\n")
	for i := 0; i <= maxSlices; i++ {
		b.WriteString("\"x\" : 1\n")
	}
	if _, ok := Mermaid(b.String(), 200); ok {
		t.Error("too many slices accepted")
	}
	// A long chain with many long edges exceeds the dummy-node budget.
	b.Reset()
	b.WriteString("graph TD\n")
	for i := 0; i < 400; i++ {
		b.WriteString("n" + itoa(i) + " --> n" + itoa(i+1) + "\n")
	}
	for i := 0; i < 40; i++ {
		b.WriteString("n0 --> n" + itoa(400-i) + "\n")
	}
	if _, ok := Mermaid(b.String(), 1000); ok {
		t.Error("expected the dummy budget to be exceeded")
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for ; i > 0; i /= 10 {
		b = append([]byte{byte('0' + i%10)}, b...)
	}
	return string(b)
}

func TestGeneratedGraphsFit(t *testing.T) {
	for _, n := range []int{1, 2, 5, 12, 30, 80} {
		for _, dir := range []string{"TD", "LR", "BT", "RL"} {
			src := genFlowchart(n, dir)
			for _, w := range []int{30, 60, 100, 200} {
				lines, ok := Mermaid(src, w)
				if !ok {
					continue
				}
				checkWidth(t, lines, w)
			}
			if _, ok := Mermaid(src, 1000); !ok {
				t.Errorf("n=%d %s should render at width 1000", n, dir)
			}
		}
	}
	a, _ := Mermaid(genFlowchart(30, "TD"), 120)
	b, _ := Mermaid(genFlowchart(30, "TD"), 120)
	if joined(plainLines(a)) != joined(plainLines(b)) {
		t.Error("output is not deterministic")
	}
}

func TestHelpers(t *testing.T) {
	if got := wrap("aaaaaaaaaa bb", 4); joined(got) != "aaaa\naaaa\naa\nbb" {
		t.Errorf("wrap long word: %q", got)
	}
	if got := wrap("a\n\n", 0); joined(got) != "a" {
		t.Errorf("wrap trailing blank: %q", got)
	}
	if got := wrap(strings.Repeat("w ", 20), 1); len(got) != maxLabelLines || !strings.HasSuffix(got[maxLabelLines-1], "…") {
		t.Errorf("wrap line cap: %q", got)
	}
	if got := wrap("漢字漢字", 3); joined(got) != "漢\n字\n漢\n字" {
		t.Errorf("wrap wide: %q", got)
	}
	if got := truncate("hello", 0); got != "" {
		t.Errorf("truncate 0: %q", got)
	}
	if got := truncate("hello world", 6); got != "hello…" {
		t.Errorf("truncate: %q", got)
	}
	if got := cleanLabel("a\x00b\u200bc<br  />d<b>"); got != "abc\nd<b>" {
		t.Errorf("cleanLabel: %q", got)
	}
	if got := unquote(`"x"`); got != "x" {
		t.Errorf("unquote: %q", got)
	}
	c := newCanvas(4, 1)
	c.put(0, 0, '漢', styled.Plain)
	c.put(1, 0, 'x', styled.Plain) // overwrite the right half
	c.put(2, 0, '字', styled.Plain)
	c.put(2, 0, 'y', styled.Plain) // overwrite the left half
	c.put(3, 0, '字', styled.Plain) // does not fit: replaced by an ellipsis
	c.put(9, 9, 'z', styled.Plain)
	c.put(0, 0, '\u200b', styled.Plain)
	c.clear(-1, 0)
	if got := joined(plainLines(c.lines())); got != " xy…" {
		t.Errorf("canvas wide runes: %q", got)
	}
	c = newCanvas(3, 3)
	c.seg(0, 1, 2, 1, styleThick, styled.Edge)
	c.seg(1, 0, 1, 2, styleDotted, styled.Edge)
	c.seg(0, 0, 0, 0, styleSolid, styled.Edge)
	if got := joined(plainLines(c.lines())); got != " ┆\n━┼━\n ┆" {
		t.Errorf("canvas junction: %q", got)
	}
	if junction(bitUp|bitRight, styleThick) != '┗' || junction(bitDown|bitLeft, styleDotted) != '╮' {
		t.Error("junction glyphs")
	}
	if portGlyph('│', bitRight, styleThick) != '┝' || portGlyph('│', bitLeft, styleThick) != '┥' || portGlyph('│', bitLeft, styleSolid) != '┤' || portGlyph('x', bitLeft, styleSolid) != 'x' {
		t.Error("port glyphs")
	}
	for name, want := range map[string]shape{"rounded": shapeRound, "stadium": shapeStadium, "subproc": shapeSubroutine, "circle": shapeCircle,
		"diamond": shapeDecision, "hex": shapeHexagon, "lean-r": shapePara, "lean-l": shapeParaAlt, "start": shapeStart,
		"stop": shapeEnd, "fork": shapeBar, "rect": shapeRect} {
		if got := namedShape(name); got != want {
			t.Errorf("namedShape(%s) = %d", name, got)
		}
	}
}

func TestRecoverSwallowsPanics(t *testing.T) {
	// Exercise the exported wrappers' happy paths; panics are caught there.
	if _, ok := Mermaid("graph TD\nA-->B", 40); !ok {
		t.Error("not ok")
	}
	if _, ok := Dot("digraph{a->b}", 40); !ok {
		t.Error("not ok")
	}
}

func FuzzMermaid(f *testing.F) {
	seeds := []string{
		"graph TD\nA[Start] --> B{Is it?}\nB -->|Yes| C[OK]\nB -- No --> D[(DB)]\nD -.-> A\nC ==> E((End))",
		"flowchart LR\nA & B --> C & D\nC --o D\nD --x A\nA <--> D\nsubgraph s [S]\nA\nend",
		"graph BT\nA-->A\nA-->|loop| A",
		"stateDiagram-v2\n[*] --> A\nA --> B : go\nstate C {\n[*] --> D\n}\nB --> [*]",
		"sequenceDiagram\nautonumber\nAlice->>Bob: Hi\nloop L\nBob-->>Alice: Yo\nend\nNote over Alice,Bob: n",
		"pie showData title T\n\"a\" : 1\n\"b\" : 2",
		"---\ntitle: X\n---\ngraph RL\nA([x]) --- B[[y]]",
		"classDiagram\ndirection LR\nclass A~T~ {\n<<interface>>\n+x int\n+f() bool\n}\nA \"1\" <|-- \"*\" B : is\nB *-- C\nC ..> D\nD ()-- E\nnamespace N {\nclass F\n}",
		"erDiagram\nA ||--o{ B : has\nB }|..|{ C : \"uses\"\nA one or more to zero or more D : x\nA {\nstring id PK, FK \"key\"\n}",
		"gantt\ntitle T\ndateFormat YYYY-MM-DD\naxisFormat %b %d\nsection S\nA :done, a, 2024-01-01, 3d\nB :crit, active, after a, 1w\nC :milestone, until a, 0d\nD :12h",
	}
	for _, s := range seeds {
		f.Add(s, 60)
	}
	f.Fuzz(func(t *testing.T, src string, width int) {
		width %= 300
		lines, ok := mermaid(src, width)
		if !ok {
			return
		}
		for i, l := range lines {
			if w := styled.LineWidth(l); w > width {
				t.Fatalf("line %d is %d wide, limit %d", i, w, width)
			}
		}
	})
}

func FuzzDot(f *testing.F) {
	f.Add("digraph { rankdir=LR; a [label=\"x\", shape=box]; a -> b -> c [label=y]; }", 60)
	f.Add("graph { a -- b; subgraph cluster { c -- d } }", 40)
	f.Fuzz(func(t *testing.T, src string, width int) {
		width %= 300
		lines, ok := dot(src, width)
		if !ok {
			return
		}
		for i, l := range lines {
			if w := styled.LineWidth(l); w > width {
				t.Fatalf("line %d is %d wide, limit %d", i, w, width)
			}
		}
	})
}
