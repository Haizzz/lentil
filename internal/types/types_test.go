package types

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
