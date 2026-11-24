// Package md is a dependency-free Markdown parser that implements the
// CommonMark 0.31.2 specification plus GitHub Flavored Markdown (tables,
// task lists, strikethrough, extended autolinks, tag filter), footnotes,
// GitHub alerts and math ($…$, $$…$$). It follows the reference
// two-phase algorithm: a line-oriented block phase that builds the
// container tree, then an inline phase using delimiter and bracket stacks.
package md

import "strings"

// NodeType identifies a node in the document tree.
type NodeType uint8

// Block node types.
const (
	Document NodeType = iota
	BlockQuote
	List
	Item
	Paragraph
	Heading
	ThematicBreak
	CodeBlock
	HTMLBlock
	Table
	TableRow
	TableCell
	FootnoteDef
	MathBlock
	FrontMatter

	// Inline node types.
	Text
	SoftBreak
	LineBreak
	Code
	Emph
	Strong
	Strike
	Link
	Image
	HTMLInline
	Math
	FootnoteRef
)

var typeNames = [...]string{"document", "block_quote", "list", "item", "paragraph", "heading",
	"thematic_break", "code_block", "html_block", "table", "table_row", "table_cell",
	"footnote_def", "math_block", "front_matter", "text", "softbreak", "linebreak", "code", "emph", "strong",
	"strike", "link", "image", "html_inline", "math", "footnote_ref"}

func (t NodeType) String() string {
	if int(t) < len(typeNames) {
		return typeNames[t]
	}
	return "unknown"
}

// IsBlock reports whether t is a block-level node type.
func (t NodeType) IsBlock() bool { return t < Text }

// Align is a table column alignment.
type Align uint8

// Column alignments.
const (
	AlignNone Align = iota
	AlignLeft
	AlignCenter
	AlignRight
)

// Task is the checkbox state of a list item.
type Task uint8

// Task states.
const (
	TaskNone Task = iota
	TaskOpen
	TaskDone
)

// Node is an element of the document tree. Fields are meaningful only for
// the node types noted in their comments.
type Node struct {
	Type                                      NodeType
	Parent, FirstChild, LastChild, Prev, Next *Node

	Literal string // Text, Code, CodeBlock, HTMLBlock, HTMLInline, Math, MathBlock, FrontMatter
	Dest    string // Link, Image
	Title   string // Link, Image
	Level   int    // Heading level
	Info    string // CodeBlock info string
	Fenced  bool   // CodeBlock
	Display bool   // Math: $$…$$

	// List and Item
	Ordered    bool
	Start      int
	Tight      bool
	BulletChar byte
	Delim      byte
	Task       Task

	Alert  string  // BlockQuote: NOTE, TIP, IMPORTANT, WARNING, CAUTION
	Aligns []Align // Table
	Header bool    // TableRow
	Align  Align   // TableCell
	Label  string  // FootnoteDef, FootnoteRef: normalized label
	Index  int     // FootnoteDef, FootnoteRef: 1-based footnote number

	// parser state
	startLine, endLine int
	open               bool
	// noExt marks text from a backslash escape or an entity, and autolinks:
	// the emoji and wiki link extensions leave them alone. (Next to open,
	// in its padding: Node does not grow.)
	noExt        bool
	content      []byte
	markerOffset int
	padding      int
	fenceChar    byte
	fenceLen     int
	fenceOffset  int
	htmlType     int
	refCount     int // FootnoteDef: number of references
}

// AppendChild adds child as the last child of n.
func (n *Node) AppendChild(child *Node) {
	child.unlink()
	child.Parent = n
	if n.LastChild != nil {
		n.LastChild.Next = child
		child.Prev = n.LastChild
	} else {
		n.FirstChild = child
	}
	n.LastChild = child
}

// insertAfter inserts sibling right after n.
func (n *Node) insertAfter(sibling *Node) {
	sibling.unlink()
	sibling.Next = n.Next
	if sibling.Next != nil {
		sibling.Next.Prev = sibling
	}
	sibling.Prev = n
	n.Next = sibling
	sibling.Parent = n.Parent
	if sibling.Next == nil && sibling.Parent != nil {
		sibling.Parent.LastChild = sibling
	}
}

// insertBefore inserts sibling right before n.
func (n *Node) insertBefore(sibling *Node) {
	sibling.unlink()
	sibling.Prev = n.Prev
	if sibling.Prev != nil {
		sibling.Prev.Next = sibling
	}
	sibling.Next = n
	n.Prev = sibling
	sibling.Parent = n.Parent
	if sibling.Prev == nil && sibling.Parent != nil {
		sibling.Parent.FirstChild = sibling
	}
}

func (n *Node) unlink() {
	if n.Prev != nil {
		n.Prev.Next = n.Next
	} else if n.Parent != nil {
		n.Parent.FirstChild = n.Next
	}
	if n.Next != nil {
		n.Next.Prev = n.Prev
	} else if n.Parent != nil {
		n.Parent.LastChild = n.Prev
	}
	n.Parent, n.Next, n.Prev = nil, nil, nil
}

// Children returns the direct children of n as a slice.
func (n *Node) Children() []*Node {
	var out []*Node
	for c := n.FirstChild; c != nil; c = c.Next {
		out = append(out, c)
	}
	return out
}

// Walk calls fn for every node in document order; entering is false when
// leaving a node that has children. Returning false from fn on entering
// skips the node's children.
func Walk(n *Node, fn func(n *Node, entering bool) bool) {
	if !fn(n, true) {
		return
	}
	if n.FirstChild == nil {
		return
	}
	for c := n.FirstChild; c != nil; {
		next := c.Next
		Walk(c, fn)
		c = next
	}
	fn(n, false)
}

// PlainText returns the visible text of n's inline content.
func PlainText(n *Node) string {
	var b []byte
	Walk(n, func(c *Node, entering bool) bool {
		if !entering {
			return true
		}
		switch c.Type {
		case Text, Code, Math:
			b = append(b, c.Literal...)
		case SoftBreak, LineBreak:
			b = append(b, ' ')
		}
		return true
	})
	return string(b)
}

// Options selects syntax extensions. The zero value is strict CommonMark.
type Options struct {
	Tables        bool // GFM tables
	Strikethrough bool // GFM ~~strike~~
	Autolinks     bool // GFM www., http(s):// and email autolinks
	TaskLists     bool // GFM - [ ] / - [x]
	Footnotes     bool // [^1] references and definitions
	Alerts        bool // > [!NOTE] … GitHub alerts
	Math          bool // $inline$, $$display$$ and ```math
	FrontMatter   bool // leading YAML front matter between --- lines
	Emoji         bool // :rocket: GitHub emoji shortcodes
	WikiLinks     bool // [[Page]], [[Page#Heading|label]] links to Page.md
}

// MaxInput is the largest document accepted (16 MiB): the tree and the
// rendered lines take tens of bytes per input byte.
const MaxInput = 16 << 20

// GFM enables every extension: GitHub Flavored Markdown and the GitHub and
// Obsidian additions above.
var GFM = Options{Tables: true, Strikethrough: true, Autolinks: true, TaskLists: true, Footnotes: true, Alerts: true,
	Math: true, FrontMatter: true, Emoji: true, WikiLinks: true}

// Section moves the part of doc that starts at the first top-level heading
// for which match is true into a new document: that heading and every block
// up to the next heading of the same or a higher level. Footnote
// definitions referenced from the section come along. It returns nil, and
// leaves doc untouched, when no heading matches.
func Section(doc *Node, match func(heading *Node) bool) *Node {
	var start *Node
	for c := doc.FirstChild; c != nil; c = c.Next {
		if c.Type == Heading && match(c) {
			start = c
			break
		}
	}
	if start == nil {
		return nil
	}
	out := &Node{Type: Document}
	refs := map[string]bool{}
	for c := start; c != nil; {
		next := c.Next
		if c != start && c.Type == Heading && c.Level <= start.Level {
			break
		}
		if c.Type != FootnoteDef {
			Walk(c, func(n *Node, entering bool) bool {
				if entering && n.Type == FootnoteRef {
					refs[n.Label] = true
				}
				return true
			})
			out.AppendChild(c)
		}
		c = next
	}
	for c := doc.FirstChild; c != nil; {
		next := c.Next
		if c.Type == FootnoteDef && refs[c.Label] {
			out.AppendChild(c)
		}
		c = next
	}
	return out
}

// comment returns the text inside a top-level <!-- … --> block, trimmed
// (ok false for any other node). Text after the closing --> is not part
// of it.
func comment(n *Node) (text string, ok bool) {
	if n.Type != HTMLBlock {
		return "", false
	}
	s, ok := strings.CutPrefix(strings.TrimSpace(n.Literal), "<!--")
	if !ok {
		return "", false
	}
	if end := strings.Index(s, "-->"); end >= 0 {
		s = s[:end]
	}
	return strings.TrimSpace(s), true
}

// IsPause reports whether n is a <!-- pause --> comment on its own, which
// reveals the rest of a slide in a later step.
func IsPause(n *Node) bool {
	s, ok := comment(n)
	return ok && s == "pause" && strings.HasSuffix(strings.TrimSpace(n.Literal), "-->")
}

// StripDirectives removes a slide's top-level <!-- pause --> and
// <!-- notes: … --> comments: they are for the presenter, and an export
// must not carry them even where HTML comments are kept.
func StripDirectives(slide *Node) {
	for c := slide.FirstChild; c != nil; {
		next := c.Next
		if s, _ := comment(c); IsPause(c) || strings.HasPrefix(s, "notes:") {
			c.unlink()
		}
		c = next
	}
}

// Notes returns the speaker notes of a slide, as Markdown: the text of its
// top-level <!-- notes: … --> comments, in order, a blank line apart.
func Notes(slide *Node) string {
	var b strings.Builder
	for c := slide.FirstChild; c != nil; c = c.Next {
		s, _ := comment(c) // "" for other blocks
		s, ok := strings.CutPrefix(s, "notes:")
		if !ok {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(strings.TrimSpace(s))
	}
	return b.String()
}

// Slides splits doc into slides, moving its blocks into one new document
// per slide: at every top-level thematic break (---), or, when there is
// none, before every level 1 or 2 heading. At most limit slides are made:
// the rest of the document goes on the last one. Front matter
// is dropped and footnote definitions go to the first slide that
// references them. A document without content gives one empty slide.
func Slides(doc *Node, limit int) []*Node {
	byBreaks := false
	for c := doc.FirstChild; c != nil; c = c.Next {
		if c.Type == ThematicBreak {
			byBreaks = true
			break
		}
	}
	var slides []*Node
	cur := &Node{Type: Document}
	defs := map[string]*Node{}
	flush := func() {
		if cur.FirstChild != nil && len(slides) < limit-1 {
			slides = append(slides, cur)
			cur = &Node{Type: Document}
		}
	}
	for c := doc.FirstChild; c != nil; {
		next := c.Next
		switch {
		case c.Type == FrontMatter:
		case c.Type == FootnoteDef:
			if _, dup := defs[c.Label]; !dup {
				defs[c.Label] = c
			}
		case byBreaks && c.Type == ThematicBreak:
			flush()
		case !byBreaks && c.Type == Heading && c.Level <= 2:
			flush()
			cur.AppendChild(c)
		default:
			cur.AppendChild(c)
		}
		c = next
	}
	if cur.FirstChild != nil {
		slides = append(slides, cur)
	}
	if len(slides) == 0 {
		return []*Node{{Type: Document}}
	}
	if len(defs) > 0 { // linear: each reference looks its definition up once
		for _, s := range slides {
			var used []*Node
			Walk(s, func(n *Node, entering bool) bool {
				if entering && n.Type == FootnoteRef {
					if d := defs[n.Label]; d != nil {
						used = append(used, d)
						delete(defs, n.Label)
					}
				}
				return true
			})
			for _, d := range used {
				s.AppendChild(d)
			}
		}
	}
	return slides
}
