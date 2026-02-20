package rag

import (
	"context"
	"fmt"
	"strings"

	"rag-translator/internal/graph"
	"rag-translator/internal/textutil"

	"github.com/rs/zerolog/log"
)

// RetrievalResult combines vector, graph, and seed context for a translation request.
type RetrievalResult struct {
	// SeedTranslations are manually-verified translations from the seed corpus (highest priority).
	SeedTranslations map[string]string
	// SimilarTexts from vector search.
	SimilarTexts []SearchResult
	// GraphContext from knowledge graph traversal.
	GraphContext *graph.QueryResult
}

// SeedQuerier is an interface for querying seed translations from the graph.
type SeedQuerier interface {
	FindSeedTranslations(ctx context.Context, text string) (map[string]string, error)
}

// Retriever combines vector store, knowledge graph, and seed corpus for RAG.
type Retriever struct {
	vectorStore     *VectorStore
	embeddingClient *EmbeddingClient
	graphQuerier    *graph.GraphQuerier
	seedQuerier     SeedQuerier // optional, nil if seeds not ingested yet
}

// NewRetriever creates a new combined retriever.
func NewRetriever(vs *VectorStore, ec *EmbeddingClient, gq *graph.GraphQuerier) *Retriever {
	return &Retriever{
		vectorStore:     vs,
		embeddingClient: ec,
		graphQuerier:    gq,
	}
}

// SetSeedQuerier attaches a seed querier for prioritized seed retrieval.
func (r *Retriever) SetSeedQuerier(sq SeedQuerier) {
	r.seedQuerier = sq
}

// Retrieve fetches relevant context for a given source text.
// Priority order: seed translations > vector search > graph context.
func (r *Retriever) Retrieve(ctx context.Context, sourceText string, topK int) (*RetrievalResult, error) {
	result := &RetrievalResult{}

	// 1. Seed translations (highest priority — manually verified).
	if r.seedQuerier != nil {
		seeds, err := r.seedQuerier.FindSeedTranslations(ctx, sourceText)
		if err != nil {
			log.Warn().Err(err).Msg("Seed query failed")
		} else if len(seeds) > 0 {
			result.SeedTranslations = seeds
		}
	}

	// 2. Vector similarity search.
	queryVec, err := r.embeddingClient.EmbedQuery(ctx, sourceText)
	if err != nil {
		log.Warn().Err(err).Str("text", textutil.Truncate(sourceText, 50)).Msg("Failed to embed query, skipping vector search")
	} else {
		similar, err := r.vectorStore.Search(ctx, queryVec, topK)
		if err != nil {
			log.Warn().Err(err).Msg("Vector search failed")
		} else {
			result.SimilarTexts = similar
		}
	}

	// 3. Graph knowledge retrieval.
	graphCtx, err := r.graphQuerier.FindRelatedTerms(ctx, sourceText)
	if err != nil {
		log.Warn().Err(err).Msg("Graph query failed")
	} else {
		result.GraphContext = graphCtx
	}

	return result, nil
}

// BuildContextString formats retrieval results into a string for the prompt.
// Seed translations appear first for highest priority.
func (r *Retriever) BuildContextString(result *RetrievalResult) string {
	var sb strings.Builder

	// Seed translations first — these are manually verified and highest priority.
	if len(result.SeedTranslations) > 0 {
		sb.WriteString("=== Verified Seed Translations (USE THESE AS REFERENCE) ===\n")
		for src, dst := range result.SeedTranslations {
			sb.WriteString(fmt.Sprintf("• %s → %s\n", src, dst))
		}
		sb.WriteString("\n")
	}

	if len(result.SimilarTexts) > 0 {
		sb.WriteString("=== Similar Translations ===\n")
		for i, st := range result.SimilarTexts {
			sb.WriteString(fmt.Sprintf("%d. [Score: %.3f] %s", i+1, st.Score, st.Source))
			if st.Context != "" {
				sb.WriteString(fmt.Sprintf(" (Context: %s)", st.Context))
			}
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	if result.GraphContext != nil && len(result.GraphContext.Terms) > 0 {
		sb.WriteString("=== Terminology from Knowledge Graph ===\n")
		for _, term := range result.GraphContext.Terms {
			sb.WriteString(fmt.Sprintf("• %s → %s", term.Chinese, term.Vietnamese))
			if term.Category != "" {
				sb.WriteString(fmt.Sprintf(" [%s]", term.Category))
			}
			sb.WriteString("\n")
		}
		sb.WriteString("\n")

		if len(result.GraphContext.Relationships) > 0 {
			sb.WriteString("=== Entity Relationships ===\n")
			for _, rel := range result.GraphContext.Relationships {
				sb.WriteString(fmt.Sprintf("• %s -[%s]-> %s\n", rel.From, rel.Type, rel.To))
			}
			sb.WriteString("\n")
		}
	}

	return sb.String()
}
