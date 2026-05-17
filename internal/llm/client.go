package llm

import "github.com/joelkram/task-cli/internal/config"

// StoryRequest is what the user provides to kick off generation.
type StoryRequest struct {
	Feature     string
	ProjectName string
}

// GeneratedStory is the structured output from the LLM.
type GeneratedStory struct {
	Story struct {
		Title               string   `json:"title"`
		Description         string   `json:"description"`
		AcceptanceCriteria  []string `json:"acceptance_criteria"`
	} `json:"story"`
	Tasks []struct {
		Title    string   `json:"title"`
		Subtasks []string `json:"subtasks"`
	} `json:"tasks"`
}

// Client is the interface every provider must implement.
type Client interface {
	// GenerateStory calls the LLM and returns a parsed story with tasks.
	GenerateStory(req StoryRequest) (*GeneratedStory, error)
}

// New returns the right Client based on config.
func New(cfg *config.Config) Client {
	switch cfg.LLM.Provider {
	case "pi":
		return &PiClient{cfg: cfg}
	case "opencode":
		return &OpencodeClient{cfg: cfg}
	default:
		// deepseek, openai, or any OpenAI-compatible endpoint
		return &OpenAIClient{cfg: cfg}
	}
}
