package parser

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"rag-translator/internal/textutil"
)

// TXTParser handles both plain text and tab-separated game data files.
type TXTParser struct{}

func NewTXTParser() *TXTParser { return &TXTParser{} }

func (p *TXTParser) CanParse(ext string) bool {
	return ext == ".txt"
}

func (p *TXTParser) Parse(filePath string) (*ParseResult, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open txt file: %w", err)
	}
	defer file.Close()

	var rawLines []string
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 4*1024*1024), 4*1024*1024)
	for scanner.Scan() {
		rawLines = append(rawLines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan txt file: %w", err)
	}

	// Detect whether this is a tab-separated file.
	isTSV := detectTSV(rawLines)

	result := &ParseResult{
		FilePath: filePath,
		RawLines: rawLines,
	}

	if isTSV {
		result.FileType = "tsv"
		p.parseTSV(result, filePath)
	} else {
		result.FileType = "txt"
		p.parsePlainText(result, filePath)
	}

	return result, nil
}

// detectTSV checks if the file has consistent tab-separated columns.
func detectTSV(lines []string) bool {
	if len(lines) < 2 {
		return false
	}

	tabCounts := make(map[int]int)
	sampleSize := min(len(lines), 20)
	nonEmptyLines := 0

	for i := 0; i < sampleSize; i++ {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			continue
		}
		nonEmptyLines++
		count := strings.Count(line, "\t")
		if count > 0 {
			tabCounts[count]++
		}
	}

	if nonEmptyLines == 0 {
		return false
	}

	// Find the most common tab count.
	maxCount := 0
	for _, c := range tabCounts {
		if c > maxCount {
			maxCount = c
		}
	}

	// If >60% of non-empty lines share the same tab count, it's TSV.
	return float64(maxCount)/float64(nonEmptyLines) > 0.6
}

func (p *TXTParser) parseTSV(result *ParseResult, filePath string) {
	for lineNum, line := range result.RawLines {
		if strings.TrimSpace(line) == "" {
			continue
		}

		cols := strings.Split(line, "\t")
		for colIdx, col := range cols {
			if !isTranslatableColumn(col) {
				continue
			}

			ctx := map[string]string{
				"file":   filePath,
				"format": "tsv",
			}
			if len(cols) > 0 && colIdx > 0 {
				ctx["id"] = cols[0]
			}

			result.Texts = append(result.Texts, ExtractedText{
				Text:    col,
				File:    filePath,
				Line:    lineNum + 1,
				Column:  colIdx,
				Context: ctx,
			})
		}
	}
}

func (p *TXTParser) parsePlainText(result *ParseResult, filePath string) {
	for lineNum, line := range result.RawLines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || !textutil.ContainsChinese(trimmed) {
			continue
		}

		ctx := map[string]string{
			"file":   filePath,
			"format": "txt",
		}

		result.Texts = append(result.Texts, ExtractedText{
			Text:    trimmed,
			File:    filePath,
			Line:    lineNum + 1,
			Column:  -1,
			Context: ctx,
		})
	}
}

// isTranslatableColumn determines if a TSV column contains human-readable text
// that should be translated.
func isTranslatableColumn(col string) bool {
	if col == "" || !textutil.ContainsChinese(col) {
		return false
	}

	// Skip if it looks like a pure identifier (no non-ASCII chars).
	hasNonASCII := false
	for _, r := range col {
		if r > 127 {
			hasNonASCII = true
			break
		}
	}
	if !hasNonASCII {
		return false
	}

	// Minimum length check — very short strings are likely codes.
	return utf8.RuneCountInString(col) >= 2
}

func (p *TXTParser) Reconstruct(result *ParseResult, translations map[string]string) ([]byte, error) {
	lines := make([]string, len(result.RawLines))
	copy(lines, result.RawLines)

	if result.FileType == "tsv" {
		return p.reconstructTSV(lines, result, translations)
	}
	return p.reconstructPlainText(lines, result, translations)
}

func (p *TXTParser) reconstructTSV(lines []string, result *ParseResult, translations map[string]string) ([]byte, error) {
	for _, et := range result.Texts {
		idx := et.Line - 1
		if idx < 0 || idx >= len(lines) {
			continue
		}
		translated, ok := translations[et.Text]
		if !ok {
			continue
		}

		cols := strings.Split(lines[idx], "\t")
		if et.Column >= 0 && et.Column < len(cols) {
			cols[et.Column] = translated
		}
		lines[idx] = strings.Join(cols, "\t")
	}

	return []byte(strings.Join(lines, "\n") + "\n"), nil
}

func (p *TXTParser) reconstructPlainText(lines []string, result *ParseResult, translations map[string]string) ([]byte, error) {
	for _, et := range result.Texts {
		idx := et.Line - 1
		if idx < 0 || idx >= len(lines) {
			continue
		}
		translated, ok := translations[et.Text]
		if !ok {
			continue
		}
		original := lines[idx]
		trimmed := strings.TrimSpace(original)
		lines[idx] = strings.Replace(original, trimmed, translated, 1)
	}

	return []byte(strings.Join(lines, "\n") + "\n"), nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
