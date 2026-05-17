package llm

import (
	"context"
	"fmt"
)

// caller is the internal transport interface: given a full prompt string,
// return the raw LLM text response. Each provider implements this.
type caller interface {
	call(ctx context.Context, prompt string) (string, error)
}

// generateWithRetry runs the LLM call, attempts to parse, and retries once
// with a stricter instruction prepended if parsing fails.
func generateWithRetry(ctx context.Context, c caller, req StoryRequest) (*GeneratedStory, error) {
	prompt := systemPrompt + "\n\n" + buildPrompt(req)

	raw, err := c.call(ctx, prompt)
	if err != nil {
		return nil, err
	}

	story, parseErr := parseStoryJSON(raw)
	if parseErr == nil {
		return story, nil
	}

	// Retry once with a stricter instruction
	strictPrompt := "You MUST respond with ONLY a valid JSON object matching the schema. " +
		"No markdown, no code fences, no prose before or after.\n\n" + prompt

	raw2, retryErr := c.call(ctx, strictPrompt)
	if retryErr != nil {
		// Surface the original parse error, not the retry transport error
		return nil, fmt.Errorf("%w (retry also failed: %s)", parseErr, retryErr)
	}

	return parseStoryJSON(raw2)
}
