//go:build kmdai

package cli

import (
	"path/filepath"

	"github.com/DipaolaFederico/kmd/internal/ai"
)

// aiSettings is the resolved experimental AI configuration.
type aiSettings struct {
	on bool
	ai.Config
}

// resolveAI applies the defaults to the ai keys the config file leaves out.
func resolveAI(cfg config, env Env) aiSettings {
	keyFile := cfg.AIKeyFile
	if dir := configDir(env.Getenv); keyFile == "" && dir != "" {
		keyFile = filepath.Join(dir, "ai-key")
	}
	return aiSettings{
		on:     cfg.AI != nil && *cfg.AI,
		Config: ai.Config{URL: firstNonEmpty(cfg.AIURL, ai.DefaultURL(env.Getenv)), KeyFile: keyFile},
	}
}

// describe adds the AI settings to kmd --config.
func (a aiSettings) describe(eff map[string]string) {
	eff["ai"] = onOff[a.on]
	eff["ai-url"] = a.URL
	eff["ai-key-file"] = firstNonEmpty(a.KeyFile, "(none)")
}
