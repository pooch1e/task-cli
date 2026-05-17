# Task Tracker — Backlog

This file tracks the development of `task-cli` itself, using the tool's own conventions.

---

## S-1 · Phase 2: Polish + Security

**As a developer, I want the tool to be secure and resilient so that I can trust it in daily use.**

### Acceptance Criteria
- Given a malformed LLM response, when parsing fails, then the tool retries once with a stricter prompt before returning an error
- Given a user runs `task config init` with no existing config, then they are guided through an interactive setup
- Given a user's feature input contains `<` or `>`, when the prompt is built, then those characters are sanitised before interpolation
- Given the tool writes config or the database, then both files have mode `0600`

### Tasks

#### T-1 · LLM retry on parse failure
- [ ] T-1.1 · Catch `parseStoryJSON` error in `openai.go` and `agents.go`
- [ ] T-1.2 · Re-prompt with `"You MUST respond with only a JSON object. No markdown."` prefix
- [ ] T-1.3 · Surface original error if second attempt also fails

#### T-2 · Context-aware HTTP timeout
- [ ] T-2.1 · Replace `http.Client{Timeout}` with `context.WithTimeout` passed into request
- [ ] T-2.2 · Add unit test asserting timeout is respected

#### T-3 · `task export`
- [ ] T-3.1 · `task export --format markdown` — one `.md` file per story to `./task-export/`
- [ ] T-3.2 · `task export --format json` — single `task-export.json` dump
- [ ] T-3.3 · Include story, tasks, subtasks, statuses, and acceptance criteria in output

#### T-4 · Interactive `task config init`
- [ ] T-4.1 · Prompt: select provider (deepseek / openai / pi / opencode)
- [ ] T-4.2 · If direct provider, prompt for API key — write to config, never echo to terminal
- [ ] T-4.3 · Print summary and instruct user to verify with `task config test`

#### T-5 · First-run onboarding
- [ ] T-5.1 · Detect missing config AND missing `TASK_API_KEY` on any command
- [ ] T-5.2 · Print friendly setup guide instead of raw error
- [ ] T-5.3 · Suggest `--agent pi` as zero-config option if `pi` is in PATH

#### T-6 · Security hardening
- [ ] T-6.1 · Sanitise `<` `>` from user input in `buildPrompt`
- [ ] T-6.2 · Validate LLM response: title ≤ 80 chars, 1–8 tasks, ≤ 6 subtasks each
- [ ] T-6.3 · Use `O_NOFOLLOW` when opening config for write
- [ ] T-6.4 · Add `~/.task/` to docs — note: never commit `config.toml`

#### T-7 · `task rm`
- [ ] T-7.1 · `task rm S-N` with `--confirm` flag (or interactive y/N prompt)
- [ ] T-7.2 · `task rm T-N` — cascades to subtasks via SQLite `ON DELETE CASCADE`
- [ ] T-7.3 · Print deleted item summary

#### T-8 · Shell completions
- [ ] T-8.1 · `task completion zsh` — cobra built-in, document in README
- [ ] T-8.2 · `task completion bash` — cobra built-in

---

## S-2 · Phase 3: Multi-Provider

**As a developer, I want to choose my LLM provider so that I can balance cost and quality.**

### Tasks

#### T-9 · DeepSeek live validation
- [ ] T-9.1 · Live test with `TASK_API_KEY` set
- [ ] T-9.2 · Confirm `response_format: json_object` works on DeepSeek endpoint

#### T-10 · Gemini Flash provider
- [ ] T-10.1 · Implement `internal/llm/gemini.go` using `generativelanguage.googleapis.com`
- [ ] T-10.2 · Map Gemini response format to `GeneratedStory`
- [ ] T-10.3 · Add `provider = "gemini"` to config docs

#### T-11 · Provider listing
- [ ] T-11.1 · `task providers` — list all available providers with cost estimate per call
- [ ] T-11.2 · Indicate which providers require an API key vs use existing tools

#### T-12 · Rate-limit guard
- [ ] T-12.1 · Track last-called timestamp in a `.task/.ratelimit` file
- [ ] T-12.2 · Warn if same command called > 5 times in 60 seconds

---

## S-3 · Phase 4: Distribution

**As a developer, I want to install `task` without cloning the repo so that I can use it anywhere.**

### Tasks

#### T-13 · GitHub Actions release pipeline
- [ ] T-13.1 · `.github/workflows/release.yml` — trigger on `git tag v*`
- [ ] T-13.2 · Build matrix: `darwin/arm64`, `darwin/amd64`, `linux/amd64`
- [ ] T-13.3 · Attach binaries to GitHub Release

#### T-14 · Install script
- [ ] T-14.1 · `install.sh` — detect OS/arch, download correct binary, install to `~/.local/bin`
- [ ] T-14.2 · Verify checksum after download

#### T-15 · Version embedding
- [ ] T-15.1 · `-ldflags "-X main.version=$(git describe --tags)"` in Makefile
- [ ] T-15.2 · `task version` command prints version + build info

#### T-16 · Homebrew tap (optional)
- [ ] T-16.1 · Create `homebrew-task-cli` repo
- [ ] T-16.2 · Write formula referencing GitHub Release binary
- [ ] T-16.3 · `brew install pooch1e/tap/task`
