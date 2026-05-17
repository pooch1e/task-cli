package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/joelkram/task-cli/internal/config"
)

// PiClient spawns `pi --mode json -p "<prompt>"` and parses the last assistant text.
type PiClient struct {
	cfg *config.Config
}

func (c *PiClient) GenerateStory(req StoryRequest) (*GeneratedStory, error) {
	prompt := systemPrompt + "\n\n" + buildPrompt(req)

	args := []string{"--mode", "json", "--no-session", "-p", prompt}
	if c.cfg.LLM.Model != "" {
		args = append(args, "--model", c.cfg.LLM.Model)
	}

	out, err := exec.Command("pi", args...).Output()
	if err != nil {
		return nil, fmt.Errorf("pi subprocess failed: %w", err)
	}

	text, err := extractLastAssistantText(out)
	if err != nil {
		return nil, err
	}

	return parseStoryJSON(text)
}

// OpencodeClient spawns `opencode run "<prompt>" --format json` and parses output.
type OpencodeClient struct {
	cfg *config.Config
}

func (c *OpencodeClient) GenerateStory(req StoryRequest) (*GeneratedStory, error) {
	prompt := systemPrompt + "\n\n" + buildPrompt(req)

	args := []string{"run", "--format", "json"}
	if c.cfg.LLM.Model != "" {
		args = append(args, "--model", c.cfg.LLM.Model)
	}
	args = append(args, prompt)

	out, err := exec.Command("opencode", args...).Output()
	if err != nil {
		return nil, fmt.Errorf("opencode subprocess failed: %w", err)
	}

	text, err := extractLastAssistantText(out)
	if err != nil {
		return nil, err
	}

	return parseStoryJSON(text)
}

// extractLastAssistantText parses a JSON event stream (one object per line)
// and returns the final assembled assistant text.
func extractLastAssistantText(output []byte) (string, error) {
	var lastText string
	var buf bytes.Buffer

	for _, line := range bytes.Split(output, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}

		var event map[string]json.RawMessage
		if err := json.Unmarshal(line, &event); err != nil {
			continue // skip non-JSON lines (e.g. spinner output)
		}

		eventType := strings.Trim(string(event["type"]), `"`)

		switch eventType {
		case "message_start":
			buf.Reset()

		case "message_update":
			// Extract delta text from assistantMessageEvent
			if raw, ok := event["assistantMessageEvent"]; ok {
				var ame map[string]json.RawMessage
				if err := json.Unmarshal(raw, &ame); err == nil {
					if t := strings.Trim(string(ame["type"]), `"`); t == "text_delta" {
						// ame["delta"] is a JSON string — unmarshal it directly
						var delta string
						if err := json.Unmarshal(ame["delta"], &delta); err == nil {
							buf.WriteString(delta)
						}
					}
				}
			}

		case "message_end":
			if buf.Len() > 0 {
				lastText = buf.String()
				buf.Reset()
			}

		case "agent_end":
			// Also try to extract from messages array if incremental deltas were empty
			if lastText == "" {
				if raw, ok := event["messages"]; ok {
					lastText = extractFromMessages(raw)
				}
			}
		}
	}

	if lastText == "" {
		return "", fmt.Errorf("no assistant text found in agent output")
	}
	return lastText, nil
}

func extractFromMessages(raw json.RawMessage) string {
	var messages []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &messages); err != nil {
		return ""
	}
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		role := strings.Trim(string(msg["role"]), `"`)
		if role != "assistant" {
			continue
		}
		var content []map[string]json.RawMessage
		if err := json.Unmarshal(msg["content"], &content); err != nil {
			continue
		}
		for _, block := range content {
			if strings.Trim(string(block["type"]), `"`) == "text" {
				var text string
				if err := json.Unmarshal(block["text"], &text); err == nil && text != "" {
					return text
				}
			}
		}
	}
	return ""
}
