package render

import (
	"os"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/DipaolaFederico/kmd/internal/md"
	"github.com/DipaolaFederico/kmd/internal/termimg"
)

var plain = Options{Width: 44}

func render(src string, o Options) string { return RenderString(src, o) }

func mustContain(t *testing.T, out string, subs ...string) {
	t.Helper()
	for _, s := range subs {
		if !strings.Contains(out, s) {
			t.Errorf("output missing %q:\n%s", s, out)
		}
	}
}

func TestPlainLayout(t *testing.T) {
	out := render("# Title\n\nSome *text* with `code` and [link](https://x.y).\n\n## Sub\n\n### H3\n#### H4\n##### H5\n\n---", plain)
	mustContain(t, out, "  TITLE\n  ═════", "`code`", "(https://x.y)", "▌ Sub", "◆ H3", "◇ H4", "· H5", strings.Repeat("─", 40))
	if strings.Contains(out, "\x1b") {
		t.Error("escape sequences in plain output")
	}
	for _, l := range strings.Split(out, "\n") {
		if w := strWidth(l); w > 44 {
			t.Errorf("line too wide (%d): %q", w, l)
		}
	}
}

func TestColorOutput(t *testing.T) {
	o := Options{Width: 60, Color: true, TrueColor: true, Hyperlinks: true}
	out := render("# T\n\n**b** _i_ ~~s~~ `c` [l](https://x.y) ![a](i.png) ![](j.png)", o)
	mustContain(t, out,
		"\x1b[0;1;38;2;26;27;38;48;2;122;162;247m T ", // H1 pill
		"\x1b[0;1;38;2;233;238;254mb",                 // bold
		"\x1b[0;3mi",                                  // italic
		"\x1b[0;9;2ms",                                // strike
		"\u00a0c\u00a0",                               // padded code
		"\x1b]8;;https://x.y\x1b\\",                   // OSC 8 open
		"\x1b]8;;\x1b\\",                              // OSC 8 close
		"▣\u00a0a", "▣\u00a0image",
	)
	if strings.Contains(out, "(https://x.y)") {
		t.Error("URL printed despite hyperlinks")
	}
	o.TrueColor = false
	mustContain(t, render("**b**", o), "\x1b[0;1;38;5;")
	o.Theme = &Light
	mustContain(t, render("# T", o), "48;5;")
}

func TestTo256(t *testing.T) {
	tests := []struct {
		r, g, b uint32
		want    int
	}{{0, 0, 0, 16}, {255, 255, 255, 231}, {128, 128, 128, 244}, {255, 0, 0, 196}, {0, 100, 0, 22}}
	for _, tc := range tests {
		if got := to256(tc.r, tc.g, tc.b); got != tc.want {
			t.Errorf("to256(%d,%d,%d) = %d, want %d", tc.r, tc.g, tc.b, got, tc.want)
		}
	}
}

func TestSafeURL(t *testing.T) {
	if safeURL("https://a\x1b]8;;evil") != "" || safeURL(strings.Repeat("a", 3000)) != "" || safeURL("ok") != "ok" {
		t.Error("safeURL")
	}
}

func TestQuotesAlertsLists(t *testing.T) {
	out := render("> quoted\n>\n> more\n\n> [!NOTE]\n> n\n\n> [!TIP]\n> t\n\n> [!IMPORTANT]\n> i\n\n> [!WARNING]\n> w\n\n> [!CAUTION]\n> c", plain)
	mustContain(t, out, "┃ quoted\n  ┃ \n  ┃ more", "┃ ℹ Note", "┃ ✦ Tip", "❖ Important", "▲ Warning", "✖ Caution")

	out = render("- a\n  - b\n    - c\n      - d\n- [x] done\n- [ ] todo\n-\n\n8. x\n9. y\n10. z\n\n- loose\n\n- items", plain)
	mustContain(t, out, "• a\n    ◦ b\n      ▪ c\n        • d", "☑ done", "☐ todo", "   8. x\n   9. y\n  10. z", "• loose\n\n  • items")
}

func TestCodeBoxes(t *testing.T) {
	out := render("```go\nfunc main() { x := \"s\" // c\n}\n```", Options{Width: 40, Color: true, TrueColor: true})
	mustContain(t, out, "╭─", " go ", "╮", "╰", "╯", "func", "\"s\"")
	// long lines are hard-wrapped inside the box
	out = render("```\n"+strings.Repeat("x", 100)+"\n```", Options{Width: 30})
	for _, l := range nonEmpty(out) {
		if w := strWidth(l); w != 28 {
			t.Errorf("box line width %d: %q", w, l)
		}
	}
	// label wider than the box is dropped
	out = render("```averyveryverylonglanguagename\nx\n```", Options{Width: 20})
	if strings.Contains(out, "averyvery") {
		t.Error("oversized label kept")
	}
	mustContain(t, render("---\na: b\n---\n", plain), " metadata ")
}

func TestTables(t *testing.T) {
	out := render("| L | C | R |\n|:--|:-:|--:|\n| a | b | c |\n| long cell | x | y |", plain)
	mustContain(t, out,
		"╭───────────┬───┬───╮",
		"│ L         │ C │ R │",
		"├───────────┼───┼───┤",
		"│ a         │ b │ c │",
		"╰───────────┴───┴───╯",
	)
	// columns shrink and wrap when the table is too wide
	long := strings.Repeat("word ", 30)
	out = render("| a | b |\n|---|---|\n| "+long+" | "+long+" |", Options{Width: 40})
	for _, l := range nonEmpty(out) {
		if w := strWidth(l); w > 38 {
			t.Errorf("table line too wide (%d): %q", w, l)
		}
	}
	// wide characters keep borders aligned
	out = render("| 日本 | x |\n|--|--|\n| a | 語 |", plain)
	var widths []int
	for _, l := range nonEmpty(out) {
		widths = append(widths, strWidth(l))
	}
	for _, w := range widths {
		if w != widths[0] {
			t.Errorf("misaligned table: %v\n%s", widths, out)
			break
		}
	}
}

func TestHTMLAndMisc(t *testing.T) {
	out := render("<p align=\"center\">\n<b>Hi</b>\n</p>\n\nx<br>y\n\n    code", plain)
	mustContain(t, out, "Hi", "x\n  y", "│ code")
	// tiny widths still render
	_ = render("# heading that is long\n\n| a | b |\n|--|--|\n| c | d |\n\n- item", Options{Width: 1})
	if Render(md.Parse("", md.GFM), Options{Width: 80}).Text != "\n\n" {
		t.Error("empty document")
	}
}

func TestWrap(t *testing.T) {
	lines := wrap([]run{{text: "aaa bbb ", st: Style{}}, {text: "ccc", st: Style{Attr: Bold}}, {text: "\n", st: Style{}}, {text: "dddddddddd", st: Style{}}}, 7)
	var got []string
	for _, l := range lines {
		var b strings.Builder
		for _, r := range l {
			b.WriteString(r.text)
		}
		got = append(got, b.String())
	}
	want := []string{"aaa bbb", "ccc", "ddddddd", "ddd"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("wrap = %q, want %q", got, want)
	}
	// a long word after text fills the line before breaking
	lines = wrap([]run{{text: "ab cdefghij", st: Style{}}}, 5)
	if len(lines) != 3 || lineWidth(lines[1]) != 5 {
		t.Errorf("long word wrap = %v", lines)
	}
	if len(wrap(nil, 0)) != 1 || len(cut(nil, 0)) != 1 {
		t.Error("degenerate wrap")
	}
}

func TestWidth(t *testing.T) {
	tests := []struct {
		s string
		w int
	}{{"abc", 3}, {"日本", 4}, {"é", 1}, {"e\u0301", 1}, {"✅", 2}, {"⚠\ufe0f", 2}, {"a\u200db", 2}, {"\u0085", 0}, {"한", 2}, {"\U0001F600", 2}, {"\U00020000", 2}}
	for _, tc := range tests {
		if got := strWidth(tc.s); got != tc.w {
			t.Errorf("strWidth(%q) = %d, want %d", tc.s, got, tc.w)
		}
	}
}

func TestRuneWidth(t *testing.T) {
	if runeWidth(0x01) != 0 || runeWidth(0x7f) != 0 || runeWidth('a') != 1 {
		t.Error("runeWidth")
	}
	// a wide word after narrow text breaks onto its own line
	lines := wrap([]run{{text: "ab 日本語日本語", st: Style{}}}, 6)
	if len(lines) != 3 {
		t.Errorf("wide wrap = %v", lines)
	}
}

func nonEmpty(out string) []string {
	var ls []string
	for _, l := range strings.Split(out, "\n") {
		if l != "" {
			ls = append(ls, l)
		}
	}
	return ls
}

func TestHighlight(t *testing.T) {
	th := &Dark
	styled := func(code, lang, tok string) Style {
		for _, l := range highlight(code, lang, th) {
			for _, r := range l {
				if r.text == tok {
					return r.st
				}
			}
		}
		t.Fatalf("%s: token %q not found in %q", lang, tok, code)
		return Style{}
	}
	cases := []struct {
		lang, code, tok string
		want            Color
	}{
		{"go", "func main() {}", "func", th.Keyword},
		{"go", "x := Foo{}", "Foo", th.Type},
		{"go", "fmt.Println(1.5)", "Println", th.Func},
		{"go", "fmt.Println(1.5)", "1.5", th.Num},
		{"go", "s := `raw`", "`raw`", th.String},
		{"go", "a /* b */ c", "/* b */", th.Comment},
		{"python", "x = 'it\\'s' # c", "'it\\'s'", th.String},
		{"python", "x = 'a' # c", "# c", th.Comment},
		{"sh", "echo $HOME ${X}", "$HOME", th.Type},
		{"sql", "SELECT * FROM t", "SELECT", th.Keyword},
		{"yaml", "name: kmd", "name", th.Func},
		{"toml", "key = 1", "key", th.Func},
		{"rust", "let x = 1;", "let", th.Keyword},
		{"js", "const a = 1", "const", th.Keyword},
		{"c", "int main", "int", th.Keyword},
		{"ruby", "def x", "def", th.Keyword},
		{"lua", "local x", "local", th.Keyword},
		{"json", "{\"a\": true}", "true", th.Keyword},
		{"css", "a { color: red !important }", "important", th.Keyword},
		{"html", "<!-- c -->", "<!-- c -->", th.Comment},
		{"dockerfile", "FROM alpine", "FROM", th.Keyword},
		{"make", "ifeq ($(X),1)", "ifeq", th.Keyword},
		{"unknownlang", "return x", "return", th.Keyword},
		{"diff", "+ add", "+ add", th.Added},
		{"diff", "- del", "- del", th.Removed},
		{"diff", "@@ -1 +1 @@", "@@ -1 +1 @@", th.Type},
	}
	for _, tc := range cases {
		if got := styled(tc.code, tc.lang, tc.tok); got.Fg != tc.want {
			t.Errorf("%s %q: token %q fg = %x, want %x", tc.lang, tc.code, tc.tok, got.Fg, tc.want)
		}
	}
	// multi-line block comments carry state
	ls := highlight("/* a\nb\nc */ x", "c", th)
	if ls[1][0].st.Fg != th.Comment || ls[2][0].text != "c */" {
		t.Errorf("block comment state: %v", ls)
	}
	if st := styled("--- a\nx", "diff", "--- a"); st.Attr&Bold == 0 {
		t.Error("diff header not bold")
	}
	if l := highlight("plain", "", th); l[0][0].st != (Style{}) {
		t.Error("no-lang code styled")
	}
	_ = highlight("'unterminated", "python", th)
}

func BenchmarkRenderSample(b *testing.B) {
	src, err := os.ReadFile("../../testdata/sample.md")
	if err != nil {
		b.Fatal(err)
	}
	o := Options{Width: 100, Color: true, TrueColor: true, Hyperlinks: true,
		Math: true, Diagrams: true, Images: termimg.Blocks, BaseDir: "../../testdata"}
	b.SetBytes(int64(len(src)))
	for b.Loop() {
		_ = Render(md.Parse(string(src), md.GFM), o)
	}
}

func FuzzRender(f *testing.F) {
	f.Add("# h\n| a | b |\n|--|--|\n| 日本 | x |\n- [x] y\n```go\nfunc(){}\n```", 20)
	f.Fuzz(func(t *testing.T, s string, w int) {
		w = w%200 + 1
		if w < 0 {
			w = -w
		}
		out := RenderString(s, Options{Width: w, Color: true, Hyperlinks: true})
		if strings.Contains(out, "\x07") {
			t.Fatal("BEL leaked")
		}
	})
}

func TestCleanInvalidUTF8(t *testing.T) {
	// a lone 0x9b is an 8-bit CSI for terminals that honour C1 controls
	for _, s := range []string{"name\x9b2J.md", "x \x9d0;t\x07 y", "\xc2"} {
		if c := Clean(s); !utf8.ValidString(c) {
			t.Errorf("Clean(%q) = %q", s, c)
		}
	}
}
