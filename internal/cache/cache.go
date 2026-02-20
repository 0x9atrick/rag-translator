package cache

import (
	"context"
	"fmt"
	"sync"

	"rag-translator/internal/dbgen"
	"rag-translator/internal/textutil"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// TranslationCache provides in-memory + PostgreSQL-backed caching for translations.
type TranslationCache struct {
	queries *dbgen.Queries
	mu      sync.RWMutex
	memory  map[string]string // hash → translated text
}

// NewTranslationCache creates a new cache backed by PostgreSQL.
func NewTranslationCache(pool *pgxpool.Pool) *TranslationCache {
	return &TranslationCache{
		queries: dbgen.New(pool),
		memory:  make(map[string]string),
	}
}

// Get retrieves a cached translation. Returns empty string and false if not found.
func (c *TranslationCache) Get(ctx context.Context, sourceText string) (string, bool) {
	hash := textutil.Hash(sourceText)

	// Check in-memory cache first.
	c.mu.RLock()
	if v, ok := c.memory[hash]; ok {
		c.mu.RUnlock()
		return v, true
	}
	c.mu.RUnlock()

	// Check PostgreSQL via sqlc.
	translated, err := c.queries.GetCachedTranslation(ctx, hash)
	if err != nil {
		return "", false
	}

	// Populate in-memory cache.
	c.mu.Lock()
	c.memory[hash] = translated
	c.mu.Unlock()

	return translated, true
}

// Set stores a translation in both in-memory and PostgreSQL cache.
func (c *TranslationCache) Set(ctx context.Context, sourceText, translated string) error {
	hash := textutil.Hash(sourceText)

	// Update in-memory.
	c.mu.Lock()
	c.memory[hash] = translated
	c.mu.Unlock()

	// Upsert via sqlc.
	err := c.queries.UpsertCachedTranslation(ctx, dbgen.UpsertCachedTranslationParams{
		Hash:       hash,
		Source:     sourceText,
		Translated: translated,
	})
	if err != nil {
		return fmt.Errorf("cache set: %w", err)
	}

	return nil
}

// SetBatch stores multiple translations efficiently.
func (c *TranslationCache) SetBatch(ctx context.Context, pairs map[string]string) error {
	for source, translated := range pairs {
		if err := c.Set(ctx, source, translated); err != nil {
			return err
		}
	}
	return nil
}

// Preload loads all cached translations into memory.
func (c *TranslationCache) Preload(ctx context.Context) error {
	rows, err := c.queries.ListAllCachedTranslations(ctx)
	if err != nil {
		return fmt.Errorf("preload cache: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	for _, row := range rows {
		c.memory[row.Hash] = row.Translated
	}

	log.Info().Int("count", len(rows)).Msg("Preloaded translation cache")
	return nil
}
