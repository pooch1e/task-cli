package llm

import (
	"fmt"

	"github.com/joelkram/task-cli/internal/config"
)

// StoryRequest is what the user provides to kick off generation.
type StoryRequest struct {
	Feature     string
	ProjectName string
}

// GeneratedStory is the structured output from the LLM.
type GeneratedStory struct {
	Story struct {
		Title              string   `json:"title"`
		Description        string   `json:"description"`
		AcceptanceCriteria []string `json:"acceptance_criteria"`
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
	// Ping performs a lightweight health check without generating a story.
	Ping() error
}

// New returns the Client for the provider named in cfg.
// Returns an error for unknown provider names so typos surface immediately
// rather than silently falling through to a wrong provider.
func New(cfg *config.Config) (Client, error) {
	switch cfg.LLM.Provider {
	case config.ProviderPi:
		return PiClient(cfg), nil
	case config.ProviderOpencode:
		return OpencodeClient(cfg), nil
	case config.ProviderGemini:
		return newGeminiClient(cfg), nil
	case "deepseek", "openai", "":
		// All OpenAI-compatible endpoints share one client.
		return newOpenAIClient(cfg), nil
	default:
		return nil, fmt.Errorf("unknown LLM provider %q — run: task providers", cfg.LLM.Provider)
	}
}
