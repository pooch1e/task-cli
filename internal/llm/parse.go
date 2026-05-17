package llm

import (
	"encoding/json"
	"fmt"
	"strings"
)

// stripFences removes ```json ... ``` or ``` ... ``` markdown fences from s.
// Returns s unchanged if no fences are present.
func stripFences(s string) string {
	if idx := strings.Index(s, "```"); idx == -1 {
		return s
	}
	s = s[strings.Index(s, "```"):]
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	if end := strings.LastIndex(s, "```"); end != -1 {
		s = s[:end]
	}
	return strings.TrimSpace(s)
}

// parseStoryJSON unmarshals a raw LLM response into a GeneratedStory,
// defensively stripping markdown fences first.
func parseStoryJSON(raw string) (*GeneratedStory, error) {
	raw = stripFences(strings.TrimSpace(raw))

	var story GeneratedStory
	if err := json.Unmarshal([]byte(raw), &story); err != nil {
		return nil, fmt.Errorf("parsing story JSON: %w\nraw response: %.500s", err, raw)
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
	if len(s.Story.Title) > 120 {
		return fmt.Errorf("story title too long (%d chars, max 120)", len(s.Story.Title))
	}
	if len(s.Tasks) == 0 {
		return fmt.Errorf("LLM returned no tasks")
	}
	if len(s.Tasks) > 12 {
		return fmt.Errorf("too many tasks (%d, max 12)", len(s.Tasks))
	}
	for i, t := range s.Tasks {
		if t.Title == "" {
			return fmt.Errorf("task %d has empty title", i+1)
		}
		if len(t.Subtasks) > 8 {
			return fmt.Errorf("task %d has too many subtasks (%d, max 8)", i+1, len(t.Subtasks))
		}
	}
	return nil
}
