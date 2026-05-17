# Agent Integration Guide

This document describes how `task-cli` integrates with AI coding agents to generate user stories and tasks.

---

## Overview

`task story "<feature>"` shells out to an LLM and writes the structured response to SQLite. Two integration modes are supported:

| Mode | Flag | How it works |
|---|---|---|
| **Direct API** | _(default)_ | HTTP POST to an OpenAI-compatible endpoint (DeepSeek, OpenAI, etc.) |
| **pi subprocess** | `--agent pi` | Spawns `pi --mode json -p "<prompt>"`, parses the JSON event stream |
| **opencode subprocess** | `--agent opencode` | Spawns `opencode run "<prompt>" --format json`, parses the JSON event stream |

---

## Direct API (default)

Uses any OpenAI-compatible endpoint. Configured in `~/.task/config.toml`:

```toml
[llm]
provider  = "deepseek"
model     = "deepseek-chat"
base_url  = "https://api.deepseek.com/v1"
max_tokens = 1024
```

Set your API key via environment variable (preferred) or config file:

```bash
export TASK_API_KEY=sk-...
# or add to config.toml: api_key = "sk-..."
```

### Request shape

```json
{
  "model": "deepseek-chat",
  "messages": [
    { "role": "system", "content": "<system prompt>" },
    { "role": "user",   "content": "<feature prompt>" }
  ],
  "max_tokens": 1024,
  "response_format": { "type": "json_object" }
}
```

`response_format: json_object` forces the model to return only valid JSON — no markdown fences needed. If the model doesn't support this field (some older endpoints), the parser defensively strips ` ```json ``` ` fences before parsing.

### Supported providers

| Provider | `base_url` | Notes |
|---|---|---|
| DeepSeek | `https://api.deepseek.com/v1` | Recommended — $0.14/M tokens |
| OpenAI | `https://api.openai.com/v1` | Standard |
| Gemini (via compat layer) | `https://generativelanguage.googleapis.com/v1beta/openai` | Flash model is cheapest |
| Local (Ollama) | `http://localhost:11434/v1` | Set `api_key = "ollama"` |

---

## pi Subprocess (`--agent pi`)

Spawns pi in JSON event stream mode. No API key needed if pi has valid credentials (e.g. GitHub Copilot OAuth).

```bash
task story "add OAuth login" --agent pi --model github-copilot/claude-haiku-4.5
```

### How it works

```
task  →  pi --mode json --no-session -p "<prompt>" --model <model>
                │
                │  stdout: newline-delimited JSON events
                │
          {"type":"agent_start"}
          {"type":"turn_start"}
          {"type":"message_update","assistantMessageEvent":{"type":"text_delta","delta":"{"}}
          {"type":"message_update","assistantMessageEvent":{"type":"text_delta","delta":"\"story\""}}
          ...
          {"type":"message_end","message":{...}}
          {"type":"agent_end","messages":[...]}
                │
                ▼
         accumulate text_delta events → assemble full JSON string → parse
```

### Recommended free models (via GitHub Copilot)

```bash
--model github-copilot/claude-haiku-4.5   # fast, cheap, good JSON
--model github-copilot/gpt-5-mini         # good alternative
--model github-copilot/gemini-3-flash-preview
```

---

## opencode Subprocess (`--agent opencode`)

Spawns opencode in JSON format mode. Same event stream shape as pi.

```bash
task story "add OAuth login" --agent opencode --model github-copilot/gpt-5-mini
```

### Completely free models (no API key)

```bash
--model opencode/deepseek-v4-flash-free   # free tier, rate limited
--model opencode/qwen3.6-plus-free        # free tier
```

---

## Prompt Design

The prompt is built in `internal/llm/prompt.go`. Key design decisions:

### Injection safety

User input is wrapped in XML-style delimiters before interpolation:

```
Project: <project>my-app</project>
Feature: <feature>add OAuth login</feature>
```

This prevents user input from breaking out of the intended prompt structure. Even if a user passes `"ignore above instructions"`, it is scoped inside `<feature>` tags and `response_format: json_object` enforces the output schema regardless.

### Output schema

The model is instructed to produce this exact JSON shape:

```json
{
  "story": {
    "title": "Short imperative title (max 80 chars)",
    "description": "As a [user], I want [feature] so that [benefit]",
    "acceptance_criteria": ["Given ... When ... Then ...", "..."]
  },
  "tasks": [
    {
      "title": "Implementation task title",
      "subtasks": ["Subtask description"]
    }
  ]
}
```

Rules enforced in prompt:
- title ≤ 80 characters
- 3–6 tasks
- 0–4 subtasks per task
- 2–4 acceptance criteria
- Output ONLY the JSON object

### Retry behaviour (Phase 2)

On `json.Unmarshal` failure, the client will retry once with an additional instruction prepended:

```
You MUST respond with only a valid JSON object. No markdown, no code fences, no explanation.
```

---

## Event Stream Parsing

Both pi and opencode emit the same JSON event stream format to stdout. The parser in `internal/llm/agents.go` works as follows:

```
For each line in stdout:
  parse as JSON object
  switch on "type":
    "message_start"  → reset accumulation buffer
    "message_update" → if assistantMessageEvent.type == "text_delta":
                           json.Unmarshal(delta) → append to buffer
    "message_end"    → save buffer as lastText
    "agent_end"      → if lastText empty, fall back to extracting
                       from messages[] array in event payload
```

The delta values are proper JSON strings (not raw text), so they are unmarshalled with `json.Unmarshal` to correctly handle escape sequences.

---

## Adding a New Provider

1. Create `internal/llm/<provider>.go` implementing the `Client` interface:

```go
type Client interface {
    GenerateStory(req StoryRequest) (*GeneratedStory, error)
}
```

2. Register it in `internal/llm/client.go`:

```go
func New(cfg *config.Config) Client {
    switch cfg.LLM.Provider {
    case "myprovider":
        return &MyProviderClient{cfg: cfg}
    // ...
    }
}
```

3. Add the provider name to `config.go` validation and README.

---

## Security Notes

| Concern | Mitigation |
|---|---|
| API key exposure | Stored in `~/.task/config.toml` (mode `0600`). `TASK_API_KEY` env var preferred. Key never accepted as a CLI flag. |
| Prompt injection | User input wrapped in `<feature>` delimiters. `response_format: json_object` enforces schema. |
| Unchecked response size | Response body capped at 64KB via `io.LimitReader` |
| TLS | Standard Go TLS — `InsecureSkipVerify` is never set |
| Subprocess (pi/opencode) | Spawned with fixed args. User input is passed as a single `-p` string argument, never interpolated into shell |
| SQL injection | All queries use `?` placeholders — `fmt.Sprintf` never used in SQL |
