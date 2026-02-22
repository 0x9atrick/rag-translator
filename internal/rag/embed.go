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

const geminiEmbedBaseURL = "https://generativelanguage.googleapis.com/v1beta/models"

// EmbeddingClient generates text embeddings via the Google Gemini Embedding API.
type EmbeddingClient struct {
	apiKey     string
	model      string
	dimensions int
	httpClient *http.Client
}

// NewEmbeddingClient creates a new Gemini embedding client.
func NewEmbeddingClient(apiKey, model string, dimensions int) *EmbeddingClient {
	if dimensions <= 0 {
		dimensions = 768
	}
	return &EmbeddingClient{
		apiKey:     apiKey,
		model:      model,
		dimensions: dimensions,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// --- Gemini Embedding API types ---

type batchEmbedRequest struct {
	Requests []singleEmbedRequest `json:"requests"`
}

type singleEmbedRequest struct {
	Model                string        `json:"model"`
	Content              geminiContent `json:"content"`
	OutputDimensionality int           `json:"outputDimensionality,omitempty"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type batchEmbedResponse struct {
	Embeddings []embeddingValues `json:"embeddings"`
}

type embeddingValues struct {
	Values []float32 `json:"values"`
}

// Embed generates embeddings for a batch of texts using Gemini batchEmbedContents.
func (ec *EmbeddingClient) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	modelPath := fmt.Sprintf("models/%s", ec.model)

	requests := make([]singleEmbedRequest, len(texts))
	for i, text := range texts {
		requests[i] = singleEmbedRequest{
			Model: modelPath,
			Content: geminiContent{
				Parts: []geminiPart{{Text: text}},
			},
			OutputDimensionality: ec.dimensions,
		}
	}

	reqBody := batchEmbedRequest{Requests: requests}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal embedding request: %w", err)
	}

	url := fmt.Sprintf("%s/%s:batchEmbedContents?key=%s", geminiEmbedBaseURL, ec.model, ec.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create embedding request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

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

	var embedResp batchEmbedResponse
	if err := json.Unmarshal(respBody, &embedResp); err != nil {
		return nil, fmt.Errorf("unmarshal embedding response: %w", err)
	}

	results := make([][]float32, len(texts))
	for i, emb := range embedResp.Embeddings {
		if i < len(results) {
			results[i] = emb.Values
		}
	}

	log.Debug().
		Int("texts", len(texts)).
		Int("embeddings", len(embedResp.Embeddings)).
		Msg("Generated embeddings")

	return results, nil
}

// EmbedBatch processes texts in batches, respecting API limits.
// Gemini batchEmbedContents supports up to 100 texts per request.
func (ec *EmbeddingClient) EmbedBatch(ctx context.Context, texts []string, batchSize int) ([][]float32, error) {
	if batchSize <= 0 {
		batchSize = 100
	}
	// Gemini limit is 100 per batch request.
	if batchSize > 100 {
		batchSize = 100
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
