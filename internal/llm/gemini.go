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

// GeminiClient calls the Google Gemini REST API directly.
// Auth is via an API key query parameter (not a Bearer header).
type GeminiClient struct {
	cfg        *config.Config
	httpClient *http.Client
}

func newGeminiClient(cfg *config.Config) *GeminiClient {
	return &GeminiClient{cfg: cfg, httpClient: &http.Client{}}
}

func (c *GeminiClient) GenerateStory(req StoryRequest) (*GeneratedStory, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(c.cfg.LLM.TimeoutSecs)*time.Second)
	defer cancel()
	return generateWithRetry(ctx, c, req)
}

// Ping checks connectivity by sending a minimal 1-token request.
func (c *GeminiClient) Ping() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := c.call(ctx, PromptParts{
		System: "Respond with valid JSON only.",
		User:   `{"ok":true}`,
	})
	return err
}

// call implements the caller interface for the Gemini generateContent endpoint.
func (c *GeminiClient) call(ctx context.Context, parts PromptParts) (string, error) {
	body := geminiRequest{
		Contents: []geminiContent{
			{Role: "user", Parts: []geminiPart{{Text: parts.User}}},
		},
		SystemInstruction: &geminiContent{
			Parts: []geminiPart{{Text: parts.System}},
		},
		GenerationConfig: geminiGenerationConfig{
			ResponseMIMEType: "application/json",
			MaxOutputTokens:  c.cfg.LLM.MaxTokens,
			Temperature:      0.3,
		},
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("%s/models/%s:generateContent?key=%s",
		strings.TrimRight(c.cfg.LLM.BaseURL, "/"),
		c.cfg.LLM.Model,
		c.cfg.LLM.APIKey,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("Gemini request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return "", fmt.Errorf("reading Gemini response: %w", err)
	}
	if int64(len(respBytes)) > maxResponseBytes {
		return "", fmt.Errorf("Gemini response exceeded %d byte limit", maxResponseBytes)
	}

	var apiResp geminiResponse
	if err := json.Unmarshal(respBytes, &apiResp); err != nil {
		return "", fmt.Errorf("parsing Gemini response: %w", err)
	}
	if apiResp.Error != nil {
		return "", fmt.Errorf("Gemini API error (%d): %s", apiResp.Error.Code, apiResp.Error.Message)
	}
	if len(apiResp.Candidates) == 0 || len(apiResp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("Gemini returned no content (body: %.200s)", string(respBytes))
	}

	return strings.TrimSpace(apiResp.Candidates[0].Content.Parts[0].Text), nil
}

// ── wire types ────────────────────────────────────────────────────────────────

type geminiRequest struct {
	Contents          []geminiContent       `json:"contents"`
	SystemInstruction *geminiContent        `json:"systemInstruction,omitempty"`
	GenerationConfig  geminiGenerationConfig `json:"generationConfig"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiGenerationConfig struct {
	ResponseMIMEType string  `json:"responseMimeType,omitempty"`
	MaxOutputTokens  int     `json:"maxOutputTokens,omitempty"`
	Temperature      float64 `json:"temperature,omitempty"`
}

type geminiResponse struct {
	Candidates []struct {
		Content geminiContent `json:"content"`
	} `json:"candidates"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}
