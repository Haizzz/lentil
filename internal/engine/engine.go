package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/anhle/lentil/internal/files"
	"github.com/anhle/lentil/internal/lint"
	"github.com/anhle/lentil/internal/llm"
)

const binaryCheckLimit = 8192

// ProgressFunc is called to report progress on each completed LLM analysis.
type ProgressFunc func(file string, rule string, done int, total int)

// StatusFunc is called to report status messages during processing.
type StatusFunc func(msg string)

// Engine orchestrates the linting process.
type Engine struct {
	client     *llm.Client
	rules      []lint.Rule
	settings   lint.SettingsConfig
	walker     *files.Walker
	targets    []string
	onProgress ProgressFunc
	onStatus   StatusFunc
}

type workItem struct {
	Rule  lint.Rule
	Chunk lint.Chunk
}

type result struct {
	Findings []lint.Finding
	Err      error
	File     string
	RuleID   string
}

// NewEngine creates a new Engine. If targets is non-empty, only files under
// those paths (files or directories) are linted.
func NewEngine(client *llm.Client, rules []lint.Rule, settings lint.SettingsConfig, walker *files.Walker, targets []string, onProgress ProgressFunc, onStatus StatusFunc) *Engine {
	return &Engine{
		client:     client,
		rules:      rules,
		settings:   settings,
		walker:     walker,
		targets:    targets,
		onProgress: onProgress,
		onStatus:   onStatus,
	}
}

// Run executes all rules against matched files and returns findings.
// Warnings contains non-fatal errors from individual LLM analyses.
func (e *Engine) Run(ctx context.Context) ([]lint.Finding, int, []error, error) {
	workItems, filesSet, err := e.buildWorkItems()
	if err != nil {
		return nil, 0, nil, err
	}

	if e.onStatus != nil {
		e.onStatus(fmt.Sprintf("Matched %d files, %d chunks to analyze", len(filesSet), len(workItems)))
	}

	if len(workItems) == 0 {
		return nil, len(filesSet), nil, nil
	}

	work := make(chan workItem)
	results := make(chan result)
	total := len(workItems)

	for range e.settings.Concurrency {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					results <- result{Err: fmt.Errorf("panic: %v", r)}
				}
			}()
			for item := range work {
				findings, err := e.client.Analyze(ctx, item.Rule, item.Chunk)
				var mapped []lint.Finding
				if err == nil {
					for _, f := range findings {
						mapped = append(mapped, lint.Finding{
							File:     item.Chunk.FilePath,
							Line:     f.Line,
							Column:   f.Column,
							Rule:     item.Rule.ID,
							Severity: item.Rule.Severity,
							Message:  f.Message,
							Snippet:  f.Snippet,
						})
					}
				}
				results <- result{Findings: mapped, Err: err, File: item.Chunk.FilePath, RuleID: item.Rule.ID}
			}
		}()
	}

	go func() {
		for _, wi := range workItems {
			work <- wi
		}
		close(work)
	}()

	var allFindings []lint.Finding
	var warnings []error
	for done := range total {
		r := <-results
		if r.Err != nil {
			warnings = append(warnings, fmt.Errorf("rule %s on %s: %w", r.RuleID, r.File, r.Err))
		} else {
			allFindings = append(allFindings, r.Findings...)
		}
		if e.onProgress != nil {
			e.onProgress(r.File, r.RuleID, done+1, total)
		}
	}

	allFindings = dedup(allFindings)

	sort.Slice(allFindings, func(i, j int) bool {
		if allFindings[i].File != allFindings[j].File {
			return allFindings[i].File < allFindings[j].File
		}
		if allFindings[i].Line != allFindings[j].Line {
			return allFindings[i].Line < allFindings[j].Line
		}

		return allFindings[i].Rule < allFindings[j].Rule
	})

	return allFindings, len(filesSet), warnings, nil
}

func (e *Engine) buildWorkItems() ([]workItem, map[string]struct{}, error) {
	var workItems []workItem
	filesSet := make(map[string]struct{})
	chunkCache := make(map[string][]lint.Chunk)

	for _, rule := range e.rules {
		base := rule.Scope
		if base == "" {
			base = e.walker.Root()
		}

		matched, err := e.walker.Glob(base, rule.Glob)
		if err != nil {
			return nil, nil, fmt.Errorf("globbing for rule %s: %w", rule.ID, err)
		}

		if len(e.targets) > 0 {
			matched = filterByTargets(matched, e.targets)
		}

		for _, file := range matched {
			filesSet[file] = struct{}{}

			chunks, ok := chunkCache[file]
			if !ok {
				content, err := os.ReadFile(file)
				if err != nil {
					return nil, nil, fmt.Errorf("reading %s: %w", file, err)
				}

				if len(content) == 0 || isBinary(content) {
					chunkCache[file] = nil
					continue
				}

				lines := strings.Split(string(content), "\n")
				chunks = ChunkFile(file, lines, e.settings.ChunkLines, e.settings.ChunkOverlap)
				chunkCache[file] = chunks
			}

			for _, chunk := range chunks {
				workItems = append(workItems, workItem{
					Rule:  rule,
					Chunk: chunk,
				})
			}
		}
	}

	return workItems, filesSet, nil
}

func dedup(findings []lint.Finding) []lint.Finding {
	seen := make(map[string]struct{})
	var result []lint.Finding
	for _, f := range findings {
		key := fmt.Sprintf("%s:%d:%s:%s", f.File, f.Line, f.Rule, f.Message)
		if _, ok := seen[key]; !ok {
			seen[key] = struct{}{}
			result = append(result, f)
		}
	}

	return result
}

func filterByTargets(files []string, targets []string) []string {
	var result []string
	for _, f := range files {
		for _, t := range targets {
			if f == t || strings.HasPrefix(f, t+string(filepath.Separator)) {
				result = append(result, f)
				break
			}
		}
	}

	return result
}

func isBinary(content []byte) bool {
	check := content
	if len(check) > binaryCheckLimit {
		check = check[:binaryCheckLimit]
	}
	for _, b := range check {
		if b == 0 {
			return true
		}
	}

	return false
}
