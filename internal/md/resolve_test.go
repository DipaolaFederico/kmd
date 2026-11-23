package md

import (
	"net/url"
	"strings"
	"testing"
)

func TestResolveLink(t *testing.T) {
	base, _ := url.Parse("https://example.com/docs/README.md")
	for dest, want := range map[string]string{
		"guide.md":            "https://example.com/docs/guide.md",
		"../a/b.md#Setup":     "https://example.com/a/b.md#Setup",
		"/top.md?x=1":         "https://example.com/top.md?x=1",
		"//other.org/x":       "https://other.org/x",
		"My%20Notes.md":       "https://example.com/docs/My%20Notes.md",
		"#anchor":             "#anchor",
		"":                    "",
		"https://a.b/c":       "https://a.b/c",
		"javascript:alert(1)": "javascript:alert(1)",
		"file:///etc/passwd":  "file:///etc/passwd",
		"mailto:a@b.c":        "mailto:a@b.c",
		"bad%zz":              "bad%zz", // does not parse: kept as written
	} {
		if got := ResolveLink(base, dest); got != want {
			t.Errorf("ResolveLink(%q) = %q, want %q", dest, got, want)
		}
	}
	if got := ResolveLink(nil, "guide.md"); got != "guide.md" {
		t.Errorf("nil base: %q", got)
	}
}

// With a base, HTML links are absolute while images stay as written: a
// browser opening the page would download them.
func TestHTMLBase(t *testing.T) {
	base, _ := url.Parse("https://example.com/docs/")
	out := HTML(Parse("[g](guide.md) ![i](i.png) [j](javascript:x)", GFM), HTMLOptions{BaseURL: base})
	for _, want := range []string{`href="https://example.com/docs/guide.md"`, `src="i.png"`, `href=""`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s in %s", want, out)
		}
	}
}
