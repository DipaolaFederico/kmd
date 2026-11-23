package main

import (
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
	if err != nil {
		t.Skip("go command not found")
	}
	// #nosec G204 -- fixed arguments; only the build tags vary, set by the test.
	cmd := exec.Command(gobin, "list", "-deps", "-tags="+tags, ".")
	// GOFLAGS=-tags=kmdai (make test-ai) must not change what is checked
	cmd.Env = append(os.Environ(), "GOFLAGS=")
	out, err := cmd.Output()
	if err != nil {
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
