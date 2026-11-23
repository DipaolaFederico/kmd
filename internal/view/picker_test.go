package view

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
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
	if err := syscallMkfifo(filepath.Join(root, "fifo.md")); err != nil {
		t.Fatal(err)
	}
	names, note := walkNames(t, root)
	want := []string{"README.md", "UP.MD", "a.markdown", "bad[31mname.md", "docs/deep/x.md", "docs/guide.md", "g.md"}
	if !reflect.DeepEqual(names, want) || note != "" {
		t.Errorf("walk:\n got %q (%q)\nwant %q", names, note, want)
	}

	// stopping (with the default limits): by the caller, or by emit
	if n := walkTree(root, func() bool { return true }, func([]doc) bool { t.Error("emitted"); return true }); n != "" {
		t.Error(n)
	}
	if n := walkTree(root, func() bool { return false }, func([]doc) bool { return false }); n != "" {
		t.Error(n)
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
	if _, _, ok := fuzzy(ptr(newDoc("", "docs/guide.md", false)), newQuery("gd"), nil); !ok {
		t.Error("subsequence not matched")
	}
	if _, _, ok := fuzzy(ptr(newDoc("", "docs/guide.md", false)), newQuery("dg"), nil); !ok {
		t.Error("d…g")
	}
	if _, _, ok := fuzzy(ptr(newDoc("", "docs/guide.md", false)), newQuery("xz"), nil); ok {
		t.Error("matched letters that are not there")
	}
	if _, pos, ok := fuzzy(ptr(newDoc("", "Ñandú/ÁRBOL.md", false)), newQuery("ñá"), nil); !ok || !reflect.DeepEqual(pos, []int{0, 8}) {
		t.Errorf("unicode case folding: %v %v", pos, ok)
	}
	var docs []doc
	for _, n := range []string{"a/guide-old.md", "guide.md", "go/unused/ide.md", "x/GUIDE.md", "zzz.md"} {
		docs = append(docs, newDoc("", n, false))
	}
	got, _ := filterDocs(docs, newQuery("guide"), nil, nil)
	if !reflect.DeepEqual(got, []int{1, 3, 0, 2}) {
		t.Errorf("ranking: %v", got)
	}
	if got, _ := filterDocs(docs, query{}, nil, nil); len(got) != len(docs) {
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
		note := searchDocs(docs, root, q, func() bool { return false }, func(h []hit, _ int) bool { hs = append(hs, h...); return true })
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
	if n := searchDocs(docs, root, "make", func() bool { return true }, nil); n != "" {
		t.Error(n)
	}
	if n := searchDocs(docs, root, "make", func() bool { return false }, func([]hit, int) bool { return false }); n != "" {
		t.Error(n)
	}
	setVar(t, &maxSearchTotal, 10)
	if _, note := search("zzz"); !strings.Contains(note, "stopped after reading") {
		t.Errorf("total limit: %q", note)
	}
	// a document replaced by something else since the walk is not read
	if readDoc(doc{path: root}, root, 10) != nil || readDoc(doc{path: filepath.Join(root, "gone.md")}, root, 10) != nil {
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

// busy reports whether the picker's background work is pending.
func (m *model) busy() bool {
	return m.pick != nil && (m.pick.walk != nil || m.pick.search != nil)
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
	m, err := newModel(Options{Load: pickerLoader(), Start: root, Root: root,
		Select: "<S>", Match: "<M>", Bar: "<B>"}, 60, 12)
	if err != nil {
		t.Fatal(err)
	}
	h.model = m
	t.Cleanup(m.stopPicker)
	h.settle(t)
	return h
}

// pickerLoader renders documents, and a folder as a one-line page (the
// picker is what shows on it).
func pickerLoader() Loader {
	docs := loader("")
	return func(path string, cols int) (*Page, error) {
		if isDir(path) {
			return &Page{Lines: []string{"listing"}}, nil
		}
		return docs(path, cols)
	}
}

// syntheticDocs is a list of n documents in a deep, regular tree.
func syntheticDocs(n int) []doc {
	docs := make([]doc, n)
	for i := range docs {
		docs[i] = newDoc("", "project-"+strconv.Itoa(i%37)+"/docs/section-"+strconv.Itoa(i%101)+"/topic-"+strconv.Itoa(i)+".md", false)
	}
	return docs
}

func pickerFixture(t *testing.T) string {
	t.Helper()
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

func TestPickerHelpers(t *testing.T) {
	h := &harness{}
	m, _ := newModel(Options{Load: func(string, int) (*Page, error) { return &Page{Lines: []string{""}}, nil },
		Select: "<S>", Match: "<M>"}, 20, 10)
	h.model = m
	long := "a/very/long/path/to/the/final-document.md"
	_, pos, _ := fuzzy(ptr(newDoc("", long, false)), newQuery("fd"), nil)
	s := m.paintName(long, pos, false)
	if !strings.Contains(s, "…") || !strings.Contains(stripEscapes(s), "document.md") {
		t.Errorf("long name: %q", s)
	}
	// a growing list keeps the selection on the same document
	p := &picker{root: "/r"}
	p.add([]doc{{name: "c.md"}, {name: "b.md"}})
	p.cur = 1
	p.add([]doc{{name: "d.md"}, {name: "a.md"}})
	if names := []string{p.docs[0].name, p.docs[1].name, p.docs[2].name, p.docs[3].name}; !reflect.DeepEqual(names, []string{"a.md", "b.md", "c.md", "d.md"}) {
		t.Errorf("merge: %v", names)
	}
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
	docs := syntheticDocs(10000)
	for _, q := range []string{"d", "docsec12"} { // everything matches / a few do
		b.Run(q, func(b *testing.B) {
			var out []int
			var ms []scored
			b.ReportAllocs()
			for b.Loop() {
				out, ms = filterDocs(docs, newQuery(q), out, ms)
			}
		})
	}
}

// TestPickerLinear: filtering and searching grow linearly with the number
// of documents and the length of a document.
func TestPickerLinear(t *testing.T) {
	testlimit.Linear(t, "filter", 1<<16, func(n int) func() {
		docs := syntheticDocs(n)
		q := newQuery("ts") // matches every name: the sort is the whole list
		var out []int
		var ms []scored
		return func() { out, ms = filterDocs(docs, q, out, ms) }
	})
	// heading tracking is timed with a needle that is not there, and "lines"
	// matches once every 100 lines: a hit on every line would time the heap
	// growing, not the scan
	for name, c := range map[string]struct{ line, needle string }{
		"lines":     {strings.Repeat("a plain line\n", 99) + "a needle\n", "needle"},
		"headings":  {"# heading\n", "needle"},
		"setext":    {"title\n===\n", "needle"},
		"fences":    {"```\ncode\n", "needle"},
		"non-ASCII": {"Ñandú grande\n", "ñandú chico"},
	} {
		testlimit.Linear(t, "search "+name, 1<<14, func(n int) func() {
			data := []byte(strings.Repeat(c.line, n))
			hay := lowerASCII(nil, data)
			fold := c.needle != strings.ToLower(c.needle) || !isASCII(c.needle)
			return func() { scanDoc(doc{}, data, hay, []byte(c.needle), fold, 1<<30) }
		})
	}
}

// A search started while the walk is still adding documents must not share
// the list the walk keeps sorting (go test -race).
func TestPickerSearchDuringWalk(t *testing.T) {
	p := &picker{root: "/r", docs: make([]doc, 0, 100)} // room to grow in place
	p.add([]doc{{name: "b.md", path: "/nonexistent/b.md"}, {name: "d.md", path: "/nonexistent/d.md"}})
	m := &model{pick: p, picking: true, bg: make(chan func(*model), 16)}
	p.text, p.tquery = true, []rune("x")
	m.startSearch()
	for i := range 50 {
		p.add([]doc{{name: strconv.Itoa(i) + ".md", path: "/nonexistent/" + strconv.Itoa(i) + ".md"}})
	}
	h := &harness{model: m}
	h.settle(t)
	if p.search != nil || p.total != 2 {
		t.Errorf("search on a copy of the list: running %v, total %d", p.search != nil, p.total)
	}
}

func TestWalkHostileLinks(t *testing.T) {
	outside := tree(t, map[string]string{"x.md": "outside"})
	root := tree(t, map[string]string{"real/a.md": "# A\nneedle\n", "d/keep": ""})
	link := func(target, name string) {
		t.Helper()
		if err := os.Symlink(target, filepath.Join(root, name)); err != nil {
			t.Skip("no symlinks:", err)
		}
	}
	link(outside, "out")
	link("out/x.md", "via.md")                                // through a symlinked folder
	link("real/a.md", "one.md")                               // one hop to a real file: listed
	link("one.md", "two.md")                                  // a chain: not listed
	link(strings.Repeat("d/../", 300)+"real/a.md", "long.md") // too long to check
	link("real/../real/a.md", "dots.md")                      // cleaned inside the root: listed
	// a farm of links whose targets are as long as allowed: cheap to check
	long := strings.Repeat("d/../", maxLinkTarget/5-3) + "zz.md"
	for i := range 300 {
		link(long, "f"+strconv.Itoa(i)+".md")
	}
	start := time.Now()
	names, note := walkNames(t, root)
	if d := time.Since(start); d > testlimit.Scale()*2*time.Second {
		t.Errorf("hostile links took %v", d)
	}
	if want := []string{"dots.md", "one.md", "real/a.md"}; !reflect.DeepEqual(names, want) || note != "" {
		t.Errorf("links: %q %q", names, note)
	}
	setVar(t, &maxPickLinks, 2)
	if names, note := walkNames(t, root); !strings.Contains(note, "first 2 symlinked") || len(names) != 2 {
		t.Errorf("link cap: %q %q", names, note)
	}

	// replaced after the walk: a retargeted link and a file become unreadable
	one := doc{path: filepath.Join(root, "one.md"), link: true}
	if readDoc(one, root, 100) == nil {
		t.Fatal("link not read")
	}
	if err := os.Remove(one.path); err != nil {
		t.Fatal(err)
	}
	link(filepath.Join(outside, "x.md"), "one.md")
	if readDoc(one, root, 100) != nil {
		t.Error("read a retargeted link outside the root")
	}
	a := doc{path: filepath.Join(root, "real/a.md")}
	if err := os.Rename(a.path, a.path+".bak"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "x.md"), a.path); err != nil {
		t.Fatal(err)
	}
	if readDoc(a, root, 100) != nil {
		t.Error("followed a symlink swapped for a document")
	}
	if err := os.Remove(a.path); err != nil {
		t.Fatal(err)
	}
	if err := syscallMkfifo(a.path); err != nil {
		t.Fatal(err)
	}
	done := make(chan []byte, 1)
	go func() { done <- readDoc(a, root, 100) }()
	select {
	case b := <-done:
		if b != nil {
			t.Error("read a FIFO")
		}
	case <-time.After(testlimit.Scale() * 2 * time.Second):
		t.Fatal("blocked on a FIFO swapped for a document")
	}
	// a folder swapped for a symlink is not opened
	if f, err := openDir(filepath.Join(root, "out")); err == nil {
		_ = f.Close()
		t.Error("opened a symlinked folder")
	}
}

func ptr[T any](v T) *T { return &v }

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

func TestWalkLongNames(t *testing.T) {
	root := tree(t, map[string]string{"ok.md": "", strings.Repeat("d", 200) + "/" + strings.Repeat("e", 200) + "/" + strings.Repeat("f", 200) + ".md": ""})
	if names, note := walkNames(t, root); len(names) != 1 || !strings.Contains(note, "longer than 512 bytes") {
		t.Errorf("long path: %q %q", names, note)
	}
}

// A match opens in its own section even when headings repeat, at the
// match chosen among those of the section.
func TestPickerOpensRepeatedSections(t *testing.T) {
	root := tree(t, map[string]string{"CHANGELOG.md": "# Changelog\n\n" +
		"## 2.0\n\n### Fixed\n\n- bug one\n\n" + strings.Repeat("filler\n\n", 20) +
		"## 1.0\n\n### Fixed\n\n- bug two\n- bug three\n\n" + strings.Repeat("filler\n\n", 20)})
	h := pickerHarness(t, root)
	h.keys("tab", "b", "u", "g", "enter")
	h.settle(t)
	if len(h.pick.hits) != 3 || h.pick.hits[2].section != 4 || h.pick.hits[2].nth != 1 {
		t.Fatalf("hits: %+v", h.pick.hits)
	}
	h.keys("end", "enter") // "bug three", in the second "Fixed"
	if h.picking || h.cur < 0 {
		t.Fatalf("not opened: %q", h.msg)
	}
	got := stripEscapes(h.page.Lines[h.matches[h.cur].line])
	if !strings.Contains(got, "bug three") {
		t.Errorf("opened at %q", got)
	}
}
