package ingestion

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type pdfParser struct{ command string }

func (p pdfParser) Parse(ctx context.Context, reader io.Reader) (ParsedDocument, error) {
	if _, err := exec.LookPath(p.command); err != nil {
		return ParsedDocument{}, fmt.Errorf("thiếu công cụ %s để đọc PDF text-layer", p.command)
	}
	input, err := os.CreateTemp("", "docs-hub-*.pdf")
	if err != nil {
		return ParsedDocument{}, fmt.Errorf("tạo file PDF tạm: %w", err)
	}
	inputPath := input.Name()
	defer os.Remove(inputPath)
	written, copyErr := io.Copy(input, io.LimitReader(reader, maxCanonicalBytes+1))
	closeErr := input.Close()
	if copyErr != nil {
		return ParsedDocument{}, fmt.Errorf("ghi file PDF tạm: %w", copyErr)
	}
	if closeErr != nil {
		return ParsedDocument{}, fmt.Errorf("đóng file PDF tạm: %w", closeErr)
	}
	if written > maxCanonicalBytes {
		return ParsedDocument{}, fmt.Errorf("PDF vượt quá 50 MiB")
	}
	parseCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var stdout, stderr bytes.Buffer
	command := exec.CommandContext(parseCtx, p.command, "-layout", "-enc", "UTF-8", inputPath, "-")
	command.Stdout = &limitedBuffer{buffer: &stdout, remaining: maxCanonicalBytes + 1}
	command.Stderr = &limitedBuffer{buffer: &stderr, remaining: 4096}
	if err = command.Run(); err != nil {
		if parseCtx.Err() != nil {
			return ParsedDocument{}, fmt.Errorf("trích xuất PDF quá thời hạn: %w", parseCtx.Err())
		}
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = err.Error()
		}
		return ParsedDocument{}, fmt.Errorf("trích xuất PDF text-layer: %s", detail)
	}
	if stdout.Len() > maxCanonicalBytes {
		return ParsedDocument{}, fmt.Errorf("nội dung PDF trích xuất vượt quá 50 MiB")
	}
	text, err := canonicalPDFText(stdout.String())
	if err != nil {
		return ParsedDocument{}, err
	}
	return ParsedDocument{Text: text, ParserVersion: "pdftotext-v1"}, nil
}

type limitedBuffer struct {
	buffer    *bytes.Buffer
	remaining int
}

func (w *limitedBuffer) Write(value []byte) (int, error) {
	if w.remaining <= 0 {
		return len(value), nil
	}
	write := len(value)
	if write > w.remaining {
		write = w.remaining
	}
	_, _ = w.buffer.Write(value[:write])
	w.remaining -= write
	return len(value), nil
}

// canonicalPDFText giữ ranh giới trang do pdftotext trả về bằng marker ổn định.
// Marker trở thành heading để chunker không trộn nội dung của hai trang khi có thể.
func canonicalPDFText(value string) (string, error) {
	pages := strings.Split(NormalizeText(value), "\f")
	var out strings.Builder
	pageCount := 0
	for index, page := range pages {
		page = strings.TrimSpace(page)
		if page == "" {
			continue
		}
		pageCount++
		if out.Len() > 0 {
			out.WriteString("\n\n")
		}
		out.WriteString("# PDF page ")
		out.WriteString(strconv.Itoa(index + 1))
		out.WriteByte('\n')
		out.WriteString(page)
	}
	if pageCount == 0 {
		return "", fmt.Errorf("PDF không có text layer; OCR chưa được hỗ trợ")
	}
	return validateCanonicalText(out.String())
}
