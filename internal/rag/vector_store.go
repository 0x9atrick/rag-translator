package rag

import (
	"context"
	"fmt"

	"rag-translator/internal/dbgen"

	"github.com/jackc/pgx/v5/pgxpool"
	pgvector "github.com/pgvector/pgvector-go"
	"github.com/rs/zerolog/log"
)

// VectorStore handles pgvector-backed embedding storage and similarity search.
type VectorStore struct {
	pool    *pgxpool.Pool
	queries *dbgen.Queries
}

// NewVectorStore creates a new vector store.
func NewVectorStore(pool *pgxpool.Pool) *VectorStore {
	return &VectorStore{
		pool:    pool,
		queries: dbgen.New(pool),
	}
}

// EmbeddingRecord represents a text with its embedding.
type EmbeddingRecord struct {
	Hash     string
	Source   string
	Context  string
	FilePath string
	Vector   []float32
}

// SearchResult represents a similarity search match.
type SearchResult struct {
	Source  string
	Context string
	Score   float64
}

// Store batch-inserts embedding records via sqlc.
func (vs *VectorStore) Store(ctx context.Context, records []EmbeddingRecord) error {
	if len(records) == 0 {
		return nil
	}

	for _, r := range records {
		err := vs.queries.InsertEmbeddingWithVector(ctx, dbgen.InsertEmbeddingWithVectorParams{
			Hash:     r.Hash,
			Source:   r.Source,
			Context:  r.Context,
			FilePath: r.FilePath,
			Column5:  pgvector.NewVector(r.Vector),
		})
		if err != nil {
			return fmt.Errorf("insert embedding %s: %w", r.Hash, err)
		}
	}

	log.Info().Int("count", len(records)).Msg("Stored embeddings")
	return nil
}

// Search finds the top-K most similar embeddings to the query vector.
func (vs *VectorStore) Search(ctx context.Context, queryVector []float32, topK int) ([]SearchResult, error) {
	rows, err := vs.queries.SearchSimilarEmbeddings(ctx, dbgen.SearchSimilarEmbeddingsParams{
		Column1: pgvector.NewVector(queryVector),
		Limit:   int32(topK),
	})
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}

	results := make([]SearchResult, 0, len(rows))
	for _, row := range rows {
		results = append(results, SearchResult{
			Source:  row.Source,
			Context: row.Context,
			Score:   row.Similarity,
		})
	}

	return results, nil
}
