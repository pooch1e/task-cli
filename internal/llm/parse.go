package llm

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Validation limits — named constants so any change is a single edit.
const (
	maxTitleLen  = 120
	maxTasks     = 12
	maxSubtasks  = 8
)

// stripFences removes a single ```json ... ``` or ``` ... ``` markdown fence.
// If no fence is present the string is returned unchanged.
func stripFences(s string) string {
	s = strings.TrimSpace(s)
	start := strings.Index(s, "```")
	if start == -1 {
		return s
	}
	s = s[start+3:]                   // skip opening ```
	s = strings.TrimPrefix(s, "json") // strip optional language tag
	s = strings.TrimSpace(s)
	if end := strings.Index(s, "```"); end != -1 {
		s = s[:end]
	}
	return strings.TrimSpace(s)
}

// parseStoryJSON unmarshals a raw LLM response into a GeneratedStory,
// defensively stripping markdown fences first.
func parseStoryJSON(raw string) (*GeneratedStory, error) {
	clean := stripFences(strings.TrimSpace(raw))

	var story GeneratedStory
	if err := json.Unmarshal([]byte(clean), &story); err != nil {
		return nil, fmt.Errorf("parsing story JSON: %w (first 200 chars: %.200s)", err, clean)
	}

	if err := validateStory(&story); err != nil {
		return nil, err
	}

	return &story, nil
}

// validateStory enforces schema bounds on a parsed GeneratedStory.
func validateStory(s *GeneratedStory) error {
	if s.Story.Title == "" {
		return fmt.Errorf("LLM returned empty story title")
	}
	if len(s.Story.Title) > maxTitleLen {
		return fmt.Errorf("story title too long (%d chars, max %d)", len(s.Story.Title), maxTitleLen)
	}
	if len(s.Tasks) == 0 {
		return fmt.Errorf("LLM returned no tasks")
	}
	if len(s.Tasks) > maxTasks {
		return fmt.Errorf("too many tasks (%d, max %d)", len(s.Tasks), maxTasks)
	}
	for i, t := range s.Tasks {
		if t.Title == "" {
			return fmt.Errorf("task %d has empty title", i+1)
		}
		if len(t.Subtasks) > maxSubtasks {
			return fmt.Errorf("task %d has too many subtasks (%d, max %d)", i+1, len(t.Subtasks), maxSubtasks)
		}
	}
	return nil
}
