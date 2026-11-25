//go:build kmdai

package ai

import (
	"strings"

	"github.com/DipaolaFederico/kmd/internal/md"
	"github.com/DipaolaFederico/kmd/internal/render"
)

// maxAnswer bounds the text RenderAnswer renders. A real answer is a few
// KB (answerTokens); only a server ignoring max_tokens sends more, up to
// the 8 MiB response cap, which takes a third of a second to render.
const maxAnswer = 256 << 10

// RenderAnswer renders the model's answer, or its reasoning, through kmd's
// Markdown pipeline, which removes terminal escapes and unsafe URLs exactly
// as in a document: a hostile document can make the model write anything.
// Only the presentation options of o are used, plus BaseDir and BaseURL so
// that links resolve as the document's do. Images are never drawn, only
// their alt text, so the answer can make kmd read no file; options added to
// render.Options later stay off until chosen here.
func RenderAnswer(answer string, o render.Options) render.Result {
	safe := render.Options{
		Width: o.Width, Theme: o.Theme, Color: o.Color, TrueColor: o.TrueColor,
		Hyperlinks: o.Hyperlinks, Navigate: o.Navigate,
		Math: o.Math, Diagrams: o.Diagrams,
		BaseDir: o.BaseDir, BaseURL: o.BaseURL,
	}
	return render.Render(md.Parse(unfence(cut(answer, maxAnswer)), md.GFM), safe)
}

// unfence unwraps an answer that is one ```markdown block, which small
// models sometimes write (#44), so that it is rendered rather than shown as
// source.
func unfence(s string) string {
	t := strings.TrimSpace(s)
	for _, open := range []string{"```markdown\n", "```md\n"} {
		body, ok := strings.CutPrefix(t, open)
		if !ok {
			continue
		}
		if body, ok = strings.CutSuffix(body, "\n```"); ok && !strings.Contains(body, "\n```") {
			return body
		}
	}
	return s
}
