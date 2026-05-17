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

// OpenAIClient works with DeepSeek, OpenAI, and any OpenAI-compatible endpoint.
type OpenAIClient struct {
	cfg        *config.Config
	httpClient *http.Client
}

func newOpenAIClient(cfg *config.Config) *OpenAIClient {
	return &OpenAIClient{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: time.Duration(cfg.LLM.TimeoutSecs) * time.Second},
	}
}

func (c *OpenAIClient) GenerateStory(req StoryRequest) (*GeneratedStory, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(c.cfg.LLM.TimeoutSecs)*time.Second)
	defer cancel()
	return generateWithRetry(ctx, c, req)
}

// call implements the caller interface: builds the OpenAI messages payload,
// POSTs to the configured endpoint, and returns the assistant text.
func (c *OpenAIClient) call(ctx context.Context, prompt string) (string, error) {
	body := openAIRequest{
		Model: c.cfg.LLM.Model,
		Messages: []openAIMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: prompt},
		},
		MaxTokens:      c.cfg.LLM.MaxTokens,
		ResponseFormat: &responseFormat{Type: "json_object"},
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPost,
		c.cfg.LLM.BaseURL+"/chat/completions",
		bytes.NewReader(raw),
	)
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.cfg.LLM.APIKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	var apiResp openAIResponse
	if err := json.Unmarshal(respBytes, &apiResp); err != nil {
		return "", fmt.Errorf("parsing API response: %w", err)
	}
	if apiResp.Error != nil {
		return "", fmt.Errorf("API error (%s): %s", apiResp.Error.Type, apiResp.Error.Message)
	}
	if len(apiResp.Choices) == 0 {
		return "", fmt.Errorf("no choices in API response")
	}

	return strings.TrimSpace(apiResp.Choices[0].Message.Content), nil
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
