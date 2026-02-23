package lint

import "fmt"

// Severity represents the severity level of a lint finding.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityInfo    Severity = "info"
)

// SeverityRank returns a numeric rank for severity comparison.
// Higher rank = more severe.
func SeverityRank(s Severity) int {
	switch s {
	case SeverityError:
		return 3
	case SeverityWarning:
		return 2
	case SeverityInfo:
		return 1
	default:
		return 0
	}
}

// ParseSeverity parses a string into a Severity, returning an error for invalid values.
func ParseSeverity(s string) (Severity, error) {
	switch s {
	case "error":
		return SeverityError, nil
	case "warning":
		return SeverityWarning, nil
	case "info":
		return SeverityInfo, nil
	default:
		return "", fmt.Errorf("invalid severity %q: must be error, warning, or info", s)
	}
}

// Rule represents a single lint rule.
type Rule struct {
	ID       string
	Severity Severity
	Prompt   string
	Glob     string // file pattern, falls back to global default
	Scope    string // directory this rule applies to (absolute path)
}

// Finding represents a single lint finding.
type Finding struct {
	File     string   `json:"file"`
	Line     int      `json:"line"`
	Column   int      `json:"column,omitempty"`
	Rule     string   `json:"rule"`
	Severity Severity `json:"severity"`
	Message  string   `json:"message"`
	Snippet  string   `json:"snippet,omitempty"`
}

// Config is the top-level configuration.
type Config struct {
	LLM      LLMConfig             `toml:"llm"`
	Settings SettingsConfig        `toml:"settings"`
	Rules    map[string]RuleConfig `toml:"rules"`
}

// LLMConfig holds LLM connection settings.
type LLMConfig struct {
	BaseURL     string  `toml:"base_url"`
	Model       string  `toml:"model"`
	Temperature float64 `toml:"temperature"`
	MaxTokens   int     `toml:"max_tokens"`
}

// SettingsConfig holds global settings.
type SettingsConfig struct {
	Concurrency  int `toml:"concurrency"`
	ChunkLines   int `toml:"chunk_lines"`
	ChunkOverlap int `toml:"chunk_overlap"`
}

// RuleConfig is the TOML representation of a single rule.
type RuleConfig struct {
	Severity string `toml:"severity"`
	Prompt   string `toml:"prompt"`
	Glob     string `toml:"glob"`
}

// Chunk represents a chunk of a file to be analyzed.
type Chunk struct {
	FilePath   string
	StartLine  int // 1-based inclusive
	EndLine    int // 1-based inclusive
	TotalLines int
	Content    string // formatted with line numbers
}

// Summary holds aggregate stats about a lint run.
type Summary struct {
	FilesScanned  int `json:"files_scanned"`
	RulesApplied  int `json:"rules_applied"`
	TotalFindings int `json:"total_findings"`
	Errors        int `json:"errors"`
	Warnings      int `json:"warnings"`
	Info          int `json:"info"`
}
