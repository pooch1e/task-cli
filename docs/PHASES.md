# Implementation Phases

## Phase 1 — Foundation ✅ Complete

**Goal:** Working binary that manages tasks manually with SQLite storage and LLM story generation via `pi` or `opencode`.

### Deliverables
- [x] `go.mod` — module `github.com/joelkram/task-cli`, zero runtime deps
- [x] SQLite schema — `projects → stories → tasks → subtasks`, auto-migrated on open
- [x] `internal/config` — TOML config at `~/.task/config.toml`, `chmod 600`, `TASK_API_KEY` env override
- [x] `internal/db` — parameterised queries, foreign keys enforced, WAL mode
- [x] `internal/project` — git root auto-detection via `git rev-parse` with manual walk fallback
- [x] `internal/ui` — ANSI tree output and progress bars, zero external deps
- [x] `internal/llm` — `Client` interface with three providers: OpenAI-compat, pi subprocess, opencode subprocess
- [x] `internal/llm/prompt` — injection-safe prompt using `<feature>` delimiters, `response_format: json_object`
- [x] `cmd/task/main.go` — full cobra command tree
- [x] `Makefile` — `build`, `install`, `release`, `test`, `lint`
- [x] Binary installed globally at `~/.local/bin/task`

### Commands shipped
| Command | Description |
|---|---|
| `task init` | Initialise project in current repo |
| `task story "<desc>"` | Generate story + tasks via LLM |
| `task list` | Tree view of all stories and tasks |
| `task show S-N` | Story detail with acceptance criteria |
| `task status` | Progress bars per story/task/subtask |
| `task start T-N` | Mark task in-progress |
| `task done T-N\|S-N` | Mark task or story done |
| `task add task "<title>" --story S-N` | Add a task manually |
| `task add subtask "<title>" --task T-N` | Add a subtask manually |
| `task config init` | Create default config |
| `task config show` | Show config (API key redacted) |

### Verified
```bash
task story "add OAuth login" --agent pi --model github-copilot/claude-haiku-4.5
task start T-1 && task done T-1 && task done T-2
task list && task show S-1 && task status
```

---

## Phase 2 — Polish + Security Hardening

**Goal:** Production-ready security posture, retry logic, and export.

### Tasks
- [ ] Retry-once on JSON parse failure (re-prompt with stricter instruction)
- [ ] HTTP timeout enforced via `context.WithTimeout` (not just client timeout)
- [ ] Max response size cap in openai.go (already 64KB — add unit test)
- [ ] `task export --format markdown` — GitHub-style issue `.md` per story
- [ ] `task export --format json` — machine-readable full dump
- [ ] `task config init` interactive wizard (prompt for provider, model, API key)
- [ ] First-run detection: if no config and no `TASK_API_KEY`, print onboarding instead of error
- [ ] `task rm <S-N|T-N>` — remove story or task (with confirmation prompt)
- [ ] Shell completions via `task completion zsh >> ~/.zshrc`

### Security
- [ ] Prompt injection sanitisation — strip/escape `<` `>` from user feature input
- [ ] Validate LLM response schema strictly (title len, task count bounds)
- [ ] `O_NOFOLLOW` flag when writing config (prevent symlink attack)
- [ ] Document: never commit `~/.task/config.toml` — add to global `.gitignore`

---

## Phase 3 — DeepSeek Direct + Multi-Provider

**Goal:** Full direct API path working alongside pi/opencode backends.

### Tasks
- [ ] Test DeepSeek direct with live `TASK_API_KEY`
- [ ] Add Gemini Flash provider (`generativelanguage.googleapis.com`) — cheapest paid option
- [ ] Add Anthropic provider (non-OpenAI-compat format)
- [ ] `--provider` flag lists available providers + their cost estimate
- [ ] Rate-limit guard: warn if same command called >5 times in 60 seconds
- [ ] `task config test` — fire a minimal prompt to verify config is working

---

## Phase 4 — Distribution

**Goal:** Installable without cloning the repo.

### Tasks
- [ ] GitHub Actions workflow — build matrix: `darwin/arm64`, `darwin/amd64`, `linux/amd64`
- [ ] Attach binaries to GitHub Release on tag push
- [ ] Install script: `curl -fsSL https://raw.githubusercontent.com/pooch1e/task-cli/main/install.sh | bash`
- [ ] Homebrew tap: `homebrew-task-cli` repo with formula
- [ ] `brew install pooch1e/tap/task`
- [ ] Version flag: embed git tag via `-ldflags "-X main.version=$(git describe --tags)"`
- [ ] `task version` command

---

## Cost Reference

| Provider | Model | Cost per `task story` call | Monthly (30 stories) |
|---|---|---|---|
| DeepSeek (direct) | deepseek-chat | ~$0.00011 | ~$0.003 |
| Google (direct) | gemini-flash | ~$0.00006 | ~$0.002 |
| pi subprocess | github-copilot/claude-haiku-4.5 | **$0** (Copilot) | **$0** |
| opencode subprocess | github-copilot/gpt-5-mini | **$0** (Copilot) | **$0** |
| opencode subprocess | deepseek-v4-flash-free | **$0** (free tier) | **$0** |
