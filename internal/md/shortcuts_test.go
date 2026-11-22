package md

import (
	"strings"
	"testing"
)

func TestEmojiShortcodes(t *testing.T) {
	long := strings.Repeat("a", 50)
	for in, want := range map[string]string{
		"Ship it :rocket: :+1: :-1: :100:":              "<p>Ship it 🚀 👍 👎 💯</p>\n",
		":white_check_mark: :x: :warning: :bulb:":       "<p>✅ ❌ \u26a0\ufe0f 💡</p>\n",
		":tada::fire::sparkles:":                        "<p>🎉🔥✨</p>\n",
		"a:rocket:b :hash: :t-rex:":                     "<p>a🚀b #\ufe0f\u20e3 🦖</p>\n",
		":nope: :Rocket: : rocket: :rocket 10:30:45 ::": "<p>:nope: :Rocket: : rocket: :rocket 10:30:45 ::</p>\n",
		":nope:rocket:":                                 "<p>:nope🚀</p>\n", // a closing colon may open the next
		":" + long + ":rocket:":                         "<p>:" + long + "🚀</p>\n",
		// never in code, escapes, entities, autolinks, destinations or raw HTML
		"`:rocket:` \\:rocket: :rocket\\: &#58;rocket:": "<p><code>:rocket:</code> :rocket: :rocket: :rocket:</p>\n",
		"<http://x/:rocket:> www.x.com/:tada:x":         "<p><a href=\"http://x/:rocket:\">http://x/:rocket:</a> <a href=\"http://www.x.com/:tada:x\">www.x.com/:tada:x</a></p>\n",
		"[:tada: go](u:rocket:) *:fire:* ~~:x:~~":       "<p><a href=\"u:rocket:\">🎉 go</a> <em>🔥</em> <del>❌</del></p>\n",
		"<b title=\":x:\">:x:</b>":                      "<p><b title=\":x:\">❌</b></p>\n",
		"```\n:rocket:\n```\n\n    :rocket:":            "<pre><code>:rocket:\n</code></pre>\n<pre><code>:rocket:\n</code></pre>\n",
		"# :memo: Notes\n\n| :lock: |\n|---|":           "<h1>📝 Notes</h1>\n<table>\n<thead>\n<tr>\n<th>🔒</th>\n</tr>\n</thead>\n</table>\n",
		"![:x: alt](i.png)":                             "<p><img src=\"i.png\" alt=\"❌ alt\" /></p>\n",
		"$:x:$":                                         "<p><span class=\"math math-inline\">:x:</span></p>\n",
		// an unclosed [[ does not hide the shortcodes after it
		"[[foo :rocket: bar":     "<p>[[foo 🚀 bar</p>\n",
		"[[a :x: ]b [[c|:x:]]":   "<p>[[a ❌ ]b <a href=\"c.md\">❌</a></p>\n",
		"[[" + long + " :x:]] x": "<p>[[" + long + " ❌]] x</p>\n",
	} {
		if got := gfmHTML(in); got != want {
			t.Errorf("input: %q\nwant:  %q\ngot:   %q", in, want, got)
		}
	}
	if got := HTML(Parse(":rocket: [[Page]]", Options{}), HTMLOptions{}); got != "<p>:rocket: [[Page]]</p>\n" {
		t.Errorf("CommonMark mode = %q", got)
	}
	if got := HTML(Parse(":rocket: [[Page]]", Options{Emoji: true}), HTMLOptions{}); got != "<p>🚀 [[Page]]</p>\n" {
		t.Errorf("emoji only = %q", got)
	}
}

// TestEmojiTable checks the generated table: sorted (binary search), unique
// and made of valid shortcodes. The codepoints come from Unicode character
// names (scripts/gen-emoji.py; make scripts-test checks the file is current).
func TestEmojiTable(t *testing.T) {
	if len(emojiTable) < 300 {
		t.Fatalf("only %d shortcodes", len(emojiTable))
	}
	for i, e := range emojiTable {
		if i > 0 && emojiTable[i-1].code >= e.code {
			t.Errorf("not sorted or duplicate at %q", e.code)
		}
		if len(e.code) > maxShortcode || e.emoji == "" {
			t.Errorf("bad entry %q", e.code)
		}
		for j := 0; j < len(e.code); j++ {
			if !isShortcodeChar(e.code[j]) {
				t.Errorf("bad shortcode %q", e.code)
			}
		}
		if got, ok := lookupEmoji(e.code); !ok || got != e.emoji {
			t.Errorf("lookup %q", e.code)
		}
	}
	for _, code := range []string{"", "zzzz", "+0", "rocke", "rockets"} {
		if _, ok := lookupEmoji(code); ok {
			t.Errorf("%q found", code)
		}
	}
}

func TestWikiLinks(t *testing.T) {
	long := strings.Repeat("a", maxWikiLink+1)
	for in, want := range map[string]string{
		"[[Page]]":                        `<p><a href="Page.md">Page</a></p>` + "\n",
		"see [[My Page|the label]].":      `<p>see <a href="My%20Page.md">the label</a>.</p>` + "\n",
		"[[Page#Some Heading]]":           `<p><a href="Page.md#Some%20Heading">Page</a></p>` + "\n",
		"[[ Page # Heading | label ]]":    `<p><a href="Page.md#Heading">label</a></p>` + "\n",
		"[[#Local]] [[Page|]]":            `<p><a href="#Local">Local</a> <a href="Page.md">Page</a></p>` + "\n",
		"[[sub/dir/Note 1.md]] [[a.PNG]]": `<p><a href="sub/dir/Note%201.md">sub/dir/Note 1.md</a> <a href="a.PNG">a.PNG</a></p>` + "\n",
		"[[Release 1.2]] [[../up]]":       `<p><a href="Release%201.2.md">Release 1.2</a> <a href="../up.md">../up</a></p>` + "\n",
		"[[my_page]] [[50%?&]]":           `<p><a href="my_page.md">my_page</a> <a href="50%25%3F&amp;.md">50%?&amp;</a></p>` + "\n",
		"[[Page|:rocket: launch]]":        `<p><a href="Page.md">🚀 launch</a></p>` + "\n",
		"[[[Page]]] [[x]]]":               `<p>[<a href="Page.md">Page</a>] <a href="x.md">x</a>]</p>` + "\n",
		"x [[Page]]y[[Two]]":              `<p>x <a href="Page.md">Page</a>y<a href="Two.md">Two</a></p>` + "\n",
		"[[a&amp;b]]":                     "<p>[[a&amp;b]]</p>\n", // the entity is not literal text
		// not wiki links
		"[[]] [[ ]] [[|x]] [[#]] [[a\nb]]":                                      "<p>[[]] [[ ]] [[|x]] [[#]] [[a\nb]]</p>\n",
		"[[https://evil.com]] [[C:x]] [[/etc/passwd]] [[//host/x]] [[\\\\srv]]": "<p>[[https://evil.com]] [[C:x]] [[/etc/passwd]] [[//host/x]] [[\\srv]]</p>\n",
		"`[[Page]]` \\[[Page]] [[Page\\]]":                                      "<p><code>[[Page]]</code> [[Page]] [[Page]]</p>\n",
		"[[*Page*]] [[<b>x</b>]]":                                               "<p>[[<em>Page</em>]] [[<b>x</b>]]</p>\n",
		"```\n[[Page]]\n```":                                                    "<pre><code>[[Page]]\n</code></pre>\n",
		"[x]: /u\n\n[[x]]":                                                      `<p>[<a href="/u">x</a>]</p>` + "\n",
		"[[x]](/u) [see [[Page]]](/v)":                                          `<p><a href="/u">[x]</a> <a href="/v">see [[Page]]</a></p>` + "\n",
		"![[pic.png]]":                                                          `<p>!<a href="pic.png">pic.png</a></p>` + "\n",
		"[[" + long + "]]":                                                      "<p>[[" + long + "]]</p>\n",
		"[[a\x01b]]":                                                            "<p>[[a\x01b]]</p>\n",
	} {
		if got := gfmHTML(in); got != want {
			t.Errorf("input: %q\nwant:  %q\ngot:   %q", in, want, got)
		}
	}
	// a table cell needs the pipe escaped, as in Obsidian
	if got := gfmHTML("| [[Page\\|label]] |\n|---|"); !strings.Contains(got, `<th><a href="Page.md">label</a></th>`) {
		t.Errorf("wiki link in a table: %q", got)
	}
	// the safe HTML mode keeps them (relative URLs)
	if got := HTML(Parse("[[A B#C]]", GFM), HTMLOptions{}); got != `<p><a href="A%20B.md#C">A B</a></p>`+"\n" {
		t.Errorf("safe mode = %q", got)
	}
	if got := HTML(Parse("[[Page]] :x:", Options{WikiLinks: true}), HTMLOptions{}); got != `<p><a href="Page.md">Page</a> :x:</p>`+"\n" {
		t.Errorf("wiki only = %q", got)
	}
}
