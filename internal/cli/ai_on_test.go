//go:build kmdai

package cli

import (
	"strings"
	"testing"
)

func TestAIConfig(t *testing.T) {
	for _, tc := range []struct {
		body string
		env  map[string]string
		want []string
	}{
		{"", map[string]string{"XDG_RUNTIME_DIR": "/run/u", "XDG_CONFIG_HOME": "/cfg"}, []string{
			"ai           off\n", "ai-url       unix:///run/u/kmd-llama.sock\n", "ai-key-file  /cfg/kmd/ai-key\n",
		}},
		{"", map[string]string{}, []string{"ai-url       http://127.0.0.1:8091\n", "ai-key-file  (none)\n"}},
		{"ai = on\nai-url = http://127.0.0.1:9000\nai-key-file = /k\n", map[string]string{}, []string{
			"ai           on\n", "ai-url       http://127.0.0.1:9000\n", "ai-key-file  /k\n",
		}},
	} {
		tc.env["KMD_CONFIG"] = writeConfig(t, tc.body)
		r := run(t, func(e *Env) { e.Getenv = envMap(tc.env) }, "--config")
		if r.code != 0 || r.errOut != "" {
			t.Errorf("%q: code %d, stderr %q", tc.body, r.code, r.errOut)
		}
		for _, w := range tc.want {
			if !strings.Contains(r.out, w) {
				t.Errorf("%q: --config lacks %q:\n%s", tc.body, w, r.out)
			}
		}
	}
}
