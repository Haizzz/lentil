package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_BasicConfig(t *testing.T) {
	dir := t.TempDir()

	configContent := `
[llm]
model = "test-model"
base_url = "http://localhost:8080/v1"

[settings]
glob = "**/*.py"
concurrency = 3

[rules.test-rule]
severity = "warning"
prompt = "Find bugs"
`
	cfgPath := filepath.Join(dir, "lentil.toml")
	if err := os.WriteFile(cfgPath, []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.LLM.Model != "test-model" {
		t.Errorf("model = %q, want test-model", cfg.LLM.Model)
	}
	if cfg.LLM.BaseURL != "http://localhost:8080/v1" {
		t.Errorf("base_url = %q, want http://localhost:8080/v1", cfg.LLM.BaseURL)
	}
	if cfg.Settings.Concurrency != 3 {
		t.Errorf("concurrency = %d, want 3", cfg.Settings.Concurrency)
	}
	if cfg.Settings.Glob != "**/*.py" {
		t.Errorf("glob = %q, want **/*.py", cfg.Settings.Glob)
	}
	if len(cfg.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(cfg.Rules))
	}
	if cfg.Rules["test-rule"].Prompt != "Find bugs" {
		t.Errorf("rule prompt = %q, want Find bugs", cfg.Rules["test-rule"].Prompt)
	}
}

func TestLoad_Defaults(t *testing.T) {
	dir := t.TempDir()

	configContent := `
[llm]
model = "test-model"

[rules.r1]
prompt = "Find issues"
`
	cfgPath := filepath.Join(dir, "lentil.toml")
	if err := os.WriteFile(cfgPath, []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.LLM.BaseURL != "http://localhost:11434/v1" {
		t.Errorf("default base_url = %q", cfg.LLM.BaseURL)
	}
	if cfg.LLM.MaxTokens != 4096 {
		t.Errorf("default max_tokens = %d", cfg.LLM.MaxTokens)
	}
	if cfg.Settings.Concurrency != 5 {
		t.Errorf("default concurrency = %d", cfg.Settings.Concurrency)
	}
	if cfg.Settings.ChunkLines != 300 {
		t.Errorf("default chunk_lines = %d", cfg.Settings.ChunkLines)
	}
	if cfg.Settings.ChunkOverlap != 20 {
		t.Errorf("default chunk_overlap = %d", cfg.Settings.ChunkOverlap)
	}
}

func TestLoad_WithIncludes(t *testing.T) {
	dir := t.TempDir()
	rulesDir := filepath.Join(dir, "rules")
	if err := os.MkdirAll(rulesDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write included rules file
	incContent := `
[rules.included-rule]
severity = "error"
prompt = "Included rule prompt"
`
	if err := os.WriteFile(filepath.Join(rulesDir, "extra.toml"), []byte(incContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Write main config — include must be before any [section] in TOML
	mainContent := `
include = ["rules/extra.toml"]

[llm]
model = "test-model"

[rules.inline-rule]
prompt = "Inline rule prompt"
`
	cfgPath := filepath.Join(dir, "lentil.toml")
	if err := os.WriteFile(cfgPath, []byte(mainContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(cfg.Rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(cfg.Rules))
	}
	if _, ok := cfg.Rules["inline-rule"]; !ok {
		t.Error("missing inline-rule")
	}
	if _, ok := cfg.Rules["included-rule"]; !ok {
		t.Error("missing included-rule")
	}
}

func TestLoad_InlineOverridesIncluded(t *testing.T) {
	dir := t.TempDir()
	rulesDir := filepath.Join(dir, "rules")
	if err := os.MkdirAll(rulesDir, 0755); err != nil {
		t.Fatal(err)
	}

	incContent := `
[rules.shared-rule]
severity = "info"
prompt = "Included version"
`
	if err := os.WriteFile(filepath.Join(rulesDir, "extra.toml"), []byte(incContent), 0644); err != nil {
		t.Fatal(err)
	}

	mainContent := `
include = ["rules/extra.toml"]

[llm]
model = "test-model"

[rules.shared-rule]
severity = "error"
prompt = "Inline version wins"
`
	cfgPath := filepath.Join(dir, "lentil.toml")
	if err := os.WriteFile(cfgPath, []byte(mainContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	rule := cfg.Rules["shared-rule"]
	if rule.Prompt != "Inline version wins" {
		t.Errorf("expected inline to win, got prompt = %q", rule.Prompt)
	}
	if rule.Severity != "error" {
		t.Errorf("expected severity=error, got %q", rule.Severity)
	}
}

func TestLoad_ValidationErrors(t *testing.T) {
	dir := t.TempDir()

	tests := []struct {
		name    string
		content string
	}{
		{
			"missing model",
			`[rules.r1]
prompt = "test"`,
		},
		{
			"no rules",
			`[llm]
model = "m"`,
		},
		{
			"empty prompt",
			`[llm]
model = "m"
[rules.r1]
severity = "error"`,
		},
		{
			"invalid severity",
			`[llm]
model = "m"
[rules.r1]
severity = "critical"
prompt = "test"`,
		},
		{
			"overlap-gte-chunk-lines",
			`[llm]
model = "m"
[settings]
chunk_lines = 10
chunk_overlap = 10
[rules.r1]
prompt = "test"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfgPath := filepath.Join(dir, tt.name+".toml")
			if err := os.WriteFile(cfgPath, []byte(tt.content), 0644); err != nil {
				t.Fatal(err)
			}
			_, err := Load(cfgPath)
			if err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

func TestResolveAPIKey(t *testing.T) {
	// Clear all relevant env vars
	for _, env := range []string{"LENTIL_LLM_API_KEY", "ANTHROPIC_API_KEY", "OPENAI_API_KEY"} {
		t.Setenv(env, "")
	}

	if got := ResolveAPIKey(); got != "" {
		t.Errorf("expected empty key, got %q", got)
	}

	t.Setenv("OPENAI_API_KEY", "openai-key")
	if got := ResolveAPIKey(); got != "openai-key" {
		t.Errorf("expected openai-key, got %q", got)
	}

	// LENTIL_LLM_API_KEY takes priority
	t.Setenv("LENTIL_LLM_API_KEY", "lentil-key")
	if got := ResolveAPIKey(); got != "lentil-key" {
		t.Errorf("expected lentil-key, got %q", got)
	}
}

func TestBuildRules(t *testing.T) {
	dir := t.TempDir()
	cfgContent := `
[llm]
model = "m"

[settings]
glob = "**/*.go"

[rules.rule1]
severity = "error"
prompt = "Find errors"
glob = "**/*.py"

[rules.rule2]
prompt = "Find warnings"
`
	cfgPath := filepath.Join(dir, "lentil.toml")
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	rules, err := BuildRules(cfg)
	if err != nil {
		t.Fatalf("BuildRules failed: %v", err)
	}

	if len(rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(rules))
	}

	ruleMap := make(map[string]bool)
	for _, r := range rules {
		ruleMap[r.ID] = true
		if r.ID == "rule1" {
			if r.Glob != "**/*.py" {
				t.Errorf("rule1 glob = %q, want **/*.py", r.Glob)
			}
		}
		if r.ID == "rule2" {
			if r.Glob != "**/*.go" {
				t.Errorf("rule2 should fall back to global glob, got %q", r.Glob)
			}
		}
	}
}
