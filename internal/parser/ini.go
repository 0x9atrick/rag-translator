package parser

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"rag-translator/internal/textutil"
)

// INIParser extracts translatable strings from INI/config files.
type INIParser struct{}

func NewINIParser() *INIParser { return &INIParser{} }

func (p *INIParser) CanParse(ext string) bool {
	return ext == ".ini"
}

func (p *INIParser) Parse(filePath string) (*ParseResult, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open ini file: %w", err)
	}
	defer file.Close()

	result := &ParseResult{
		FilePath: filePath,
		FileType: "ini",
	}

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	lineNum := 0
	currentSection := ""

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		result.RawLines = append(result.RawLines, line)

		trimmed := strings.TrimSpace(line)

		// Skip empty lines and comments.
		if trimmed == "" || strings.HasPrefix(trimmed, ";") || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Section header.
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			currentSection = trimmed[1 : len(trimmed)-1]
			continue
		}

		// Key=Value pair.
		eqIdx := strings.Index(trimmed, "=")
		if eqIdx < 0 {
			continue
		}

		value := strings.TrimSpace(trimmed[eqIdx+1:])
		if value == "" || !textutil.ContainsChinese(value) {
			continue
		}

		key := strings.TrimSpace(trimmed[:eqIdx])

		ctx := map[string]string{
			"file":    filePath,
			"section": currentSection,
			"key":     key,
		}

		result.Texts = append(result.Texts, ExtractedText{
			Text:    value,
			File:    filePath,
			Line:    lineNum,
			Column:  -1,
			Context: ctx,
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan ini file: %w", err)
	}

	return result, nil
}

func (p *INIParser) Reconstruct(result *ParseResult, translations map[string]string) ([]byte, error) {
	lines := make([]string, len(result.RawLines))
	copy(lines, result.RawLines)

	for _, et := range result.Texts {
		idx := et.Line - 1
		if idx < 0 || idx >= len(lines) {
			continue
		}

		translated, ok := translations[et.Text]
		if !ok {
			continue
		}

		line := lines[idx]
		eqIdx := strings.Index(line, "=")
		if eqIdx < 0 {
			continue
		}

		// Preserve leading whitespace after =.
		afterEq := line[eqIdx+1:]
		leadingSpaces := ""
		for _, ch := range afterEq {
			if ch == ' ' || ch == '\t' {
				leadingSpaces += string(ch)
			} else {
				break
			}
		}

		lines[idx] = line[:eqIdx+1] + leadingSpaces + translated
	}

	return []byte(strings.Join(lines, "\n") + "\n"), nil
}
