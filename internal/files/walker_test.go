package files

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestWalker_Glob_Basic(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.go"), "package a")
	writeFile(t, filepath.Join(dir, "b.py"), "print('hi')")
	writeFile(t, filepath.Join(dir, "sub", "c.go"), "package sub")

	w, err := NewWalker(dir)
	if err != nil {
		t.Fatal(err)
	}

	files, err := w.Glob(dir, "**/*.go")
	if err != nil {
		t.Fatalf("Glob failed: %v", err)
	}

	if len(files) != 2 {
		t.Fatalf("expected 2 .go files, got %d: %v", len(files), files)
	}
}

func TestWalker_Glob_GitignoreFiltering(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".gitignore"), "build/\n*.log\n")
	writeFile(t, filepath.Join(dir, "main.go"), "package main")
	writeFile(t, filepath.Join(dir, "debug.log"), "log content")
	writeFile(t, filepath.Join(dir, "build", "output.go"), "package build")
	writeFile(t, filepath.Join(dir, "src", "app.go"), "package src")

	w, err := NewWalker(dir)
	if err != nil {
		t.Fatal(err)
	}

	files, err := w.Glob(dir, "**/*")
	if err != nil {
		t.Fatalf("Glob failed: %v", err)
	}

	for _, f := range files {
		rel, _ := filepath.Rel(dir, f)
		if rel == "debug.log" {
			t.Error("debug.log should be filtered by .gitignore")
		}
		if filepath.Dir(rel) == "build" {
			t.Errorf("build/ files should be filtered by .gitignore: %s", rel)
		}
	}

	found := false
	for _, f := range files {
		rel, _ := filepath.Rel(dir, f)
		if rel == "main.go" {
			found = true
		}
	}
	if !found {
		t.Error("main.go should not be filtered")
	}
}

func TestWalker_Glob_NestedGitignore(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".gitignore"), "*.log\n")
	writeFile(t, filepath.Join(dir, "src", ".gitignore"), "*.tmp\n")
	writeFile(t, filepath.Join(dir, "main.go"), "package main")
	writeFile(t, filepath.Join(dir, "app.log"), "log")
	writeFile(t, filepath.Join(dir, "src", "app.go"), "package src")
	writeFile(t, filepath.Join(dir, "src", "cache.tmp"), "tmp")

	w, err := NewWalker(dir)
	if err != nil {
		t.Fatal(err)
	}

	files, err := w.Glob(dir, "**/*")
	if err != nil {
		t.Fatalf("Glob failed: %v", err)
	}

	for _, f := range files {
		rel, _ := filepath.Rel(dir, f)
		if rel == "app.log" {
			t.Error("app.log should be filtered by root .gitignore")
		}
		if rel == filepath.Join("src", "cache.tmp") {
			t.Error("src/cache.tmp should be filtered by nested .gitignore")
		}
	}
}

func TestWalker_Glob_NoGitignore(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.txt"), "hello")
	writeFile(t, filepath.Join(dir, "b.txt"), "world")

	w, err := NewWalker(dir)
	if err != nil {
		t.Fatal(err)
	}

	files, err := w.Glob(dir, "**/*.txt")
	if err != nil {
		t.Fatalf("Glob failed: %v", err)
	}

	if len(files) != 2 {
		t.Fatalf("expected 2 files (no gitignore), got %d", len(files))
	}
}

func TestWalker_Glob_SortedByDepth(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a", "b", "deep.toml"), "")
	writeFile(t, filepath.Join(dir, "root.toml"), "")
	writeFile(t, filepath.Join(dir, "a", "mid.toml"), "")

	w, err := NewWalker(dir)
	if err != nil {
		t.Fatal(err)
	}

	files, err := w.Glob(dir, "**/*.toml")
	if err != nil {
		t.Fatalf("Glob failed: %v", err)
	}

	if len(files) != 3 {
		t.Fatalf("expected 3 files, got %d", len(files))
	}

	rel0, _ := filepath.Rel(dir, files[0])
	if rel0 != "root.toml" {
		t.Errorf("first file should be shallowest (root.toml), got %q", rel0)
	}
}

func TestWalker_Glob_SkipsDotGit(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "main.go"), "package main")
	writeFile(t, filepath.Join(dir, ".git", "config"), "[core]")
	writeFile(t, filepath.Join(dir, ".git", "HEAD"), "ref: refs/heads/main")

	w, err := NewWalker(dir)
	if err != nil {
		t.Fatal(err)
	}

	files, err := w.Glob(dir, "**/*")
	if err != nil {
		t.Fatalf("Glob failed: %v", err)
	}

	for _, f := range files {
		rel, _ := filepath.Rel(dir, f)
		if len(rel) >= 4 && rel[:4] == ".git" {
			t.Errorf(".git files should be excluded, got %s", rel)
		}
	}

	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d: %v", len(files), files)
	}
}

func TestFindRoot_NonGitDir(t *testing.T) {
	dir := t.TempDir()

	root, err := FindRoot(dir)
	if err != nil {
		t.Fatalf("FindRoot failed: %v", err)
	}

	if root != dir {
		t.Errorf("expected cwd fallback %q, got %q", dir, root)
	}
}
