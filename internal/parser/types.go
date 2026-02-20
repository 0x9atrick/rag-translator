package parser

// ExtractedText represents a translatable string extracted from a game file.
type ExtractedText struct {
	// Text is the original translatable string.
	Text string
	// File is the source file path.
	File string
	// Line is the 1-based line number in the source file.
	Line int
	// Column is the 0-based column for tab-separated files (-1 if not applicable).
	Column int
	// Context holds additional context (function name, section, etc.)
	Context map[string]string
}

// ParseResult holds parsing output for a single file.
type ParseResult struct {
	// FilePath is the absolute path to the parsed file.
	FilePath string
	// FileType is the detected type (lua, ini, txt, tsv).
	FileType string
	// Texts are the extracted translatable strings.
	Texts []ExtractedText
	// RawLines preserves the original file content for reconstruction.
	RawLines []string
}

// Parser is the interface for all file format parsers.
type Parser interface {
	// CanParse returns true if this parser handles the given file extension.
	CanParse(ext string) bool
	// Parse extracts translatable strings from a file.
	Parse(filePath string) (*ParseResult, error)
	// Reconstruct rebuilds the file with translated strings.
	Reconstruct(result *ParseResult, translations map[string]string) ([]byte, error)
}
