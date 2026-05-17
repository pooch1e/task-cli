package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	"github.com/joelkram/task-cli/internal/config"
)

// subprocessClient runs an external agent binary (pi or opencode) and parses
// its JSON event stream output. Both tools emit the same event format.
type subprocessClient struct {
	cfg        *config.Config
	binary     string
	buildArgs  func(prompt, model string) []string
}

func (c *subprocessClient) GenerateStory(req StoryRequest) (*GeneratedStory, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(c.cfg.LLM.TimeoutSecs)*time.Second)
	defer cancel()
	return generateWithRetry(ctx, c, req)
}

// Ping checks that the agent binary exists in PATH.
func (c *subprocessClient) Ping() error {
	_, err := exec.LookPath(c.binary)
	if err != nil {
		return fmt.Errorf("%s not found in PATH: %w", c.binary, err)
	}
	return nil
}

// call implements the caller interface: spawns the binary with a combined
// system+user prompt and extracts the last assistant text from the event stream.
func (c *subprocessClient) call(ctx context.Context, parts PromptParts) (string, error) {
	// Subprocess agents receive a single string prompt — concatenate the parts.
	combined := parts.System + "\n\n" + parts.User
	args := c.buildArgs(combined, c.cfg.LLM.Model)

	out, err := exec.CommandContext(ctx, c.binary, args...).Output()
	if err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("%s subprocess timed out after %ds", c.binary, c.cfg.LLM.TimeoutSecs)
		}
		return "", fmt.Errorf("%s subprocess failed: %w", c.binary, err)
	}
	return extractLastAssistantText(out)
}

// ── factory constructors ──────────────────────────────────────────────────────

// PiClient returns a subprocessClient configured for pi's JSON event stream.
func PiClient(cfg *config.Config) Client {
	return &subprocessClient{
		cfg:    cfg,
		binary: "pi",
		buildArgs: func(prompt, model string) []string {
			args := []string{"--mode", "json", "--no-session", "-p", prompt}
			if model != "" {
				args = append(args, "--model", model)
			}
			return args
		},
	}
}

// OpencodeClient returns a subprocessClient configured for opencode's JSON output.
func OpencodeClient(cfg *config.Config) Client {
	return &subprocessClient{
		cfg:    cfg,
		binary: "opencode",
		buildArgs: func(prompt, model string) []string {
			args := []string{"run", "--format", "json"}
			if model != "" {
				args = append(args, "--model", model)
			}
			return append(args, prompt)
		},
	}
}

// ── event stream parsing ──────────────────────────────────────────────────────

// extractLastAssistantText parses a newline-delimited JSON event stream and
// returns the last fully-assembled assistant message text.
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
			continue // skip non-JSON lines (e.g. spinner/progress output)
		}

		switch jsonString(event["type"]) {
		case "message_start":
			buf.Reset()

		case "message_update":
			ameRaw, ok := event["assistantMessageEvent"]
			if !ok {
				break
			}
			var ame map[string]json.RawMessage
			if err := json.Unmarshal(ameRaw, &ame); err != nil {
				break
			}
			if jsonString(ame["type"]) == "text_delta" {
				var delta string
				if err := json.Unmarshal(ame["delta"], &delta); err == nil {
					buf.WriteString(delta)
				}
			}

		case "message_end":
			if buf.Len() > 0 {
				lastText = buf.String()
				buf.Reset()
			}

		case "agent_end":
			if lastText == "" {
				lastText = lastAssistantTextFromMessages(event["messages"])
			}
		}
	}

	if lastText == "" {
		return "", fmt.Errorf("no assistant text found in agent output")
	}
	return lastText, nil
}

// lastAssistantTextFromMessages walks a messages JSON array in reverse and
// returns the text content of the last assistant message.
func lastAssistantTextFromMessages(raw json.RawMessage) string {
	var messages []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &messages); err != nil {
		return ""
	}
	for i := len(messages) - 1; i >= 0; i-- {
		if jsonString(messages[i]["role"]) != "assistant" {
			continue
		}
		var blocks []map[string]json.RawMessage
		if err := json.Unmarshal(messages[i]["content"], &blocks); err != nil {
			continue
		}
		for _, b := range blocks {
			if jsonString(b["type"]) == "text" {
				var text string
				if err := json.Unmarshal(b["text"], &text); err == nil && text != "" {
					return text
				}
			}
		}
	}
	return ""
}

// jsonString safely decodes a json.RawMessage that is expected to be a JSON
// string, returning an empty string on any error.
func jsonString(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	return s
}
