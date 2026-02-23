package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Haizzz/lentil/internal/files"
	"github.com/Haizzz/lentil/internal/lint"
)

func TestLoad_BasicConfig(t *testing.T) {
	dir := t.TempDir()

	configContent := `
[llm]
model = "test-model"
base_url = "http://localhost:8080/v1"

[settings]
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

	if cfg.LLM.BaseURL != lint.DefaultBaseURL {
		t.Errorf("default base_url = %q", cfg.LLM.BaseURL)
	}
	if cfg.LLM.MaxTokens != lint.DefaultMaxTokens {
		t.Errorf("default max_tokens = %d", cfg.LLM.MaxTokens)
	}
	if cfg.Settings.Concurrency != lint.DefaultConcurrency {
		t.Errorf("default concurrency = %d", cfg.Settings.Concurrency)
	}
	if cfg.Settings.ChunkLines != lint.DefaultChunkLines {
		t.Errorf("default chunk_lines = %d", cfg.Settings.ChunkLines)
	}
	if cfg.Settings.ChunkOverlap != lint.DefaultChunkOverlap {
		t.Errorf("default chunk_overlap = %d", cfg.Settings.ChunkOverlap)
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

	rules, err := BuildRules(cfg, dir, nil)
	if err != nil {
		t.Fatalf("BuildRules failed: %v", err)
	}

	if len(rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(rules))
	}

	for _, r := range rules {
		if r.Scope != dir {
			t.Errorf("rule %q scope = %q, want %q", r.ID, r.Scope, dir)
		}
		if r.ID == "rule1" && r.Glob != "**/*.py" {
			t.Errorf("rule1 glob = %q, want **/*.py", r.Glob)
		}
		if r.ID == "rule2" && r.Glob != "**/*" {
			t.Errorf("rule2 should default to **/* when no glob set, got %q", r.Glob)
		}
	}
}

func TestDiscoverConfigs(t *testing.T) {
	dir := t.TempDir()

	writeFile := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	writeFile(filepath.Join(dir, "lentil.toml"), `
[llm]
model = "root-model"
[rules.root-rule]
prompt = "root"
`)
	writeFile(filepath.Join(dir, "src", "lentil.toml"), `
[rules.src-rule]
prompt = "src"
`)
	writeFile(filepath.Join(dir, "src", "frontend", "lentil.toml"), `
[rules.frontend-rule]
prompt = "frontend"
`)

	w, err := files.NewWalker(dir)
	if err != nil {
		t.Fatal(err)
	}

	configs, err := DiscoverConfigs(w)
	if err != nil {
		t.Fatalf("DiscoverConfigs failed: %v", err)
	}

	if len(configs) != 3 {
		t.Fatalf("expected 3 configs, got %d", len(configs))
	}

	if configs[0].Dir != dir {
		t.Errorf("configs[0].Dir = %q, want %q", configs[0].Dir, dir)
	}
	if configs[0].Config.LLM.Model != "root-model" {
		t.Errorf("root config model = %q, want root-model", configs[0].Config.LLM.Model)
	}
}

func TestDiscoverConfigs_RespectsGitignore(t *testing.T) {
	dir := t.TempDir()

	writeFile := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	writeFile(filepath.Join(dir, ".gitignore"), "vendor/\nnode_modules/\n")
	writeFile(filepath.Join(dir, "lentil.toml"), `
[rules.ok]
prompt = "ok"
`)
	writeFile(filepath.Join(dir, "vendor", "lentil.toml"), `
[rules.vendored]
prompt = "should not appear"
`)
	writeFile(filepath.Join(dir, "node_modules", "lentil.toml"), `
[rules.nm]
prompt = "should not appear"
`)

	w, err := files.NewWalker(dir)
	if err != nil {
		t.Fatal(err)
	}

	configs, err := DiscoverConfigs(w)
	if err != nil {
		t.Fatalf("DiscoverConfigs failed: %v", err)
	}

	if len(configs) != 1 {
		t.Fatalf("expected 1 config (vendor and node_modules gitignored), got %d", len(configs))
	}
}

func TestMergeConfigs_AdditiveRules(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "src")

	configs := []DiscoveredConfig{
		mustParseInline(t, dir, `
[llm]
model = "root-model"
base_url = "http://root:8080/v1"
[settings]
concurrency = 10
[rules.root-rule]
severity = "error"
prompt = "root prompt"
`),
		mustParseInline(t, srcDir, `
[llm]
model = "src-model"
[rules.src-rule]
severity = "warning"
prompt = "src prompt"
`),
	}

	merged, scopes, err := MergeConfigs(configs)
	if err != nil {
		t.Fatalf("MergeConfigs failed: %v", err)
	}

	if len(merged.Rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(merged.Rules))
	}
	if _, ok := merged.Rules["root-rule"]; !ok {
		t.Error("missing root-rule")
	}
	if _, ok := merged.Rules["src-rule"]; !ok {
		t.Error("missing src-rule")
	}

	if merged.LLM.Model != "src-model" {
		t.Errorf("model = %q, want src-model (deeper wins)", merged.LLM.Model)
	}
	if merged.LLM.BaseURL != "http://root:8080/v1" {
		t.Errorf("base_url = %q, want root value (src didn't set it)", merged.LLM.BaseURL)
	}

	if merged.Settings.Concurrency != 10 {
		t.Errorf("concurrency = %d, want 10 (from root, src didn't override)", merged.Settings.Concurrency)
	}

	if scopes["root-rule"] != dir {
		t.Errorf("root-rule scope = %q, want %q", scopes["root-rule"], dir)
	}
	if scopes["src-rule"] != srcDir {
		t.Errorf("src-rule scope = %q, want %q", scopes["src-rule"], srcDir)
	}
}

func TestMergeConfigs_DuplicateRuleID_Error(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "src")

	configs := []DiscoveredConfig{
		mustParseInline(t, dir, `
[llm]
model = "m"
[rules.shared-rule]
prompt = "from root"
`),
		mustParseInline(t, srcDir, `
[rules.shared-rule]
prompt = "from src"
`),
	}

	_, _, err := MergeConfigs(configs)
	if err == nil {
		t.Fatal("expected error for duplicate rule ID, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate rule") {
		t.Errorf("expected 'duplicate rule' in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "shared-rule") {
		t.Errorf("expected rule ID in error, got: %v", err)
	}
}

func TestMergeConfigs_FieldByFieldOverride(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "sub")

	configs := []DiscoveredConfig{
		mustParseInline(t, dir, `
[llm]
model = "base"
base_url = "http://base/v1"
temperature = 0.5
max_tokens = 2048
[settings]
concurrency = 4
chunk_lines = 200
chunk_overlap = 10
[rules.r1]
prompt = "base rule"
`),
		mustParseInline(t, subDir, `
[llm]
temperature = 0.1
[settings]
chunk_lines = 500
[rules.r2]
prompt = "sub rule"
`),
	}

	merged, _, err := MergeConfigs(configs)
	if err != nil {
		t.Fatalf("MergeConfigs failed: %v", err)
	}

	if merged.LLM.Model != "base" {
		t.Errorf("model = %q, want base", merged.LLM.Model)
	}
	if merged.LLM.BaseURL != "http://base/v1" {
		t.Errorf("base_url = %q, want http://base/v1", merged.LLM.BaseURL)
	}
	if merged.LLM.Temperature != 0.1 {
		t.Errorf("temperature = %v, want 0.1", merged.LLM.Temperature)
	}
	if merged.LLM.MaxTokens != 2048 {
		t.Errorf("max_tokens = %d, want 2048", merged.LLM.MaxTokens)
	}

	if merged.Settings.Concurrency != 4 {
		t.Errorf("concurrency = %d, want 4", merged.Settings.Concurrency)
	}
	if merged.Settings.ChunkLines != 500 {
		t.Errorf("chunk_lines = %d, want 500", merged.Settings.ChunkLines)
	}
	if merged.Settings.ChunkOverlap != 10 {
		t.Errorf("chunk_overlap = %d, want 10", merged.Settings.ChunkOverlap)
	}
}

func TestResolve_ExplicitConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "custom.toml")

	content := `
[llm]
model = "explicit"
[rules.r1]
prompt = "test"
`
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, rules, walker, err := Resolve(cfgPath, true)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	if cfg.LLM.Model != "explicit" {
		t.Errorf("model = %q, want explicit", cfg.LLM.Model)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}
	if rules[0].ID != "r1" {
		t.Errorf("rule ID = %q, want r1", rules[0].ID)
	}
	if walker == nil {
		t.Error("expected non-nil walker")
	}
}

func mustParseInline(t *testing.T, dir, content string) DiscoveredConfig {
	t.Helper()
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "lentil.toml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, meta, err := parseFile(path)
	if err != nil {
		t.Fatal(err)
	}

	return DiscoveredConfig{Dir: dir, Config: cfg, Meta: meta}
}
