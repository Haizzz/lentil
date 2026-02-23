package config

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
	"github.com/Haizzz/lentil/internal/lint"
)

// Load reads and parses a single lentil config file, applying defaults and validation.
func Load(path string) (*lint.Config, error) {
	cfg, _, err := parseFile(path)
	if err != nil {
		return nil, fmt.Errorf("loading config %s: %w", path, err)
	}

	applyDefaults(cfg)

	if err := validate(cfg); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return cfg, nil
}

func parseFile(path string) (*lint.Config, toml.MetaData, error) {
	var cfg lint.Config
	meta, err := toml.DecodeFile(path, &cfg)
	if err != nil {
		return nil, toml.MetaData{}, err
	}

	return &cfg, meta, nil
}

func applyDefaults(cfg *lint.Config) {
	if cfg.LLM.BaseURL == "" {
		cfg.LLM.BaseURL = lint.DefaultBaseURL
	}
	if cfg.LLM.MaxTokens == 0 {
		cfg.LLM.MaxTokens = lint.DefaultMaxTokens
	}
	if cfg.Settings.Concurrency == 0 {
		cfg.Settings.Concurrency = lint.DefaultConcurrency
	}
	if cfg.Settings.ChunkLines == 0 {
		cfg.Settings.ChunkLines = lint.DefaultChunkLines
	}
	if cfg.Settings.ChunkOverlap == 0 {
		cfg.Settings.ChunkOverlap = lint.DefaultChunkOverlap
	}
}

func validate(cfg *lint.Config) error {
	if cfg.LLM.Model == "" {
		return fmt.Errorf("llm.model is required")
	}
	if cfg.LLM.Temperature < 0 || cfg.LLM.Temperature > lint.MaxTemperature {
		return fmt.Errorf("llm.temperature must be between 0 and %.1f", lint.MaxTemperature)
	}
	if cfg.LLM.MaxTokens <= 0 {
		return fmt.Errorf("llm.max_tokens must be positive")
	}
	if cfg.Settings.Concurrency <= 0 {
		return fmt.Errorf("settings.concurrency must be positive")
	}
	if cfg.Settings.ChunkLines <= 0 {
		return fmt.Errorf("settings.chunk_lines must be positive")
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
// If scopeOverrides is non-nil and contains an entry for a rule ID, that
// scope is used; otherwise defaultScope is used.
func BuildRules(cfg *lint.Config, defaultScope string, scopeOverrides map[string]string) ([]lint.Rule, error) {
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
		scope := defaultScope
		if s, ok := scopeOverrides[id]; ok {
			scope = s
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
