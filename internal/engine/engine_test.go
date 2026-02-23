package engine

import (
	"path/filepath"
	"testing"

	"github.com/Haizzz/lentil/internal/lint"
)

func TestDedup(t *testing.T) {
	findings := []lint.Finding{
		{File: "a.go", Line: 1, Rule: "r1", Message: "msg1"},
		{File: "a.go", Line: 1, Rule: "r1", Message: "msg1"}, // duplicate
		{File: "a.go", Line: 2, Rule: "r1", Message: "msg2"},
		{File: "b.go", Line: 1, Rule: "r1", Message: "msg1"}, // different file
	}

	got := dedup(findings)
	if len(got) != 3 {
		t.Fatalf("expected 3 unique findings, got %d", len(got))
	}
}

func TestDedup_Empty(t *testing.T) {
	got := dedup(nil)
	if len(got) != 0 {
		t.Fatalf("expected 0 findings for nil input, got %d", len(got))
	}
}

func TestDedup_PreservesOrder(t *testing.T) {
	findings := []lint.Finding{
		{File: "a.go", Line: 1, Rule: "r1", Message: "msg", Severity: "error"},
		{File: "a.go", Line: 1, Rule: "r1", Message: "msg", Severity: "warning"}, // duplicate, different severity
	}

	got := dedup(findings)
	if len(got) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(got))
	}
	if got[0].Severity != "error" {
		t.Errorf("expected first occurrence (error) to win, got %s", got[0].Severity)
	}
}

func TestFilterByTargets(t *testing.T) {
	sep := string(filepath.Separator)
	files := []string{
		"src" + sep + "main.go",
		"src" + sep + "util.go",
		"lib" + sep + "helper.go",
		"README.md",
	}

	t.Run("prefix match", func(t *testing.T) {
		got := filterByTargets(files, []string{"src"})
		if len(got) != 2 {
			t.Fatalf("expected 2 files under src/, got %d", len(got))
		}
	})

	t.Run("exact match", func(t *testing.T) {
		got := filterByTargets(files, []string{"README.md"})
		if len(got) != 1 || got[0] != "README.md" {
			t.Fatalf("expected [README.md], got %v", got)
		}
	})

	t.Run("no match", func(t *testing.T) {
		got := filterByTargets(files, []string{"nonexistent"})
		if len(got) != 0 {
			t.Fatalf("expected 0 files, got %d", len(got))
		}
	})

	t.Run("empty targets", func(t *testing.T) {
		got := filterByTargets(files, nil)
		if len(got) != 0 {
			t.Fatalf("expected 0 files for empty targets, got %d", len(got))
		}
	})

	t.Run("multiple targets", func(t *testing.T) {
		got := filterByTargets(files, []string{"src", "README.md"})
		if len(got) != 3 {
			t.Fatalf("expected 3 files, got %d", len(got))
		}
	})
}

func TestIsBinary(t *testing.T) {
	t.Run("text content", func(t *testing.T) {
		if isBinary([]byte("hello world\nline two\n")) {
			t.Error("text content should not be binary")
		}
	})

	t.Run("binary content", func(t *testing.T) {
		content := []byte("hello\x00world")
		if !isBinary(content) {
			t.Error("content with null byte should be binary")
		}
	})

	t.Run("empty content", func(t *testing.T) {
		if isBinary([]byte{}) {
			t.Error("empty content should not be binary")
		}
	})

	t.Run("null byte beyond 8KB limit", func(t *testing.T) {
		content := make([]byte, binaryCheckLimit+100)
		for i := range content {
			content[i] = 'a'
		}
		content[binaryCheckLimit+50] = 0 // null byte past the check limit
		if isBinary(content) {
			t.Error("null byte beyond 8KB check limit should not trigger binary detection")
		}
	})

	t.Run("null byte within 8KB limit", func(t *testing.T) {
		content := make([]byte, binaryCheckLimit+100)
		for i := range content {
			content[i] = 'a'
		}
		content[binaryCheckLimit-1] = 0 // null byte at end of check window
		if !isBinary(content) {
			t.Error("null byte within 8KB check limit should trigger binary detection")
		}
	})
}
