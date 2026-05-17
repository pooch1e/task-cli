package llm

import (
	"bytes"
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
	cfg *config.Config
}

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

func (c *OpenAIClient) GenerateStory(req StoryRequest) (*GeneratedStory, error) {
	body := openAIRequest{
		Model: c.cfg.LLM.Model,
		Messages: []openAIMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: buildPrompt(req)},
		},
		MaxTokens:      c.cfg.LLM.MaxTokens,
		ResponseFormat: &responseFormat{Type: "json_object"},
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: time.Duration(c.cfg.LLM.TimeoutSecs) * time.Second}

	httpReq, err := http.NewRequest("POST", c.cfg.LLM.BaseURL+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.cfg.LLM.APIKey)

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024)) // cap at 64KB
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	var apiResp openAIResponse
	if err := json.Unmarshal(respBytes, &apiResp); err != nil {
		return nil, fmt.Errorf("parsing API response: %w", err)
	}

	if apiResp.Error != nil {
		return nil, fmt.Errorf("API error (%s): %s", apiResp.Error.Type, apiResp.Error.Message)
	}

	if len(apiResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in API response")
	}

	content := strings.TrimSpace(apiResp.Choices[0].Message.Content)
	return parseStoryJSON(content)
}

// parseStoryJSON parses the LLM response, stripping markdown fences defensively.
func parseStoryJSON(raw string) (*GeneratedStory, error) {
	// Strip ```json ... ``` or ``` ... ``` fences if present
	if idx := strings.Index(raw, "```"); idx != -1 {
		raw = raw[idx:]
		raw = strings.TrimPrefix(raw, "```json")
		raw = strings.TrimPrefix(raw, "```")
		if end := strings.LastIndex(raw, "```"); end != -1 {
			raw = raw[:end]
		}
	}
	raw = strings.TrimSpace(raw)

	var story GeneratedStory
	if err := json.Unmarshal([]byte(raw), &story); err != nil {
		return nil, fmt.Errorf("parsing story JSON: %w\nraw: %s", err, raw)
	}

	if story.Story.Title == "" {
		return nil, fmt.Errorf("LLM returned empty story title")
	}
	if len(story.Tasks) == 0 {
		return nil, fmt.Errorf("LLM returned no tasks")
	}

	return &story, nil
}
