package files

import (
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
		if err != nil || info.IsDir() {
			continue
		}

		if w.ignore != nil {
			rel, err := filepath.Rel(w.root, fullPath)
			if err == nil {
				segments := strings.Split(filepath.ToSlash(rel), "/")
				if w.ignore.Match(segments, false) {
					continue
				}
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
