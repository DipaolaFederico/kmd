package render

import "strings"

// HTMLPage wraps an HTML fragment in a standalone, theme-colored page whose
// Content-Security-Policy forbids scripts. generator names the program in
// a <meta name="generator"> tag.
func HTMLPage(title, body, generator string, t *Theme) string {
	if t == nil {
		t = &Dark
	}
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
body { background: var(--bg); color: var(--fg); font: 16px/1.6 system-ui, sans-serif; max-width: 52rem; margin: 2rem auto; padding: 0 1rem; }
h1, h2 { border-bottom: 1px solid var(--border); padding-bottom: .3em; } h1 { color: var(--h1); } h2 { color: var(--h2); } h3 { color: var(--h3); }
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
