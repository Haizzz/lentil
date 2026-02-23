package lint

import "testing"

func TestParseSeverity(t *testing.T) {
	tests := []struct {
		input   string
		want    Severity
		wantErr bool
	}{
		{"error", SeverityError, false},
		{"warning", SeverityWarning, false},
		{"info", SeverityInfo, false},
		{"", "", true},
		{"critical", "", true},
		{"ERROR", "", true},
	}

	for _, tt := range tests {
		got, err := ParseSeverity(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseSeverity(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			continue
		}
		if got != tt.want {
			t.Errorf("ParseSeverity(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestSeverityRank(t *testing.T) {
	if SeverityRank(SeverityError) <= SeverityRank(SeverityWarning) {
		t.Error("error should rank higher than warning")
	}
	if SeverityRank(SeverityWarning) <= SeverityRank(SeverityInfo) {
		t.Error("warning should rank higher than info")
	}
	if SeverityRank(SeverityInfo) <= SeverityRank("unknown") {
		t.Error("info should rank higher than unknown")
	}
}

func TestNewSummary(t *testing.T) {
	t.Run("mixed severities", func(t *testing.T) {
		findings := []Finding{
			{Severity: SeverityError},
			{Severity: SeverityError},
			{Severity: SeverityWarning},
			{Severity: SeverityInfo},
			{Severity: SeverityInfo},
			{Severity: SeverityInfo},
		}
		s := NewSummary(findings, 10, 3)
		if s.Errors != 2 {
			t.Errorf("expected 2 errors, got %d", s.Errors)
		}
		if s.Warnings != 1 {
			t.Errorf("expected 1 warning, got %d", s.Warnings)
		}
		if s.Info != 3 {
			t.Errorf("expected 3 info, got %d", s.Info)
		}
		if s.TotalFindings != 6 {
			t.Errorf("expected 6 total findings, got %d", s.TotalFindings)
		}
		if s.FilesScanned != 10 {
			t.Errorf("expected 10 files scanned, got %d", s.FilesScanned)
		}
		if s.RulesApplied != 3 {
			t.Errorf("expected 3 rules applied, got %d", s.RulesApplied)
		}
	})

	t.Run("empty findings", func(t *testing.T) {
		s := NewSummary(nil, 5, 2)
		if s.TotalFindings != 0 || s.Errors != 0 || s.Warnings != 0 || s.Info != 0 {
			t.Errorf("expected all zeros for empty findings, got %+v", s)
		}
		if s.FilesScanned != 5 {
			t.Errorf("expected 5 files scanned, got %d", s.FilesScanned)
		}
		if s.RulesApplied != 2 {
			t.Errorf("expected 2 rules applied, got %d", s.RulesApplied)
		}
	})
}
