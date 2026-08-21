package ingestion

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/xml"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
)

const maxArchiveEntryBytes = maxCanonicalBytes

type csvParser struct{}

func (csvParser) Parse(_ context.Context, reader io.Reader) (ParsedDocument, error) {
	r := csv.NewReader(io.LimitReader(reader, maxCanonicalBytes+1))
	r.FieldsPerRecord = -1
	r.ReuseRecord = true
	var out strings.Builder
	row := 0
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return ParsedDocument{}, fmt.Errorf("đọc CSV: %w", err)
		}
		row++
		fmt.Fprintf(&out, "Row %d", row)
		for i, value := range record {
			fmt.Fprintf(&out, "\t%s=%s", spreadsheetColumn(i+1), normalizeCell(value))
		}
		out.WriteByte('\n')
		if out.Len() > maxCanonicalBytes {
			return ParsedDocument{}, fmt.Errorf("nội dung trích xuất vượt quá 50 MiB")
		}
	}
	text, err := validateCanonicalText(out.String())
	if err != nil {
		return ParsedDocument{}, err
	}
	return ParsedDocument{Text: text, ParserVersion: "csv-v1"}, nil
}

type docxParser struct{}

func (docxParser) Parse(_ context.Context, reader io.Reader) (ParsedDocument, error) {
	archive, err := readZIP(reader)
	if err != nil {
		return ParsedDocument{}, fmt.Errorf("mở DOCX: %w", err)
	}
	documentXML, err := readZIPEntry(archive, "word/document.xml")
	if err != nil {
		return ParsedDocument{}, fmt.Errorf("đọc DOCX: %w", err)
	}
	decoder := xml.NewDecoder(bytes.NewReader(documentXML))
	var out, paragraph strings.Builder
	var style string
	inText := false
	for {
		token, decodeErr := decoder.Token()
		if decodeErr == io.EOF {
			break
		}
		if decodeErr != nil {
			return ParsedDocument{}, fmt.Errorf("parse DOCX XML: %w", decodeErr)
		}
		switch node := token.(type) {
		case xml.StartElement:
			switch node.Name.Local {
			case "p":
				paragraph.Reset()
				style = ""
			case "pStyle":
				style = xmlAttribute(node.Attr, "val")
			case "t":
				inText = true
			case "tab":
				paragraph.WriteByte('\t')
			case "br", "cr":
				paragraph.WriteByte('\n')
			}
		case xml.CharData:
			if inText {
				paragraph.Write([]byte(node))
			}
		case xml.EndElement:
			switch node.Name.Local {
			case "t":
				inText = false
			case "p":
				appendDOCXParagraph(&out, paragraph.String(), style)
			}
		}
	}
	text, err := validateCanonicalText(out.String())
	if err != nil {
		return ParsedDocument{}, err
	}
	return ParsedDocument{Text: text, ParserVersion: "docx-v1"}, nil
}

func appendDOCXParagraph(out *strings.Builder, value, style string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	styleLower := strings.ToLower(style)
	if strings.HasPrefix(styleLower, "heading") {
		level, err := strconv.Atoi(strings.TrimPrefix(styleLower, "heading"))
		if err == nil && level >= 1 && level <= 6 {
			out.WriteString(strings.Repeat("#", level))
			out.WriteByte(' ')
		}
	}
	out.WriteString(value)
	out.WriteByte('\n')
}

type xlsxParser struct{}

type xlsxSheet struct {
	name, relationshipID string
}

func (xlsxParser) Parse(_ context.Context, reader io.Reader) (ParsedDocument, error) {
	archive, err := readZIP(reader)
	if err != nil {
		return ParsedDocument{}, fmt.Errorf("mở XLSX: %w", err)
	}
	workbookXML, err := readZIPEntry(archive, "xl/workbook.xml")
	if err != nil {
		return ParsedDocument{}, fmt.Errorf("đọc XLSX workbook: %w", err)
	}
	sheets, err := parseWorkbook(workbookXML)
	if err != nil {
		return ParsedDocument{}, err
	}
	relationshipXML, err := readZIPEntry(archive, "xl/_rels/workbook.xml.rels")
	if err != nil {
		return ParsedDocument{}, fmt.Errorf("đọc XLSX relationships: %w", err)
	}
	relationships, err := parseRelationships(relationshipXML)
	if err != nil {
		return ParsedDocument{}, err
	}
	sharedStrings := []string{}
	if data, sharedErr := readZIPEntry(archive, "xl/sharedStrings.xml"); sharedErr == nil {
		sharedStrings, err = parseSharedStrings(data)
		if err != nil {
			return ParsedDocument{}, err
		}
	}
	var out strings.Builder
	for _, sheet := range sheets {
		target, ok := relationships[sheet.relationshipID]
		if !ok {
			return ParsedDocument{}, fmt.Errorf("XLSX thiếu relationship cho sheet %q", sheet.name)
		}
		entryName := path.Clean(path.Join("xl", strings.TrimPrefix(target, "/")))
		if strings.HasPrefix(target, "/") {
			entryName = strings.TrimPrefix(path.Clean(target), "/")
		}
		data, readErr := readZIPEntry(archive, entryName)
		if readErr != nil {
			return ParsedDocument{}, fmt.Errorf("đọc sheet %q: %w", sheet.name, readErr)
		}
		rows, parseErr := parseWorksheet(data, sharedStrings)
		if parseErr != nil {
			return ParsedDocument{}, fmt.Errorf("parse sheet %q: %w", sheet.name, parseErr)
		}
		fmt.Fprintf(&out, "# Sheet: %s\n", normalizeCell(sheet.name))
		out.WriteString(rows)
		if out.Len() > maxCanonicalBytes {
			return ParsedDocument{}, fmt.Errorf("nội dung trích xuất vượt quá 50 MiB")
		}
	}
	text, err := validateCanonicalText(out.String())
	if err != nil {
		return ParsedDocument{}, err
	}
	return ParsedDocument{Text: text, ParserVersion: "xlsx-v1"}, nil
}

func readZIP(reader io.Reader) (*zip.Reader, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maxCanonicalBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxCanonicalBytes {
		return nil, fmt.Errorf("archive vượt quá 50 MiB")
	}
	return zip.NewReader(bytes.NewReader(data), int64(len(data)))
}

func readZIPEntry(archive *zip.Reader, name string) ([]byte, error) {
	for _, file := range archive.File {
		if file.Name != name {
			continue
		}
		if file.UncompressedSize64 > maxArchiveEntryBytes {
			return nil, fmt.Errorf("entry %s vượt quá giới hạn", name)
		}
		entry, err := file.Open()
		if err != nil {
			return nil, err
		}
		defer entry.Close()
		data, err := io.ReadAll(io.LimitReader(entry, maxArchiveEntryBytes+1))
		if err != nil {
			return nil, err
		}
		if len(data) > maxArchiveEntryBytes {
			return nil, fmt.Errorf("entry %s vượt quá giới hạn", name)
		}
		return data, nil
	}
	return nil, fmt.Errorf("không tìm thấy entry %s", name)
}

func parseWorkbook(data []byte) ([]xlsxSheet, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var sheets []xlsxSheet
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parse XLSX workbook: %w", err)
		}
		start, ok := token.(xml.StartElement)
		if ok && start.Name.Local == "sheet" {
			sheets = append(sheets, xlsxSheet{name: xmlAttribute(start.Attr, "name"), relationshipID: xmlAttribute(start.Attr, "id")})
		}
	}
	if len(sheets) == 0 {
		return nil, fmt.Errorf("XLSX không có worksheet")
	}
	return sheets, nil
}

func parseRelationships(data []byte) (map[string]string, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	out := map[string]string{}
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("parse XLSX relationships: %w", err)
		}
		start, ok := token.(xml.StartElement)
		if ok && start.Name.Local == "Relationship" {
			out[xmlAttribute(start.Attr, "Id")] = xmlAttribute(start.Attr, "Target")
		}
	}
}

func parseSharedStrings(data []byte) ([]string, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var out []string
	var value strings.Builder
	inItem, inText := false, false
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("parse XLSX shared strings: %w", err)
		}
		switch node := token.(type) {
		case xml.StartElement:
			if node.Name.Local == "si" {
				inItem = true
				value.Reset()
			} else if inItem && node.Name.Local == "t" {
				inText = true
			}
		case xml.CharData:
			if inText {
				value.Write([]byte(node))
			}
		case xml.EndElement:
			if node.Name.Local == "t" {
				inText = false
			} else if node.Name.Local == "si" {
				out = append(out, value.String())
				inItem = false
			}
		}
	}
}

func parseWorksheet(data []byte, sharedStrings []string) (string, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var out strings.Builder
	var rowNumber, cellRef, cellType, cellValue string
	inValue, inInlineText := false, false
	var cells []string
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return out.String(), nil
		}
		if err != nil {
			return "", err
		}
		switch node := token.(type) {
		case xml.StartElement:
			switch node.Name.Local {
			case "row":
				rowNumber = xmlAttribute(node.Attr, "r")
				cells = cells[:0]
			case "c":
				cellRef, cellType, cellValue = xmlAttribute(node.Attr, "r"), xmlAttribute(node.Attr, "t"), ""
			case "v":
				inValue = true
			case "t":
				if cellType == "inlineStr" {
					inInlineText = true
				}
			}
		case xml.CharData:
			if inValue || inInlineText {
				cellValue += string(node)
			}
		case xml.EndElement:
			switch node.Name.Local {
			case "v":
				inValue = false
			case "t":
				inInlineText = false
			case "c":
				value, valueErr := xlsxCellValue(cellType, cellValue, sharedStrings)
				if valueErr != nil {
					return "", valueErr
				}
				if value != "" {
					cells = append(cells, spreadsheetCellReference(cellRef)+"="+normalizeCell(value))
				}
			case "row":
				if len(cells) > 0 {
					if rowNumber == "" {
						rowNumber = "?"
					}
					fmt.Fprintf(&out, "Row %s\t%s\n", rowNumber, strings.Join(cells, "\t"))
				}
			}
		}
	}
}

func xlsxCellValue(cellType, value string, sharedStrings []string) (string, error) {
	if cellType != "s" {
		return value, nil
	}
	index, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || index < 0 || index >= len(sharedStrings) {
		return "", fmt.Errorf("XLSX shared string index không hợp lệ")
	}
	return sharedStrings[index], nil
}

func xmlAttribute(attributes []xml.Attr, local string) string {
	for _, attribute := range attributes {
		if attribute.Name.Local == local {
			return attribute.Value
		}
	}
	return ""
}

func spreadsheetCellReference(reference string) string {
	reference = strings.TrimSpace(reference)
	for i, char := range reference {
		if char >= '0' && char <= '9' {
			return reference[:i]
		}
	}
	return reference
}

func spreadsheetColumn(number int) string {
	var out string
	for number > 0 {
		number--
		out = string(rune('A'+number%26)) + out
		number /= 26
	}
	return out
}

func normalizeCell(value string) string {
	value = NormalizeText(value)
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "\t", " ")
	return strings.TrimSpace(value)
}
