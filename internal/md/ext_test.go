package md

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DipaolaFederico/kmd/internal/testlimit"
)

func gfmHTML(src string) string {
	return HTML(Parse(src, GFM), HTMLOptions{TagFilter: true, Unsafe: true})
}

func TestExtensions(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"inline math", "Euler: $e^{i\\pi}+1=0$ done", `<p>Euler: <span class="math math-inline">e^{i\pi}+1=0</span> done</p>` + "\n"},
		{"display math inline", "a $$x^2$$ b", `<p>a <span class="math math-display">x^2</span> b</p>` + "\n"},
		{"money is not math", "costs $5 and $10", "<p>costs $5 and $10</p>\n"},
		{"escaped dollar", "\\$x$", "<p>$x$</p>\n"},
		{"closing before digit", "$a$1 $b$", `<p>$a$1 <span class="math math-inline">b</span></p>` + "\n"},
		{"math block", "$$\n\\frac{a}{b}\n$$\n", `<div class="math math-display">\frac{a}{b}</div>` + "\n"},
		{"math block one line", "$$ x $$\nafter", `<div class="math math-display">x</div>` + "\n<p>after</p>\n"},
		{"unterminated $$", "a $$b", "<p>a $$b</p>\n"},
		{"alert", "> [!note]\n> Be careful", `<div class="markdown-alert markdown-alert-note">` + "\n" + `<p class="markdown-alert-title">Note</p>` + "\n<p>Be careful</p>\n</div>\n"},
		{"alert only marker", "> [!TIP]", `<div class="markdown-alert markdown-alert-tip">` + "\n" + `<p class="markdown-alert-title">Tip</p>` + "\n</div>\n"},
		{"not alert", "> [!FOO]\n> x", "<blockquote>\n<p>[!FOO]\nx</p>\n</blockquote>\n"},
		{"loose task", "- [x] a\n\n- [ ] b", "<ul>\n<li>\n<p>" + `<input checked="" disabled="" type="checkbox"> ` + "a</p>\n</li>\n<li>\n<p>" + `<input disabled="" type="checkbox"> ` + "b</p>\n</li>\n</ul>\n"},
		{"not task", "- [y] a\n- [x]b", "<ul>\n<li>[y] a</li>\n<li>[x]b</li>\n</ul>\n"},
		{"footnotes", "A[^1] B[^n].\n\n[^n]: Second\n[^1]: First\n    more\n\n[^unused]: x", "<p>A<sup class=\"footnote-ref\"><a href=\"#fn-1\" id=\"fnref-1\" data-footnote-ref>1</a></sup> B<sup class=\"footnote-ref\"><a href=\"#fn-N\" id=\"fnref-N\" data-footnote-ref>2</a></sup>.</p>\n<section class=\"footnotes\" data-footnotes>\n<ol>\n<li id=\"fn-1\">\n<p>First\nmore</p>\n<a href=\"#fnref-1\" class=\"footnote-backref\" data-footnote-backref>↩</a>\n</li>\n<li id=\"fn-N\">\n<p>Second</p>\n<a href=\"#fnref-N\" class=\"footnote-backref\" data-footnote-backref>↩</a>\n</li>\n</ol>\n</section>\n"},
		{"undefined footnote", "x[^nope]", "<p>x[^nope]</p>\n"},
		{"table after paragraph", "intro\n| a | b |\n|:-|-:|\n| 1 | 2 |", "<p>intro</p>\n<table>\n<thead>\n<tr>\n<th align=\"left\">a</th>\n<th align=\"right\">b</th>\n</tr>\n</thead>\n<tbody>\n<tr>\n<td align=\"left\">1</td>\n<td align=\"right\">2</td>\n</tr>\n</tbody>\n</table>\n"},
		{"table with refs", "[r]: /u\n| [a][r] |\n| :-: |", "<table>\n<thead>\n<tr>\n<th align=\"center\"><a href=\"/u\">a</a></th>\n</tr>\n</thead>\n</table>\n"},
		{"table escaped pipe in code", "| `a\\|b` |\n|---|", "<table>\n<thead>\n<tr>\n<th><code>a|b</code></th>\n</tr>\n</thead>\n</table>\n"},
		{"bad delimiter", "a | b\n|:-:-|---|", "<p>a | b\n|:-:-|---|</p>\n"},
		{"email in text", "write to me@x.io, or [link](u) a@b.cd", "<p>write to <a href=\"mailto:me@x.io\">me@x.io</a>, or <a href=\"u\">link</a> <a href=\"mailto:a@b.cd\">a@b.cd</a></p>\n"},
		{"entity in autolink", "www.x.com/a&amp;", "<p><a href=\"http://www.x.com/a\">www.x.com/a</a>&amp;</p>\n"},
		{"underscore domain", "www.x_y.com", "<p>www.x_y.com</p>\n"},
		{"no autolink in link text", "[www.x.com](u)", "<p><a href=\"u\">www.x.com</a></p>\n"},
		{"strike tildes", "~a~ ~~b~~ ~~~c~~~", "<p><del>a</del> <del>b</del> ~~~c~~~</p>\n"},
		{"tagfilter", "<script>x</script>", "&lt;script>x&lt;/script>\n"},
	}
	for _, tc := range tests {
		if got := gfmHTML(tc.in); got != tc.want {
			t.Errorf("%s\ninput: %q\nwant:  %q\ngot:   %q", tc.name, tc.in, tc.want, got)
		}
	}
}

func TestFrontMatter(t *testing.T) {
	doc := Parse("---\ntitle: x\ntags: [a]\n---\n# H\n", GFM)
	if fm := doc.FirstChild; fm.Type != FrontMatter || fm.Literal != "title: x\ntags: [a]" || fm.Next.Type != Heading {
		t.Fatalf("front matter = %+v", fm)
	}
	if got := HTML(doc, HTMLOptions{}); got != "<h1>H</h1>\n" {
		t.Errorf("html = %q", got)
	}
	for _, src := range []string{"---\nno colon\n---\n", "---\ntitle: x\n", "x\n---\na: b\n---"} {
		if Parse(src, GFM).FirstChild.Type == FrontMatter {
			t.Errorf("%q parsed as front matter", src)
		}
	}
	if d := Parse("---\r\na: b\r\n...", GFM); d.FirstChild == nil || d.FirstChild.Type != FrontMatter || d.FirstChild.Next != nil {
		t.Error("front matter at EOF")
	}
	if d := Parse("---\na: b\n---\n", Options{}); d.FirstChild.Type == FrontMatter {
		t.Error("front matter without the extension")
	}
}

func TestTreeAPI(t *testing.T) {
	doc := Parse("# T *x*\n\n- a\n- b\n\n![i `c`](s)", GFM)
	if doc.Type.String() != "document" || NodeType(200).String() != "unknown" || !Heading.IsBlock() || Text.IsBlock() {
		t.Fatal("type helpers")
	}
	kids := doc.Children()
	if len(kids) != 3 || kids[0].Type != Heading || kids[1].Type != List || !kids[1].Tight {
		t.Fatalf("children = %v", kids)
	}
	if got := PlainText(kids[0]); got != "T x" {
		t.Errorf("PlainText = %q", got)
	}
	if got := PlainText(kids[2]); got != "i c" {
		t.Errorf("image alt = %q", got)
	}
	// insertBefore keeps the tree consistent
	n := &Node{Type: ThematicBreak}
	kids[0].insertBefore(n)
	if doc.FirstChild != n || n.Next != kids[0] || kids[0].Prev != n {
		t.Error("insertBefore")
	}
	if len(Footnotes(doc)) != 0 {
		t.Error("footnotes")
	}
}

func TestEntitiesAndURIs(t *testing.T) {
	for in, want := range map[string]string{
		"&#0;": "�", "&#x110000;": "�", "&#xD800;": "�", "&#12345678;": "&#12345678;",
		"&copycat;": "&copycat;", "&semi;": ";", "&;": "&;", "&#;": "&#;", "&#x;": "&#x;",
	} {
		if got := unescape(in); got != want {
			t.Errorf("unescape(%q) = %q, want %q", in, got, want)
		}
	}
	if got := normalizeURI("a b%2Fü"); got != "a%20b%2F%C3%BC" {
		t.Errorf("normalizeURI = %q", got)
	}
}

func backtickLadder(k int) string {
	var b strings.Builder
	for i := 1; i <= k; i++ {
		b.WriteString(strings.Repeat("`", i))
		b.WriteString(" x ")
	}
	return b.String()
}

// TestPathological checks inputs known to make naive parsers quadratic:
// each case builds n repetitions of a hostile pattern, rendered within a
// time limit at the size given and timed at growing sizes, where the time
// must grow linearly (testlimit.Linear).
func TestPathological(t *testing.T) {
	rep := func(s string) func(int) string { return func(n int) string { return strings.Repeat(s, n) } }
	cases := []struct {
		name string
		n    int
		gen  func(n int) string
		// quadratic: the input itself grows with n², so its time cannot
		// grow linearly (only the absolute limit applies)
		quadratic bool
	}{
		{"nested brackets", 50000, func(n int) string { return strings.Repeat("[", n) + strings.Repeat("]", n) }, false},
		{"nested quotes", 50000, func(n int) string { return strings.Repeat("> ", n) + "x" }, false},
		{"nested lists", 50000, func(n int) string { return strings.Repeat("- ", n) + "x" }, false},
		{"emphasis openers", 50000, rep("*a "), false},
		{"underscores", 50000, func(n int) string { return strings.Repeat("_a ", n) + strings.Repeat("a_ ", n) }, false},
		{"backticks", 30000, rep("`a``"), false},
		{"links in links", 30000, func(n int) string { return strings.Repeat("[a](b)", n) + strings.Repeat("[", n) }, false},
		{"unclosed links", 50000, rep("[a]("), false},
		{"many refs", 30000, rep("[x]: /u\n"), false},
		{"table rows", 30000, func(n int) string { return "| a | b |\n|---|---|\n" + strings.Repeat("| x | y |\n", n) }, false},
		{"math", 50000, rep("$a "), false},
		{"emails", 50000, rep("a@ "), false},
		{"emphasis + links", 20000, rep("*a* **b** `c` [d](e) "), false},
		{"underscore links", 20000, rep("_a [x](y) "), false},
		{"backtick lengths", 1500, backtickLadder, true},
		{"colons", 100000, rep(":"), false},
		{"shortcode-like", 5000, rep(":" + strings.Repeat("a", 39)), false},
		{"long shortcodes", 3000, rep(":" + strings.Repeat("a", 60) + ":"), false},
		{"split colons", 30000, rep(":_a_"), false},
		{"emoji", 30000, rep(":rocket:"), false},
		{"wiki openers", 100000, rep("[["), false},
		{"wiki closers", 100000, rep("]]"), false},
		{"wiki unclosed", 50000, rep("[[a"), false},
		{"wiki long", 50, rep("[[" + strings.Repeat("a", 2000)), false},
		{"wiki long colons", 50, rep("[[" + strings.Repeat("a:", 1000)), false},
		{"unclosed + emoji", 30000, rep("[[a :x: "), false},
		{"wiki links", 30000, rep("[[a|:x:]] "), false},
		{"wiki in emphasis", 30000, rep("*[[a_b]]* "), false},
		// security review #72
		{"nested footnote openers", 20000, func(n int) string { return strings.Repeat("[^", n) + strings.Repeat("]", n) }, false},
		{"emphasis bottoms", 20000, rep("*a**b* _a"), false},
		{"strike bottoms", 20000, rep("~b~~*c"), false},
		{"www underscores", 20000, rep("www.a_"), false},
		{"image openers + links", 20000, rep("![[]()"), false},
		{"table-like lines", 20000, rep("a\n-|-|-\n"), false},
		{"autolink parentheses", 50000, func(n int) string { return "http://a.b/" + strings.Repeat(")", n) }, false},
	}
	for _, c := range cases {
		in := c.gen(c.n)
		start := time.Now()
		_ = HTML(Parse(in, GFM), HTMLOptions{})
		if d := time.Since(start); d > raceFactor()*500*time.Millisecond {
			t.Errorf("%s took %v", c.name, d)
		}
		if c.quadratic {
			continue
		}
		testlimit.Linear(t, c.name, 8*c.n, func(n int) func() {
			in := c.gen(n)
			return func() { _ = HTML(Parse(in, GFM), HTMLOptions{}) }
		})
	}
}

func FuzzParse(f *testing.F) {
	for _, e := range []string{
		"# h\n\n- a\n  - b\n\n> q\n\n```go\nx\n```\n| a | b |\n|---|---|\n| 1 | 2 |",
		"***a** b* [l](u) ![i](s) <http://x> `c` ~~s~~ &amp; $x$ [^1]\n\n[^1]: n",
		"> [!NOTE]\n> x\n\n- [ ] t\n<div>\n\n<!--\n$$\nx\n$$\nwww.a.com a@b.co",
		"\t\t- a\n\t> b\x00\r\n",
		":rocket: :+1: ::x:: :nope: `:x:` \\:x: [[Page#H|l :tada:]] [[a]]] [[[b]] [[]] [[x\\]] *[[y]]*",
		"[r]: /u\n\n[[r]] [[r]](/v) <http://a/:x:> www.b.com/:x: [:x:](:x:) | [[p\\|q]] |\n|-|",
	} {
		f.Add(e)
	}
	f.Fuzz(func(t *testing.T, s string) {
		_ = HTML(Parse(s, GFM), HTMLOptions{TagFilter: true})
		_ = HTML(Parse(s, Options{}), HTMLOptions{})
	})
}

// Regressions from the security review.
func TestHTMLSafeMode(t *testing.T) {
	src := "[x](javascript:alert(1)) [y](JaVaScRiPt:x) ![i](vbscript:x) [f](file:///etc/passwd) [d](data:text/html,x) ![ok](data:image/png;base64,AA)\n\n" +
		"<img src=x onerror=alert(3)>\n\ninline <b onclick=x>b</b>\n\n![<b title=\"x\" onerror=\"alert(1)\">](n.png)"
	got := HTML(Parse(src, GFM), HTMLOptions{TagFilter: true})
	for _, bad := range []string{"javascript", "JaVaScRiPt", "vbscript", "file:", "data:text", "onerror=alert(3)", "onclick", `alt="<b`} {
		if strings.Contains(got, bad) {
			t.Errorf("safe HTML contains %q:\n%s", bad, got)
		}
	}
	for _, want := range []string{`<a href="">x</a>`, `src="data:image/png;base64,AA"`, "<!-- raw HTML omitted -->", `alt="&lt;b title=&quot;x&quot;`} {
		if !strings.Contains(got, want) {
			t.Errorf("safe HTML missing %q:\n%s", want, got)
		}
	}
	if u := HTML(Parse("<b>x</b>", GFM), HTMLOptions{Unsafe: true}); u != "<p><b>x</b></p>\n" {
		t.Errorf("unsafe mode = %q", u)
	}
}

func TestSecurityLimits(t *testing.T) {
	timed := func(name, src string, limit time.Duration) *Node {
		t.Helper()
		start := time.Now()
		doc := Parse(src, GFM)
		_ = HTML(doc, HTMLOptions{})
		if d := time.Since(start); d > raceFactor()*limit {
			t.Errorf("%s took %v", name, d)
		}
		return doc
	}
	// a 2000-column header + 5000 pipe-less rows must not become 10M cells
	wide := "|" + strings.Repeat("h|", 2000) + "\n|" + strings.Repeat("-|", 2000) + "\n" + strings.Repeat("a\n", 5000)
	timed("table padding", wide, time.Second)
	cols := "|" + strings.Repeat("h|", 900) + "\n|" + strings.Repeat("-|", 900) + "\n" + strings.Repeat("a\n", 5000)
	doc := timed("table budget", cols, time.Second)
	rows := 0
	Walk(doc, func(n *Node, entering bool) bool {
		if entering && n.Type == TableRow {
			rows++
		}
		return true
	})
	if rows*900 > maxTableCells {
		t.Errorf("cell budget exceeded: %d rows", rows)
	}
	// deep inline nesting is flattened (no stack overflow in renderers)
	deep := strings.Repeat("*a **a ", 200000) + "b" + strings.Repeat(" a** a*", 200000)
	doc = timed("deep emphasis", deep, 3*time.Second)
	maxDepth := 0
	var depth func(n *Node, d int)
	depth = func(n *Node, d int) {
		maxDepth = max(maxDepth, d)
		for c := n.FirstChild; c != nil; c = c.Next {
			depth(c, d+1)
		}
	}
	depth(doc, 0)
	if maxDepth > maxInlineDepth+5 {
		t.Errorf("inline depth %d not bounded", maxDepth)
	}
	timed("deep images", strings.Repeat("![", 100000)+"x"+strings.Repeat("](u)", 100000), 3*time.Second)
	// unterminated comments / CDATA / PI / declarations stay linear
	for _, open := range []string{"<!--", "<![CDATA[", "<?", "<!X"} {
		timed("unterminated "+open, "</"+strings.Repeat(open, 200000), time.Second)
	}
	// many footnotes defined in reverse order
	var fn strings.Builder
	for i := 0; i < 20000; i++ {
		fn.WriteString("[^" + strconv.Itoa(i) + "]")
	}
	fn.WriteString("\n\n")
	for i := 19999; i >= 0; i-- {
		fn.WriteString("[^" + strconv.Itoa(i) + "]: n\n")
	}
	timed("footnote sort", fn.String(), 2*time.Second)

	doc = timed("block budget", strings.Repeat("a\n\n", MaxBlocks+10), 3*time.Second)
	if last := doc.LastChild; last == nil || !strings.Contains(PlainText(last), "truncated") {
		t.Error("block budget notice missing")
	}
}

func TestHTMLCodeHook(t *testing.T) {
	doc := Parse("```go\nx <- 1\n```\n\n```\nplain <b>\n```\n", GFM)
	code := func(info, text string) (string, string, bool) {
		if info != "go" {
			return "", "", false
		}
		return "<i>" + strings.TrimSpace(text) + "</i>", "drawn", true
	}
	got := HTML(doc, HTMLOptions{Code: code})
	if !strings.Contains(got, `<pre class="drawn"><code class="language-go"><i>x <- 1</i></code></pre>`) ||
		!strings.Contains(got, "<pre><code>plain &lt;b&gt;\n</code></pre>") {
		t.Errorf("hook: %s", got)
	}
}
