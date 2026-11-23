package view

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/DipaolaFederico/kmd/internal/testlimit"
)

// tree writes files (name → content) under a new directory.
func tree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func walkNames(t *testing.T, root string) ([]string, string) {
	t.Helper()
	var names []string
	note := walkTree(root, func() bool { return false }, func(b []doc) bool {
		for _, d := range b {
			names = append(names, d.name)
		}
		return true
	})
	sort.Strings(names)
	return names, note
}

// setVar lowers a limit for one test.
func setVar(t *testing.T, p *int, v int) {
	old := *p
	*p = v
	t.Cleanup(func() { *p = old })
}

func TestWalkTree(t *testing.T) {
	outside := tree(t, map[string]string{"secret.md": "outside", "id_rsa": "key"})
	root := tree(t, map[string]string{
		"README.md": "", "a.markdown": "", "notes.txt": "", "UP.MD": "",
		"docs/guide.md": "", "docs/deep/x.md": "",
		".git/HEAD.md": "", ".hidden.md": "", "node_modules/p/readme.md": "", "vendor/v.md": "",
		"bad\x1b[31m\u202ename.md": "",
	})
	link := func(target, name string) {
		if err := os.Symlink(target, filepath.Join(root, name)); err != nil {
			t.Skip("no symlinks:", err)
		}
	}
	link(root, "docs/loop")                            // a loop: never entered
	link(outside, "out")                               // a folder outside: never entered
	link(filepath.Join(outside, "secret.md"), "s.md")  // a document outside: not listed
	link(filepath.Join(outside, "id_rsa"), "key.md")   // not even Markdown
	link(filepath.Join(root, "docs/guide.md"), "g.md") // inside: listed
	link(filepath.Join(root, "notes.txt"), "notes.md") // extension changes: not listed
	link(filepath.Join(root, "missing.md"), "dead.md") // dangling
	if err := syscall.Mkfifo(filepath.Join(root, "fifo.md"), 0o600); err != nil {
		t.Fatal(err)
	}
	names, note := walkNames(t, root)
	want := []string{"README.md", "UP.MD", "a.markdown", "bad[31mname.md", "docs/deep/x.md", "docs/guide.md", "g.md"}
	if !reflect.DeepEqual(names, want) || note != "" {
		t.Errorf("walk:\n got %q (%q)\nwant %q", names, note, want)
	}

	setVar(t, &maxPickDepth, 1)
	if names, note := walkNames(t, root); len(names) != 6 || !strings.Contains(note, "deeper than 1") {
		t.Errorf("depth limit: %q %q", names, note)
	}
	setVar(t, &maxPickFiles, 3)
	if names, note := walkNames(t, root); len(names) != 3 || !strings.Contains(note, "first 3 documents") {
		t.Errorf("file limit: %q %q", names, note)
	}
	setVar(t, &maxPickFiles, 100)
	setVar(t, &maxPickEntries, 4)
	if names, note := walkNames(t, root); len(names) > 4 || !strings.Contains(note, "4 entries") {
		t.Errorf("entry limit: %q %q", names, note)
	}
	if _, note := walkNames(t, filepath.Join(root, "nope")); note == "" {
		t.Error("missing root not reported")
	}
	// stopping: by the caller, or by emit
	if n := walkTree(root, func() bool { return true }, func([]doc) bool { t.Error("emitted"); return true }); n != "" {
		t.Error(n)
	}
	if n := walkTree(root, func() bool { return false }, func([]doc) bool { return false }); n != "" {
		t.Error(n)
	}
}

func TestWalkStopsInBigFolders(t *testing.T) {
	files := map[string]string{}
	for i := range 600 {
		files["d/f"+strconv.Itoa(i)+".md"] = ""
	}
	root := tree(t, files)
	batches := 0
	walkTree(root, func() bool { return false }, func(b []doc) bool { batches++; return false })
	if batches != 1 {
		t.Errorf("%d batches after emit refused", batches)
	}
}

func TestFuzzy(t *testing.T) {
	if _, _, ok := fuzzy("docs/guide.md", newQuery("gd"), nil); !ok {
		t.Error("subsequence not matched")
	}
	if _, _, ok := fuzzy("docs/guide.md", newQuery("dg"), nil); !ok {
		t.Error("d…g")
	}
	if _, _, ok := fuzzy("docs/guide.md", newQuery("xz"), nil); ok {
		t.Error("matched letters that are not there")
	}
	if _, pos, ok := fuzzy("Ñandú/ÁRBOL.md", newQuery("ñá"), nil); !ok || !reflect.DeepEqual(pos, []int{0, 8}) {
		t.Errorf("unicode case folding: %v %v", pos, ok)
	}
	docs := []doc{{name: "a/guide-old.md"}, {name: "guide.md"}, {name: "go/unused/ide.md"}, {name: "x/GUIDE.md"}, {name: "zzz.md"}}
	got := filterDocs(docs, newQuery("guide"), nil)
	if !reflect.DeepEqual(got, []int{1, 3, 0, 2}) {
		t.Errorf("ranking: %v", got)
	}
	if got := filterDocs(docs, query{}, nil); len(got) != len(docs) {
		t.Errorf("empty query: %v", got)
	}
}

func TestSearchDocs(t *testing.T) {
	root := tree(t, map[string]string{
		"a.md": "# Install ##\n\nRun make.\n\n```sh\n# not a heading\nmake install\n```\n\nSetext\n======\n\nmake again\n" +
			"    # indented code\nmake it\nOther\n---\nlast Make\n",
		"b.md": "Ñandú grande\n#nope make\n####### seven make\n",
		"c.md": "nothing here",
	})
	names, _ := walkNames(t, root)
	var docs []doc
	for _, n := range names {
		docs = append(docs, doc{path: filepath.Join(root, n), name: n})
	}
	search := func(q string) ([]hit, string) {
		var hs []hit
		note := searchDocs(docs, q, func() bool { return false }, func(h []hit, _ int) bool { hs = append(hs, h...); return true })
		return hs, note
	}
	hs, note := search("make")
	type got struct {
		name    string
		line    int
		heading string
	}
	var g []got
	for _, h := range hs {
		g = append(g, got{h.doc.name, h.line, h.heading})
	}
	want := []got{{"a.md", 3, "Install"}, {"a.md", 7, "Install"}, {"a.md", 13, "Setext"}, {"a.md", 15, "Setext"},
		{"a.md", 18, "Other"}, {"b.md", 2, ""}, {"b.md", 3, ""}}
	if !reflect.DeepEqual(g, want) || note != "" {
		t.Errorf("hits:\n got %v %q\nwant %v", g, note, want)
	}
	if hs, _ := search("Make"); len(hs) != 1 || hs[0].text != "last Make" {
		t.Errorf("smart case: %+v", hs)
	}
	if hs, _ := search("ñandú"); len(hs) != 1 || hs[0].line != 1 {
		t.Errorf("non-ASCII fold: %+v", hs)
	}

	setVar(t, &maxFileHits, 2)
	if hs, _ := search("make"); len(hs) != 4 {
		t.Errorf("per-document limit: %d", len(hs))
	}
	setVar(t, &maxHits, 3)
	if hs, note := search("make"); len(hs) != 3 || !strings.Contains(note, "first 3 matches") {
		t.Errorf("match limit: %d %q", len(hs), note)
	}
	setVar(t, &maxHits, 100)
	setVar(t, &maxSearchFile, 4) // "# In": no match
	if hs, _ := search("make"); len(hs) != 0 {
		t.Errorf("file read limit: %+v", hs)
	}
	setVar(t, &maxSearchFile, 1<<20)
	if n := searchDocs(docs, "make", func() bool { return true }, nil); n != "" {
		t.Error(n)
	}
	if n := searchDocs(docs, "make", func() bool { return false }, func([]hit, int) bool { return false }); n != "" {
		t.Error(n)
	}
	setVar(t, &maxSearchTotal, 10)
	if _, note := search("zzz"); !strings.Contains(note, "stopped after reading") {
		t.Errorf("total limit: %q", note)
	}
	// a document replaced by something else since the walk is not read
	if readDoc(root, 10) != nil || readDoc(filepath.Join(root, "gone.md"), 10) != nil {
		t.Error("read a folder or a missing file")
	}
	long := strings.Repeat("é", maxHitText)
	if c := clipBytes([]byte(long)); len(c) > maxHitText || !strings.HasPrefix(long, c) {
		t.Errorf("clip: %d bytes", len(c))
	}
	if h, ok := atxHeading([]byte("## A # B ##")); !ok || h != "A # B" {
		t.Errorf("closing sequence: %q", h)
	}
	if h, _ := atxHeading([]byte("# C#")); h != "C#" {
		t.Errorf("no closing sequence: %q", h)
	}
}

// settle applies background results until no work is pending.
func (h *harness) settle(t *testing.T) {
	t.Helper()
	deadline := time.After(testlimit.Scale() * 5 * time.Second)
	for h.busy() {
		select {
		case f := <-h.bg:
			f(h.model)
		case <-deadline:
			t.Fatal("background work did not finish")
		}
	}
}

func pickerHarness(t *testing.T, root string) *harness {
	t.Helper()
	h := &harness{}
	docs := loader("")
	load := func(path string, cols int) (*Page, error) {
		if isDir(path) {
			return &Page{Lines: []string{"listing"}}, nil
		}
		return docs(path, cols)
	}
	m, err := newModel(Options{Load: load, Start: root, Root: root,
		Select: "<S>", Match: "<M>", Bar: "<B>"}, 60, 12)
	if err != nil {
		t.Fatal(err)
	}
	h.model = m
	t.Cleanup(m.stopPicker)
	h.settle(t)
	return h
}

func pickerFixture(t *testing.T) string {
	files := map[string]string{
		"guide/install.md": "# Install\n\nIntro.\n\n" + strings.Repeat("filler\n\n", 30) + "## Linux\n\nrun the needle command\n",
		"guide/usage.md":   "# Usage\n\nno match\n",
		"notes.md":         "# Notes\n\nanother needle\n",
	}
	for i := range 20 {
		files["more/m"+strconv.Itoa(i)+".md"] = "# M\n"
	}
	return tree(t, files)
}

func TestPickerFilterAndOpen(t *testing.T) {
	root := pickerFixture(t)
	h := pickerHarness(t, root)
	if !h.picking || len(h.pick.docs) != 23 || h.pick.walk != nil {
		t.Fatalf("picker: %v %d", h.picking, len(h.pick.docs))
	}
	s := stripEscapes(h.screen())
	if !strings.Contains(s, "23 documents") || !strings.Contains(s, "guide/install.md") || !strings.Contains(s, "Find: ") {
		t.Errorf("screen: %q", s)
	}
	h.keys("i", "n", "s", "t")
	if got := h.pick.docs[h.pick.shown[0]].name; got != "guide/install.md" || len(h.pick.shown) != 1 {
		t.Fatalf("filter: %q %d", got, len(h.pick.shown))
	}
	if s := h.screen(); !strings.Contains(s, "1/23 documents") || !strings.Contains(s, "Find: inst") ||
		!strings.Contains(s, "\x1b[4mi") {
		t.Errorf("filtered screen: %q", s)
	}
	h.keys("backspace", "backspace", "backspace", "backspace", "x", "q", "z")
	if !strings.Contains(h.screen(), "no document matches") {
		t.Error("no match message")
	}
	h.keys("ctrl-u", "down", "down", "up", "end")
	if h.pick.cur != 22 {
		t.Errorf("end: %d", h.pick.cur)
	}
	h.keys("home", "pgdn")
	if h.pick.cur != h.listH()-1 {
		t.Errorf("pgdn: %d", h.pick.cur)
	}
	h.keys("pgup", "u", "s", "a", "enter")
	if h.picking || !strings.HasSuffix(h.path, "usage.md") {
		t.Fatalf("enter did not open: %q %q", h.path, h.msg)
	}
	h.keys("backspace") // back to the list, as it was
	if !h.picking || string(h.pick.query) != "usa" {
		t.Fatalf("back: picking %v query %q", h.picking, string(h.pick.query))
	}
	h.keys("esc") // clears the query, then closes
	if !h.picking || len(h.pick.query) != 0 {
		t.Error("esc did not clear the query")
	}
	h.keys("esc")
	if h.picking {
		t.Error("esc did not close")
	}
	h.keys("o")
	if !h.picking {
		t.Error("o did not reopen")
	}
	h.handle(key{name: "mouse", btn: 65})
	h.handle(key{name: "mouse", btn: 64})
	h.handle(key{name: "mouse", btn: 0, press: true, y: 1}) // first row
	if h.picking || !strings.HasSuffix(h.path, "install.md") {
		t.Errorf("click: %q", h.path)
	}
	h.keys("o") // over a document: Backspace with an empty query closes
	h.keys("backspace")
	if h.picking || !strings.HasSuffix(h.path, "install.md") {
		t.Error("backspace over a document")
	}
}

func TestPickerTextSearch(t *testing.T) {
	root := pickerFixture(t)
	h := pickerHarness(t, root)
	h.keys("tab")
	if !strings.Contains(h.screen(), "type text and press Enter") {
		t.Error("text mode prompt")
	}
	h.keys("enter") // nothing to search
	for _, r := range "needle" {
		h.handle(key{name: "char", r: r})
	}
	h.keys("enter")
	if h.pick.search == nil {
		t.Fatal("search did not start")
	}
	if s := stripEscapes(h.screen()); !strings.Contains(s, "searching") {
		t.Errorf("progress not shown: %q", s)
	}
	h.settle(t)
	if len(h.pick.hits) != 2 {
		t.Fatalf("hits: %+v", h.pick.hits)
	}
	s := stripEscapes(h.screen())
	if !strings.Contains(s, "2 matches") || !strings.Contains(s, "guide/install.md:") || !strings.Contains(s, "Linux › run the needle") {
		t.Errorf("results: %q", s)
	}
	h.keys("enter") // opens at the section, the match highlighted
	if h.picking || !strings.HasSuffix(h.path, "install.md") || h.query != "needle" || len(h.matches) != 1 || h.top == 0 {
		t.Fatalf("open hit: %q picking %v query %q top %d", h.path, h.picking, h.query, h.top)
	}
	h.keys("S") // back in the results, as they were
	if !h.picking || !h.pick.text || len(h.pick.hits) != 2 {
		t.Error("S did not show the results")
	}
	h.keys("tab", "tab")
	h.keys("enter") // same query: opens the selection again
	if h.picking {
		t.Error("second open")
	}

	h.keys("S", "backspace", "enter") // a new query starts a new search
	if h.pick.search == nil {
		t.Fatal("no new search")
	}
	h.keys("esc") // cancels it
	if h.pick.search != nil || !strings.Contains(h.screen(), "search cancelled") {
		t.Error("esc did not cancel")
	}
	h.settle(t)
	for _, r := range "zzz" {
		h.handle(key{name: "char", r: r})
	}
	h.keys("enter")
	h.settle(t)
	if !strings.Contains(h.screen(), "no matches") {
		t.Error("no matches")
	}
	h.keys("ctrl-c")
	if h.picking {
		t.Error("ctrl-c did not close")
	}
}

func TestPickerElsewhere(t *testing.T) {
	h := newHarness(t, fixture(t))
	h.keys("o")
	if h.msg != "no folder to list" || h.picking {
		t.Errorf("no root: %q", h.msg)
	}
	dir := filepath.Dir(h.path)
	h.opt.Root = dir
	h.keys("o")
	h.settle(t)
	t.Cleanup(h.stopPicker)
	if !h.picking || h.pick.root != dir || len(h.pick.docs) != 4 {
		t.Fatalf("picker of the document's folder: %+v", h.pick)
	}
	h.keys("?") // typed, not help
	if h.help || string(h.pick.query) != "?" {
		t.Error("? in the picker")
	}
	// a document that disappeared stays in the list with a message
	h.keys("ctrl-u")
	if err := os.Remove(filepath.Join(dir, "one.md")); err != nil {
		t.Fatal(err)
	}
	for i, d := range h.pick.shown {
		if h.pick.docs[d].name == "one.md" {
			h.pick.cur = i
		}
	}
	h.keys("enter")
	if !h.picking || !strings.Contains(h.msg, "not found") || !strings.Contains(stripEscapes(h.screen()), "not found") {
		t.Errorf("missing document: %q", h.msg)
	}
	h.slides = []*Page{h.page}
	h.picking = false
	h.keys("o")
	if h.picking || !strings.Contains(h.msg, "presentation") {
		t.Error("picker during a presentation")
	}
}

func TestPickerNameFitsAndGrows(t *testing.T) {
	h := &harness{}
	m, _ := newModel(Options{Load: func(string, int) (*Page, error) { return &Page{Lines: []string{""}}, nil },
		Select: "<S>", Match: "<M>"}, 20, 10)
	h.model = m
	long := "a/very/long/path/to/the/final-document.md"
	_, pos, _ := fuzzy(long, newQuery("fd"), nil)
	s := m.paintName(long, pos, false)
	if !strings.Contains(s, "…") || !strings.Contains(stripEscapes(s), "document.md") {
		t.Errorf("long name: %q", s)
	}
	// a growing list keeps the selection on the same document
	p := &picker{root: "/r"}
	p.add([]doc{{name: "b.md"}, {name: "c.md"}})
	p.cur = 1
	p.add([]doc{{name: "a.md"}})
	if p.docs[p.shown[p.cur]].name != "c.md" {
		t.Errorf("selection moved: %d", p.cur)
	}
	if plural(1, "x", "xs") != "1 x" || !strings.HasSuffix(rootName("/"), "/") {
		t.Error("helpers")
	}
	var nilTask *task
	nilTask.cancel()
	tk := newTask()
	tk.cancel()
	tk.cancel()
	if tk.send(make(chan func(*model)), nil) {
		t.Error("send after cancel")
	}
}

// BenchmarkPickerFilter filters 10,000 paths, as for each keystroke.
func BenchmarkPickerFilter(b *testing.B) {
	docs := make([]doc, 10000)
	for i := range docs {
		docs[i] = doc{name: "project-" + strconv.Itoa(i%37) + "/docs/section-" + strconv.Itoa(i%101) + "/topic-" + strconv.Itoa(i) + ".md"}
	}
	for _, q := range []string{"d", "docsec12"} { // everything matches / a few do
		b.Run(q, func(b *testing.B) {
			var out []int
			b.ReportAllocs()
			for b.Loop() {
				out = filterDocs(docs, newQuery(q), out)
			}
		})
	}
}

// TestPickerFilterFast backs the per-keystroke budget (16 ms for 10,000
// files) generously, scaled for slow builders.
func TestPickerFilterFast(t *testing.T) {
	docs := make([]doc, 10000)
	for i := range docs {
		docs[i] = doc{name: "project-" + strconv.Itoa(i%37) + "/docs/section-" + strconv.Itoa(i%101) + "/topic-" + strconv.Itoa(i) + ".md"}
	}
	start := time.Now()
	for _, q := range []string{"d", "do", "doc", "docs", "docsec", "docsec12", "zz"} {
		filterDocs(docs, newQuery(q), nil)
	}
	if d := time.Since(start) / 7; d > testlimit.Scale()*16*time.Millisecond {
		t.Errorf("filtering 10,000 files took %v per keystroke", d)
	}
}

// TestPickerLinear: filtering and searching grow linearly with the number
// of documents and the length of a document.
func TestPickerLinear(t *testing.T) {
	testlimit.Linear(t, "filter", 1<<16, func(n int) func() {
		docs := make([]doc, n)
		for i := range docs {
			docs[i] = doc{name: "dir-" + strconv.Itoa(i%97) + "/sub/file-" + strconv.Itoa(i) + ".md"}
		}
		q := newQuery("ds") // matches every name: the sort is the whole list
		var out []int
		return func() { out = filterDocs(docs, q, out) }
	})
	for name, line := range map[string]string{
		"lines":    "a needle in a line\n",
		"headings": "# needle\n",
		"setext":   "needle\n===\n",
		"fences":   "```\nneedle\n",
		"no match": "nothing to see here\n",
	} {
		testlimit.Linear(t, "search "+name, 1<<16, func(n int) func() {
			data := []byte(strings.Repeat(line, n))
			hay := lowerASCII(nil, data)
			return func() { scanDoc(doc{}, data, hay, []byte("needle"), false, 1<<30) }
		})
	}
}
