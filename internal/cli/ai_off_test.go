//go:build !kmdai

package cli

import (
	"strings"
	"testing"
)

// A build without the AI accepts a config file written for one with it.
func TestAIKeysWithoutAI(t *testing.T) {
	keys := "ai-url = http://127.0.0.1:9000\nai-key-file = /k\n"
	for _, tc := range []struct{ body, notice string }{
		{"ai = on\n" + keys, "kmd: ai = on ignored: this kmd was built without the experimental AI\n"},
		{"ai = off\n" + keys, ""},
	} {
		cfg := writeConfig(t, tc.body)
		r := run(t, func(e *Env) { e.Getenv = envMap(map[string]string{"KMD_CONFIG": cfg}) }, "--config")
		if r.code != 0 || r.errOut != tc.notice {
			t.Errorf("%q: code %d, stderr %q, want %q", tc.body, r.code, r.errOut, tc.notice)
		}
		if strings.Contains(r.out, "  ai") {
			t.Errorf("--config shows AI settings in a build without it:\n%s", r.out)
		}
	}
}
