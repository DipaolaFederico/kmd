package main

import (
	"errors"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// deps lists the packages the kmd binary links when built with tags.
func deps(t *testing.T, tags string) []string {
	t.Helper()
	gobin, err := exec.LookPath("go")
	if err != nil && os.Getenv("CI") == "" {
		t.Skip("go command not found")
	} else if err != nil {
		t.Fatal(err)
	}
	// #nosec G204 -- fixed arguments; only the build tags vary, set by the test.
	cmd := exec.Command(gobin, "list", "-deps", "-tags="+tags, ".")
	// as released: no cgo, and no -tags from a GOFLAGS in the environment
	cmd.Env = append(os.Environ(), "GOFLAGS=", "CGO_ENABLED=0")
	out, err := cmd.Output()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		// e.g. an untagged file imports internal/ai: "build constraints exclude all Go files"
		t.Fatalf("go list: %v\n%s", err, ee.Stderr)
	} else if err != nil {
		t.Fatalf("go list: %v", err)
	}
	return strings.Fields(string(out))
}

// The released binary keeps the README's promise of making no network
// requests: the experimental AI, and the network stack it needs, are only
// linked with -tags kmdai (docs/experimental/ai.md).
func TestDefaultBuildHasNoAI(t *testing.T) {
	const aiPkg = "github.com/DipaolaFederico/kmd/internal/ai"
	for _, p := range deps(t, "") {
		if p == aiPkg || p == "net" {
			t.Errorf("the default build links %s; only -tags kmdai may", p)
		}
	}
	if !slices.Contains(deps(t, "kmdai"), aiPkg) {
		t.Errorf("-tags kmdai does not link %s: this test would miss it", aiPkg)
	}
}
