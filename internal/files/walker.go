package files

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5"
	gitignore "github.com/go-git/go-git/v5/plumbing/format/gitignore"
)

// Walker provides gitignore-aware file discovery over a directory tree.
type Walker struct {
	root   string
	ignore gitignore.Matcher
}

// NewWalker creates a Walker rooted at the given directory. It loads all
// .gitignore files under root and builds a matcher for filtering.
func NewWalker(root string) (*Walker, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	fs := osfs.New(absRoot)
	patterns, err := gitignore.ReadPatterns(fs, nil)
	if err != nil {
		patterns = nil
	}

	var matcher gitignore.Matcher
	if len(patterns) > 0 {
		matcher = gitignore.NewMatcher(patterns)
	}

	return &Walker{root: absRoot, ignore: matcher}, nil
}

// NewWalkerWithoutGitignore returns a Walker rooted at root without loading
// .gitignore files. Use when the CLI already names the files to lint: there is
// no recursive ignore scan, and explicitly listed paths (including normally
// ignored files) are eligible. The .git directory is still excluded from Glob
// and [Walker.IsLintablePath].
func NewWalkerWithoutGitignore(root string) (*Walker, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	return &Walker{root: absRoot, ignore: nil}, nil
}

// Root returns the absolute root directory of the walker.
func (w *Walker) Root() string {
	return w.root
}

// Glob finds files matching pattern under base, applying gitignore rules.
// Returns absolute paths sorted by directory depth.
func (w *Walker) Glob(base, pattern string) ([]string, error) {
	if base == "" {
		base = w.root
	}

	fsys := os.DirFS(base)
	matches, err := doublestar.Glob(fsys, pattern)
	if err != nil {
		return nil, err
	}

	var result []string
	for _, m := range matches {
		fullPath := filepath.Join(base, m)

		info, err := os.Stat(fullPath)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}

		rel, err := filepath.Rel(w.root, fullPath)
		if err == nil {
			segments := strings.Split(filepath.ToSlash(rel), "/")
			if segments[0] == ".git" {
				continue
			}
			if w.ignore != nil && w.ignore.Match(segments, false) {
				continue
			}
		}

		result = append(result, fullPath)
	}

	sort.Slice(result, func(i, j int) bool {
		di := strings.Count(filepath.ToSlash(result[i]), "/")
		dj := strings.Count(filepath.ToSlash(result[j]), "/")
		if di != dj {
			return di < dj
		}

		return result[i] < result[j]
	})

	return result, nil
}

func pathWithinBase(base, path string) bool {
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return false
	}

	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// IsLintablePath reports whether absPath is a regular file under the walker's
// root that passes the same .git and gitignore checks as [Walker.Glob].
func (w *Walker) IsLintablePath(absPath string) bool {
	info, err := os.Stat(absPath)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}

	if !pathWithinBase(w.root, absPath) {
		return false
	}

	rel, err := filepath.Rel(w.root, absPath)
	if err != nil {
		return false
	}

	segments := strings.Split(filepath.ToSlash(rel), "/")
	if len(segments) > 0 && segments[0] == ".git" {
		return false
	}

	if w.ignore != nil && w.ignore.Match(segments, false) {
		return false
	}

	return true
}

// ExpandTargets turns absolute file and directory paths into a deduplicated,
// sorted list of absolute file paths. Directories are expanded with
// [Walker.Glob] using "**/*". Paths that are not lintable (ignored, non-regular,
// or outside the walker's root) are skipped for files; missing paths return an error.
func (w *Walker) ExpandTargets(absTargets []string) ([]string, error) {
	seen := make(map[string]struct{})
	var out []string

	for _, t := range absTargets {
		fi, err := os.Stat(t)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", t, err)
		}

		if fi.IsDir() {
			matches, err := w.Glob(t, "**/*")
			if err != nil {
				return nil, err
			}

			for _, f := range matches {
				if _, ok := seen[f]; !ok {
					seen[f] = struct{}{}
					out = append(out, f)
				}
			}

			continue
		}

		if !w.IsLintablePath(t) {
			continue
		}

		if _, ok := seen[t]; !ok {
			seen[t] = struct{}{}
			out = append(out, t)
		}
	}

	sort.Strings(out)

	return out, nil
}

// FileMatchesGlob reports whether absFile is under base and matches pattern
// relative to base (same semantics as [Walker.Glob] with os.DirFS(base)).
// An empty base uses the walker's root.
func (w *Walker) FileMatchesGlob(base, pattern, absFile string) (bool, error) {
	if base == "" {
		base = w.root
	}

	if !pathWithinBase(base, absFile) {
		return false, nil
	}

	rel, err := filepath.Rel(base, absFile)
	if err != nil {
		return false, nil
	}

	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false, nil
	}

	return doublestar.Match(filepath.ToSlash(pattern), filepath.ToSlash(rel))
}

// FindRoot detects the project root directory. If cwd is inside a git
// repository, returns the repo root. Otherwise returns cwd.
func FindRoot(cwd string) (string, error) {
	absCwd, err := filepath.Abs(cwd)
	if err != nil {
		return "", err
	}

	repo, err := git.PlainOpenWithOptions(absCwd, &git.PlainOpenOptions{
		DetectDotGit: true,
	})
	if err != nil {
		return absCwd, nil
	}

	wt, err := repo.Worktree()
	if err != nil {
		return absCwd, nil
	}

	return wt.Filesystem.Root(), nil
}
