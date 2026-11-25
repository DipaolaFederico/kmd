package md

import (
	"net/url"
	"sort"
	"strings"
)

// Emoji shortcodes (:rocket:) and wiki links ([[Page]], [[Page#Heading|label]])
// are applied after the CommonMark inline algorithm, to the text it left
// literal: never inside code spans, raw HTML, autolinks or link destinations
// (those are not Text nodes), never to backslash-escaped or entity-encoded
// characters, and a bracket pair that already formed a link is not text.

const (
	maxShortcode = 40   // longest shortcode looked up (the longest known is 32)
	maxWikiLink  = 1024 // longest [[…]] content: page, heading and label
)

type shortcode struct{ code, emoji string }

// lookupEmoji returns the emoji of a shortcode name (without colons).
func lookupEmoji(code string) (string, bool) {
	i := sort.Search(len(emojiTable), func(i int) bool { return emojiTable[i].code >= code })
	if i < len(emojiTable) && emojiTable[i].code == code {
		return emojiTable[i].emoji, true
	}
	return "", false
}

func isShortcodeChar(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_' || c == '+' || c == '-'
}

// shortcodeAt matches a known :shortcode: at s[i] (a colon). It returns the
// emoji and the end of the match, or "" and the position where scanning
// resumes: after the name, so a closing colon may open the next shortcode
// and every byte is examined a bounded number of times.
func shortcodeAt(s string, i int) (string, int) {
	j := i + 1
	for j < len(s) && j-i-1 <= maxShortcode && isShortcodeChar(s[j]) {
		j++
	}
	if j < len(s) && s[j] == ':' && j > i+1 && j-i-1 <= maxShortcode {
		if e, ok := lookupEmoji(s[i+1 : j]); ok {
			return e, j + 1
		}
	}
	return "", j
}

// wikiAt matches [[…]] at s[i] (a '['). It returns the link, or nil and the
// position where scanning resumes: just after the "[[", so shortcodes in
// the text that follows are still found. Scanning stays linear: the next
// wiki attempt starts at a '[' at or after k, where this scan stopped.
func wikiAt(s string, i int, opt *Options) (*Node, int) {
	if i+1 >= len(s) || s[i+1] != '[' {
		return nil, i + 1
	}
	k := i + 2
	for k < len(s) && k-i-2 <= maxWikiLink && s[k] != '[' && s[k] != ']' {
		k++
	}
	if k+1 < len(s) && s[k] == ']' && s[k+1] == ']' && k-i-2 <= maxWikiLink {
		if l := wikiLink(s[i+2:k], opt); l != nil {
			return l, k + 2
		}
	}
	if k == i+2 { // "[[[" or "[[]": a match may start at i+1
		return nil, i + 1
	}
	return nil, i + 2
}

// wikiLink builds the link of the content of [[page#heading|label]]: a
// relative link to page (".md" appended when it has no known extension),
// shown as the label, or else the page name (or the heading).
func wikiLink(content string, opt *Options) *Node {
	for i := 0; i < len(content); i++ {
		if c := content[i]; c < 0x20 || c == 0x7f {
			return nil
		}
	}
	target, label, _ := strings.Cut(content, "|")
	page, heading, _ := strings.Cut(target, "#")
	page, heading, label = strings.TrimSpace(page), strings.TrimSpace(heading), strings.TrimSpace(label)
	// a scheme ("https:", "C:") or an absolute or protocol-relative path is
	// not a page of this document's directory
	if page == "" && heading == "" || strings.IndexByte(page, ':') >= 0 || strings.HasPrefix(page, "/") || strings.HasPrefix(page, `\`) {
		return nil
	}
	var dest strings.Builder
	if page != "" {
		for i, seg := range strings.Split(page, "/") {
			if i > 0 {
				dest.WriteByte('/')
			}
			dest.WriteString(url.PathEscape(seg))
		}
		if !knownExt(page) {
			dest.WriteString(".md")
		}
	}
	if heading != "" {
		dest.WriteByte('#')
		dest.WriteString(url.PathEscape(heading))
	}
	switch {
	case label != "":
	case page != "":
		label = page
	default:
		label = heading
	}
	l := &Node{Type: Link, Dest: dest.String()}
	if kids := expandText(label, opt, nil); kids != nil { // a label holds no wiki link
		for _, k := range kids {
			l.AppendChild(k)
		}
	} else {
		l.AppendChild(text(label))
	}
	return l
}

// wikiExt are the extensions a wiki link target keeps as is; any other name
// ("Release 1.2", "Notes") names a Markdown page.
var wikiExt = [...]string{".md", ".markdown", ".mdown", ".mkd", ".mkdn", ".mdwn", ".txt", ".pdf",
	".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".csv", ".json", ".yaml", ".yml", ".toml"}

func knownExt(page string) bool {
	dot := strings.LastIndexByte(page, '.')
	if dot < 0 || strings.IndexByte(page[dot:], '/') >= 0 {
		return false
	}
	for _, e := range wikiExt {
		if strings.EqualFold(page[dot:], e) {
			return true
		}
	}
	return false
}

// triggers returns the bytes that may start an extension, "" when none is on.
func (o *Options) triggers(wiki bool) string {
	switch {
	case o.Emoji && wiki:
		return ":["
	case o.Emoji:
		return ":"
	case wiki:
		return "["
	}
	return ""
}

// expandText rewrites shortcodes in s, and wiki links when wiki, the
// budget they are charged to, is not nil. It returns nil when nothing
// changed, else the replacement nodes.
func expandText(s string, opt *Options, wiki *nodeBudget) []*Node {
	set := opt.triggers(wiki != nil && opt.WikiLinks)
	if set == "" {
		return nil
	}
	var (
		nodes   []*Node
		buf     []byte // pending text, with emoji substituted
		changed bool
	)
	last, i := 0, 0
	for i < len(s) {
		k := strings.IndexAny(s[i:], set)
		if k < 0 {
			break
		}
		i += k
		if s[i] == ':' {
			e, end := shortcodeAt(s, i)
			if e != "" {
				buf = append(append(buf, s[last:i]...), e...)
				last, changed = end, true
			}
			i = end
			continue
		}
		if wiki.spent() {
			i++ // no budget left for a link: the "[" stays text
			continue
		}
		l, end := wikiAt(s, i, opt)
		if l != nil {
			wiki.take(splitLinkNodes)
			if buf = append(buf, s[last:i]...); len(buf) > 0 {
				nodes = append(nodes, text(string(buf)))
				buf = buf[:0]
			}
			nodes = append(nodes, l)
			last, changed = end, true
		}
		i = end
	}
	if !changed {
		return nil
	}
	if buf = append(buf, s[last:]...); len(buf) > 0 {
		nodes = append(nodes, text(string(buf)))
	}
	return nodes
}

// shortcuts applies the emoji and wiki link extensions to the inline
// children of n. mergeText has joined adjacent text, so each Text node is
// a whole run of it. Its recursion depth is bounded by flattenDeep.
func shortcuts(n *Node, opt *Options, inLink bool, nodes *nodeBudget) {
	wiki := nodes
	if inLink {
		wiki = nil // no links inside links
	}
	set := opt.triggers(wiki != nil && opt.WikiLinks)
	for c := n.FirstChild; c != nil; {
		next := c.Next
		switch {
		case c.Type == Text && !c.noExt:
			if strings.ContainsAny(c.Literal, set) {
				rewrite(c, opt, wiki)
			}
		case c.Type == Link || c.Type == Image:
			// autolinks show their URL verbatim
			if !c.noExt && c.FirstChild != nil {
				shortcuts(c, opt, true, nodes)
			}
		default:
			if c.FirstChild != nil {
				shortcuts(c, opt, inLink, nodes)
			}
		}
		c = next
	}
}

// rewrite replaces Text node t with its expansion.
func rewrite(t *Node, opt *Options, wiki *nodeBudget) {
	repl := expandText(t.Literal, opt, wiki)
	if repl == nil {
		return
	}
	for _, nd := range repl {
		t.insertBefore(nd)
	}
	t.unlink()
}
