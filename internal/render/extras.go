package render

import (
	"html"
	"strconv"
	"strings"

	"github.com/DipaolaFederico/kmd/internal/diagram"
	"github.com/DipaolaFederico/kmd/internal/md"
	"github.com/DipaolaFederico/kmd/internal/styled"
	"github.com/DipaolaFederico/kmd/internal/termimg"
	"github.com/DipaolaFederico/kmd/internal/tex"
)

// classStyle maps a sub-renderer class (math, diagrams) to a theme style.
func (r *renderer) classStyle(c styled.Class) Style {
	th := r.th
	switch c {
	case styled.Muted:
		return Style{Fg: th.Muted}
	case styled.Border:
		return Style{Fg: th.DiagBorder}
	case styled.Edge:
		return Style{Fg: th.DiagEdge}
	case styled.Arrow:
		return Style{Fg: th.DiagArrow, Attr: Bold}
	case styled.Label:
		return Style{Fg: th.DiagLabel, Attr: Italic}
	case styled.Title:
		return Style{Fg: th.DiagTitle, Attr: Bold}
	case styled.Node:
		return Style{Fg: th.DiagNode}
	case styled.Var:
		return Style{Fg: th.MathVar, Attr: Italic}
	case styled.Num:
		return Style{Fg: th.MathNum}
	case styled.Op:
		return Style{Fg: th.MathOp}
	case styled.Symbol:
		return Style{Fg: th.MathSymbol}
	case styled.Func:
		return Style{Fg: th.MathFunc}
	}
	if c >= styled.Series0 && c <= styled.Series7 {
		return Style{Fg: th.Series[c-styled.Series0]}
	}
	return Style{}
}

func (r *renderer) styledLine(l styled.Line) rline {
	out := make(rline, 0, len(l))
	for _, s := range l {
		out = append(out, run{text: s.Text, st: r.classStyle(s.Class)})
	}
	return out
}

func (r *renderer) mathInline(src string, base Style, out []run) []run {
	r.mathCount++
	if !r.opt.Math || r.mathCount > maxInlineMath {
		return append(out, run{text: "$" + src + "$", st: Style{Fg: r.th.Code, Attr: base.Attr}})
	}
	for _, s := range tex.Inline(src) {
		st := r.classStyle(s.Class)
		st.Attr |= base.Attr &^ Italic
		st.URL = base.URL
		// non-breaking spaces keep a formula on one line when possible
		out = append(out, run{text: strings.ReplaceAll(s.Text, " ", " "), st: st})
	}
	return out
}

// mathDisplay renders a display formula centered in width.
func (r *renderer) mathDisplay(src string, width int) []rline {
	r.specials++
	if r.specials > maxSpecials {
		return r.codeBox(src, "math", "math", width)
	}
	var out []rline
	for _, l := range tex.Display(src, width) {
		rl := r.styledLine(l)
		if pad := (width - lineWidth(rl)) / 2; pad > 0 {
			rl = append(rline{{text: strings.Repeat(" ", pad)}}, rl...)
		}
		out = append(out, rl)
	}
	return out
}

// special renders diagram and math code blocks; ok is false to fall back
// to a regular code box.
func (r *renderer) special(src, lang string, width int) ([]rline, bool) {
	var lines []styled.Line
	ok := false
	if lang == "mermaid" || lang == "dot" || lang == "graphviz" {
		r.specials++
		if r.specials > maxSpecials {
			return nil, false
		}
	}
	switch lang {
	case "mermaid":
		if r.opt.Diagrams {
			lines, ok = diagram.Mermaid(src, width)
		}
	case "dot", "graphviz":
		if r.opt.Diagrams {
			lines, ok = diagram.Dot(src, width)
		}
	case "math":
		if r.opt.Math {
			return r.mathDisplay(src, width), true
		}
	}
	if !ok {
		return nil, false
	}
	out := make([]rline, 0, len(lines))
	for _, l := range lines {
		out = append(out, r.styledLine(l))
	}
	return out, true
}

// imageParagraph draws a paragraph made only of images as real pictures.
func (r *renderer) imageParagraph(p *md.Node, width int) ([]rline, bool) {
	if r.opt.Images == termimg.None || !r.opt.Color {
		return nil, false
	}
	var imgs []*md.Node
	for c := p.FirstChild; c != nil; c = c.Next {
		switch {
		case c.Type == md.Image:
			imgs = append(imgs, c)
		case c.Type == md.Link && c.FirstChild != nil && c.FirstChild == c.LastChild && c.FirstChild.Type == md.Image:
			imgs = append(imgs, c.FirstChild) // [![badge](img)](link)
		case c.Type == md.SoftBreak || c.Type == md.LineBreak || (c.Type == md.Text && strings.TrimSpace(c.Literal) == ""):
		default:
			return nil, false
		}
	}
	if len(imgs) == 0 {
		return nil, false
	}
	rows := r.opt.ImageRows
	if rows <= 0 {
		rows = 20
	}
	var out []rline
	graphics := false
	for i, n := range imgs {
		res, ok := r.loadImage(n.Dest, width, rows)
		if !ok {
			return nil, false
		}
		if i > 0 {
			out = append(out, nil)
		}
		for _, l := range res.frame.Lines {
			out = append(out, rline{{text: l, raw: true, w: res.frame.Cols}})
		}
		if alt := md.PlainText(n); alt != "" {
			out = append(out, rline{{text: alt, st: Style{Fg: r.th.Muted, Attr: Italic}}})
		}
		graphics = graphics || r.opt.Images != termimg.Blocks
	}
	r.graphics = r.graphics || graphics // only once every image succeeded
	return out, true
}

type imageResult struct {
	frame termimg.Frame
	ok    bool
}

// loadImage decodes and renders an image once per document (repeated
// references reuse the frame) within the per-document budgets.
func (r *renderer) loadImage(src string, width, rows int) (imageResult, bool) {
	key := src + "\x00" + strconv.Itoa(width) + "x" + strconv.Itoa(rows)
	if res, ok := r.images[key]; ok {
		if r.imgBytes+frameBytes(res.frame) > maxImageBytes {
			return imageResult{}, false
		}
		r.imgBytes += frameBytes(res.frame)
		return res, res.ok
	}
	if r.images == nil {
		r.images = map[string]imageResult{}
	}
	if len(r.images) >= maxImages {
		return imageResult{}, false
	}
	var res imageResult
	if img, err := termimg.Load(src, r.opt.BaseDir); err == nil {
		if f, err := termimg.Render(img, r.opt.Images, width, rows, r.opt.Cell); err == nil && len(f.Lines) > 0 {
			res = imageResult{frame: f, ok: true}
		}
	}
	r.images[key] = res
	if !res.ok || r.imgBytes+frameBytes(res.frame) > maxImageBytes {
		return imageResult{}, false
	}
	r.imgBytes += frameBytes(res.frame)
	return res, true
}

func frameBytes(f termimg.Frame) int {
	n := 0
	for _, l := range f.Lines {
		n += len(l)
	}
	return n
}

// htmlInline renders raw HTML as text: tags are dropped except <br> (line
// break) and <img> (image reference); entities are decoded.
func (r *renderer) htmlInline(s string, base Style, out []run) []run {
	var text strings.Builder
	flush := func() {
		if text.Len() > 0 {
			t := strings.Join(strings.Fields(html.UnescapeString(text.String())), " ")
			if t != "" {
				out = append(out, run{text: t, st: base})
			}
			text.Reset()
		}
	}
	for i := 0; i < len(s); {
		if s[i] != '<' {
			j := strings.IndexByte(s[i:], '<')
			if j < 0 {
				j = len(s) - i
			}
			chunk := s[i : i+j]
			if strings.TrimSpace(chunk) == "" && text.Len() == 0 {
				chunk = ""
			}
			if chunk != "" && (chunk[0] == ' ' || chunk[0] == '\n') && text.Len() == 0 && len(out) > 0 {
				text.WriteByte(' ')
			}
			text.WriteString(chunk)
			i += j
			continue
		}
		if strings.HasPrefix(s[i:], "<!--") {
			e := strings.Index(s[i+4:], "-->")
			if e < 0 {
				break
			}
			i += e + 7
			continue
		}
		e := strings.IndexByte(s[i:], '>')
		if e < 0 {
			text.WriteString(s[i:])
			break
		}
		tag := s[i+1 : i+e]
		i += e + 1
		name := strings.ToLower(strings.TrimLeft(tag, "/"))
		if k := strings.IndexAny(name, " \t\n/"); k >= 0 {
			name = name[:k]
		}
		switch name {
		case "br":
			flush()
			out = append(out, run{text: "\n", st: base})
		case "img":
			flush()
			out = r.imageRef(attr(tag, "alt"), attr(tag, "src"), base, out)
		case "p", "div", "li", "tr", "h1", "h2", "h3", "h4", "h5", "h6":
			if text.Len() > 0 {
				text.WriteByte(' ')
			}
		}
	}
	flush()
	return out
}

// attr extracts an attribute value from the inside of an HTML tag. The
// name match is ASCII case-insensitive on the original bytes, so offsets
// stay valid for any UTF-8 content.
func attr(tag, name string) string {
	for i := 0; i+len(name)+2 <= len(tag); i++ {
		if i > 0 && tag[i-1] != ' ' && tag[i-1] != '\n' && tag[i-1] != '\t' {
			continue
		}
		if !strings.EqualFold(tag[i:i+len(name)], name) || tag[i+len(name)] != '=' {
			continue
		}
		q := tag[i+len(name)+1]
		if q != '"' && q != '\'' {
			continue
		}
		rest := tag[i+len(name)+2:]
		if e := strings.IndexByte(rest, q); e >= 0 {
			return html.UnescapeString(rest[:e])
		}
	}
	return ""
}
