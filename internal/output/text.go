package output

import (
	"fmt"
	"io"

	"github.com/fatih/color"

	"github.com/anhle/lentil/internal/types"
)

// Text writes findings in a human-readable format with colors.
func Text(w io.Writer, findings []types.Finding, summary types.Summary) {
	sevColor := map[types.Severity]*color.Color{
		types.SeverityError:   color.New(color.FgRed, color.Bold),
		types.SeverityWarning: color.New(color.FgYellow),
		types.SeverityInfo:    color.New(color.FgCyan),
	}

	for _, f := range findings {
		c := sevColor[f.Severity]
		if c == nil {
			c = color.New(color.Reset)
		}

		col := f.Column
		if col == 0 {
			col = 1
		}

		// file:line:col: severity[rule] message
		fmt.Fprintf(w, "%s:%d:%d: ", f.File, f.Line, col)
		c.Fprintf(w, "%s", f.Severity)
		fmt.Fprintf(w, "[%s] %s\n", f.Rule, f.Message)
	}

	if len(findings) > 0 {
		fmt.Fprintln(w)
	}

	fmt.Fprintf(w, "%d findings (%d errors, %d warnings, %d info) in %d files\n",
		summary.TotalFindings, summary.Errors, summary.Warnings, summary.Info, summary.FilesScanned)
}
