//go:build kmdai

package ai

import (
	"strings"

	"github.com/DipaolaFederico/kmd/internal/md"
	"github.com/DipaolaFederico/kmd/internal/render"
	"github.com/DipaolaFederico/kmd/internal/termimg"
)

// RenderAnswer renders the model's answer through kmd's Markdown pipeline,
// which removes terminal escapes and unsafe URLs exactly as in a document:
// a hostile document can make the model write anything. Unlike a document's,
// the answer's images are never drawn, so it can make kmd read no file;
// they show as their alt text. o otherwise sets width, theme and colours.
func RenderAnswer(answer string, o render.Options) render.Result {
	o.Images = termimg.None
	o.BaseDir, o.BaseURL, o.AnyImagePath = "", nil, false
	o.Placements, o.Pauses, o.Budget = false, false, nil
	return render.Render(md.Parse(unfence(answer), md.GFM), o)
}

// unfence unwraps an answer that is one ```markdown block, which small
// models sometimes write (#44), so that it is rendered rather than shown as
// source.
func unfence(s string) string {
	t := strings.TrimSpace(s)
	for _, open := range []string{"```markdown\n", "```md\n"} {
		if inner, ok := strings.CutPrefix(t, open); ok {
			if inner, ok := strings.CutSuffix(inner, "\n```"); ok && !strings.Contains(inner, "\n```") {
				return inner
			}
		}
	}
	return s
}
