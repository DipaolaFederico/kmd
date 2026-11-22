package render_test

import (
	"bytes"
	"flag"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

var updateAPI = flag.Bool("update-api", false, "rewrite testdata/api.txt with the current exported API")

// TestAPI guards the public API: testdata/api.txt lists every exported
// declaration. A removed or changed line is a breaking change, which after
// v1.0.0 needs a new major version; added lines are accepted by
// re-running with -update-api.
func TestAPI(t *testing.T) {
	got := exportedAPI(t, ".")
	path := filepath.Join("testdata", "api.txt")
	if *updateAPI {
		if err := os.WriteFile(path, []byte(strings.Join(got, "\n\n")+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n\n")
	for _, w := range want {
		if !slices.Contains(got, w) {
			t.Errorf("BREAKING: removed or changed exported API:\n%s", w)
		}
	}
	for _, g := range got {
		if !slices.Contains(want, g) {
			t.Errorf("new exported API (accept with go test ./render -run TestAPI -update-api):\n%s", g)
		}
	}
}

// exportedAPI returns the exported declarations of the package in dir, one
// entry per function, method, type, constant or variable, without comments
// and without unexported struct fields, sorted.
func exportedAPI(t *testing.T, dir string) []string {
	t.Helper()
	fset := token.NewFileSet()
	matches, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	var files []*ast.File
	for _, name := range matches {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, f)
	}
	// Type-check for the values of constants (iota). They do not depend on
	// imports, so imports are stubbed and the resulting errors ignored:
	// type-checking the dependencies from source would take seconds.
	conf := types.Config{Importer: stubImporter{}, Error: func(error) {}}
	pkg, _ := conf.Check("render", fset, files, nil)
	var decls []string
	for _, f := range files {
		ast.FileExports(f)
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				d.Doc, d.Body = nil, nil
				decls = append(decls, print(t, fset, d))
			case *ast.GenDecl:
				for _, s := range d.Specs {
					one := &ast.GenDecl{Tok: d.Tok, Specs: []ast.Spec{s}}
					if vs, ok := s.(*ast.ValueSpec); ok && d.Tok == token.CONST {
						for _, n := range vs.Names { // the value is part of the API
							c := pkg.Scope().Lookup(n.Name).(*types.Const)
							decls = append(decls, "const "+n.Name+" "+types.TypeString(c.Type(), types.RelativeTo(pkg))+" = "+c.Val().ExactString())
						}
						continue
					}
					if vs, ok := s.(*ast.ValueSpec); ok {
						vs.Doc, vs.Comment = nil, nil
					}
					if ts, ok := s.(*ast.TypeSpec); ok {
						ts.Doc, ts.Comment = nil, nil
						if st, ok := ts.Type.(*ast.StructType); ok {
							for _, f := range st.Fields.List {
								f.Doc, f.Comment = nil, nil
							}
						}
					}
					decls = append(decls, print(t, fset, one))
				}
			}
		}
	}
	slices.Sort(decls)
	return decls
}

func print(t *testing.T, fset *token.FileSet, n ast.Node) string {
	t.Helper()
	var b bytes.Buffer
	if err := (&printer.Config{Mode: printer.UseSpaces | printer.TabIndent, Tabwidth: 8}).Fprint(&b, fset, n); err != nil {
		t.Fatal(err)
	}
	// Drop the blank lines left by comments: entries are separated by them.
	var lines []string
	for _, l := range strings.Split(b.String(), "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	return strings.Join(lines, "\n")
}

// stubImporter returns an empty package for every import.
type stubImporter struct{}

func (stubImporter) Import(path string) (*types.Package, error) {
	p := types.NewPackage(path, filepath.Base(path))
	p.MarkComplete()
	return p, nil
}
