package seed

import (
	"context"
	"fmt"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/rs/zerolog/log"
)

// GraphSeeder creates and updates Neo4j nodes for seed translation entries.
type GraphSeeder struct {
	driver neo4j.DriverWithContext
}

// NewGraphSeeder creates a new graph seeder.
func NewGraphSeeder(driver neo4j.DriverWithContext) *GraphSeeder {
	return &GraphSeeder{driver: driver}
}

// EnsureSchema creates constraints for seed nodes.
func (gs *GraphSeeder) EnsureSchema(ctx context.Context) error {
	session := gs.driver.NewSession(ctx, neo4j.SessionConfig{})
	defer session.Close(ctx)

	_, err := session.Run(ctx,
		"CREATE CONSTRAINT IF NOT EXISTS FOR (s:SeedTranslation) REQUIRE s.hash IS UNIQUE",
		nil,
	)
	if err != nil {
		return fmt.Errorf("create seed constraint: %w", err)
	}

	log.Info().Msg("Graph seed schema ensured")
	return nil
}

// UpsertSeedNodes creates or updates SeedTranslation nodes and links them to matching Term nodes.
func (gs *GraphSeeder) UpsertSeedNodes(ctx context.Context, entries []SeedEntry) error {
	session := gs.driver.NewSession(ctx, neo4j.SessionConfig{})
	defer session.Close(ctx)

	for _, e := range entries {
		// Create/update the SeedTranslation node.
		_, err := session.Run(ctx, `
			MERGE (s:SeedTranslation {hash: $hash})
			SET s.source_text = $source,
			    s.translated_text = $translated,
			    s.file = $file,
			    s.function_name = $function,
			    s.entity_type = $entity_type,
			    s.is_seed = true
		`, map[string]any{
			"hash":        e.Hash,
			"source":      e.SourceText,
			"translated":  e.TranslatedText,
			"file":        e.File,
			"function":    e.Function,
			"entity_type": e.EntityType,
		})
		if err != nil {
			log.Warn().Err(err).Str("hash", e.Hash).Msg("Failed to upsert seed node")
			continue
		}

		// Link to matching Term nodes (terminology that appears in the source text).
		_, err = session.Run(ctx, `
			MATCH (term:Term)
			WHERE $source CONTAINS term.chinese
			MATCH (s:SeedTranslation {hash: $hash})
			MERGE (s)-[:DEMONSTRATES_TERM]->(term)
		`, map[string]any{
			"source": e.SourceText,
			"hash":   e.Hash,
		})
		if err != nil {
			log.Warn().Err(err).Str("hash", e.Hash).Msg("Failed to link seed to terms")
		}

		// Also link to TextNode if exists (from prior ingestion).
		_, err = session.Run(ctx, `
			MATCH (t:TextNode {text: $source})
			MATCH (s:SeedTranslation {hash: $hash})
			MERGE (s)-[:TRANSLATES]->(t)
		`, map[string]any{
			"source": e.SourceText,
			"hash":   e.Hash,
		})
		if err != nil {
			// Not an error — TextNode may not exist yet.
			log.Debug().Str("hash", e.Hash).Msg("No matching TextNode for seed")
		}
	}

	log.Info().Int("entries", len(entries)).Msg("Upserted seed nodes in graph")
	return nil
}

// FindSeedTranslations queries the graph for seed translations relevant to a source text.
// Returns source→translated pairs from seed entries whose source_text appears in the input
// or whose associated terms match.
func (gs *GraphSeeder) FindSeedTranslations(ctx context.Context, text string) (map[string]string, error) {
	session := gs.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	// Find seeds where the source text contains matching terms, or where the seed's
	// source text overlaps with the query.
	result, err := session.Run(ctx, `
		MATCH (s:SeedTranslation)
		WHERE $text CONTAINS s.source_text
		   OR s.source_text CONTAINS $text
		RETURN s.source_text AS source, s.translated_text AS translated
		UNION
		MATCH (term:Term)
		WHERE $text CONTAINS term.chinese
		MATCH (s:SeedTranslation)-[:DEMONSTRATES_TERM]->(term)
		RETURN s.source_text AS source, s.translated_text AS translated
	`, map[string]any{"text": text})
	if err != nil {
		return nil, fmt.Errorf("find seed translations: %w", err)
	}

	pairs := make(map[string]string)
	for result.Next(ctx) {
		record := result.Record()
		source, _ := record.Get("source")
		translated, _ := record.Get("translated")
		pairs[fmt.Sprintf("%v", source)] = fmt.Sprintf("%v", translated)
	}

	return pairs, nil
}
