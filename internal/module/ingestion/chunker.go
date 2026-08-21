// Package ingestion xử lý revision thành canonical chunks và embeddings.
package ingestion

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

type Chunk struct {
	Ordinal, LineStart, LineEnd, TokenCount int
	Content, Hash                           string
	Embedding                               []float32
}

// ChunkText chia canonical text theo dòng, giữ nguyên nội dung và locator.
// Khi có thể, chunk kết thúc ở dòng trống hoặc ngay trước heading Markdown.
func ChunkText(text string, linesPerChunk, overlap int) []Chunk {
	lines := strings.Split(NormalizeText(text), "\n")
	if linesPerChunk < 1 {
		linesPerChunk = 80
	}
	if overlap < 0 || overlap >= linesPerChunk {
		overlap = 0
	}
	out := []Chunk{}
	for start := 0; start < len(lines); {
		end := start + linesPerChunk
		if end > len(lines) {
			end = len(lines)
		} else {
			end = naturalBoundary(lines, start, end)
		}
		lineStart, lineEnd := contentRange(lines, start, end)
		if lineStart < lineEnd {
			content := strings.Join(lines[lineStart:lineEnd], "\n")
			sum := sha256.Sum256([]byte(content))
			out = append(out, Chunk{
				Ordinal: len(out), LineStart: lineStart + 1, LineEnd: lineEnd,
				TokenCount: len(strings.Fields(content)), Content: content,
				Hash: hex.EncodeToString(sum[:]),
			})
		}
		if end == len(lines) {
			break
		}
		next := end - overlap
		if next <= start {
			next = end
		}
		start = next
	}
	return out
}

func naturalBoundary(lines []string, start, end int) int {
	min := start + (end-start)*2/3
	if min <= start {
		min = start + 1
	}
	for i := end - 1; i >= min; i-- {
		if isMarkdownHeading(lines[i]) || strings.TrimSpace(lines[i-1]) == "" {
			return i
		}
	}
	return end
}

func contentRange(lines []string, start, end int) (int, int) {
	for start < end && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	for end > start && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	return start, end
}

func isMarkdownHeading(line string) bool {
	line = strings.TrimSpace(line)
	return strings.HasPrefix(line, "# ") || strings.HasPrefix(line, "## ") ||
		strings.HasPrefix(line, "### ") || strings.HasPrefix(line, "#### ") ||
		strings.HasPrefix(line, "##### ") || strings.HasPrefix(line, "###### ")
}
