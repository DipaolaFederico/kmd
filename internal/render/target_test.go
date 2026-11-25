package render

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DipaolaFederico/kmd/internal/md"
	"github.com/DipaolaFederico/kmd/internal/testlimit"
)

// osc8Targets returns the OSC 8 targets opened in out, in order.
func osc8Targets(out string) []string {
	var got []string
	for _, s := range strings.Split(out, "\x1b]8;;")[1:] {
		if t, _, _ := strings.Cut(s, "\x1b\\"); t != "" {
			got = append(got, t)
		}
	}
	return got
}

// A terminal hands a clicked OSC 8 target to the desktop, past the
// viewer's checks: only web and mail URLs and local Markdown or document
// files may become one, whatever the spelling (SR-09, SR-16).
func TestOSC8TargetPolicy(t *testing.T) {
	dir := t.TempDir()
	abs := "file://" + filepath.ToSlash(dir)
	for src, want := range map[string]string{
		// SR-09: images and bare paths
		"![Build status](file:///tmp/evil.desktop)":    "",
		"[setup](/tmp/evil.desktop)":                   "",
		"[x](evil.sh)":                                 "",
		"[u](//attacker.example/share/evil.desktop)":   "",
		`<img alt="h" src="file:///tmp/evil.command">`: "",
		"![logo](logo.png)":                            abs + "/logo.png",
		"[[Page]]":                                     abs + "/Page.md",
		"[[run.sh]]":                                   abs + "/run.sh.md", // wiki links are to Markdown pages
		"<https://example.com/a>":                      "https://example.com/a",
		"www.example.com":                              "http://www.example.com",
		"[a](#top)":                                    "",
		"[d](/tmp/doc.md)":                             "file:///tmp/doc.md",
		"[d](file://localhost/tmp/doc.md#s)":           "file:///tmp/doc.md#s",
		"[i](data:image/png;base64,AAAA)":              "",
		"![i](data:image/png;base64,AAAA)":             "",
		"[s](https://example.com/a?b=c#d)":             "https://example.com/a?b=c#d",
		"[m](mailto:a@b.c?subject=hi&body=x)":          "mailto:a@b.c?body=x&subject=hi",
		// SR-16: other schemes, remote and NUL-hidden files, mailto
		"[docs](ssh://-oProxyCommand=touch$IFS/tmp/pwn@x/)": "",
		"[docs](smb://attacker.example/share)":              "",
		"[docs](vscode://file/etc/passwd)":                  "",
		"[docs](javascript:alert(1))":                       "",
		"[m](mailto:a@b.c?attach=/etc/passwd&subject=hi)":   "mailto:a@b.c?subject=hi",
		"[f](file://attacker.example/share/x.md)":           "",
		"[f](file:///tmp/evil.desktop%00.md)":               "",
		"[f](evil.desktop%00.md)":                           "",
		"[w](https:///nohost)":                              "",
		"[w](https://github.com%2Flogin@evil.example/x)":    "",
		"[w](https://user:pw@example.com/)":                 "",
		// network shares named by file: URLs or paths, either separator
		"[a](file:////host/share/a.md)":          "",
		"[d](file://localhost//host/share/a.md)": "",
		"[g](file:/%2F%2Fhost/share/a.md)":       "",
		"[b](file:///%5C%5Chost%5Cshare%5Ca.md)": "",
		"[r](%5C%5Chost%5Cshare%5Ca.md)":         "",
		"[r](%2F%5Chost/share/a.md)":             "",
		"[u](file:///tmp/doc\u200b.md)":          "",
		// a parameter hidden in the address
		"[m](mailto:%3Fattach=/etc/passwd@x)":  "",
		"[m](mailto:a%26attach=/etc/passwd@x)": "",
	} {
		res := Render(md.Parse(src, md.GFM), Options{Width: 80, Color: true, Hyperlinks: true, Images: 0, BaseDir: dir})
		if got := strings.Join(osc8Targets(res.Text), " "); got != want {
			t.Errorf("%s: OSC 8 target %q, want %q", src, got, want)
		}
	}
}

// Without hyperlinks no target is computed or emitted, and the viewer
// still gets every destination to explain why it does not open it.
func TestNoTargetsWithoutHyperlinks(t *testing.T) {
	src := "[a](evil.sh) [b](https://e.x/) ![c](c.png)"
	res := Render(md.Parse(src, md.GFM), Options{Width: 80, Color: true, Navigate: true, BaseDir: t.TempDir()})
	if strings.Contains(res.Text, "]8;;") {
		t.Errorf("OSC 8 without hyperlinks: %q", res.Text)
	}
	if len(res.Links) != 2 || !strings.HasSuffix(res.Links[0].Href, "/evil.sh") {
		t.Errorf("links %+v", res.Links)
	}
}

// NoWorkingDir (the public package) never resolves relative links against
// the working directory, which a service would disclose (SR-25).
func TestNoWorkingDir(t *testing.T) {
	src := "[g](guide.md) [a](/abs/a.md)"
	res := Render(md.Parse(src, md.GFM), Options{Width: 80, Hyperlinks: true, NoWorkingDir: true})
	if got := strings.Join(osc8Targets(res.Text), " "); got != "file:///abs/a.md" {
		t.Errorf("targets %q", got)
	}
	res = Render(md.Parse(src, md.GFM), Options{Width: 80, Hyperlinks: true, NoWorkingDir: true, BaseDir: "/docs"})
	if got := strings.Join(osc8Targets(res.Text), " "); got != "file:///docs/guide.md file:///abs/a.md" {
		t.Errorf("targets with BaseDir %q", got)
	}
}

// The runs of a link on one row form one span; zero-width characters do
// not count against the line width, so their text was concatenated
// quadratically and without bound (SR-15).
func TestLinkSpanText(t *testing.T) {
	res := Render(md.Parse("[a *b* c](u) [d](v)", md.GFM), Options{Width: 80, Color: true, Navigate: true})
	if len(res.Links) != 2 || len(res.Links[0].Spans) != 1 || res.Links[0].Spans[0].Text != "a b c" ||
		res.Links[0].Spans[0].Width != 5 || res.Links[1].Spans[0].Text != "d" {
		t.Errorf("spans %+v", res.Links)
	}
	zw := func(n int) *md.Node { return md.Parse("["+strings.Repeat("&#x200b;", n)+"x](u)\n", md.GFM) }
	start := time.Now()
	res = Render(zw(200_000), Options{Width: 80, Navigate: true})
	if d := time.Since(start); d > testlimit.Scale()*2*time.Second {
		t.Errorf("200,000 zero-width characters in a link took %v", d)
	}
	if s := res.Links[0].Spans[0]; len(s.Text) > maxSpanText || s.Width != 1 {
		t.Errorf("span of %d bytes, width %d", len(s.Text), s.Width)
	}
	testlimit.Linear(t, "zero-width link text", 1<<20, func(n int) func() {
		doc := zw(n)
		return func() { _ = Render(doc, Options{Width: 80, Navigate: true}) }
	})
}

// A long target is validated once per link, not once per painted run (a
// word each), and not at all without hyperlinks (SR-26): rendering a long
// link costs about the same with a 2 KiB target as with a short one.
// maxURL bounds the cost
// per run, so the regression is a constant factor that only time shows,
// not a growth testlimit.Linear could catch: short and long targets are
// timed alternately, keeping the best of several runs of each. Emphasis
// keeps every word a run of its own, and wide lines keep the cost of
// writing the target on each line small: validating per run is about 6×
// slower there, against a limit of 3×.
func TestLongTargetValidatedOnce(t *testing.T) {
	text := strings.Repeat("a *b* ", 50_000)
	for _, links := range []bool{false, true} {
		docs := [2]*md.Node{}
		for i, n := range []int{10, maxURL - 20} {
			docs[i] = md.Parse("["+text+"x](http://e.x/"+strings.Repeat("a", n)+")\n", md.GFM)
		}
		best := [2]time.Duration{time.Hour, time.Hour}
		for range 5 {
			for i, doc := range docs {
				start := time.Now()
				res := Render(doc, Options{Width: 1000, Color: true, Hyperlinks: links})
				best[i] = min(best[i], time.Since(start))
				if links != (len(osc8Targets(res.Text)) > 0) {
					t.Fatalf("hyperlinks %v: %d targets", links, len(osc8Targets(res.Text)))
				}
			}
		}
		if short, long := best[0], best[1]; long > testlimit.Scale()*2*time.Second || long > 3*short {
			t.Errorf("hyperlinks %v: %v with a long target, %v with a short one", links, long, short)
		}
	}
}
