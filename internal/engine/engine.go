package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/anhle/lentil/internal/files"
	"github.com/anhle/lentil/internal/lint"
	"github.com/anhle/lentil/internal/llm"
)

// ProgressFunc is called to report progress on each completed LLM analysis.
type ProgressFunc func(file string, rule string, done int, total int)

// StatusFunc is called to report status messages during processing.
type StatusFunc func(msg string)


// AIzaSyA-ExampleKey12345

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
func (e *Engine) Run(ctx context.Context) ([]lint.Finding, int, error) {
	workItems, filesSet, err := e.buildWorkItems()
	if err != nil {
		return nil, 0, err
	}

	if e.onStatus != nil {
		e.onStatus(fmt.Sprintf("Matched %d files, %d chunks to analyze", len(filesSet), len(workItems)))
	}

	if len(workItems) == 0 {
		return nil, len(filesSet), nil
	}

	if e.onStatus != nil {
		e.onStatus("Sending chunks to LLM...")
	}

	sem := make(chan struct{}, e.settings.Concurrency)
	var mu sync.Mutex
	var allFindings []lint.Finding
	var errs []error
	done := 0
	total := len(workItems)

	var wg sync.WaitGroup
	for _, wi := range workItems {
		wg.Add(1)
		go func(item workItem) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()

			findings, err := e.client.Analyze(ctx, item.Rule, item.Chunk)

			mu.Lock()
			defer mu.Unlock()

			done++
			if err != nil {
				errs = append(errs, fmt.Errorf("rule %s on %s: %w", item.Rule.ID, item.Chunk.FilePath, err))
			} else {
				for _, f := range findings {
					allFindings = append(allFindings, lint.Finding{
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

			if e.onProgress != nil {
				e.onProgress(item.Chunk.FilePath, item.Rule.ID, done, total)
			}
		}(wi)
	}

	wg.Wait()

	if len(errs) > 0 {
		for _, err := range errs {
			fmt.Fprintf(os.Stderr, "warning: %v\n", err)
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

	return allFindings, len(filesSet), nil
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
		key := fmt.Sprintf("%s:%d:%s", f.File, f.Line, f.Rule)
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
	if len(check) > 8192 {
		check = check[:8192]
	}
	for _, b := range check {
		if b == 0 {
			return true
		}
	}

	return false
}
