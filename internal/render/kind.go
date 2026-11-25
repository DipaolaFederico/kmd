package render

import (
	"path/filepath"
	"strings"
)

// SameKind reports whether the files named a and b are the same kind of
// document: both Markdown (any of MarkdownExt), or with the same extension.
// A symlink must point at a file of its own kind: notes.md → ~/.ssh/id_rsa
// or doc.pdf → run.sh is refused, notes.md → notes.markdown is not.
func SameKind(a, b string) bool {
	ea, eb := strings.ToLower(filepath.Ext(a)), strings.ToLower(filepath.Ext(b))
	return ea == eb || MarkdownExt[ea] && MarkdownExt[eb]
}
