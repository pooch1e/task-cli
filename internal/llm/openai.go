package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/joelkram/task-cli/internal/config"
)

// maxResponseBytes caps the LLM response body read. Exceeding this returns an
// explicit error rather than silently truncating.
const maxResponseBytes = 64 * 1024

// OpenAIClient works with DeepSeek, OpenAI, and any OpenAI-compatible endpoint.
type OpenAIClient struct {
	cfg        *config.Config
	httpClient *http.Client
}

func newOpenAIClient(cfg *config.Config) *OpenAIClient {
	// No Timeout on the http.Client — context-based timeout in GenerateStory
	// is the single source of truth to avoid compounding timeouts.
	return &OpenAIClient{cfg: cfg, httpClient: &http.Client{}}
}

func (c *OpenAIClient) GenerateStory(req StoryRequest) (*GeneratedStory, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(c.cfg.LLM.TimeoutSecs)*time.Second)
	defer cancel()
	return generateWithRetry(ctx, c, req)
}

// Ping sends a minimal single-token request to verify credentials and connectivity.
func (c *OpenAIClient) Ping() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := c.call(ctx, PromptParts{
		System: "Respond with valid JSON only.",
		User:   `Respond with exactly: {"ok":true}`,
	})
	return err
}

// call implements the caller interface: builds the OpenAI messages payload,
// POSTs to the configured endpoint, and returns the assistant text.
func (c *OpenAIClient) call(ctx context.Context, parts PromptParts) (string, error) {
	body := openAIRequest{
		Model: c.cfg.LLM.Model,
		Messages: []openAIMessage{
			{Role: "system", Content: parts.System},
			{Role: "user", Content: parts.User},
		},
		MaxTokens:      c.cfg.LLM.MaxTokens,
		ResponseFormat: &responseFormat{Type: "json_object"},
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost,
		c.cfg.LLM.BaseURL+"/chat/completions",
		bytes.NewReader(raw),
	)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.LLM.APIKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// Sanitize error: ensure API key cannot appear in the error string.
		return "", fmt.Errorf("API request failed: %s", sanitizeErrorMsg(err.Error(), c.cfg.LLM.APIKey))
	}
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, maxResponseBytes+1)
	respBytes, err := io.ReadAll(limited)
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}
	if int64(len(respBytes)) > maxResponseBytes {
		return "", fmt.Errorf("response exceeded %d byte limit", maxResponseBytes)
	}

	var apiResp openAIResponse
	if err := json.Unmarshal(respBytes, &apiResp); err != nil {
		return "", fmt.Errorf("parsing API response: %w", err)
	}
	if apiResp.Error != nil {
		return "", fmt.Errorf("API error (%s): %s", apiResp.Error.Type, apiResp.Error.Message)
	}
	if len(apiResp.Choices) == 0 {
		return "", fmt.Errorf("no choices in API response (body: %.200s)", string(respBytes))
	}

	return strings.TrimSpace(apiResp.Choices[0].Message.Content), nil
}

// sanitizeErrorMsg replaces any occurrence of the API key in an error message
// with a redacted placeholder.
func sanitizeErrorMsg(msg, apiKey string) string {
	if apiKey == "" || len(apiKey) < 4 {
		return msg
	}
	return strings.ReplaceAll(msg, apiKey, apiKey[:4]+"…[redacted]")
}

// ── wire types ────────────────────────────────────────────────────────────────

type openAIRequest struct {
	Model          string          `json:"model"`
	Messages       []openAIMessage `json:"messages"`
	MaxTokens      int             `json:"max_tokens"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type responseFormat struct {
	Type string `json:"type"`
}

type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}
