package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/anhle/lentil/internal/llm"
	"github.com/anhle/lentil/internal/types"
)

// ProgressFunc is called to report progress on each completed work item.
type ProgressFunc func(file string, rule string, done int, total int)

// Engine orchestrates the linting process.
type Engine struct {
	client      *llm.Client
	rules       []types.Rule
	settings    types.SettingsConfig
	basePaths   []string
	onProgress  ProgressFunc
}

// NewEngine creates a new Engine.
func NewEngine(client *llm.Client, rules []types.Rule, settings types.SettingsConfig, basePaths []string, onProgress ProgressFunc) *Engine {
	if len(basePaths) == 0 {
		basePaths = []string{"."}
	}
	return &Engine{
		client:     client,
		rules:      rules,
		settings:   settings,
		basePaths:  basePaths,
		onProgress: onProgress,
	}
}

// Run executes all rules against matched files and returns findings.
func (e *Engine) Run(ctx context.Context) ([]types.Finding, int, error) {
	// Build work items
	workItems, filesSet, err := e.buildWorkItems()
	if err != nil {
		return nil, 0, err
	}

	if len(workItems) == 0 {
		return nil, len(filesSet), nil
	}

	// Fan out with bounded concurrency
	sem := make(chan struct{}, e.settings.Concurrency)
	var mu sync.Mutex
	var allFindings []types.Finding
	var errs []error
	done := 0
	total := len(workItems)

	var wg sync.WaitGroup
	for _, wi := range workItems {
		wg.Add(1)
		go func(item types.WorkItem) {
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
					allFindings = append(allFindings, types.Finding{
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
		// Log errors to stderr but don't fail — partial results are still useful
		for _, err := range errs {
			fmt.Fprintf(os.Stderr, "warning: %v\n", err)
		}
	}

	// Deduplicate findings (same file + line + rule = keep first)
	allFindings = dedup(allFindings)

	// Sort by file then line number
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

func (e *Engine) buildWorkItems() ([]types.WorkItem, map[string]struct{}, error) {
	var workItems []types.WorkItem
	filesSet := make(map[string]struct{})

	for _, rule := range e.rules {
		files, err := e.globFiles(rule.Glob)
		if err != nil {
			return nil, nil, fmt.Errorf("globbing for rule %s: %w", rule.ID, err)
		}

		for _, file := range files {
			filesSet[file] = struct{}{}

			content, err := os.ReadFile(file)
			if err != nil {
				return nil, nil, fmt.Errorf("reading %s: %w", file, err)
			}

			// Skip empty files and likely binary files
			if len(content) == 0 {
				continue
			}
			if isBinary(content) {
				continue
			}

			lines := strings.Split(string(content), "\n")
			chunks := ChunkFile(file, lines, e.settings.ChunkLines, e.settings.ChunkOverlap)

			for _, chunk := range chunks {
				workItems = append(workItems, types.WorkItem{
					Rule:  rule,
					Chunk: chunk,
				})
			}
		}
	}

	return workItems, filesSet, nil
}

func (e *Engine) globFiles(pattern string) ([]string, error) {
	var allFiles []string

	for _, base := range e.basePaths {
		fsys := os.DirFS(base)
		matches, err := doublestar.Glob(fsys, pattern)
		if err != nil {
			return nil, err
		}
		for _, m := range matches {
			fullPath := filepath.Join(base, m)

			// Check exclude patterns
			excluded := false
			for _, excl := range e.settings.Exclude {
				if ok, _ := doublestar.Match(excl, m); ok {
					excluded = true
					break
				}
			}
			if excluded {
				continue
			}

			info, err := os.Stat(fullPath)
			if err != nil || info.IsDir() {
				continue
			}

			allFiles = append(allFiles, fullPath)
		}
	}

	return allFiles, nil
}

func dedup(findings []types.Finding) []types.Finding {
	seen := make(map[string]struct{})
	var result []types.Finding
	for _, f := range findings {
		key := fmt.Sprintf("%s:%d:%s", f.File, f.Line, f.Rule)
		if _, ok := seen[key]; !ok {
			seen[key] = struct{}{}
			result = append(result, f)
		}
	}
	return result
}

// isBinary checks if content looks like a binary file by searching for null bytes in the first 8KB.
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
