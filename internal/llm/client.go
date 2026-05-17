package llm

import "github.com/joelkram/task-cli/internal/config"

// Note: provider name constants live in the config package to avoid a circular
// import (llm already imports config).

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
	GenerateStory(req StoryRequest) (*GeneratedStory, error)
}

// New returns the right Client based on the provider in cfg.
func New(cfg *config.Config) Client {
	switch cfg.LLM.Provider {
	case config.ProviderPi:
		return PiClient(cfg)
	case config.ProviderOpencode:
		return OpencodeClient(cfg)
	default:
		return newOpenAIClient(cfg)
	}
}
