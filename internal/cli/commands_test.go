package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// hostileThemes are theme file names that a shell or a terminal would
// interpret: installing someone else's themes must never run code.
var hostileThemes = []string{
	"$(touch PWNED).theme",
	"`touch PWNED`.theme",
	"esc\x1b]0;TITLE\a\x1b[31mred.theme",
}

// TestListThemesIsInert: --list-themes never writes a hostile theme name,
// or a load error, raw to the terminal.
func TestListThemesIsInert(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".config", "kmd", "themes")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	for _, n := range append(hostileThemes, "fine.theme") {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("inherit = nord\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// a broken theme whose error quotes what is in the file
	if err := os.WriteFile(filepath.Join(dir, "broken.theme"), []byte("red = \x1b]0;T\a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	withHome := func(e *Env) { e.Getenv = envMap(map[string]string{"HOME": home}) }
	plain := run(t, withHome, "--list-themes", "-c", "never")
	if strings.ContainsAny(plain.out, "\x1b\a$`") || !strings.Contains(plain.out, "fine\n") {
		t.Errorf("plain list: %q", plain.out)
	}
	pretty := run(t, withHome, "--list-themes", "-c", "always")
	ownSGR := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	if rest := ownSGR.ReplaceAllString(pretty.out, ""); strings.ContainsAny(rest, "\x1b\a") || !strings.Contains(rest, "broken") {
		t.Errorf("pretty list: %q", rest)
	}
}

// TestBashCompletionDoesNotExpandThemes runs the generated bash completion
// against a kmd that lists hostile names (an older kmd, or names that got
// past the filter): completing them must not run the commands they spell.
func TestBashCompletionDoesNotExpandThemes(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not installed")
	}
	work, bin := t.TempDir(), t.TempDir()
	fake := "#!/bin/sh\nprintf '%s\\n' good '$(touch PWNED)' '`touch PWNED`' 'go$(touch PWNED)'\n"
	if err := os.WriteFile(filepath.Join(bin, "kmd"), []byte(fake), 0o700); err != nil { // #nosec G306 -- an executable test stub
		t.Fatal(err)
	}
	comp := filepath.Join(bin, "kmd.bash")
	if err := os.WriteFile(comp, []byte(bashCompletion), 0o600); err != nil {
		t.Fatal(err)
	}
	complete := func(cur string) string {
		t.Helper()
		script := `PATH="$1:$PATH"; source "$2"; COMP_WORDS=(kmd -t "$3"); COMP_CWORD=2; _kmd; printf '%s\n' "${COMPREPLY[@]}"`
		cmd := exec.Command(bash, "--norc", "--noprofile", "-c", script, "bash", bin, comp, cur) // #nosec G204 -- fixed script, test paths
		cmd.Dir = work
		// only stdout: bash builds without programmable completion (the Nix
		// sandbox's) complain about the final "complete" line on stderr
		var stderr strings.Builder
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("bash: %v\n%s%s", err, out, stderr.String())
		}
		return string(out)
	}
	if got := complete("g"); got != "good\ngo$(touch PWNED)\n" {
		t.Errorf("completing g: %q", got)
	}
	if got := complete(""); strings.Count(got, "\n") != 4 {
		t.Errorf("completing nothing: %q", got)
	}
	if _, err := os.Stat(filepath.Join(work, "PWNED")); err == nil {
		t.Error("the completion ran a command from a theme name")
	}
}
