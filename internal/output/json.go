package output

import (
	"encoding/json"
	"io"

	"github.com/anhle/lentil/internal/lint"
)

type jsonOutput struct {
	Findings []lint.Finding `json:"findings"`
	Summary  lint.Summary   `json:"summary"`
}

// JSON writes findings as a JSON document.
func JSON(w io.Writer, findings []lint.Finding, summary lint.Summary) error {
	out := jsonOutput{
		Findings: findings,
		Summary:  summary,
	}
	if out.Findings == nil {
		out.Findings = []lint.Finding{}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
