package llm

import "fmt"

// systemPrompt is the fixed system message sent with every story request.
const systemPrompt = `You are a software project planning assistant.
When given a feature description, produce a user story and a list of concrete implementation tasks.
Respond ONLY with valid JSON. No markdown, no explanation, no code fences.`

// BuildPrompt is exported so commands can print it for --dry-run.
func BuildPrompt(req StoryRequest) string { return buildPrompt(req) }

// buildPrompt builds the user message for story generation.
func buildPrompt(req StoryRequest) string {
	return fmt.Sprintf(`Generate a user story and implementation tasks for the following feature.

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
- Output ONLY the JSON object, nothing else`, req.ProjectName, req.Feature)
}
