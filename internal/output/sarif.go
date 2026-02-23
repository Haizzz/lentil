package output

import (
	"io"

	"github.com/owenrumney/go-sarif/v3/pkg/report"
	"github.com/owenrumney/go-sarif/v3/pkg/report/v210/sarif"

	"github.com/anhle/lentil/internal/lint"
)

// SARIF writes findings in SARIF v2.1.0 format.
func SARIF(w io.Writer, findings []lint.Finding, rules []lint.Rule) error {
	r := report.NewV210Report()

	run := sarif.NewRunWithInformationURI("lentil", "https://github.com/anhle/lentil")

	// Add rules as reporting descriptors
	for _, rule := range rules {
		run.AddRule(rule.ID).WithDescription(rule.Prompt)
	}

	// Add results
	for _, f := range findings {
		level := mapSeverity(f.Severity)
		result := run.CreateResultForRule(f.Rule).
			WithLevel(level).
			WithMessage(sarif.NewTextMessage(f.Message))

		loc := sarif.NewPhysicalLocation().
			WithArtifactLocation(sarif.NewSimpleArtifactLocation(f.File)).
			WithRegion(sarif.NewSimpleRegion(f.Line, f.Line))

		if f.Column > 0 {
			loc.Region.StartColumn = &f.Column
		}

		result.AddLocation(
			sarif.NewLocationWithPhysicalLocation(loc),
		)
	}

	r.AddRun(run)

	return r.Write(w)
}

func mapSeverity(s lint.Severity) string {
	switch s {
	case lint.SeverityError:
		return "error"
	case lint.SeverityWarning:
		return "warning"
	case lint.SeverityInfo:
		return "note"
	default:
		return "none"
	}
}
