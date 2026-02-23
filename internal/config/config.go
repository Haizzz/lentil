package config

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
	"github.com/anhle/lentil/internal/lint"
)

// Load reads and parses a single lentil config file, applying defaults and validation.
func Load(path string) (*lint.Config, error) {
	cfg, err := parseFile(path)
	if err != nil {
		return nil, fmt.Errorf("loading config %s: %w", path, err)
	}

	applyDefaults(cfg)

	if err := validate(cfg); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return cfg, nil
}

func parseFile(path string) (*lint.Config, error) {
	var cfg lint.Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func applyDefaults(cfg *lint.Config) {
	if cfg.LLM.BaseURL == "" {
		cfg.LLM.BaseURL = "https://api.openai.com/v1"
	}
	if cfg.LLM.MaxTokens == 0 {
		cfg.LLM.MaxTokens = 4096
	}
	if cfg.Settings.Concurrency == 0 {
		cfg.Settings.Concurrency = 4
	}
	if cfg.Settings.ChunkLines == 0 {
		cfg.Settings.ChunkLines = 300
	}
	if cfg.Settings.ChunkOverlap == 0 {
		cfg.Settings.ChunkOverlap = 20
	}
}

func validate(cfg *lint.Config) error {
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
			if _, err := lint.ParseSeverity(rule.Severity); err != nil {
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
// defaulting severity to "warning" and glob to "**/*" (all files).
func BuildRules(cfg *lint.Config, scope string) ([]lint.Rule, error) {
	var rules []lint.Rule
	for id, rc := range cfg.Rules {
		sev := lint.SeverityWarning
		if rc.Severity != "" {
			var err error
			sev, err = lint.ParseSeverity(rc.Severity)
			if err != nil {
				return nil, err
			}
		}
		glob := rc.Glob
		if glob == "" {
			glob = "**/*"
		}
		rules = append(rules, lint.Rule{
			ID:       id,
			Severity: sev,
			Prompt:   rc.Prompt,
			Glob:     glob,
			Scope:    scope,
		})
	}

	return rules, nil
}
