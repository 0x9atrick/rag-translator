package rag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
)

// EmbeddingClient generates text embeddings via an OpenAI-compatible API (Qwen3-Embedding).
type EmbeddingClient struct {
	apiKey     string
	model      string
	baseURL    string
	dimensions int
	httpClient *http.Client
}

// NewEmbeddingClient creates a new embedding client for Qwen3-Embedding.
// baseURL should be the embedding API endpoint (e.g. https://dashscope.aliyuncs.com/compatible-mode/v1).
func NewEmbeddingClient(apiKey, model, baseURL string, dimensions int) *EmbeddingClient {
	if dimensions <= 0 {
		dimensions = 1024
	}
	return &EmbeddingClient{
		apiKey:     apiKey,
		model:      model,
		baseURL:    baseURL,
		dimensions: dimensions,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// --- OpenAI-compatible request/response types ---

type embeddingRequest struct {
	Input      []string `json:"input"`
	Model      string   `json:"model"`
	Dimensions int      `json:"dimensions,omitempty"`
}

type embeddingResponse struct {
	Data  []embeddingData `json:"data"`
	Usage embeddingUsage  `json:"usage"`
}

type embeddingData struct {
	Embedding []float32 `json:"embedding"`
	Index     int       `json:"index"`
}

type embeddingUsage struct {
	TotalTokens int `json:"total_tokens"`
}

// Embed generates embeddings for a batch of texts.
func (ec *EmbeddingClient) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	reqBody := embeddingRequest{
		Input:      texts,
		Model:      ec.model,
		Dimensions: ec.dimensions,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal embedding request: %w", err)
	}

	url := ec.baseURL + "/embeddings"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create embedding request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ec.apiKey)

	resp, err := ec.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedding API call: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read embedding response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embedding API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var embedResp embeddingResponse
	if err := json.Unmarshal(respBody, &embedResp); err != nil {
		return nil, fmt.Errorf("unmarshal embedding response: %w", err)
	}

	// Build result ordered by index.
	results := make([][]float32, len(texts))
	for _, d := range embedResp.Data {
		if d.Index < len(results) {
			results[d.Index] = d.Embedding
		}
	}

	log.Debug().
		Int("texts", len(texts)).
		Int("tokens", embedResp.Usage.TotalTokens).
		Msg("Generated embeddings")

	return results, nil
}

// EmbedBatch processes texts in batches, respecting API limits.
func (ec *EmbeddingClient) EmbedBatch(ctx context.Context, texts []string, batchSize int) ([][]float32, error) {
	if batchSize <= 0 {
		batchSize = 32
	}

	var allEmbeddings [][]float32

	for i := 0; i < len(texts); i += batchSize {
		end := i + batchSize
		if end > len(texts) {
			end = len(texts)
		}

		batch := texts[i:end]
		embeddings, err := ec.Embed(ctx, batch)
		if err != nil {
			return nil, fmt.Errorf("embed batch [%d:%d]: %w", i, end, err)
		}

		allEmbeddings = append(allEmbeddings, embeddings...)

		log.Info().
			Int("batch", i/batchSize+1).
			Int("processed", len(allEmbeddings)).
			Int("total", len(texts)).
			Msg("Embedding progress")
	}

	return allEmbeddings, nil
}

// EmbedQuery generates an embedding for a search query.
func (ec *EmbeddingClient) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	results, err := ec.Embed(ctx, []string{text})
	if err != nil {
		return nil, fmt.Errorf("query embedding: %w", err)
	}
	if len(results) == 0 || results[0] == nil {
		return nil, fmt.Errorf("no embedding returned for query")
	}
	return results[0], nil
}
