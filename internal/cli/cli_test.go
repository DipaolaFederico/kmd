package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DipaolaFederico/kmd/internal/view"
)

type result struct {
	code        int
	out, errOut string
	paged       string
}

// safeBuffer is a bytes.Buffer usable from the follow goroutine in tests.
type safeBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *safeBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *safeBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func envMap(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func run(t *testing.T, mod func(*Env), args ...string) result {
	t.Helper()
	var out, errOut bytes.Buffer
	var res result
	home := t.TempDir()
	env := Env{
		Args:   args,
		Stdin:  strings.NewReader("# from stdin"),
		Stdout: &out,
		Stderr: &errOut,
		Getenv: envMap(map[string]string{"HOME": home}),
		Pager: func(c, _ string) error {
			res.paged = c
			return nil
		},
		// never open the real terminal from tests
		View: func(view.Options) error { return errors.New("no viewer in tests") },
	}
	if mod != nil {
		mod(&env)
	}
	res.code = Run(env)
	res.out, res.errOut = out.String(), errOut.String()
	return res
}

func writeFile(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRenderFile(t *testing.T) {
	p := writeFile(t, "a.md", "# Hello\n\n**world** $x^2$")
	r := run(t, nil, p)
	if r.code != 0 || !strings.Contains(r.out, "HELLO") || strings.Contains(r.out, "\x1b") || !strings.Contains(r.out, "x²") {
		t.Fatalf("code=%d out=%q err=%q", r.code, r.out, r.errOut)
	}
	r = run(t, nil, p, "--color", "always", "-w", "40", "-t", "dracula", "--no-math")
	if r.code != 0 || !strings.Contains(r.out, "\x1b[") || !strings.Contains(r.out, "$x^2$") {
		t.Fatalf("flags ignored: %q", r.out)
	}
	if strings.Contains(r.out, "\x1b]8;;") {
		t.Error("hyperlinks emitted to a non-terminal")
	}
	r = run(t, nil, p, "--hyperlinks", "always", "-c", "always")
	if !strings.Contains(r.out, "Hello") {
		t.Error("hyperlinks always")
	}
	r = run(t, nil, "--commonmark", writeFile(t, "c.md", "~~x~~ www.x.com"))
	if !strings.Contains(r.out, "~~x~~") {
		t.Errorf("--commonmark kept extensions: %q", r.out)
	}
}

func TestStdinHelpVersion(t *testing.T) {
	if r := run(t, nil); r.code != 0 || !strings.Contains(r.out, "FROM STDIN") {
		t.Fatalf("stdin: %+v", r)
	}
	if r := run(t, nil, "-"); !strings.Contains(r.out, "FROM STDIN") {
		t.Fatalf("dash: %+v", r)
	}
	if r := run(t, func(e *Env) { e.StdinTTY = true }); r.code != 2 || !strings.Contains(r.errOut, "Usage:") {
		t.Fatalf("no input: %+v", r)
	}
	if r := run(t, nil, "--help"); r.code != 0 || !strings.Contains(r.errOut, "--theme-template") {
		t.Fatalf("help: %+v", r)
	}
	Version = "1.2.3"
	if r := run(t, nil, "-v"); r.code != 0 || r.out != "kmd 1.2.3\n" {
		t.Fatalf("version: %+v", r)
	}
}

func TestErrors(t *testing.T) {
	for _, args := range [][]string{
		{"--color", "sometimes"}, {"--hyperlinks", "x"}, {"--theme", "pink"}, {"-w", "-3"},
		{"--bogus"}, {"--images", "fax"}, {"--theme-template", "nope"}, {"--completion", "tcsh"},
		{"-f"}, {"-f", "a", "b"}, {"-w", "100000"},
	} {
		if r := run(t, nil, args...); r.code != 2 || !strings.Contains(r.errOut, "kmd") {
			t.Errorf("%v: code=%d err=%q", args, r.code, r.errOut)
		}
	}
	p := writeFile(t, "ok.md", "ok")
	r := run(t, nil, "/does/not/exist.md", p)
	if r.code != 1 || !strings.Contains(r.errOut, "exist.md") || !strings.Contains(r.out, "ok") {
		t.Errorf("missing file: %+v", r)
	}
	if r := run(t, nil, t.TempDir()); r.code != 1 || !strings.Contains(r.errOut, "is a directory") {
		t.Errorf("directory: %+v", r)
	}
	if r := run(t, func(e *Env) { e.Stdin = errReader{} }); r.code != 1 {
		t.Errorf("read error: %+v", r)
	}
	if r := run(t, func(e *Env) { e.Stdout = errWriter{} }, p); r.code != 1 {
		t.Errorf("write error: %+v", r)
	}
	if _, err := readSource("-", strings.NewReader(strings.Repeat("a", maxInput+1))); err == nil {
		t.Error("oversized input accepted")
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("boom") }

type errWriter struct{}

func (errWriter) Write([]byte) (int, error) { return 0, errors.New("boom") }

func TestMultipleFilesAndModes(t *testing.T) {
	a := writeFile(t, "a_b.md", "# Alpha\n\n## Sub")
	b := writeFile(t, "-weird.md", "beta")
	r := run(t, nil, a, "--", b)
	for _, s := range []string{"A_B.MD", "-WEIRD.MD", "ALPHA", "beta"} {
		if !strings.Contains(r.out, s) {
			t.Errorf("missing %q in %q", s, r.out)
		}
	}
	r = run(t, nil, "--toc", a)
	if !strings.Contains(r.out, "1  Alpha") || !strings.Contains(r.out, "1.1  Sub") {
		t.Errorf("toc: %q", r.out)
	}
	r = run(t, nil, "--html", a, b)
	for _, s := range []string{"<!DOCTYPE html>", "<title>Alpha</title>", "<h1>Alpha</h1>", "<p>beta</p>", "--bg: #"} {
		if !strings.Contains(r.out, s) {
			t.Errorf("html missing %q", s)
		}
	}
	if r := run(t, nil, "--html"); !strings.Contains(r.out, "<title>from stdin</title>") {
		t.Errorf("html stdin title: %q", r.out[:200])
	}
	if r := run(t, func(e *Env) { e.Stdin = strings.NewReader("text") }, "--html"); !strings.Contains(r.out, "<title>kmd</title>") {
		t.Error("html default title")
	}
	if r := run(t, nil, "-", "-"); strings.Count(r.out, "FROM STDIN") != 2 {
		t.Errorf("stdin twice: %q", r.out)
	}
	evil := writeFile(t, "x.md", "[a](javascript:alert(1)) <script>x</script>")
	if r := run(t, nil, "--html", evil); strings.Contains(r.out, "javascript:") || strings.Contains(r.out, "<script>") ||
		!strings.Contains(r.out, "Content-Security-Policy") {
		t.Errorf("unsafe content in --html: %q", r.out)
	}
	if r := run(t, nil, "--html", "--unsafe-html", evil); !strings.Contains(r.out, "javascript:alert") {
		t.Error("--unsafe-html ignored")
	}
	// control characters in file names never reach the terminal
	bad := filepath.Join(t.TempDir(), "x\x1b]0;PWNED\x07.md")
	if r := run(t, nil, bad); strings.ContainsAny(r.errOut, "\x1b\x07") || r.code != 1 {
		t.Errorf("unsanitized stderr: %q", r.errOut)
	}
	if fileTitle("-") != "stdin" {
		t.Error("stdin title")
	}
}

func TestTerminalBehaviour(t *testing.T) {
	long := strings.Repeat("line\n\n", 50)
	tty := func(e *Env) {
		e.StdoutTTY, e.Cols, e.Rows = true, 200, 20
		e.Stdin = strings.NewReader(long + "[l](https://x.y)")
	}
	r := run(t, tty)
	if r.paged == "" || r.out != "" {
		t.Fatalf("long output was not paged: out=%d paged=%d", len(r.out), len(r.paged))
	}
	if !strings.Contains(r.paged, "\x1b]8;;https://x.y") {
		t.Error("hyperlinks missing on a terminal")
	}
	if r := run(t, tty, "-P"); r.paged != "" || r.out == "" {
		t.Fatal("--no-pager ignored")
	}
	r = run(t, func(e *Env) {
		tty(e)
		e.Pager = func(string, string) error { return errors.New("no pager") }
	})
	if r.out == "" {
		t.Fatal("no fallback when pager fails")
	}
	for _, kv := range [][2]string{{"NO_COLOR", "1"}, {"TERM", "dumb"}} {
		r = run(t, func(e *Env) {
			e.StdoutTTY, e.Cols = true, 80
			e.Getenv = envMap(map[string]string{kv[0]: kv[1]})
		})
		if strings.Contains(r.out, "\x1b") {
			t.Errorf("%s ignored", kv[0])
		}
	}
	r = run(t, func(e *Env) {
		e.StdoutTTY, e.Cols = true, 400
		e.Stdin = strings.NewReader("---")
	}, "-c", "never")
	if n := strings.Count(r.out, "─"); n != defaultMaxWidth-4 {
		t.Errorf("rule width %d", n)
	}
	// graphics images are never paged
	dir := t.TempDir()
	r = run(t, func(e *Env) {
		tty(e)
		e.Getenv = envMap(map[string]string{"TERM": "xterm-kitty", "COLORTERM": "truecolor"})
		e.Stdin = strings.NewReader(long)
	}, "--images", "auto", filepath.Join(dir, "missing.md"))
	if r.code != 1 {
		t.Errorf("missing file with images: %+v", r)
	}
}

func TestConfigFile(t *testing.T) {
	home := t.TempDir()
	cfgDir := filepath.Join(home, ".config", "kmd")
	if err := os.MkdirAll(filepath.Join(cfgDir, "themes"), 0o750); err != nil {
		t.Fatal(err)
	}
	cfg := "# settings\ntheme = nord\nwidth = 50\nmax-width = 90\ncolor = always\nmath = off\ndiagrams = no\nimages = none\nimage-rows = 5\nhyperlinks = never\npager = \"\"\n"
	if err := os.WriteFile(filepath.Join(cfgDir, "config"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "themes", "mine.theme"), []byte("inherit = nord\nred = #ff0000\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	withHome := func(extra map[string]string) func(*Env) {
		return func(e *Env) {
			m := map[string]string{"HOME": home}
			for k, v := range extra {
				m[k] = v
			}
			e.Getenv = envMap(m)
			e.Stdin = strings.NewReader("$x$\n\n---")
		}
	}
	r := run(t, withHome(nil))
	if !strings.Contains(r.out, "\x1b[") || !strings.Contains(r.out, "$x$") || strings.Count(r.out, "─") != 46 {
		t.Errorf("config not applied: %q", r.out)
	}
	r = run(t, withHome(nil), "--config")
	for _, s := range []string{"theme        nord", "math         off", "pager        (disabled)", "# dark-theme"} {
		if !strings.Contains(r.out, s) {
			t.Errorf("--config missing %q:\n%s", s, r.out)
		}
	}
	r = run(t, withHome(nil), "--list-themes", "-c", "never")
	if !strings.Contains(r.out, "catppuccin-mocha\n") || !strings.Contains(r.out, "mine\n") {
		t.Errorf("list-themes plain: %q", r.out)
	}
	r = run(t, withHome(nil), "--list-themes")
	if !strings.Contains(r.out, "▸ nord") || !strings.Contains(r.out, "Your themes") {
		t.Errorf("list-themes pretty: %q", r.out)
	}
	if r := run(t, withHome(map[string]string{"KMD_THEME": "mine"}), "--config"); !strings.Contains(r.out, "theme        mine") {
		t.Errorf("KMD_THEME ignored: %s", r.out)
	}
	// auto theme uses COLORFGBG
	if r := run(t, withHome(map[string]string{"KMD_THEME": "auto", "COLORFGBG": "0;15"}), "--config"); !strings.Contains(r.out, "tokyo-night-day") {
		t.Errorf("auto light: %s", r.out)
	}
	// invalid config
	for _, bad := range []string{"nokey", "width = x", "color = sometimes", "math = maybe", "unknown = 1", "images = fax"} {
		if err := os.WriteFile(filepath.Join(cfgDir, "config"), []byte(bad), 0o600); err != nil {
			t.Fatal(err)
		}
		if r := run(t, withHome(nil)); r.code != 2 {
			t.Errorf("bad config %q accepted", bad)
		}
	}
	if configPath(envMap(map[string]string{"KMD_CONFIG": "/c"})) != "/c" ||
		configPath(envMap(map[string]string{"XDG_CONFIG_HOME": "/x"})) != "/x/kmd/config" ||
		configPath(envMap(nil)) != "" {
		t.Error("configPath")
	}
	if _, err := loadConfig(t.TempDir()); err == nil {
		t.Error("directory as config accepted")
	}
}

func TestCompletionAndTemplate(t *testing.T) {
	for shell, want := range map[string]string{"bash": "complete -o filenames", "zsh": "#compdef kmd", "fish": "complete -c kmd"} {
		if r := run(t, nil, "--completion", shell); r.code != 0 || !strings.Contains(r.out, want) {
			t.Errorf("%s completion: %+v", shell, r)
		}
	}
	if r := run(t, nil, "--theme-template", "nord"); r.code != 0 || !strings.Contains(r.out, "inherit = nord") {
		t.Errorf("template: %+v", r)
	}
}

func TestFollow(t *testing.T) {
	p := writeFile(t, "live.md", "# One")
	stop := make(chan struct{})
	var out safeBuffer
	done := make(chan int)
	go func() {
		done <- Run(Env{Args: []string{"-f", p}, Stdout: &out, Stderr: &out, Getenv: envMap(nil),
			StdoutTTY: true, Interrupt: stop, Poll: 5 * time.Millisecond})
	}()
	waitFor := func(s string) {
		deadline := time.Now().Add(3 * time.Second)
		for !strings.Contains(out.String(), s) {
			if time.Now().After(deadline) {
				t.Fatalf("timeout waiting for %q in %q", s, out.String())
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	waitFor("One")
	if err := os.WriteFile(p, []byte("# Two, longer"), 0o600); err != nil {
		t.Fatal(err)
	}
	waitFor("Two")
	// an editor replacing the file must not stop the preview
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	time.Sleep(30 * time.Millisecond)
	if err := os.WriteFile(p, []byte("# Three, after replace"), 0o600); err != nil {
		t.Fatal(err)
	}
	waitFor("Three")
	if !strings.Contains(out.String(), "\x1b[H\x1b[2J") {
		t.Error("screen not cleared")
	}
	close(stop)
	if code := <-done; code != 0 {
		t.Errorf("follow exit = %d", code)
	}
	if r := run(t, nil, "-f", "-"); r.code != 2 {
		t.Error("follow stdin")
	}
	if r := run(t, nil, "-f", filepath.Join(t.TempDir(), "gone.md")); r.code != 1 {
		t.Error("follow missing file")
	}
}

func TestSystemPager(t *testing.T) {
	var out, errOut bytes.Buffer
	if err := systemPager(&out, &errOut)("hello", "cat"); err != nil || out.String() != "hello" {
		t.Fatalf("cat pager: %v %q", err, out.String())
	}
	if err := systemPager(&out, &errOut)("x", "false"); err != nil {
		t.Fatal("a pager exiting non-zero must not trigger a second copy")
	}
	if err := systemPager(&out, &errOut)("x", "definitely-not-a-pager-kmd"); err == nil {
		t.Fatal("missing pager did not fail")
	}
	if err := systemPager(&out, &errOut)("x", ""); err == nil {
		t.Fatal("empty pager did not fail")
	}
}

func TestBuiltinViewer(t *testing.T) {
	dir := t.TempDir()
	long := strings.Repeat("line\n\n", 50)
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	a := write("a.md", "# A\n\n[to b](b.md#end)\n\n"+long+"![img](pic.png)\n")
	b := write("b.md", "# B\n\n"+long+"## End\n\n[back](a.md)\n")

	var got view.Options
	viewer := func(e *Env) {
		e.StdoutTTY, e.Cols, e.Rows = true, 100, 20
		e.Getenv = envMap(map[string]string{"TERM": "xterm-kitty", "COLORTERM": "truecolor", "HOME": t.TempDir()})
		e.View = func(o view.Options) error { got = o; return nil }
	}
	r := run(t, viewer, a)
	if r.out != "" || r.paged != "" || r.code != 0 {
		t.Fatalf("viewer not used: %+v", r)
	}
	if got.Start != a || !got.Mouse || got.Select == "" || got.Bar == "" {
		t.Errorf("options: %+v", got)
	}
	p, err := got.Load(got.Start, 60)
	if err != nil || len(p.Links) != 1 || !strings.HasSuffix(p.Links[0].Href, "/b.md#end") || p.Anchors["a"] == 0 {
		t.Fatalf("load a: %v %+v", err, p)
	}
	if strings.Contains(strings.Join(p.Lines, "\n"), "\x1b_G") {
		t.Error("kitty graphics in the viewer (should be blocks)")
	}
	for _, l := range p.Lines {
		if w := len([]rune(stripTest(l))); w > 60 {
			t.Errorf("line wider than the terminal: %d", w)
		}
	}
	p, err = got.Load(b, 300) // explicit page; wide terminal capped by max-width
	if err != nil || p.Anchors["end"] == 0 {
		t.Fatalf("load b: %v", err)
	}
	if _, err := got.Load(filepath.Join(dir, "missing.md"), 80); err == nil || !strings.Contains(err.Error(), "missing.md") {
		t.Errorf("missing page: %v", err)
	}

	// several files: one page, anchors and links shifted to their file
	r = run(t, viewer, a, b)
	if got.Start != "" {
		t.Errorf("start for several files: %q", got.Start)
	}
	p, _ = got.Load("", 80)
	if len(p.Links) != 2 || p.Links[1].Spans[0].Line <= p.Links[0].Spans[0].Line || p.Anchors["end"] <= p.Anchors["a"] {
		t.Errorf("merged page: %+v %v", p.Links, p.Anchors)
	}
	if !strings.Contains(strings.Join(p.Lines, "\n"), "b.md") {
		t.Error("file titles missing")
	}

	// stdin is read once and kept for re-renders
	r = run(t, func(e *Env) {
		viewer(e)
		e.Stdin = strings.NewReader(long + "[x](x.md)")
	})
	p1, _ := got.Load("", 80)
	p2, _ := got.Load("", 50)
	if r.code != 0 || len(p1.Links) != 1 || len(p2.Links) != 1 || len(p2.Lines) <= len(p1.Lines)-1 {
		t.Errorf("stdin re-render: %d %d", len(p1.Lines), len(p2.Lines))
	}

	// a fixed width is kept (but never wider than the terminal)
	run(t, viewer, "-w", "70", a)
	p, _ = got.Load("", 50)
	for _, l := range p.Lines {
		if w := len([]rune(stripTest(l))); w > 50 {
			t.Errorf("fixed width exceeds terminal: %d", w)
		}
	}

	// mouse = off; KMD_PAGER selects an external pager instead
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".config", "kmd"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".config", "kmd", "config"), []byte("mouse = off\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	run(t, func(e *Env) {
		viewer(e)
		e.Getenv = envMap(map[string]string{"HOME": home})
	}, a)
	if got.Mouse {
		t.Error("mouse = off ignored")
	}
	r = run(t, func(e *Env) {
		viewer(e)
		e.Getenv = envMap(map[string]string{"HOME": home, "KMD_PAGER": "less -R"})
		e.View = func(view.Options) error { t.Error("viewer used with KMD_PAGER"); return nil }
	}, a)
	if r.paged == "" {
		t.Error("KMD_PAGER pager not used")
	}
	r = run(t, nil, "--config")
	if !strings.Contains(r.out, "pager        builtin") || !strings.Contains(r.out, "mouse        on") {
		t.Errorf("--config: %s", r.out)
	}
	if _, err := loadConfig(writeConfig(t, "mouse = maybe\n")); err == nil {
		t.Error("bad mouse value accepted")
	}
}

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// stripTest removes SGR and OSC sequences from a painted line.
func stripTest(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[':
			for i < len(s) && (s[i] < 0x40 || s[i] > 0x7e || s[i] == '[') {
				i++
			}
		case s[i] == 0x1b && i+1 < len(s) && s[i+1] == ']':
			i += strings.Index(s[i:], "\x1b\\") + 1
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

func TestViewAlwaysAndFollow(t *testing.T) {
	dir := t.TempDir()
	short := filepath.Join(dir, "short.md")
	if err := os.WriteFile(short, []byte("# Short\n\n[x](x.md)\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var got *view.Options
	tty := func(home string, extra ...string) func(*Env) {
		return func(e *Env) {
			e.StdoutTTY, e.Cols, e.Rows = true, 100, 40
			env := map[string]string{"HOME": home}
			for i := 0; i+1 < len(extra); i += 2 {
				env[extra[i]] = extra[i+1]
			}
			e.Getenv = envMap(env)
			e.View = func(o view.Options) error { got = &o; return nil }
		}
	}
	home := t.TempDir()

	got = nil
	if r := run(t, tty(home), short); got != nil || r.out == "" {
		t.Error("short document opened the viewer without --view")
	}
	got = nil
	if r := run(t, tty(home), "--view", short); got == nil || r.out != "" || got.Watch {
		t.Error("--view ignored")
	}
	got = nil
	run(t, tty(home, "KMD_PAGER", "less -R"), "--view", short) // explicit flag wins over the pager
	if got == nil {
		t.Error("--view with an external pager")
	}

	cfgHome := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cfgHome, ".config", "kmd"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgHome, ".config", "kmd", "config"), []byte("view = always\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got = nil
	if run(t, tty(cfgHome), short); got == nil {
		t.Error("view = always ignored")
	}
	got = nil
	if run(t, tty(cfgHome), "-P", short); got != nil {
		t.Error("-P must win over view = always")
	}
	if r := run(t, tty(cfgHome), "--config"); !strings.Contains(r.out, "view         always") {
		t.Errorf("--config view: %s", r.out)
	}
	if _, err := loadConfig(writeConfig(t, "view = sometimes\n")); err == nil {
		t.Error("bad view value accepted")
	}

	// -f on a terminal: the viewer, watching the file
	got = nil
	if r := run(t, tty(home), "-f", short); got == nil || !got.Watch || got.Start != short || r.code != 0 {
		t.Errorf("-f did not use the watching viewer: %+v", got)
	}
	// stdin, --html and -P keep the classic follow
	stop := make(chan struct{})
	close(stop)
	got = nil
	run(t, func(e *Env) { tty(home)(e); e.Interrupt = stop; e.Poll = time.Millisecond }, "-f", "-P", short)
	if got != nil {
		t.Error("-f -P used the viewer")
	}
}

func TestViewerRendersOnce(t *testing.T) {
	dir := t.TempDir()
	png := filepath.Join(dir, "p.png")
	if err := os.WriteFile(png, gradientPNG(t), 0o600); err != nil {
		t.Fatal(err)
	}
	long := filepath.Join(dir, "long.md")
	short := filepath.Join(dir, "short.md")
	_ = os.WriteFile(long, []byte("![p](p.png)\n\n"+strings.Repeat("line\n\n", 60)), 0o600)
	_ = os.WriteFile(short, []byte("![p](p.png)\n\n[x](x.md)\n"), 0o600)
	var got *view.Options
	kitty := func(e *Env) {
		e.StdoutTTY, e.Cols, e.Rows = true, 90, 30
		e.Getenv = envMap(map[string]string{"TERM": "xterm-kitty", "COLORTERM": "truecolor", "HOME": t.TempDir()})
		e.View = func(o view.Options) error { got = &o; return nil }
	}
	run(t, kitty, long)
	if got == nil || got.First == nil || got.FirstCols != 90 || len(got.First.Lines) < 60 {
		t.Fatalf("long document: first page not handed over: %+v", got)
	}
	if strings.Contains(strings.Join(got.First.Lines, ""), "\x1b_G") {
		t.Error("first page has kitty graphics")
	}
	got = nil
	r := run(t, kitty, short) // fits: printed with real terminal graphics
	if got != nil || !strings.Contains(r.out, "\x1b_G") {
		t.Errorf("short document: viewer=%v kitty=%v", got != nil, strings.Contains(r.out, "\x1b_G"))
	}
}

// gradientPNG returns the repository's small test image.
func gradientPNG(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "gradient.png"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestVersionFromBuildInfo(t *testing.T) {
	info := func(v string) func() (*debug.BuildInfo, bool) {
		return func() (*debug.BuildInfo, bool) {
			return &debug.BuildInfo{Main: debug.Module{Version: v}}, true
		}
	}
	none := func() (*debug.BuildInfo, bool) { return nil, false }
	for _, c := range []struct {
		stamped string
		read    func() (*debug.BuildInfo, bool)
		want    string
	}{
		{"v1.2.3", info("v9.9.9"), "v1.2.3"}, // -ldflags wins
		{"dev", info("v0.4.0"), "v0.4.0"},    // go install …@v0.4.0
		{"dev", info("(devel)"), "dev"},      // go build in a checkout
		{"dev", info(""), "dev"},
		{"dev", none, "dev"},
	} {
		if got := versionFrom(c.stamped, c.read); got != c.want {
			t.Errorf("versionFrom(%q) = %q, want %q", c.stamped, got, c.want)
		}
	}
}
