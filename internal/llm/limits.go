package llm

// maxResponseBytes caps the body read for all LLM providers.
// Defined here (not per-provider file) so it's a single edit if changed.
const maxResponseBytes = 64 * 1024
