package render

import (
	"html"
	"strconv"
	"strings"
)

// HTMLPage wraps an HTML fragment in a standalone, theme-colored page whose
// Content-Security-Policy forbids scripts. generator names the program in
// a <meta name="generator"> tag.
func HTMLPage(title, body, generator string, t *Theme) string {
	return page(title, body, generator, t, documentCSS)
}

// SlidesPage is HTMLPage for a presentation: one 16:9 slide per section,
// stacked on screen and one per page when printed (the browser's "Print to
// PDF" makes the deck), each with a footer of footer and its number unless
// hideFooter.
func SlidesPage(title string, slides []string, footer string, hideFooter bool, generator string, t *Theme) string {
	n := strconv.Itoa(len(slides))
	footer = html.EscapeString(Clean(footer)) // callers clean it too: this is exported
	size := 0
	for _, s := range slides {
		size += len(s) + len(footer) + 128
	}
	var b strings.Builder
	b.Grow(size)
	for i, s := range slides {
		b.WriteString("<section class=\"slide\">\n<div class=\"content\">\n")
		b.WriteString(s)
		b.WriteString("</div>\n")
		if !hideFooter {
			b.WriteString("<footer><span>" + footer + "</span><span>" + strconv.Itoa(i+1) + " / " + n + "</span></footer>\n")
		}
		b.WriteString("</section>\n")
	}
	return page(title, b.String(), generator, t, slidesCSS)
}

// documentCSS lays out a document as a readable column.
const documentCSS = `body { max-width: 52rem; margin: 2rem auto; padding: 0 1rem; }
`

// slidesCSS lays out the slides: 16:9 boxes, the content centered, the
// footer at the bottom; printed, one slide per 16:9 page of 320 × 180 mm,
// colors kept. Screen sizes are the printed ones over the page width
// (9 mm / 320 mm = 2.8vw…), so the browser shows what the PDF will be.
// Content taller than a slide is cut.
const slidesCSS = `body { margin: 0; }
.slide { box-sizing: border-box; position: relative; width: 100%; aspect-ratio: 16 / 9; overflow: hidden;
  padding: 3.1vw 6.9vw 4.4vw; display: flex; flex-direction: column; justify-content: center;
  font-size: 2.8vw; border-bottom: 1px solid var(--border); }
.slide img { max-height: 34vw; }
.slide > footer { position: absolute; left: 6.9vw; right: 6.9vw; bottom: 1.6vw; display: flex;
  justify-content: space-between; font-size: .7em; color: var(--muted); }
@page { size: 320mm 180mm; margin: 0; }
@media print {
  html, body { -webkit-print-color-adjust: exact; print-color-adjust: exact; }
  .slide { width: 320mm; height: 180mm; aspect-ratio: auto; font-size: 9mm; padding: 10mm 22mm 14mm;
    border: 0; break-after: page; page-break-after: always; }
  .slide:last-child { break-after: auto; page-break-after: auto; }
  .slide img { max-height: 110mm; }
  .slide > footer { left: 22mm; right: 22mm; bottom: 5mm; }
}
`

func page(title, body, generator string, t *Theme, layout string) string {
	if t == nil {
		t = &Dark
	}
	// the title is text; the generator, in an attribute, is kmd's own
	esc := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'; img-src * data:; base-uri 'none'; form-action 'none'">
<meta name="generator" content="` + esc.Replace(generator) + `">
<title>` + esc.Replace(Clean(title)) + `</title>
<style>
:root { --bg: ` + t.Bg.Hex() + `; --fg: ` + t.Fg.Hex() + `; --muted: ` + t.Muted.Hex() + `; --border: ` + t.Border.Hex() + `;
  --code-bg: ` + t.CodeBg.Hex() + `; --code: ` + t.Code.Hex() + `; --link: ` + t.Link.Hex() + `; --h1: ` + t.H1Bg.Hex() + `;
  --h2: ` + t.H2.Hex() + `; --h3: ` + t.H3.Hex() + `; --note: ` + t.Note.Hex() + `; --tip: ` + t.Tip.Hex() + `;
  --important: ` + t.Important.Hex() + `; --warning: ` + t.Warning.Hex() + `; --caution: ` + t.Caution.Hex() + `; }
body { background: var(--bg); color: var(--fg); font: 16px/1.6 system-ui, sans-serif; }
` + layout + `h1, h2 { border-bottom: 1px solid var(--border); padding-bottom: .3em; } h1 { color: var(--h1); } h2 { color: var(--h2); } h3 { color: var(--h3); }
a { color: var(--link); } img { max-width: 100%; }
code, pre { font-family: ui-monospace, monospace; background: var(--code-bg); border-radius: 6px; }
code { padding: .15em .35em; color: var(--code); } pre { padding: 1em; overflow: auto; } pre code { padding: 0; color: inherit; }
blockquote { margin: 0; padding: 0 1em; border-left: 4px solid var(--border); color: var(--muted); }
table { border-collapse: collapse; } th, td { border: 1px solid var(--border); padding: .4em .8em; }
hr { border: 0; border-top: 1px solid var(--border); }
.markdown-alert { padding: .5em 1em; border-left: 4px solid var(--note); margin: 1em 0; }
.markdown-alert-title { font-weight: 600; margin: 0; }
.markdown-alert-note { border-color: var(--note); } .markdown-alert-tip { border-color: var(--tip); }
.markdown-alert-important { border-color: var(--important); } .markdown-alert-warning { border-color: var(--warning); }
.markdown-alert-caution { border-color: var(--caution); }
.math { font-family: "STIX Two Math", "Cambria Math", serif; } div.math { text-align: center; margin: 1em 0; }
.footnotes { font-size: .9em; color: var(--muted); border-top: 1px solid var(--border); margin-top: 2em; }
</style>
</head>
<body>
` + body + `</body>
</html>
`
}
