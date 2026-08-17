package ingestion

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

const maxCanonicalBytes = 50 << 20

// ParsedDocument là canonical text cùng version parser dùng để tái lập citation.
type ParsedDocument struct {
	Text          string
	ParserVersion string
}

// Parser chuyển object gốc thành canonical UTF-8 text.
type Parser interface {
	Parse(context.Context, io.Reader) (ParsedDocument, error)
}

// ParserRegistry chọn parser theo MIME đã được API xác minh.
type ParserRegistry struct {
	parsers map[string]Parser
}

// NewParserRegistry tạo registry mặc định cho các định dạng ingestion hỗ trợ.
func NewParserRegistry() *ParserRegistry {
	plain := textParser{version: "text-v1"}
	return &ParserRegistry{parsers: map[string]Parser{
		"text/plain":      plain,
		"text/markdown":   textParser{version: "markdown-v1"},
		"text/csv":        csvParser{},
		"application/pdf": pdfParser{command: "pdftotext"},
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document": docxParser{},
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":       xlsxParser{},
	}}
}

// Parse trả lỗi rõ ràng khi MIME chưa có parser thay vì xử lý sai dữ liệu.
func (r *ParserRegistry) Parse(ctx context.Context, mediaType string, reader io.Reader) (ParsedDocument, error) {
	parser, ok := r.parsers[mediaType]
	if !ok {
		return ParsedDocument{}, fmt.Errorf("định dạng %s chưa có parser", mediaType)
	}
	return parser.Parse(ctx, reader)
}

type textParser struct{ version string }

func (p textParser) Parse(_ context.Context, reader io.Reader) (ParsedDocument, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maxCanonicalBytes+1))
	if err != nil {
		return ParsedDocument{}, fmt.Errorf("đọc nội dung: %w", err)
	}
	if len(data) > maxCanonicalBytes {
		return ParsedDocument{}, fmt.Errorf("nội dung vượt quá 50 MiB")
	}
	if !utf8.Valid(data) {
		return ParsedDocument{}, fmt.Errorf("nội dung không phải UTF-8 hợp lệ")
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return ParsedDocument{}, fmt.Errorf("nội dung chứa ký tự null không hợp lệ")
	}
	text, err := validateCanonicalText(string(data))
	if err != nil {
		return ParsedDocument{}, err
	}
	return ParsedDocument{Text: text, ParserVersion: p.version}, nil
}

func validateCanonicalText(value string) (string, error) {
	if !utf8.ValidString(value) {
		return "", fmt.Errorf("nội dung không phải UTF-8 hợp lệ")
	}
	if strings.IndexByte(value, 0) >= 0 {
		return "", fmt.Errorf("nội dung chứa ký tự null không hợp lệ")
	}
	text := NormalizeText(value)
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("tài liệu không có nội dung")
	}
	if len([]byte(text)) > maxCanonicalBytes {
		return "", fmt.Errorf("nội dung trích xuất vượt quá 50 MiB")
	}
	return text, nil
}

// NormalizeText chuẩn hóa newline nhưng không trim/gộp dòng để locator ổn định.
func NormalizeText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.ReplaceAll(text, "\r", "\n")
}
