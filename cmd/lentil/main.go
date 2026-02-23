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
	"time"

	"github.com/briandowns/spinner"
	"github.com/spf13/cobra"

	"github.com/Haizzz/lentil/internal/config"
	"github.com/Haizzz/lentil/internal/engine"
	"github.com/Haizzz/lentil/internal/lint"
	"github.com/Haizzz/lentil/internal/llm"
	"github.com/Haizzz/lentil/internal/output"
)

var version = "dev"

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
		Version:       version,
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
	switch flagFormat {
	case "text", "json", "sarif":
	default:
		return fmt.Errorf("unknown output format %q: must be text, json, or sarif", flagFormat)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt)
		<-sig
		fmt.Fprintf(os.Stderr, "\nForce exit\n")
		os.Exit(130)
	}()

	quiet := flagQuiet
	s := spinner.New(spinner.CharSets[14], 80*time.Millisecond, spinner.WithWriter(os.Stderr))

	status := func(msg string) {
		if !quiet {
			s.Suffix = " " + msg
			if !s.Active() {
				s.Start()
			}
		}
	}
	clearStatus := func() {
		if !quiet && s.Active() {
			s.Stop()
		}
	}

	status("Discovering config...")
	configExplicit := cmd.Flags().Changed("config")
	cfg, rules, walker, err := config.Resolve(flagConfig, configExplicit)
	if err != nil {
		clearStatus()

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
			clearStatus()

			return fmt.Errorf("no matching rules found for: %v", flagRules)
		}
		rules = filtered
	}

	minSeverity, err := lint.ParseSeverity(flagSeverity)
	if err != nil {
		clearStatus()

		return err
	}
	minRank := lint.SeverityRank(minSeverity)

	apiKey := config.ResolveAPIKey()
	client := llm.NewClient(cfg.LLM.BaseURL, cfg.LLM.Model, apiKey, cfg.LLM.Temperature, cfg.LLM.MaxTokens)

	var targets []string
	for _, arg := range args {
		abs, err := filepath.Abs(arg)
		if err != nil {
			clearStatus()

			return fmt.Errorf("resolving path %q: %w", arg, err)
		}
		targets = append(targets, abs)
	}

	var progress engine.ProgressFunc
	if !quiet {
		progress = func(file, rule string, done, total int) {
			status(fmt.Sprintf("Analyzing (%d/%d) %s — %s", done, total, filepath.Base(file), rule))
		}
	}

	eng := engine.NewEngine(client, rules, cfg.Settings, walker, targets, progress, status)
	findings, filesScanned, warnings, err := eng.Run(ctx)
	if err != nil {
		clearStatus()

		return err
	}

	for _, w := range warnings {
		status(fmt.Sprintf("warning: %v", w))
	}

	clearStatus()

	var filtered []lint.Finding
	for _, f := range findings {
		if lint.SeverityRank(f.Severity) >= minRank {
			filtered = append(filtered, f)
		}
	}
	findings = filtered

	summary := lint.NewSummary(findings, filesScanned, len(rules))

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

	return f, func() {
		if err := f.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "warning: closing output file: %v\n", err)
		}
	}, nil
}
