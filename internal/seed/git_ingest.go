package seed

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"rag-translator/internal/textutil"

	"github.com/rs/zerolog/log"
)

// SeedEntry represents a source→translated pair extracted from a Git diff.
type SeedEntry struct {
	SourceText     string `json:"source_text"`
	TranslatedText string `json:"translated_text"`
	File           string `json:"file"`
	Function       string `json:"function,omitempty"`
	EntityType     string `json:"entity_type,omitempty"`
	Hash           string `json:"hash"`
}

// GitIngestor extracts translation pairs from Git diffs.
type GitIngestor struct{}

// NewGitIngestor creates a new Git ingestor.
func NewGitIngestor() *GitIngestor {
	return &GitIngestor{}
}

// supportedExts lists file extensions to process.
var supportedExts = map[string]bool{
	".lua": true,
	".ini": true,
	".txt": true,
}

// IngestFromGit extracts seed translation pairs by diffing two git refs for a given folder.
func (gi *GitIngestor) IngestFromGit(ctx context.Context, repoRoot, commitBase, commitTarget, folder string) ([]SeedEntry, error) {
	files, err := gi.getChangedFiles(ctx, repoRoot, commitBase, commitTarget, folder)
	if err != nil {
		return nil, fmt.Errorf("get changed files: %w", err)
	}

	log.Info().Int("files", len(files)).Msg("Found changed files in Git diff")

	var allEntries []SeedEntry

	for _, file := range files {
		ext := strings.ToLower(filepath.Ext(file))
		if !supportedExts[ext] {
			continue
		}

		entries, err := gi.extractPairsFromDiff(ctx, repoRoot, commitBase, commitTarget, file)
		if err != nil {
			log.Warn().Err(err).Str("file", file).Msg("Failed to extract pairs from diff")
			continue
		}

		allEntries = append(allEntries, entries...)
		log.Debug().Str("file", file).Int("pairs", len(entries)).Msg("Extracted translation pairs")
	}

	log.Info().Int("total_pairs", len(allEntries)).Msg("Git diff ingestion complete")
	return allEntries, nil
}

// getChangedFiles retrieves the list of changed files between two commits in a folder.
func (gi *GitIngestor) getChangedFiles(ctx context.Context, repoRoot, commitBase, commitTarget, folder string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", "diff", "--name-only", commitBase, commitTarget, "--", folder)
	cmd.Dir = repoRoot

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff --name-only: %w", err)
	}

	var files []string
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			files = append(files, line)
		}
	}

	return files, nil
}

// diffHunk represents a group of removed/added lines from a diff.
type diffHunk struct {
	removed []string
	added   []string
}

// extractPairsFromDiff parses `git diff` output and extracts source→translated pairs.
func (gi *GitIngestor) extractPairsFromDiff(ctx context.Context, repoRoot, commitBase, commitTarget, file string) ([]SeedEntry, error) {
	cmd := exec.CommandContext(ctx, "git", "diff", "-U0", commitBase, commitTarget, "--", file)
	cmd.Dir = repoRoot

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff: %w", err)
	}

	ext := strings.ToLower(filepath.Ext(file))
	hunks := parseHunks(string(output))

	var entries []SeedEntry
	for _, hunk := range hunks {
		pairs := matchPairs(hunk, ext, file)
		entries = append(entries, pairs...)
	}

	return entries, nil
}

// parseHunks groups diff output into hunks of removed/added lines.
func parseHunks(diffOutput string) []diffHunk {
	var hunks []diffHunk
	var current diffHunk
	inHunk := false

	scanner := bufio.NewScanner(strings.NewReader(diffOutput))
	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "diff ") || strings.HasPrefix(line, "index ") ||
			strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "+++ ") {
			continue
		}

		if strings.HasPrefix(line, "@@") {
			if inHunk && (len(current.removed) > 0 || len(current.added) > 0) {
				hunks = append(hunks, current)
			}
			current = diffHunk{}
			inHunk = true
			continue
		}

		if !inHunk {
			continue
		}

		if strings.HasPrefix(line, "-") {
			current.removed = append(current.removed, line[1:])
		} else if strings.HasPrefix(line, "+") {
			current.added = append(current.added, line[1:])
		}
	}

	if inHunk && (len(current.removed) > 0 || len(current.added) > 0) {
		hunks = append(hunks, current)
	}

	return hunks
}

// luaFuncExtractor captures function context from Lua lines.
var luaFuncExtractor = regexp.MustCompile(`([a-zA-Z_][a-zA-Z0-9_.:]*)s*\(`)

// luaStringRe matches quoted strings in Lua.
var luaStringRe = regexp.MustCompile(`"([^"\\]*(?:\\.[^"\\]*)*)"|'([^'\\]*(?:\\.[^'\\]*)*)'`)

// matchPairs matches removed (source) lines with added (translated) lines.
func matchPairs(hunk diffHunk, ext, file string) []SeedEntry {
	var entries []SeedEntry

	pairCount := len(hunk.removed)
	if len(hunk.added) < pairCount {
		pairCount = len(hunk.added)
	}

	for i := 0; i < pairCount; i++ {
		srcText, dstText, fnName := extractTextPair(hunk.removed[i], hunk.added[i], ext)

		if srcText == "" || dstText == "" || !textutil.ContainsChinese(srcText) {
			continue
		}

		entries = append(entries, SeedEntry{
			SourceText:     srcText,
			TranslatedText: dstText,
			File:           file,
			Function:       fnName,
			EntityType:     detectEntityType(file, fnName, srcText),
			Hash:           textutil.Hash(srcText),
		})
	}

	return entries
}

// extractTextPair extracts actual text content from removed/added diff lines.
func extractTextPair(source, translated, ext string) (string, string, string) {
	switch ext {
	case ".lua":
		return extractLuaPair(source, translated)
	case ".ini":
		return extractINIPair(source, translated)
	case ".txt":
		return extractTXTPair(source, translated)
	default:
		return strings.TrimSpace(source), strings.TrimSpace(translated), ""
	}
}

func extractLuaPair(source, translated string) (string, string, string) {
	srcMatches := luaStringRe.FindStringSubmatch(source)
	dstMatches := luaStringRe.FindStringSubmatch(translated)

	if srcMatches == nil || dstMatches == nil {
		return "", "", ""
	}

	srcText := srcMatches[1]
	if srcText == "" {
		srcText = srcMatches[2]
	}

	dstText := dstMatches[1]
	if dstText == "" {
		dstText = dstMatches[2]
	}

	fnName := ""
	if fnMatch := luaFuncExtractor.FindStringSubmatch(source); fnMatch != nil {
		fnName = fnMatch[1]
	}

	return srcText, dstText, fnName
}

func extractINIPair(source, translated string) (string, string, string) {
	srcParts := strings.SplitN(source, "=", 2)
	dstParts := strings.SplitN(translated, "=", 2)

	if len(srcParts) != 2 || len(dstParts) != 2 {
		return "", "", ""
	}

	if strings.TrimSpace(srcParts[0]) != strings.TrimSpace(dstParts[0]) {
		return "", "", ""
	}

	return strings.TrimSpace(srcParts[1]), strings.TrimSpace(dstParts[1]), ""
}

func extractTXTPair(source, translated string) (string, string, string) {
	srcCols := strings.Split(source, "\t")
	dstCols := strings.Split(translated, "\t")

	if len(srcCols) != len(dstCols) || len(srcCols) < 2 {
		return "", "", ""
	}

	for i := range srcCols {
		if srcCols[i] != dstCols[i] && textutil.ContainsChinese(srcCols[i]) {
			return srcCols[i], dstCols[i], ""
		}
	}

	return "", "", ""
}

// entityPatterns maps file name patterns to entity types.
var entityPatterns = map[string]string{
	"skill": "skill", "buff": "buff", "item": "item", "equip": "item",
	"weapon": "item", "quest": "quest", "npc": "character", "char": "character",
	"map": "location", "scene": "location", "ui": "ui", "dialog": "dialog",
	"chat": "dialog", "faction": "faction", "guild": "faction",
	"mount": "mount", "pet": "pet",
}

// termEntityMap maps known wuxia terms to entity types.
var termEntityMap = map[string]string{
	"技能": "skill", "武功": "skill", "心法": "skill",
	"装备": "item", "丹药": "item", "秘籍": "item",
	"副本": "dungeon", "任务": "quest",
	"门派": "faction", "帮派": "faction", "坐骑": "mount",
}

// detectEntityType infers entity type from file name, function, and text content.
func detectEntityType(file, function, text string) string {
	fileLower := strings.ToLower(file)
	funcLower := strings.ToLower(function)

	for pattern, entityType := range entityPatterns {
		if strings.Contains(fileLower, pattern) || strings.Contains(funcLower, pattern) {
			return entityType
		}
	}

	for term, entityType := range termEntityMap {
		if strings.Contains(text, term) {
			return entityType
		}
	}

	return "general"
}
