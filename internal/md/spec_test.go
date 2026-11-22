package md

import (
	"encoding/json"
	"os"
	"testing"
)

type specExample struct {
	Markdown  string `json:"markdown"`
	HTML      string `json:"html"`
	Example   int    `json:"example"`
	Section   string `json:"section"`
	Extension string `json:"extension"`
}

func loadSpec(t *testing.T, name string) []specExample {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	var ex []specExample
	if err := json.Unmarshal(data, &ex); err != nil {
		t.Fatal(err)
	}
	return ex
}

// TestCommonMarkSpec runs all 652 examples of the CommonMark 0.31.2 spec.
func TestCommonMarkSpec(t *testing.T) {
	ex := loadSpec(t, "commonmark-0.31.2.json")
	failed := 0
	for _, e := range ex {
		got := HTML(Parse(e.Markdown, Options{}), HTMLOptions{Unsafe: true})
		if got != e.HTML {
			failed++
			t.Errorf("example %d (%s)\ninput: %q\nwant:  %q\ngot:   %q", e.Example, e.Section, e.Markdown, e.HTML, got)
		}
	}
	t.Logf("CommonMark 0.31.2: %d/%d examples pass", len(ex)-failed, len(ex))
}

// TestGFMSpec runs the GitHub Flavored Markdown extension examples.
func TestGFMSpec(t *testing.T) {
	ex := loadSpec(t, "gfm-extensions.json")
	failed := 0
	for _, e := range ex {
		got := HTML(Parse(e.Markdown, GFM), HTMLOptions{TagFilter: true, Unsafe: true})
		if got != e.HTML {
			failed++
			t.Errorf("GFM example %d (%s)\ninput: %q\nwant:  %q\ngot:   %q", e.Example, e.Extension, e.Markdown, e.HTML, got)
		}
	}
	t.Logf("GFM extensions: %d/%d examples pass", len(ex)-failed, len(ex))
}
