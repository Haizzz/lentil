package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/anhle/lentil/internal/types"
)

// Load reads and parses a lentil config file, resolving includes and applying defaults.
func Load(path string) (*types.Config, error) {
	cfg, err := parseFile(path)
	if err != nil {
		return nil, fmt.Errorf("loading config %s: %w", path, err)
	}

	configDir := filepath.Dir(path)

	// Process includes
	for _, inc := range cfg.Include {
		incPath := inc
		if !filepath.IsAbs(incPath) {
			incPath = filepath.Join(configDir, incPath)
		}
		incCfg, err := parseFile(incPath)
		if err != nil {
			return nil, fmt.Errorf("loading included config %s: %w", incPath, err)
		}
		// Merge rules — inline rules win on conflict
		for id, rule := range incCfg.Rules {
			if _, exists := cfg.Rules[id]; !exists {
				if cfg.Rules == nil {
					cfg.Rules = make(map[string]types.RuleConfig)
				}
				cfg.Rules[id] = rule
			}
		}
	}

	applyDefaults(cfg)

	if err := validate(cfg); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return cfg, nil
}

func parseFile(path string) (*types.Config, error) {
	var cfg types.Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func applyDefaults(cfg *types.Config) {
	if cfg.LLM.BaseURL == "" {
		cfg.LLM.BaseURL = "http://localhost:11434/v1"
	}
	if cfg.LLM.MaxTokens == 0 {
		cfg.LLM.MaxTokens = 4096
	}
	if cfg.Settings.Glob == "" {
		cfg.Settings.Glob = "**/*.{py,js,ts,go,rs}"
	}
	if cfg.Settings.Concurrency == 0 {
		cfg.Settings.Concurrency = 5
	}
	if cfg.Settings.ChunkLines == 0 {
		cfg.Settings.ChunkLines = 300
	}
	if cfg.Settings.ChunkOverlap == 0 {
		cfg.Settings.ChunkOverlap = 20
	}
}

func validate(cfg *types.Config) error {
	if cfg.LLM.Model == "" {
		return fmt.Errorf("llm.model is required")
	}
	if len(cfg.Rules) == 0 {
		return fmt.Errorf("at least one rule must be defined")
	}
	for id, rule := range cfg.Rules {
		if rule.Prompt == "" {
			return fmt.Errorf("rule %q: prompt is required", id)
		}
		if rule.Severity != "" {
			if _, err := types.ParseSeverity(rule.Severity); err != nil {
				return fmt.Errorf("rule %q: %w", id, err)
			}
		}
	}
	if cfg.Settings.ChunkOverlap >= cfg.Settings.ChunkLines {
		return fmt.Errorf("settings.chunk_overlap (%d) must be less than settings.chunk_lines (%d)",
			cfg.Settings.ChunkOverlap, cfg.Settings.ChunkLines)
	}
	return nil
}

// ResolveAPIKey looks up the API key from environment variables.
// Returns empty string if no key is found (some endpoints don't require auth).
func ResolveAPIKey() string {
	for _, env := range []string{"LENTIL_LLM_API_KEY", "ANTHROPIC_API_KEY", "OPENAI_API_KEY"} {
		if key := os.Getenv(env); key != "" {
			return key
		}
	}
	return ""
}

// BuildRules converts the config's rule map into a slice of Rule structs,
// applying the global glob as fallback and defaulting severity to "warning".
func BuildRules(cfg *types.Config) ([]types.Rule, error) {
	var rules []types.Rule
	for id, rc := range cfg.Rules {
		sev := types.SeverityWarning
		if rc.Severity != "" {
			var err error
			sev, err = types.ParseSeverity(rc.Severity)
			if err != nil {
				return nil, err
			}
		}
		glob := rc.Glob
		if glob == "" {
			glob = cfg.Settings.Glob
		}
		rules = append(rules, types.Rule{
			ID:       id,
			Severity: sev,
			Prompt:   rc.Prompt,
			Glob:     glob,
		})
	}
	return rules, nil
}
