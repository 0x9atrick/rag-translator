package filewalker

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"rag-translator/internal/parser"

	"github.com/rs/zerolog/log"
)

// SupportedExtensions lists file types handled by the tool.
var SupportedExtensions = map[string]bool{
	".lua": true,
	".ini": true,
	".txt": true,
}

// Walker traverses directories and dispatches files to the correct parser.
type Walker struct {
	parsers []parser.Parser
}

// NewWalker creates a Walker with default parsers.
func NewWalker() *Walker {
	return &Walker{
		parsers: []parser.Parser{
			parser.NewLuaParser(),
			parser.NewINIParser(),
			parser.NewTXTParser(),
		},
	}
}

// FileEntry represents a discovered file ready for processing.
type FileEntry struct {
	Path   string
	Ext    string
	Parser parser.Parser
}

// Walk discovers all supported files under the given root directory.
func (w *Walker) Walk(root string) ([]FileEntry, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve root path: %w", err)
	}

	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("stat root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("root is not a directory: %s", root)
	}

	var entries []FileEntry

	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Warn().Err(err).Str("path", path).Msg("Error walking path")
			return nil
		}

		if info.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if !SupportedExtensions[ext] {
			return nil
		}

		for _, p := range w.parsers {
			if p.CanParse(ext) {
				entries = append(entries, FileEntry{
					Path:   path,
					Ext:    ext,
					Parser: p,
				})
				break
			}
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("walk directory: %w", err)
	}

	log.Info().Int("count", len(entries)).Str("root", root).Msg("Discovered files")
	return entries, nil
}

// ParseFile parses a single file using the appropriate parser.
func (w *Walker) ParseFile(entry FileEntry) (*parser.ParseResult, error) {
	return entry.Parser.Parse(entry.Path)
}
