package render

import (
	"strings"
	"testing"
)

func TestHTMLPage(t *testing.T) {
	p := HTMLPage("A <b>", "<p>x</p>\n", "kmd", nil)
	if !strings.Contains(p, "<title>A &lt;b&gt;</title>") || !strings.Contains(p, "Content-Security-Policy") ||
		!strings.Contains(p, "max-width: 52rem") || strings.Contains(p, "@page") {
		t.Errorf("page:\n%s", p)
	}
}

func TestSlidesPage(t *testing.T) {
	p := SlidesPage("T", []string{"<p>a</p>\n", "<p>b</p>\n"}, "x <y> \x1b[31m", false, "kmd", nil)
	if strings.Count(p, `<section class="slide">`) != 2 || !strings.Contains(p, "<span>x &lt;y&gt; [31m</span><span>2 / 2</span>") ||
		strings.Contains(p, "\x1b") || !strings.Contains(p, "@page") {
		t.Errorf("page:\n%s", p)
	}
	if p := SlidesPage("T", []string{"<p>a</p>\n"}, "x", true, "kmd", nil); strings.Contains(p, "<footer>") {
		t.Error("hidden footer shown")
	}
}

func TestHTMLCode(t *testing.T) {
	code := HTMLCode(Options{Diagrams: true, Math: true}, HTMLDiagramWidth)
	got, class, ok := code("go extra", "func f() { s := \"<script>\" }\n")
	if !ok || class != "" || !strings.Contains(got, `<span style="`) || !strings.Contains(got, "&lt;script&gt;") || strings.Contains(got, "<script>") {
		t.Errorf("go: %v %s", ok, got)
	}
	// left as written: no language, nothing in it, math (like $$ formulas)
	for _, info := range []string{"", "math"} {
		if _, _, ok := code(info, "x^2"); ok {
			t.Errorf("%q was changed", info)
		}
	}
	if _, _, ok := code("go", "\n"); ok {
		t.Error("an empty block was changed")
	}
	got, class, ok = code("Mermaid", "flowchart LR\n  A[one] --> B[two]\n")
	if !ok || class != "diagram" || strings.Contains(got, "flowchart") || !strings.Contains(got, "one") || !strings.ContainsAny(got, "─│") {
		t.Errorf("diagram %q: %s", class, got)
	}
	// diagrams stay within the document's budget: past it, highlighted source
	for range maxSpecials {
		code("mermaid", "flowchart LR\n  A --> B\n")
	}
	if got, class, _ := code("mermaid", "flowchart LR\n  A --> B\n"); class != "" || !strings.Contains(got, "flowchart") {
		t.Errorf("past the diagram budget: %s", got)
	}
	if _, class, _ := HTMLCode(Options{}, HTMLDiagramWidth)("mermaid", "flowchart LR\n  A --> B\n"); class != "" {
		t.Error("drawn without Diagrams")
	}
	// so does highlighting: past its budget, the escaped text
	big := HTMLCode(Options{}, HTMLDiagramWidth)
	if _, _, ok := big("go", strings.Repeat("x := 1\n", maxHTMLHighlight/7+1)); ok {
		t.Error("highlighted past the budget")
	}
}

func TestStyleCSS(t *testing.T) {
	st := Style{Fg: 0x112233, Bg: 0x445566, Attr: Bold | Italic | Underline | Strike | Faint}
	if got := st.css(); got != "color:#112233;background:#445566;font-weight:bold;font-style:italic;text-decoration:underline line-through;opacity:.6" {
		t.Errorf("css: %s", got)
	}
	for st, want := range map[Style]string{{}: "", {Attr: Underline}: "text-decoration:underline", {Attr: Strike}: "text-decoration:line-through"} {
		if got := st.css(); got != want {
			t.Errorf("%+v: %q", st, got)
		}
	}
}
