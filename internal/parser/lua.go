package parser

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"

	"rag-translator/internal/textutil"
)

// LuaParser extracts translatable strings from Lua source files.
type LuaParser struct{}

func NewLuaParser() *LuaParser { return &LuaParser{} }

func (p *LuaParser) CanParse(ext string) bool {
	return ext == ".lua"
}

// luaStringPattern matches quoted strings in Lua (double and single quoted).
var luaStringPattern = regexp.MustCompile(`"([^"\\]*(?:\\.[^"\\]*)*)"|'([^'\\]*(?:\\.[^'\\]*)*)'`)

// luaFuncPattern captures the function name before a parenthesized argument.
var luaFuncPattern = regexp.MustCompile(`([a-zA-Z_][a-zA-Z0-9_.:]*)s*\(\s*$`)

// luaMultilineOpen matches the opening of --[[ or --[=[ blocks.
var luaMultilineCommentOpen = regexp.MustCompile(`--\[=*\[`)
var luaMultilineCommentClose = regexp.MustCompile(`\]=*\]`)

func (p *LuaParser) Parse(filePath string) (*ParseResult, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open lua file: %w", err)
	}
	defer file.Close()

	result := &ParseResult{
		FilePath: filePath,
		FileType: "lua",
	}

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	lineNum := 0
	inMultilineComment := false

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		result.RawLines = append(result.RawLines, line)

		// Handle multiline comments.
		if inMultilineComment {
			if luaMultilineCommentClose.MatchString(line) {
				inMultilineComment = false
			}
			continue
		}

		if luaMultilineCommentOpen.MatchString(line) {
			if !luaMultilineCommentClose.MatchString(line) {
				inMultilineComment = true
			}
			continue
		}

		// Skip single-line comments.
		codePart := line
		if idx := strings.Index(line, "--"); idx >= 0 {
			if !isInsideString(line, idx) {
				codePart = line[:idx]
			}
		}

		// Find all string literals.
		matches := luaStringPattern.FindAllStringSubmatchIndex(codePart, -1)
		for _, loc := range matches {
			var text string
			if loc[2] >= 0 {
				text = codePart[loc[2]:loc[3]] // double quoted
			} else if loc[4] >= 0 {
				text = codePart[loc[4]:loc[5]] // single quoted
			}

			if text == "" || !textutil.ContainsChinese(text) {
				continue
			}

			// Try to extract function context.
			ctx := make(map[string]string)
			ctx["file"] = filePath
			prefix := codePart[:loc[0]]
			if funcMatch := luaFuncPattern.FindStringSubmatch(prefix); funcMatch != nil {
				ctx["function"] = funcMatch[1]
			}

			result.Texts = append(result.Texts, ExtractedText{
				Text:    text,
				File:    filePath,
				Line:    lineNum,
				Column:  -1,
				Context: ctx,
			})
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan lua file: %w", err)
	}

	return result, nil
}

func (p *LuaParser) Reconstruct(result *ParseResult, translations map[string]string) ([]byte, error) {
	lines := make([]string, len(result.RawLines))
	copy(lines, result.RawLines)

	// Group by line number and process.
	lineReplacements := make(map[int][]ExtractedText)
	for _, et := range result.Texts {
		lineReplacements[et.Line] = append(lineReplacements[et.Line], et)
	}

	for lineNum, texts := range lineReplacements {
		idx := lineNum - 1
		if idx < 0 || idx >= len(lines) {
			continue
		}
		line := lines[idx]
		for _, et := range texts {
			if translated, ok := translations[et.Text]; ok {
				line = strings.Replace(line, et.Text, translated, 1)
			}
		}
		lines[idx] = line
	}

	return []byte(strings.Join(lines, "\n") + "\n"), nil
}

// isInsideString checks if position idx is inside a string literal.
func isInsideString(line string, idx int) bool {
	inDouble := false
	inSingle := false
	for i := 0; i < idx; i++ {
		ch := line[i]
		if ch == '\\' {
			i++ // skip escaped char
			continue
		}
		if ch == '"' && !inSingle {
			inDouble = !inDouble
		}
		if ch == '\'' && !inDouble {
			inSingle = !inSingle
		}
	}
	return inDouble || inSingle
}
