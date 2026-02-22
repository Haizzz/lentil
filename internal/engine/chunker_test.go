package engine

import (
	"strings"
	"testing"
)

func TestChunkFile_SmallFile(t *testing.T) {
	lines := []string{"line1", "line2", "line3"}
	chunks := ChunkFile("test.py", lines, 300, 20)

	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	c := chunks[0]
	if c.StartLine != 1 || c.EndLine != 3 {
		t.Errorf("expected lines 1-3, got %d-%d", c.StartLine, c.EndLine)
	}
	if c.TotalLines != 3 {
		t.Errorf("expected TotalLines=3, got %d", c.TotalLines)
	}
	if !strings.Contains(c.Content, "1 | line1") {
		t.Errorf("content should contain line numbers, got: %s", c.Content)
	}
}

func TestChunkFile_EmptyFile(t *testing.T) {
	chunks := ChunkFile("empty.py", nil, 300, 20)
	if len(chunks) != 0 {
		t.Fatalf("expected 0 chunks for empty file, got %d", len(chunks))
	}
}

func TestChunkFile_ExactChunkSize(t *testing.T) {
	lines := make([]string, 300)
	for i := range lines {
		lines[i] = "code"
	}
	chunks := ChunkFile("exact.py", lines, 300, 20)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk for file exactly chunk_lines, got %d", len(chunks))
	}
}

func TestChunkFile_LargeFile(t *testing.T) {
	lines := make([]string, 600)
	for i := range lines {
		lines[i] = "code"
	}
	chunks := ChunkFile("large.py", lines, 300, 20)

	if len(chunks) < 2 {
		t.Fatalf("expected at least 2 chunks, got %d", len(chunks))
	}

	// First chunk should be lines 1-300
	if chunks[0].StartLine != 1 || chunks[0].EndLine != 300 {
		t.Errorf("chunk 0: expected 1-300, got %d-%d", chunks[0].StartLine, chunks[0].EndLine)
	}

	// Second chunk should start at 281 (300 - 20 + 1) with overlap
	if chunks[1].StartLine != 281 {
		t.Errorf("chunk 1: expected start=281, got %d", chunks[1].StartLine)
	}

	// Last chunk should end at 600
	last := chunks[len(chunks)-1]
	if last.EndLine != 600 {
		t.Errorf("last chunk: expected end=600, got %d", last.EndLine)
	}

	// All chunks should have correct TotalLines
	for i, c := range chunks {
		if c.TotalLines != 600 {
			t.Errorf("chunk %d: expected TotalLines=600, got %d", i, c.TotalLines)
		}
	}
}

func TestChunkFile_OverlapCorrectness(t *testing.T) {
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = "line"
	}
	chunks := ChunkFile("overlap.py", lines, 50, 10)

	// With 100 lines, chunk_lines=50, overlap=10, step=40:
	// Chunk 0: 1-50
	// Chunk 1: 41-90
	// Chunk 2: 81-100
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}

	// Check overlap between chunk 0 and 1
	if chunks[1].StartLine != 41 {
		t.Errorf("chunk 1 should start at 41, got %d", chunks[1].StartLine)
	}

	// Lines 41-50 should be in both chunk 0 and chunk 1
	if chunks[0].EndLine < 50 || chunks[1].StartLine > 41 {
		t.Error("chunks should overlap at lines 41-50")
	}
}

func TestChunkFile_LineNumbersInContent(t *testing.T) {
	lines := []string{"alpha", "beta", "gamma"}
	chunks := ChunkFile("test.py", lines, 300, 20)
	content := chunks[0].Content

	if !strings.Contains(content, "1 | alpha") {
		t.Errorf("expected '1 | alpha' in content, got: %s", content)
	}
	if !strings.Contains(content, "2 | beta") {
		t.Errorf("expected '2 | beta' in content, got: %s", content)
	}
	if !strings.Contains(content, "3 | gamma") {
		t.Errorf("expected '3 | gamma' in content, got: %s", content)
	}
}
