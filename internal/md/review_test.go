package md

import (
	"strings"
	"testing"
)

// TestReferenceBudget: reusing a reference definition must not copy its
// destination and title without bound (each use repeats them in the output).
func TestReferenceBudget(t *testing.T) {
	url := "https://e.com/" + strings.Repeat("a", 64<<10)
	src := "[x]: " + url + ` "title"` + "\n\n" + strings.Repeat("[x] ", 10000)
	doc := Parse(src, GFM)
	copied, links := 0, 0
	Walk(doc, func(n *Node, entering bool) bool {
		if entering && n.Type == Link {
			links++
			copied += len(n.Dest) + len(n.Title)
		}
		return true
	})
	if links != 10000 {
		t.Fatalf("%d links, want 10000 (text kept when the budget runs out)", links)
	}
	if limit := refBudget(len(src)); copied > limit {
		t.Errorf("copied %d bytes of destinations, budget %d", copied, limit)
	}
	if out := HTML(doc, HTMLOptions{}); len(out) > 4*len(src)+2<<20 {
		t.Errorf("%d bytes of input gave %d bytes of HTML", len(src), len(out))
	}
	// Ordinary documents are untouched.
	small := Parse("[a]: /u \"t\"\n\n"+strings.Repeat("[a] ", 1000), GFM)
	if n := small.FirstChild.FirstChild; n.Type != Link || n.Dest != "/u" || n.Title != "t" {
		t.Errorf("small reference resolved to %+v", n)
	}
	if n := small.FirstChild.LastChild; n.Type != Link || n.Dest != "/u" {
		t.Errorf("last small reference resolved to %+v", n)
	}
}

// TestHTMLDropsControls: control characters in the document never reach
// the HTML (they are invalid there and could drive a terminal).
func TestHTMLDropsControls(t *testing.T) {
	src := "# hi \x1b]0;pwn\a\n\ntext \x1b[2J \u009b31m `\x1b[1m` [l](/u \"t\x1b\") ![a\x7f](/i)\n\n```\n\x1b]52;c;x\a\n```\n"
	out := HTML(Parse(src, GFM), HTMLOptions{})
	for _, bad := range []string{"\x1b", "\a", "\u009b", "\x7f"} {
		if strings.Contains(out, bad) {
			t.Errorf("HTML contains %q:\n%q", bad, out)
		}
	}
	if !strings.Contains(out, "\ttab") && !strings.Contains(HTML(Parse("```\n\ttab\n```", GFM), HTMLOptions{}), "\ttab") {
		t.Error("tabs must be kept")
	}
}

// inlineNodes counts the nodes below doc's blocks, and the text nodes.
func inlineNodes(doc *Node) (all, texts int) {
	Walk(doc, func(n *Node, entering bool) bool {
		if entering && n.Parent != nil && n.Parent.Type != Document {
			all++
			if n.Type == Text {
				texts++
			}
		}
		return true
	})
	return all, texts
}

// TestInlineText: text the parser handles one character at a time ("]",
// escapes, unmatched delimiters) ends up in one node, not one per byte.
func TestInlineText(t *testing.T) {
	for _, src := range []string{
		strings.Repeat("]", 1<<16),
		strings.Repeat(`\*`, 1<<15),
		strings.Repeat("! < & ", 1<<14),
		strings.Repeat("*a ", 1<<14),
		strings.Repeat("a_b ", 1<<14),
	} {
		doc := Parse(src, GFM)
		if all, _ := inlineNodes(doc); all != 1 {
			t.Errorf("%.12q…: %d inline nodes, want 1", src, all)
		}
	}
	// text with an escape mark stays apart from text the extensions rewrite
	doc := Parse(`\:rocket: :rocket:`, GFM)
	if got := PlainText(doc); got != ":rocket: 🚀" {
		t.Errorf("escaped shortcode: %q", got)
	}
}

// TestInlineBudget: hostile text cannot make more than MaxInlines inline
// nodes, through the parser or the wiki link and e-mail extensions; the
// rest stays text and a notice says so.
func TestInlineBudget(t *testing.T) {
	for name, src := range map[string]string{
		"emphasis":    strings.Repeat("*a ", MaxInlines),
		"soft breaks": strings.Repeat("a\n", MaxInlines),
		"links":       strings.Repeat("[a](b)", MaxInlines/2),
		"wiki links":  strings.Repeat("[[a]] ", MaxInlines/4),
		"emails":      strings.Repeat("a@b.co ", MaxInlines/2),
	} {
		doc := Parse(src, GFM)
		all, _ := inlineNodes(doc)
		if all > MaxInlines+10 {
			t.Errorf("%s: %d inline nodes, limit %d", name, all, MaxInlines)
		}
		if !strings.Contains(PlainText(doc.LastChild), "inline markup ignored") {
			t.Errorf("%s: no notice", name)
		}
	}
	if doc := Parse(strings.Repeat("*a* ", 1000), GFM); strings.Contains(PlainText(doc), "ignored") {
		t.Error("notice on a small document")
	}
}

// TestLinearFixesKeepBehaviour: the bounds that keep hostile inlines linear
// (security review #72) do not change what ordinary documents produce.
func TestLinearFixesKeepBehaviour(t *testing.T) {
	html := func(src string) string { return HTML(Parse(src, GFM), HTMLOptions{}) }
	domain := func(n int) string { return "www." + strings.Repeat("a", n-8) + ".com" }
	label := strings.Repeat("a", maxLabel)
	for _, c := range []struct{ name, src, want, not string }{
		{"longest domain", domain(maxDomain), `<a href="http://` + domain(maxDomain) + `"`, ""},
		{"domain too long", domain(maxDomain + 1), "", "<a "},
		{"trailing parentheses", "http://a.b/(x))) end", `<a href="http://a.b/(x)">http://a.b/(x)</a>)) end`, ""},
		{"balanced parentheses", "www.a.b/(x)(y))", `<a href="http://www.a.b/(x)(y)">www.a.b/(x)(y)</a>)`, ""},
		{"footnote", "x[^1]\n\n[^1]: n", `<sup class="footnote-ref">`, ""},
		{"footnote label too long", "x[^a" + label + "]\n\n[^a" + label + "]: n", "", "footnote-ref"},
		{"reference label at the limit", "[" + label + "]\n\n[" + label + "]: /u", `<a href="/u">`, ""},
		{"no link in a link", "[a [b](c) d](e)", `[a <a href="c">b</a> d](e)`, ""},
		{"image around a link", "![a [b](c) d](e)", `<img src="e" alt="a b d" />`, ""},
		{"opener after a link", "[a](b) [c](d)", `<a href="d">c</a>`, ""},
		{"emphasis after a removed opener", "*a **b* _c_ **d**", `*a *<em>b</em> <em>c</em> <strong>d</strong>`, ""},
		{"table after paragraph lines", "x\ny\n| a | b |\n|---|---|\n| 1 | 2 |", "<p>x\ny</p>\n<table>", ""},
	} {
		out := html(c.src)
		if c.want != "" && !strings.Contains(out, c.want) {
			t.Errorf("%s: %q lacks %q", c.name, out, c.want)
		}
		if c.not != "" && strings.Contains(out, c.not) {
			t.Errorf("%s: %q has %q", c.name, out, c.not)
		}
	}
}

// TestHTMLDropsHiddenCharacters: bidi controls ("Trojan Source"), zero-width
// characters and carriage returns are dropped from the export, as the
// terminal renderer drops them, in prose, code, titles, alt text and
// footnote labels; --unsafe-html keeps them.
func TestHTMLDropsHiddenCharacters(t *testing.T) {
	trojan := "if access != \"user\u202e \u2066// check admin\u2069 \u2066\" {"
	src := "# title\u200b\n\nprose \u202e and &#x202e; and &#13; `code\u2066`\n\n" +
		"[l](/u \"t\u2067\") ![alt\u200f](/i)[^n\u2068]\n\n[^n\u2068]: note\n\n```\n" + trojan + "\n```\n\n    indented\u2069\n"
	out := HTML(Parse(src, GFM), HTMLOptions{})
	for _, bad := range []string{"\u202e", "\u2066", "\u2067", "\u2068", "\u2069", "\u200b", "\u200f", "\r"} {
		if strings.Contains(out, bad) {
			t.Errorf("HTML contains %q:\n%q", bad, out)
		}
	}
	if !strings.Contains(out, `if access != &quot;user // check admin &quot; {`) {
		t.Errorf("code lost more than its bidi controls:\n%s", out)
	}
	if unsafe := HTML(Parse(src, GFM), HTMLOptions{Unsafe: true}); !strings.Contains(unsafe, "\u202e") {
		t.Errorf("--unsafe-html must keep bidi controls:\n%q", unsafe)
	}
}
