package engine

import (
	"fmt"
	"strings"

	"github.com/anhle/lentil/internal/types"
)

// ChunkFile splits a file's lines into chunks with overlap, preserving absolute line numbers.
// Each chunk's Content is formatted with line number prefixes.
func ChunkFile(filePath string, lines []string, chunkLines, chunkOverlap int) []types.Chunk {
	totalLines := len(lines)
	if totalLines == 0 {
		return nil
	}

	// If the file fits in a single chunk, don't split
	if totalLines <= chunkLines {
		return []types.Chunk{{
			FilePath:   filePath,
			StartLine:  1,
			EndLine:    totalLines,
			TotalLines: totalLines,
			Content:    formatLines(lines, 1),
		}}
	}

	var chunks []types.Chunk
	step := chunkLines - chunkOverlap
	if step <= 0 {
		step = 1
	}

	for start := 0; start < totalLines; start += step {
		end := start + chunkLines
		if end > totalLines {
			end = totalLines
		}

		chunk := types.Chunk{
			FilePath:   filePath,
			StartLine:  start + 1, // 1-based
			EndLine:    end,        // 1-based inclusive
			TotalLines: totalLines,
			Content:    formatLines(lines[start:end], start+1),
		}
		chunks = append(chunks, chunk)

		// If we've reached the end, stop
		if end >= totalLines {
			break
		}
	}

	return chunks
}

// formatLines formats lines with absolute line number prefixes.
func formatLines(lines []string, startLine int) string {
	var b strings.Builder
	width := len(fmt.Sprintf("%d", startLine+len(lines)-1))
	for i, line := range lines {
		fmt.Fprintf(&b, "%*d | %s\n", width, startLine+i, line)
	}
	return b.String()
}
