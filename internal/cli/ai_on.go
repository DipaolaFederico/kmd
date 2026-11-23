//go:build kmdai

package cli

import "github.com/DipaolaFederico/kmd/internal/ai"

// aiSettings is the resolved experimental AI configuration.
type aiSettings struct {
	on bool
	ai.Config
}

// resolveAI applies the defaults to the ai keys the config file leaves out.
func resolveAI(cfg config, env Env) aiSettings {
	d := ai.Defaults(env.Getenv)
	return aiSettings{
		on: cfg.AI != nil && *cfg.AI,
		Config: ai.Config{
			URL:     firstNonEmpty(cfg.AIURL, d.URL),
			KeyFile: firstNonEmpty(cfg.AIKeyFile, d.KeyFile),
		},
	}
}

// describe adds the AI settings to kmd --config.
func (a aiSettings) describe(eff map[string]string) {
	eff["ai"] = map[bool]string{true: "on", false: "off"}[a.on]
	eff["ai-url"] = a.URL
	eff["ai-key-file"] = firstNonEmpty(a.KeyFile, "(none)")
}
