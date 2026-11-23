//go:build kmdai

package ai

import "testing"

func TestDefaults(t *testing.T) {
	for _, tc := range []struct {
		env  map[string]string
		want Config
	}{
		{nil, Config{URL: "http://127.0.0.1:8091"}},
		{
			map[string]string{"XDG_RUNTIME_DIR": "/run/user/1000", "HOME": "/home/u"},
			Config{URL: "unix:///run/user/1000/kmd-llama.sock", KeyFile: "/home/u/.config/kmd/ai-key"},
		},
		{
			map[string]string{"XDG_CONFIG_HOME": "/cfg", "HOME": "/home/u"},
			Config{URL: "http://127.0.0.1:8091", KeyFile: "/cfg/kmd/ai-key"},
		},
	} {
		if got := Defaults(func(k string) string { return tc.env[k] }); got != tc.want {
			t.Errorf("Defaults(%v) = %+v, want %+v", tc.env, got, tc.want)
		}
	}
}
