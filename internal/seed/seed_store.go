package seed

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"rag-translator/internal/dbgen"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// SeedStore handles persistence of seed translation pairs in PostgreSQL and file export.
type SeedStore struct {
	queries *dbgen.Queries
}

// NewSeedStore creates a new seed store.
func NewSeedStore(pool *pgxpool.Pool) *SeedStore {
	return &SeedStore{
		queries: dbgen.New(pool),
	}
}

// Upsert inserts or updates seed entries, deduplicating by hash.
func (ss *SeedStore) Upsert(ctx context.Context, entries []SeedEntry) (inserted, updated int, err error) {
	for _, e := range entries {
		tag, execErr := ss.queries.UpsertSeedTranslation(ctx, dbgen.UpsertSeedTranslationParams{
			Hash:           e.Hash,
			SourceText:     e.SourceText,
			TranslatedText: e.TranslatedText,
			File:           e.File,
			FunctionName:   e.Function,
			EntityType:     e.EntityType,
		})
		if execErr != nil {
			return inserted, updated, fmt.Errorf("upsert seed entry: %w", execErr)
		}
		if tag.RowsAffected() > 0 {
			inserted++
		}
	}

	log.Info().Int("inserted", inserted).Msg("Upserted seed entries")
	return inserted, updated, nil
}

// GetAll retrieves all seed entries from the store.
func (ss *SeedStore) GetAll(ctx context.Context) ([]SeedEntry, error) {
	rows, err := ss.queries.GetAllSeedTranslations(ctx)
	if err != nil {
		return nil, fmt.Errorf("query seed entries: %w", err)
	}

	entries := make([]SeedEntry, 0, len(rows))
	for _, row := range rows {
		entries = append(entries, SeedEntry{
			Hash:           row.Hash,
			SourceText:     row.SourceText,
			TranslatedText: row.TranslatedText,
			File:           row.File,
			Function:       row.FunctionName,
			EntityType:     row.EntityType,
		})
	}

	return entries, nil
}

// GetByEntityType retrieves seed entries filtered by entity type.
func (ss *SeedStore) GetByEntityType(ctx context.Context, entityType string) ([]SeedEntry, error) {
	rows, err := ss.queries.GetSeedTranslationsByEntityType(ctx, entityType)
	if err != nil {
		return nil, fmt.Errorf("query seed by entity type: %w", err)
	}

	entries := make([]SeedEntry, 0, len(rows))
	for _, row := range rows {
		entries = append(entries, SeedEntry{
			Hash:           row.Hash,
			SourceText:     row.SourceText,
			TranslatedText: row.TranslatedText,
			File:           row.File,
			Function:       row.FunctionName,
			EntityType:     row.EntityType,
		})
	}

	return entries, nil
}

// ExportTSV writes all seed entries to a TSV file.
func (ss *SeedStore) ExportTSV(ctx context.Context, outputPath string) error {
	entries, err := ss.GetAll(ctx)
	if err != nil {
		return err
	}

	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create TSV file: %w", err)
	}
	defer f.Close()

	fmt.Fprintln(f, "source_text\ttranslated_text\tfile\tfunction\tentity_type")

	for _, e := range entries {
		fmt.Fprintf(f, "%s\t%s\t%s\t%s\t%s\n",
			escapeTSV(e.SourceText),
			escapeTSV(e.TranslatedText),
			e.File,
			e.Function,
			e.EntityType,
		)
	}

	log.Info().Str("path", outputPath).Int("entries", len(entries)).Msg("Exported seed corpus to TSV")
	return nil
}

// ExportJSON writes all seed entries to a JSON file.
func (ss *SeedStore) ExportJSON(ctx context.Context, outputPath string) error {
	entries, err := ss.GetAll(ctx)
	if err != nil {
		return err
	}

	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create JSON file: %w", err)
	}
	defer f.Close()

	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)

	if err := encoder.Encode(entries); err != nil {
		return fmt.Errorf("encode JSON: %w", err)
	}

	log.Info().Str("path", outputPath).Int("entries", len(entries)).Msg("Exported seed corpus to JSON")
	return nil
}

// BuildTranslationMap returns a map of source_text → translated_text from all seeds.
func (ss *SeedStore) BuildTranslationMap(ctx context.Context) (map[string]string, error) {
	entries, err := ss.GetAll(ctx)
	if err != nil {
		return nil, err
	}

	m := make(map[string]string, len(entries))
	for _, e := range entries {
		m[e.SourceText] = e.TranslatedText
	}

	return m, nil
}

// escapeTSV replaces tabs and newlines in a string for TSV safety.
func escapeTSV(s string) string {
	s = strings.ReplaceAll(s, "\t", "\\t")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	return s
}
