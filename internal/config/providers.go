package config

// ProviderMeta describes a known LLM provider's defaults and cost profile.
// Used by both applyDefaults (to fill zero-value config fields) and by
// the `task providers` command (for display).
type ProviderMeta struct {
	Name           string
	Description    string
	DefaultModel   string
	DefaultBaseURL string
	KeyRequired    bool    // false for subprocess providers that carry own auth
	KeyEnvVar      string  // primary env var; TASK_API_KEY is always accepted
	CostPer1KInput float64 // USD per 1K input tokens; 0 means free
	Notes          string
}

// KnownProviders is the single source of truth for all supported providers.
// Order determines display order in `task providers`.
var KnownProviders = []ProviderMeta{
	{
		Name:           ProviderPi,
		Description:    "pi agent subprocess",
		DefaultModel:   "github-copilot/claude-haiku-4.5",
		DefaultBaseURL: "",
		KeyRequired:    false,
		CostPer1KInput: 0,
		Notes:          "Free — uses pi's own credentials (GitHub Copilot etc.)",
	},
	{
		Name:           ProviderOpencode,
		Description:    "opencode agent subprocess",
		DefaultModel:   "github-copilot/gpt-5-mini",
		DefaultBaseURL: "",
		KeyRequired:    false,
		CostPer1KInput: 0,
		Notes:          "Free — uses opencode credentials; free-tier models available",
	},
	{
		Name:           "gemini",
		Description:    "Google Gemini (native API)",
		DefaultModel:   "gemini-2.0-flash",
		DefaultBaseURL: "https://generativelanguage.googleapis.com/v1beta",
		KeyRequired:    true,
		KeyEnvVar:      "GEMINI_API_KEY",
		CostPer1KInput: 0.000075,
		Notes:          "Cheapest paid option — get key at aistudio.google.com",
	},
	{
		Name:           "deepseek",
		Description:    "DeepSeek (OpenAI-compatible)",
		DefaultModel:   "deepseek-chat",
		DefaultBaseURL: "https://api.deepseek.com/v1",
		KeyRequired:    true,
		KeyEnvVar:      "TASK_API_KEY",
		CostPer1KInput: 0.00014,
		Notes:          "Very cheap — get key at platform.deepseek.com",
	},
	{
		Name:           "openai",
		Description:    "OpenAI",
		DefaultModel:   "gpt-4o-mini",
		DefaultBaseURL: "https://api.openai.com/v1",
		KeyRequired:    true,
		KeyEnvVar:      "TASK_API_KEY",
		CostPer1KInput: 0.00015,
		Notes:          "",
	},
}

// LookupProvider returns the ProviderMeta for name, or nil if unknown.
func LookupProvider(name string) *ProviderMeta {
	for i := range KnownProviders {
		if KnownProviders[i].Name == name {
			return &KnownProviders[i]
		}
	}
	return nil
}
