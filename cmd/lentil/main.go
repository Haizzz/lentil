// Copyright (c) 2025 anhle. MIT License.

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/anhle/lentil/internal/config"
	"github.com/anhle/lentil/internal/engine"
	"github.com/anhle/lentil/internal/lint"
	"github.com/anhle/lentil/internal/llm"
	"github.com/anhle/lentil/internal/output"
)

var (
	flagConfig   string
	flagFormat   string
	flagRules    []string
	flagSeverity string
	flagQuiet    bool
	flagOutput   string
)

func main() {
	rootCmd := &cobra.Command{
		Use:           "lentil",
		Short:         "LLM-powered linter with natural language rules",
		Long:          "lentil uses LLM models to lint source code against rules defined as natural language prompts.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	lintCmd := &cobra.Command{
		Use:   "lint [flags] [paths...]",
		Short: "Run linting rules against source files",
		RunE:  run,
	}

	lintCmd.Flags().StringVarP(&flagConfig, "config", "c", "", "Config file path (default: discover from git root or cwd)")
	lintCmd.Flags().StringVarP(&flagFormat, "format", "f", "text", "Output format: text|json|sarif")
	lintCmd.Flags().StringSliceVarP(&flagRules, "rule", "r", nil, "Run only specific rules (comma-separated)")
	lintCmd.Flags().StringVarP(&flagSeverity, "severity", "s", "info", "Minimum severity to report: info|warning|error")
	lintCmd.Flags().BoolVarP(&flagQuiet, "quiet", "q", false, "Suppress progress output")
	lintCmd.Flags().StringVarP(&flagOutput, "output", "o", "", "Write results to file (default: stdout)")

	rootCmd.AddCommand(lintCmd)

	if err := rootCmd.Execute(); err != nil {
		var ee *exitError
		if errors.As(err, &ee) {
			os.Exit(ee.code)
		}
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(2)
	}
}

func run(cmd *cobra.Command, args []string) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	var statusLines int
	status := func(msg string) {
		if !flagQuiet {
			fmt.Fprintf(os.Stderr, "• %s\n", msg)
			statusLines++
		}
	}
	clearStatus := func() {
		if !flagQuiet {
			for range statusLines {
				fmt.Fprintf(os.Stderr, "\033[A\033[2K")
			}
			statusLines = 0
		}
	}

	status("Discovering config...")
	configExplicit := cmd.Flags().Changed("config")
	cfg, rules, walker, err := config.Resolve(flagConfig, configExplicit)
	if err != nil {
		return err
	}
	status(fmt.Sprintf("Loaded %d rules from %s", len(rules), walker.Root()))

	if len(flagRules) > 0 {
		allowed := make(map[string]bool, len(flagRules))
		for _, r := range flagRules {
			allowed[r] = true
		}
		var filtered []lint.Rule
		for _, r := range rules {
			if allowed[r.ID] {
				filtered = append(filtered, r)
			}
		}
		if len(filtered) == 0 {
			return fmt.Errorf("no matching rules found for: %v", flagRules)
		}
		rules = filtered
	}

	minSeverity, err := lint.ParseSeverity(flagSeverity)
	if err != nil {
		return err
	}
	minRank := lint.SeverityRank(minSeverity)

	apiKey := config.ResolveAPIKey()
	client := llm.NewClient(cfg.LLM.BaseURL, cfg.LLM.Model, apiKey, cfg.LLM.Temperature, cfg.LLM.MaxTokens)

	statusCleared := false
	var progress engine.ProgressFunc
	if !flagQuiet {
		progress = func(file, rule string, done, total int) {
			if !statusCleared {
				clearStatus()
				statusCleared = true
			}
			fmt.Fprintf(os.Stderr, "\r  [%d/%d] %s — %s", done, total, file, rule)
			if done == total {
				fmt.Fprintln(os.Stderr)
			}
		}
	}

	var targets []string
	for _, arg := range args {
		abs, err := filepath.Abs(arg)
		if err != nil {
			return fmt.Errorf("resolving path %q: %w", arg, err)
		}
		targets = append(targets, abs)
	}

	eng := engine.NewEngine(client, rules, cfg.Settings, walker, targets, progress, status)
	findings, filesScanned, err := eng.Run(ctx)
	if err != nil {
		return err
	}
	status(fmt.Sprintf("Analyzed %d files, found %d findings", filesScanned, len(findings)))

	var filtered []lint.Finding
	for _, f := range findings {
		if lint.SeverityRank(f.Severity) >= minRank {
			filtered = append(filtered, f)
		}
	}
	findings = filtered

	summary := lint.Summary{
		FilesScanned:  filesScanned,
		RulesApplied:  len(rules),
		TotalFindings: len(findings),
	}
	for _, f := range findings {
		switch f.Severity {
		case lint.SeverityError:
			summary.Errors++
		case lint.SeverityWarning:
			summary.Warnings++
		case lint.SeverityInfo:
			summary.Info++
		}
	}

	w, cleanup, err := openOutput(flagOutput)
	if err != nil {
		return err
	}
	defer cleanup()

	switch flagFormat {
	case "text":
		output.Text(w, findings, summary)
	case "json":
		if err := output.JSON(w, findings, summary); err != nil {
			return fmt.Errorf("writing JSON output: %w", err)
		}
	case "sarif":
		if err := output.SARIF(w, findings, rules); err != nil {
			return fmt.Errorf("writing SARIF output: %w", err)
		}
	default:
		return fmt.Errorf("unknown output format %q: must be text, json, or sarif", flagFormat)
	}

	if !flagQuiet && flagFormat == "text" && flagOutput != "" {
		fmt.Fprintf(os.Stderr, "Results written to %s\n", flagOutput)
	}

	if summary.TotalFindings > 0 {
		return &exitError{code: 1}
	}

	return nil
}

type exitError struct {
	code int
}

func (e *exitError) Error() string {
	return fmt.Sprintf("exit %d", e.code)
}

func openOutput(path string) (io.Writer, func(), error) {
	if path == "" {
		return os.Stdout, func() {}, nil
	}

	f, err := os.Create(path)
	if err != nil {
		return nil, nil, fmt.Errorf("creating output file: %w", err)
	}

	return f, func() { f.Close() }, nil
}
