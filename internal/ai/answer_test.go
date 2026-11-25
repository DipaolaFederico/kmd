//go:build kmdai

package ai

import (
	"context"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DipaolaFederico/kmd/internal/render"
	"github.com/DipaolaFederico/kmd/internal/termimg"
)

// hostileAnswer is what an injected model might write: escapes in text, a
// code block, a link and a heading, unsafe URLs, and a local image.
const hostileAnswer = "\x1b]52;c;cm0gLXJmIH4=\x07 PWNED \x1b[2J\x1b[31mred\u009b0m\x07\x1bP+q\x1b\\\n\n" +
	"# \x1b]0;title\x07Heading\n\n" +
	"```sh\nrm -rf ~ \x1b]8;;https://evil\x1b\\x\x1b]8;;\x1b\\\n```\n\n" +
	"[click](javascript:alert(1)) [ok](https://example.com/\x1b[0m) <file:///etc/passwd>\n\n" +
	"![secret](pic.png) ![abs](/etc/passwd)\n"

// checkTerminalSafe fails unless every escape in s is one kmd itself
// writes: SGR colours, and OSC 8 hyperlinks whose targets follow the rules
// for a document's (no control or invisible characters).
func checkTerminalSafe(t *testing.T, s string) {
	t.Helper()
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '\x1b' && strings.HasPrefix(s[i:], "\x1b["):
			j := i + 2
			for j < len(s) && (s[j] >= '0' && s[j] <= '9' || s[j] == ';') {
				j++
			}
			if j == len(s) || s[j] != 'm' {
				t.Fatalf("CSI other than SGR at %d: %q", i, s[i:min(len(s), i+20)])
			}
			i = j
		case c == '\x1b' && strings.HasPrefix(s[i:], "\x1b]8;;"):
			end := strings.Index(s[i:], "\x1b\\")
			if end < 0 {
				t.Fatalf("unterminated OSC 8 at %d", i)
			}
			if u := s[i+5 : i+end]; u != "" && !render.SafeURL(u) {
				t.Fatalf("unsafe hyperlink target %q", u)
			}
			i += end + 1
		case c < 0x20 && c != '\n':
			t.Fatalf("control byte %#x at %d: %q", c, i, s[max(0, i-20):min(len(s), i+20)])
		}
	}
	for _, r := range s {
		if r >= 0x80 && r < 0xa0 {
			t.Fatalf("C1 control %U", r)
		}
	}
}

func writePNG(t *testing.T, dir string) {
	t.Helper()
	f, err := os.Create(filepath.Join(dir, "pic.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, image.NewGray(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
}

// The model's output, streamed in pieces that split every escape, never
// gets an escape of its own to the terminal, and makes kmd read no image.
func TestHostileAnswer(t *testing.T) {
	var events []string
	runes := []rune(hostileAnswer)
	for i := 0; i < len(runes); i += 3 {
		events = append(events, contentEvent(string(runes[i:min(i+3, len(runes))])))
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sse(w, append(events, `[DONE]`)...)
	}))
	defer srv.Close()
	answer, _, err := collect(context.Background(), newClient(t, srv.URL), Request{})
	if err != nil || answer != hostileAnswer {
		t.Fatalf("Chat = %q, %v", answer, err)
	}
	dir := t.TempDir()
	writePNG(t, dir)
	for _, o := range []render.Options{
		{Width: 60},
		{Width: 60, Color: true, TrueColor: true, Hyperlinks: true, Images: termimg.Kitty, BaseDir: dir, AnyImagePath: true, Navigate: true, Placements: true},
		{Width: 60, Color: true, Hyperlinks: true, Images: termimg.Blocks, BaseDir: dir, Math: true, Diagrams: true},
	} {
		res := RenderAnswer(answer, o)
		checkTerminalSafe(t, res.Text)
		if res.Graphics || len(res.Images) > 0 || strings.Contains(res.Text, "▀") {
			t.Errorf("an image was drawn with %+v:\n%s", o, res.Text)
		}
		for _, want := range []string{"pwned", "heading", "rm -rf", "click", "secret"} {
			if !strings.Contains(strings.ToLower(res.Text), want) {
				t.Errorf("%q missing from the answer:\n%s", want, res.Text)
			}
		}
	}
}

func TestUnfence(t *testing.T) {
	for in, want := range map[string]string{
		"```markdown\n# Title\n\n- a\n```\n": "# Title\n\n- a",
		"  ```md\nText\n```":                 "Text",
		"```markdown\nA\n```\n\nB":           "```markdown\nA\n```\n\nB",
		"```markdown\nA\n```\n```\nB\n```":   "```markdown\nA\n```\n```\nB\n```",
		"```sh\nls\n```":                     "```sh\nls\n```",
		"Plain":                              "Plain",
	} {
		if got := unfence(in); got != want {
			t.Errorf("unfence(%q) = %q, want %q", in, got, want)
		}
	}
	res := RenderAnswer("```markdown\n# Title\n```", render.Options{Width: 40})
	if strings.ContainsAny(res.Text, "#`") || !strings.Contains(res.Text, "TITLE") {
		t.Errorf("fenced answer not rendered as Markdown:\n%s", res.Text)
	}
}
