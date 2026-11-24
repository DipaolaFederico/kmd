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
