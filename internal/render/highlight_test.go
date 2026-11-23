package render

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DipaolaFederico/kmd/internal/testlimit"
)

var updateGolden = flag.Bool("update", false, "rewrite the golden sections of testdata/highlight/*.txt")

// goldenSep separates a language's sample from its expected highlighting
// in testdata/highlight/<first alias>.txt.
const goldenSep = "\n-- golden --\n"

// markKinds is how each token kind is written in a golden file: «k|func»
// is a keyword, «s|"x"» a string, and so on.
func markKinds(th *Theme) map[Style]byte {
	return map[Style]byte{
		th.tokStyle(kKeyword): 'k', th.tokStyle(kType): 't', th.tokStyle(kFunc): 'f',
		th.tokStyle(kString): 's', th.tokStyle(kNumber): 'n', th.tokStyle(kComment): 'c',
	}
}

// annotate writes highlighted lines in the golden-file notation. Variables
// share the type colour and are written as 't'.
func annotate(lines []rline, th *Theme) string {
	marks := markKinds(th)
	var b strings.Builder
	for i, l := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		for _, r := range l {
			if m, ok := marks[r.st]; ok {
				b.WriteString("«" + string(m) + "|" + r.text + "»")
			} else {
				b.WriteString(r.text)
			}
		}
	}
	return b.String()
}

func goldenPath(d *langDef) string {
	return filepath.Join("testdata", "highlight", strings.Fields(d.aliases)[0]+".txt")
}

// TestHighlightGolden: every language has a sample with keywords, strings,
// comments and numbers, ending with unterminated constructs, and its
// expected highlighting. Run `go test ./internal/render -run Golden -update`
// after adding a language, then review the golden section.
func TestHighlightGolden(t *testing.T) {
	th := &Dark
	for i := range langDefs {
		d := &langDefs[i]
		path := goldenPath(d)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("%s: no golden test: %v", d.name, err)
			continue
		}
		sample, golden, _ := strings.Cut(string(data), goldenSep)
		sample = strings.TrimSuffix(sample, "\n")
		got := annotate(highlightWith(sample, "", langFor(strings.Fields(d.aliases)[0]), th), th)
		if *updateGolden {
			if err := os.WriteFile(path, []byte(sample+goldenSep+got+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if got != strings.TrimSuffix(golden, "\n") {
			t.Errorf("%s (%s): highlighting differs from the golden file; got:\n%s", d.name, path, got)
		}
		// the sample exercises every kind the language has
		need := map[string]bool{"«c|": true, "«n|": true}
		if d.keywords != "" || d.prefix != "" {
			need["«k|"] = true
		}
		if d.quotes != "" || d.long != "" {
			need["«s|"] = true
		}
		for m := range need {
			if !strings.Contains(got, m) {
				t.Errorf("%s: the sample has no %s…» token", d.name, m)
			}
		}
	}
}

// TestHighlightUnterminated: strings and comments left open run to the end
// of the line (one-line strings, line comments) or of the block.
func TestHighlightUnterminated(t *testing.T) {
	th := &Dark
	for i := range langDefs {
		d := &langDefs[i]
		lg := langFor(strings.Fields(d.aliases)[0])
		check := func(src string, k tokKind, what string) {
			t.Helper()
			for n, l := range highlightWith(src, "", lg, th) {
				last := l[len(l)-1]
				if last.st != th.tokStyle(k) || (n > 0 && len(l) != 1) {
					t.Errorf("%s: unterminated %s %q: line %d = %v", d.name, what, src, n, l)
				}
			}
		}
		for _, q := range []byte(d.quotes) {
			check("x "+string(q)+"abc def \\", kString, "string") // not first: Vim's " comments
		}
		if lg.open != "" {
			check(lg.open+" abc\nstill "+lg.open+" open", kComment, "block comment")
		}
		for _, p := range lg.long {
			check(p[0]+" abc\nstill open", kString, "multi-line string")
		}
		for _, p := range lg.line {
			check(p+" "+lg.open+" abc", kComment, "line comment")
		}
	}
}

func TestHighlightNested(t *testing.T) {
	th := &Dark
	for _, tc := range []struct{ lang, src, after string }{
		{"haskell", "{- a {- b -} c -} x", " x"},
		{"ocaml", "(* (* *) still *) let", " "},
		{"julia", "#= #= =# =#\nx = 1", "x = "},
		{"d", "/+ /+ +/ +/ y", " y"},
	} {
		ls := highlight(tc.src, tc.lang, th)
		last := ls[len(ls)-1]
		var plain []string
		for _, r := range last {
			if r.st == (Style{}) {
				plain = append(plain, r.text)
			}
		}
		if len(plain) == 0 || plain[0] != tc.after {
			t.Errorf("%s %q: after the nested comment %v, want plain %q", tc.lang, tc.src, last, tc.after)
		}
	}
	// unbalanced: the comment never closes, however deep
	ls := highlight(strings.Repeat("(* ", 1000)+"*)\nx", "ocaml", th)
	if ls[1][0].st != th.tokStyle(kComment) {
		t.Errorf("unbalanced nested comment closed: %v", ls[1])
	}
}

// TestLangTable keeps the table consistent: names and aliases unique,
// aliases lowercase (fence info strings are lowercased), every language
// listed in the README and the man page.
func TestLangTable(t *testing.T) {
	readme, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	man, err := os.ReadFile("../../docs/kmd.1")
	if err != nil {
		t.Fatal(err)
	}
	names, aliases := map[string]bool{}, map[string]string{}
	for i := range langDefs {
		d := &langDefs[i]
		if d.name == "" || names[d.name] {
			t.Errorf("language %d: empty or duplicate name %q", i, d.name)
		}
		names[d.name] = true
		for _, a := range strings.Fields(d.aliases) {
			if a != strings.ToLower(a) {
				t.Errorf("%s: alias %q is not lowercase", d.name, a)
			}
			if other, dup := aliases[a]; dup {
				t.Errorf("alias %q is used by %s and %s", a, other, d.name)
			}
			aliases[a] = d.name
			if langIndex(a) != i {
				t.Errorf("%s: alias %q resolves to another language", d.name, a)
			}
		}
		if b := strings.Fields(d.block); len(b) != 0 && len(b) != 2 {
			t.Errorf("%s: block comment %q is not \"open close\"", d.name, d.block)
		}
		if len(strings.Fields(d.long))%2 != 0 {
			t.Errorf("%s: multi-line strings %q are not open/close pairs", d.name, d.long)
		}
		// man page: roff escapes '-' and may break the line anywhere
		if !strings.Contains(string(readme), d.name) {
			t.Errorf("%s is missing from the README's language list", d.name)
		}
		if !strings.Contains(strings.ReplaceAll(string(man), `\-`, "-"), d.name) {
			t.Errorf("%s is missing from the man page's language list", d.name)
		}
	}
	if len(langDefs)+1 < 50 { // + diff
		t.Errorf("only %d languages", len(langDefs)+1)
	}
}

// TestHighlightPathological: tokenizing stays linear on hostile input.
func TestHighlightPathological(t *testing.T) {
	const n = 200_000
	inputs := map[string]string{
		"unterminated quotes": strings.Repeat(`"'`+"`", n/3),
		"escapes":             strings.Repeat(`"\`, n/2),
		"comment openers":     strings.Repeat("/*(*{-#|#[#=/+<#<!--", n/20),
		"comment closers":     strings.Repeat("*/*)-}|#]#=#+/#>-->", n/20),
		"long strings":        strings.Repeat(`"""'''''`, n/8),
		"identifiers":         strings.Repeat("a1_b-c ", n/7),
		"sigils and prefixes": strings.Repeat(`$@%\.$`+"`", n/7),
		"keys":                strings.Repeat("k"+strings.Repeat(" ", 50), n/51) + "=",
		"numbers":             strings.Repeat("1.", n/2),
		"indentation":         strings.Repeat(" ", n) + "x\n" + strings.Repeat("\t", n) + "rem x",
		"many lines":          strings.Repeat("x /* y\n", n/7),
	}
	limit := testlimit.Scale() * 2 * time.Second
	for name, in := range inputs {
		start := time.Now()
		for i := range langDefs {
			_ = highlight(in, strings.Fields(langDefs[i].aliases)[0], &Dark)
		}
		_ = highlight(in, "unknownlang", &Dark)
		if d := time.Since(start); d > limit {
			t.Errorf("%s: %v for %d languages", name, d, len(langDefs)+1)
		}
	}
}

func FuzzHighlight(f *testing.F) {
	f.Add("func main() { /* x */ return \"s\" }", uint8(0))
	f.Add("{- {- -} \"\"\" '' $x @y \\cmd % c", uint8(30))
	f.Add("(* (* *)\n*) ''\nrem x", uint8(255))
	f.Fuzz(func(t *testing.T, src string, which uint8) {
		name := "unknownlang"
		if i := int(which) % (len(langDefs) + 1); i < len(langDefs) {
			name = strings.Fields(langDefs[i].aliases)[0]
		}
		lines := highlight(src, name, &Dark)
		in := strings.Split(src, "\n")
		if len(lines) != len(in) {
			t.Fatalf("%d lines, want %d", len(lines), len(in))
		}
		for i, l := range lines {
			var b strings.Builder
			for _, r := range l {
				if r.text == "" {
					t.Fatalf("line %d: empty run in %v", i, l)
				}
				b.WriteString(r.text)
			}
			if b.String() != in[i] {
				t.Fatalf("line %d: runs %q, want %q", i, b.String(), in[i])
			}
		}
	})
}

func BenchmarkHighlight(b *testing.B) {
	var src strings.Builder
	for i := range langDefs {
		data, err := os.ReadFile(goldenPath(&langDefs[i]))
		if err != nil {
			b.Fatal(err)
		}
		sample, _, _ := strings.Cut(string(data), goldenSep)
		src.WriteString(sample)
	}
	code := src.String()
	b.SetBytes(int64(len(code)) * int64(len(langDefs)))
	for b.Loop() {
		for i := range langDefs {
			_ = highlight(code, strings.Fields(langDefs[i].aliases)[0], &Dark)
		}
	}
}
