package llm

import (
	"context"
	"errors"
	"fmt"
)

// caller is the internal transport interface: given a PromptParts, return the
// raw LLM text response. Each provider implements this.
type caller interface {
	call(ctx context.Context, parts PromptParts) (string, error)
}

// generateWithRetry runs the LLM call, attempts to parse, and retries once
// with a stricter instruction prepended to the user message if parsing fails.
func generateWithRetry(ctx context.Context, c caller, req StoryRequest) (*GeneratedStory, error) {
	parts := buildPromptParts(req)

	raw, err := c.call(ctx, parts)
	if err != nil {
		return nil, err
	}

	story, parseErr := parseStoryJSON(raw)
	if parseErr == nil {
		return story, nil
	}

	// Retry once: prepend a stricter instruction to the user message.
	strict := PromptParts{
		System: parts.System,
		User: "You MUST respond with ONLY a valid JSON object matching the schema. " +
			"No markdown, no code fences, no prose before or after.\n\n" + parts.User,
	}
	raw2, retryErr := c.call(ctx, strict)
	if retryErr != nil {
		return nil, fmt.Errorf("parse failed: %w; retry call also failed: %s", parseErr, retryErr)
	}

	story2, parseErr2 := parseStoryJSON(raw2)
	if parseErr2 != nil {
		return nil, fmt.Errorf("parse failed after retry: %w (initial error: %s)", parseErr2, parseErr)
	}
	return story2, nil
}

// errRetry is a sentinel used by tests to distinguish a first-attempt parse
// failure from a transport error.
var errRetry = errors.New("retry")
