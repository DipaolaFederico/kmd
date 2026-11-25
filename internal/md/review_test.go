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
