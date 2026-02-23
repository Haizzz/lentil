package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/anhle/lentil/internal/files"
	"github.com/anhle/lentil/internal/lint"
)

const configFileName = "lentil.toml"

// DiscoveredConfig pairs a parsed config with the directory it was found in.
type DiscoveredConfig struct {
	Dir    string
	Config *lint.Config
}

// DiscoverConfigs uses the walker to find all lentil.toml files under root,
// parses each, and returns them sorted by depth (shallowest first).
func DiscoverConfigs(w *files.Walker) ([]DiscoveredConfig, error) {
	paths, err := w.Glob(w.Root(), "**/"+configFileName)
	if err != nil {
		return nil, fmt.Errorf("discovering config files: %w", err)
	}

	var configs []DiscoveredConfig
	for _, path := range paths {
		cfg, err := parseFile(path)
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", path, err)
		}

		configs = append(configs, DiscoveredConfig{
			Dir:    filepath.Dir(path),
			Config: cfg,
		})
	}

	return configs, nil
}

// MergeConfigs merges a chain of discovered configs (shallowest first) into
// a single resolved config. Rules are additive — duplicate rule IDs across
// configs produce an error. LLM and settings fields override on a per-field
// basis (deeper configs win for fields they explicitly set).
func MergeConfigs(configs []DiscoveredConfig) (*lint.Config, map[string]string, error) {
	merged := &lint.Config{
		Rules: make(map[string]lint.RuleConfig),
	}
	ruleScopes := make(map[string]string)

	for _, dc := range configs {
		mergeLLM(&merged.LLM, &dc.Config.LLM)
		mergeSettings(&merged.Settings, &dc.Config.Settings)

		for id, rule := range dc.Config.Rules {
			if prevDir, exists := ruleScopes[id]; exists {
				return nil, nil, fmt.Errorf(
					"duplicate rule %q: defined in both %s and %s",
					id,
					filepath.Join(prevDir, configFileName),
					filepath.Join(dc.Dir, configFileName),
				)
			}
			merged.Rules[id] = rule
			ruleScopes[id] = dc.Dir
		}
	}

	applyDefaults(merged)

	if err := validate(merged); err != nil {
		return nil, nil, fmt.Errorf("invalid config: %w", err)
	}

	return merged, ruleScopes, nil
}

func mergeLLM(dst, src *lint.LLMConfig) {
	if src.BaseURL != "" {
		dst.BaseURL = src.BaseURL
	}
	if src.Model != "" {
		dst.Model = src.Model
	}
	if src.Temperature != 0 {
		dst.Temperature = src.Temperature
	}
	if src.MaxTokens != 0 {
		dst.MaxTokens = src.MaxTokens
	}
}

func mergeSettings(dst, src *lint.SettingsConfig) {
	if src.Concurrency != 0 {
		dst.Concurrency = src.Concurrency
	}
	if src.ChunkLines != 0 {
		dst.ChunkLines = src.ChunkLines
	}
	if src.ChunkOverlap != 0 {
		dst.ChunkOverlap = src.ChunkOverlap
	}
}

// Resolve is the main entry point for config resolution.
// If configExplicit is true, it loads the single file at configPath.
// Otherwise, it discovers all lentil.toml files from the project root
// down and merges them hierarchically.
// Returns the merged config, built rules, and the walker for the engine.
func Resolve(configPath string, configExplicit bool) (*lint.Config, []lint.Rule, *files.Walker, error) {
	if configExplicit {
		cfg, err := Load(configPath)
		if err != nil {
			return nil, nil, nil, err
		}

		absDir, err := filepath.Abs(filepath.Dir(configPath))
		if err != nil {
			return nil, nil, nil, err
		}

		w, err := files.NewWalker(absDir)
		if err != nil {
			return nil, nil, nil, err
		}

		rules, err := BuildRules(cfg, absDir)
		if err != nil {
			return nil, nil, nil, err
		}

		return cfg, rules, w, nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("getting working directory: %w", err)
	}

	root, err := files.FindRoot(cwd)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("finding project root: %w", err)
	}

	w, err := files.NewWalker(root)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("creating file walker: %w", err)
	}

	discovered, err := DiscoverConfigs(w)
	if err != nil {
		return nil, nil, nil, err
	}

	if len(discovered) == 0 {
		return nil, nil, nil, fmt.Errorf("no %s found in %s or its subdirectories", configFileName, root)
	}

	cfg, ruleScopes, err := MergeConfigs(discovered)
	if err != nil {
		return nil, nil, nil, err
	}

	var allRules []lint.Rule
	for id, rc := range cfg.Rules {
		sev := lint.SeverityWarning
		if rc.Severity != "" {
			var parseErr error
			sev, parseErr = lint.ParseSeverity(rc.Severity)
			if parseErr != nil {
				return nil, nil, nil, parseErr
			}
		}
		glob := rc.Glob
		if glob == "" {
			glob = "**/*"
		}
		allRules = append(allRules, lint.Rule{
			ID:       id,
			Severity: sev,
			Prompt:   rc.Prompt,
			Glob:     glob,
			Scope:    ruleScopes[id],
		})
	}

	return cfg, allRules, w, nil
}
