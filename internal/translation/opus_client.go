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

const anthropicAPIURL = "https://api.anthropic.com/v1/messages"

// OpusClient handles translation requests via the Anthropic Messages API.
type OpusClient struct {
	apiKey     string
	model      string
	httpClient *http.Client
}

// NewOpusClient creates a new Anthropic translation client.
func NewOpusClient(apiKey, model string) *OpusClient {
	return &OpusClient{
		apiKey: apiKey,
		model:  model,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system"`
	Messages  []anthropicMessage `json:"messages"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicResponse struct {
	Content []anthropicContent `json:"content"`
	Usage   anthropicUsage     `json:"usage"`
	Error   *anthropicError    `json:"error,omitempty"`
}

type anthropicContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type anthropicError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// Translate sends a translation request to Anthropic and returns the translated text.
func (oc *OpusClient) Translate(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	reqBody := anthropicRequest{
		Model:     oc.model,
		MaxTokens: 4096,
		System:    systemPrompt,
		Messages: []anthropicMessage{
			{Role: "user", Content: userPrompt},
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
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, anthropicAPIURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", oc.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

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

	var apiResp anthropicResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return "", fmt.Errorf("unmarshal response: %w", err)
	}

	if apiResp.Error != nil {
		return "", fmt.Errorf("API error: %s: %s", apiResp.Error.Type, apiResp.Error.Message)
	}

	if len(apiResp.Content) == 0 {
		return "", fmt.Errorf("empty response content")
	}

	// Extract text content.
	var result strings.Builder
	for _, c := range apiResp.Content {
		if c.Type == "text" {
			result.WriteString(c.Text)
		}
	}

	log.Debug().
		Int("input_tokens", apiResp.Usage.InputTokens).
		Int("output_tokens", apiResp.Usage.OutputTokens).
		Msg("Translation complete")

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
