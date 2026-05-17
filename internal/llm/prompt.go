package llm

import (
	"fmt"
	"html"
	"strings"
)

// PromptParts holds the system and user sections of an LLM prompt separately,
// so providers that support distinct roles (OpenAI) can pass them correctly,
// while subprocess providers concatenate them.
type PromptParts struct {
	System string
	User   string
}

// systemPrompt is the fixed system message for every story request.
const systemPrompt = `You are a software project planning assistant.
When given a feature description, produce a user story and a list of concrete implementation tasks.
Respond ONLY with valid JSON. No markdown, no explanation, no code fences.`

// BuildPrompt is exported so commands can display it via --dry-run.
func BuildPrompt(req StoryRequest) string {
	return buildPromptParts(req).User
}

// buildPromptParts returns the separated system + user prompt for a story request.
func buildPromptParts(req StoryRequest) PromptParts {
	user := fmt.Sprintf(`Generate a user story and implementation tasks for the following feature.

Project: <project>%s</project>
Feature: <feature>%s</feature>

Respond with JSON in this exact schema:
{
  "story": {
    "title": "Short imperative title (max 80 chars)",
    "description": "As a [user], I want [feature] so that [benefit]",
    "acceptance_criteria": ["Given ... When ... Then ...", "..."]
  },
  "tasks": [
    {
      "title": "Implementation task title",
      "subtasks": ["Subtask description", "..."]
    }
  ]
}

Rules:
- title must be under 80 characters
- Include 3-6 tasks
- Each task may have 0-4 subtasks
- acceptance_criteria should have 2-4 items
- Output ONLY the JSON object, nothing else`,
		sanitiseInput(req.ProjectName),
		sanitiseInput(req.Feature),
	)
	return PromptParts{System: systemPrompt, User: user}
}

// sanitiseInput applies HTML escaping to prevent XML-delimiter injection and
// trims surrounding whitespace.
func sanitiseInput(s string) string {
	return strings.TrimSpace(html.EscapeString(s))
}
