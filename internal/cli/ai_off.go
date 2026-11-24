//go:build !kmdai

package cli

import "fmt"

// aiSettings is empty: this kmd was built without the experimental AI,
// which needs -tags kmdai (docs/experimental/ai.md).
type aiSettings struct{}

// resolveAI accepts the ai keys, so one config file serves both builds, and
// says why ai = on has no effect here rather than failing on every run.
func resolveAI(cfg config, env Env) aiSettings {
	if cfg.AI != nil && *cfg.AI {
		fmt.Fprintln(env.Stderr, "kmd: ai = on ignored: this kmd was built without the experimental AI")
	}
	return aiSettings{}
}

// describe adds nothing to kmd --config: the AI does not exist in this build.
func (aiSettings) describe(map[string]string) {}
