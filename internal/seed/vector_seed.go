package seed

import (
	"context"
	"fmt"

	"rag-translator/internal/rag"
	"rag-translator/internal/textutil"

	"github.com/rs/zerolog/log"
)

// VectorSeeder computes and stores embeddings for seed translation entries.
type VectorSeeder struct {
	embeddingClient *rag.EmbeddingClient
	vectorStore     *rag.VectorStore
}

// NewVectorSeeder creates a new vector seeder.
func NewVectorSeeder(ec *rag.EmbeddingClient, vs *rag.VectorStore) *VectorSeeder {
	return &VectorSeeder{
		embeddingClient: ec,
		vectorStore:     vs,
	}
}

// IngestEmbeddings generates embeddings for seed entries and stores them in pgvector.
// Seed entries get a special "seed=true" context marker for prioritized retrieval.
func (vs *VectorSeeder) IngestEmbeddings(ctx context.Context, entries []SeedEntry, batchSize int) error {
	if len(entries) == 0 {
		return nil
	}

	// Collect unique source texts.
	seen := make(map[string]bool)
	var texts []string
	var contextStrs []string
	var hashes []string

	for _, e := range entries {
		if seen[e.Hash] {
			continue
		}
		seen[e.Hash] = true
		texts = append(texts, e.SourceText)
		contextStrs = append(contextStrs, buildSeedContext(e))
		hashes = append(hashes, e.Hash)
	}

	log.Info().Int("unique_texts", len(texts)).Msg("Generating seed embeddings")

	// Generate embeddings in batches.
	embeddings, err := vs.embeddingClient.EmbedBatch(ctx, texts, batchSize)
	if err != nil {
		return fmt.Errorf("generate seed embeddings: %w", err)
	}

	// Build records for vector store.
	var records []rag.EmbeddingRecord
	for i, text := range texts {
		if i >= len(embeddings) || embeddings[i] == nil {
			log.Warn().Str("text", textutil.Truncate(text, 30)).Msg("Missing embedding for seed text")
			continue
		}
		records = append(records, rag.EmbeddingRecord{
			Hash:     hashes[i],
			Source:   text,
			Context:  contextStrs[i],
			FilePath: "",
			Vector:   embeddings[i],
		})
	}

	if err := vs.vectorStore.Store(ctx, records); err != nil {
		return fmt.Errorf("store seed embeddings: %w", err)
	}

	log.Info().Int("stored", len(records)).Msg("Seed embeddings stored in pgvector")
	return nil
}

// buildSeedContext creates a context string for a seed entry, marked as seed for priority.
func buildSeedContext(e SeedEntry) string {
	ctx := fmt.Sprintf("seed=true; entity_type=%s; file=%s", e.EntityType, e.File)
	if e.Function != "" {
		ctx += fmt.Sprintf("; function=%s", e.Function)
	}
	ctx += fmt.Sprintf("; translated=%s", e.TranslatedText)
	return ctx
}
