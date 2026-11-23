package render_test

import (
	"os/exec"
	"strings"
	"testing"
)

// kmd never makes network requests (README, "Remote documents"): neither
// the command nor this package may link a network client. The test lives
// here, in the public package, because it guards both: the library and the
// binary built on it. net/url and net/netip only parse text.
func TestNoNetworkDependency(t *testing.T) {
	gobin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go command not found")
	}
	out, err := exec.Command(gobin, "list", "-deps", "github.com/DipaolaFederico/kmd", "github.com/DipaolaFederico/kmd/render").Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	pkgs := strings.Fields(string(out))
	if len(pkgs) < 20 { // go list printing nothing must not pass
		t.Fatalf("go list returned only %d packages", len(pkgs))
	}
	for _, p := range pkgs {
		if (p == "net" || strings.HasPrefix(p, "net/")) && p != "net/url" && p != "net/netip" {
			t.Errorf("kmd depends on %s", p)
		}
	}
}
