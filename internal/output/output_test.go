package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/anhle/lentil/internal/lint"
)

var testFindings = []lint.Finding{
	{
		File:     "src/auth.py",
		Line:     42,
		Column:   1,
		Rule:     "no-magic-numbers",
		Severity: lint.SeverityWarning,
		Message:  "Numeric literal used directly",
	},
	{
		File:     "src/auth.py",
		Line:     87,
		Column:   5,
		Rule:     "no-hardcoded-secrets",
		Severity: lint.SeverityError,
		Message:  "Hardcoded API key",
	},
}

var testSummary = lint.Summary{
	FilesScanned:  10,
	RulesApplied:  2,
	TotalFindings: 2,
	Errors:        1,
	Warnings:      1,
	Info:          0,
}

func TestText(t *testing.T) {
	var buf bytes.Buffer
	Text(&buf, testFindings, testSummary)
	out := buf.String()

	if !strings.Contains(out, "src/auth.py:42:1:") {
		t.Errorf("missing file:line:col, got: %s", out)
	}
	if !strings.Contains(out, "warning") {
		t.Errorf("missing severity, got: %s", out)
	}
	if !strings.Contains(out, "[no-magic-numbers]") {
		t.Errorf("missing rule name, got: %s", out)
	}
	if !strings.Contains(out, "2 findings") {
		t.Errorf("missing summary, got: %s", out)
	}
}

func TestText_NoFindings(t *testing.T) {
	var buf bytes.Buffer
	s := lint.Summary{FilesScanned: 5}
	Text(&buf, nil, s)
	out := buf.String()
	if !strings.Contains(out, "0 findings") {
		t.Errorf("expected 0 findings in summary, got: %s", out)
	}
}

func TestJSON(t *testing.T) {
	var buf bytes.Buffer
	err := JSON(&buf, testFindings, testSummary)
	if err != nil {
		t.Fatalf("JSON failed: %v", err)
	}

	var result struct {
		Findings []lint.Finding `json:"findings"`
		Summary  lint.Summary   `json:"summary"`
	}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if len(result.Findings) != 2 {
		t.Errorf("expected 2 findings, got %d", len(result.Findings))
	}
	if result.Summary.FilesScanned != 10 {
		t.Errorf("files_scanned = %d, want 10", result.Summary.FilesScanned)
	}
	if result.Summary.TotalFindings != 2 {
		t.Errorf("total_findings = %d, want 2", result.Summary.TotalFindings)
	}
}

func TestJSON_EmptyFindings(t *testing.T) {
	var buf bytes.Buffer
	err := JSON(&buf, nil, lint.Summary{})
	if err != nil {
		t.Fatalf("JSON failed: %v", err)
	}

	// Should output [] not null for findings
	if !strings.Contains(buf.String(), `"findings": []`) {
		t.Errorf("empty findings should be [], got: %s", buf.String())
	}
}

func TestSARIF(t *testing.T) {
	rules := []lint.Rule{
		{ID: "no-magic-numbers", Severity: lint.SeverityWarning, Prompt: "Find magic numbers"},
		{ID: "no-hardcoded-secrets", Severity: lint.SeverityError, Prompt: "Find secrets"},
	}

	var buf bytes.Buffer
	err := SARIF(&buf, testFindings, rules)
	if err != nil {
		t.Fatalf("SARIF failed: %v", err)
	}

	out := buf.String()

	// Basic SARIF structure checks
	if !strings.Contains(out, "sarifLog") && !strings.Contains(out, "$schema") && !strings.Contains(out, "2.1.0") {
		// At minimum it should be valid JSON
		var raw map[string]interface{}
		if err := json.Unmarshal(buf.Bytes(), &raw); err != nil {
			t.Errorf("SARIF output is not valid JSON: %v", err)
		}
	}

	if !strings.Contains(out, "lentil") {
		t.Errorf("SARIF should mention tool name 'lentil', got: %s", out)
	}
	if !strings.Contains(out, "no-magic-numbers") {
		t.Errorf("SARIF should contain rule ID, got: %s", out)
	}
}
