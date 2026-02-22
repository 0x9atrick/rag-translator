package translation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

const geminiBaseURL = "https://generativelanguage.googleapis.com/v1beta/models"

// OpusClient handles translation requests via the Google Gemini API.
type OpusClient struct {
	apiKey     string
	model      string
	httpClient *http.Client
}

// NewOpusClient creates a new Gemini translation client.
func NewOpusClient(apiKey, model string) *OpusClient {
	return &OpusClient{
		apiKey: apiKey,
		model:  model,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

// --- Gemini API request/response types ---

type geminiRequest struct {
	SystemInstruction *geminiContent  `json:"systemInstruction,omitempty"`
	Contents          []geminiContent `json:"contents"`
	GenerationConfig  *genConfig      `json:"generationConfig,omitempty"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
	Role  string       `json:"role,omitempty"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type genConfig struct {
	MaxOutputTokens int     `json:"maxOutputTokens,omitempty"`
	Temperature     float64 `json:"temperature,omitempty"`
}

type geminiResponse struct {
	Candidates    []geminiCandidate `json:"candidates"`
	UsageMetadata *geminiUsage      `json:"usageMetadata,omitempty"`
	Error         *geminiError      `json:"error,omitempty"`
}

type geminiCandidate struct {
	Content geminiContent `json:"content"`
}

type geminiUsage struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

type geminiError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Status  string `json:"status"`
}

// Translate sends a translation request to Gemini and returns the translated text.
func (oc *OpusClient) Translate(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	reqBody := geminiRequest{
		SystemInstruction: &geminiContent{
			Parts: []geminiPart{{Text: systemPrompt}},
		},
		Contents: []geminiContent{
			{
				Role:  "user",
				Parts: []geminiPart{{Text: userPrompt}},
			},
		},
		GenerationConfig: &genConfig{
			MaxOutputTokens: 8192,
			Temperature:     0.3,
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal translation request: %w", err)
	}

	var lastErr error
	maxRetries := 3

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(attempt*2) * time.Second
			log.Warn().Int("attempt", attempt+1).Dur("backoff", backoff).Msg("Retrying translation")
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(backoff):
			}
		}

		result, err := oc.doRequest(ctx, bodyBytes)
		if err == nil {
			return result, nil
		}
		lastErr = err

		// Don't retry on context cancellation.
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
	}

	return "", fmt.Errorf("translation failed after %d retries: %w", maxRetries, lastErr)
}

func (oc *OpusClient) doRequest(ctx context.Context, bodyBytes []byte) (string, error) {
	url := fmt.Sprintf("%s/%s:generateContent?key=%s", geminiBaseURL, oc.model, oc.apiKey)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := oc.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("API call: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return "", fmt.Errorf("retryable error (status %d): %s", resp.StatusCode, string(respBody))
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var apiResp geminiResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return "", fmt.Errorf("unmarshal response: %w", err)
	}

	if apiResp.Error != nil {
		return "", fmt.Errorf("API error [%s]: %s", apiResp.Error.Status, apiResp.Error.Message)
	}

	if len(apiResp.Candidates) == 0 {
		return "", fmt.Errorf("empty response: no candidates")
	}

	// Extract text from the first candidate.
	var result strings.Builder
	for _, p := range apiResp.Candidates[0].Content.Parts {
		result.WriteString(p.Text)
	}

	if apiResp.UsageMetadata != nil {
		log.Debug().
			Int("prompt_tokens", apiResp.UsageMetadata.PromptTokenCount).
			Int("output_tokens", apiResp.UsageMetadata.CandidatesTokenCount).
			Msg("Translation complete")
	}

	return strings.TrimSpace(result.String()), nil
}

// TranslateBatch translates multiple texts using a single API call for efficiency.
func (oc *OpusClient) TranslateBatch(ctx context.Context, systemPrompt string, texts []string) ([]string, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	// Build a combined prompt for batch translation.
	var sb strings.Builder
	sb.WriteString("Translate each of the following texts. Return ONLY the translations, one per line, in the same order.\n")
	sb.WriteString("Use ||| as a delimiter between translations.\n\n")
	for i, t := range texts {
		sb.WriteString(fmt.Sprintf("[%d] %s\n", i+1, t))
	}

	response, err := oc.Translate(ctx, systemPrompt, sb.String())
	if err != nil {
		return nil, err
	}

	// Parse batch response.
	parts := strings.Split(response, "|||")
	results := make([]string, len(texts))
	for i := range results {
		if i < len(parts) {
			results[i] = strings.TrimSpace(parts[i])
		} else {
			results[i] = texts[i] // fallback to original if parsing fails
		}
	}

	return results, nil
}
