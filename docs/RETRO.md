# task-cli — Project Retrospective

_Date: 2026-05-17_  
_Repo: https://github.com/pooch1e/task-cli_  
_Latest release: [v0.1.0](https://github.com/pooch1e/task-cli/releases/tag/v0.1.0)_

---

## What We Built

A personal CLI tool for tracking user stories, tasks, and subtasks across projects — with LLM-generated story/task creation via DeepSeek, Gemini, pi, or opencode.

```
task story "add OAuth login" --agent pi --model github-copilot/claude-haiku-4.5
task list
task start T-3
task done T-3
task status
task export --format markdown
```

**Binary:** 10MB, zero runtime dependencies, SQLite storage at `~/.task/tasks.db`.

---

## Phase Delivery

| Phase | Goal | Outcome |
|---|---|---|
| **1** | Foundation — CLI, SQLite, project detection | ✅ Full cobra command tree, schema, db layer, ANSI output |
| **2** | LLM integration — retry, export, security hardening | ✅ OpenAI-compat + pi/opencode subprocess providers, `task export`, `task rm` |
| **3** | Multi-provider — Gemini, providers listing, rate limit | ✅ Native Gemini API, `task providers`, `task config test`, rate-limit guard |
| **4** | Distribution — versioning, CI/CD, install script | ✅ GitHub Actions release pipeline, `v0.1.0` live on GitHub Releases |

---

## What Went Well

### Architecture decisions that held up
- **`PromptParts{System, User}`** — separating system and user prompt parts worked cleanly across all three provider types (OpenAI messages, Gemini `systemInstruction`, subprocess concat). Adding a new provider never required touching this.
- **`generateWithRetry` + `caller` interface** — retry logic is in one place. Each provider implements a single `call(ctx, parts)` method. Zero duplication.
- **`db.PersistStory` transaction** — writing story+tasks+subtasks atomically meant zero orphaned records throughout all testing. The single transaction pattern held up under failure injection too.
- **`KnownProviders` in `config/providers.go`** — pulling provider metadata (default model, base URL, key env var, cost) into a single data structure meant `applyDefaults`, `task providers`, and env-var resolution all stayed DRY as providers were added.
- **`StoryView` / `LoadProjectView`** — moving the subtask-loading loop into the DB layer eliminated the repeated `for task in tasks { ListSubtasksForTask }` pattern that had appeared in three separate commands.

### Sub-agent reviews caught real bugs
The two Haiku sub-agent code reviews surfaced **73 issues across all phases** that would have stayed in production:
- A stats query Cartesian product bug (`SUM` vs `COUNT DISTINCT CASE WHEN`) that caused panics in `task status`
- `*sql.DB` embedding exposing 30+ raw methods, bypassing all custom error handling
- Three independent implementations of the same slug-generation SQL (`COUNT` → `MAX`)
- API key potentially leaking in Gemini error strings
- `fmt.Scanln` hanging forever in non-TTY environments
- JSON parsing in `install.sh` using `grep | sed` vulnerable to malformed API responses

---

## What Was Harder Than Expected

### LLM event stream parsing
The pi JSON mode delta stream double-encodes escape sequences. `strings.Trim(string(raw), `"`)` then re-wrapping in quotes broke on any LLM response containing JSON (which is exactly what we were requesting). The fix — `json.Unmarshal(ame["delta"], &delta)` directly on the raw message — was simple once diagnosed, but debugging the malformed JSON coming back took iteration.

### `*sql.DB` embed removal
Changing `DB struct { *sql.DB }` to `DB struct { conn *sql.DB }` required updating every `db.Query`, `db.Exec`, `db.QueryRow` call to `db.conn.Query` etc., and adding explicit `Begin()` and `Close()` method forwarders. It was the right call — callers can no longer accidentally bypass the wrappers — but it was a broader change than anticipated.

### Import cycle: `config ↔ llm`
Adding `ProviderPi`/`ProviderOpencode` constants to `llm/client.go` and then importing them from `config/config.go` (for `Validate()`) created a cycle immediately. Resolution: move constants to `config` (the leaf), have `llm` reference `config.ProviderPi`. This is the correct direction but not obvious at first.

### Makefile `--dirty` flag
Using `git describe --tags --always --dirty` made checksums non-deterministic — identical source at different times produces different version strings and therefore different binaries. The sub-agent review caught this. The fix (remove `--dirty`) is one character but the implication (reproducible builds) is significant for a distribution tool.

---

## What We'd Do Differently

### Start with the DB interface boundary
`DB` embedding `*sql.DB` was convenient early on but created a leaky abstraction that the review correctly flagged. Starting with `DB struct { conn *sql.DB }` from day one would have avoided the broad refactor in the Phase 3 review round.

### Slug generation design upfront
The slug generation function was written three times before converging on `nextSlug(execer, prefix, table, col, projectID)`. The `execer` interface abstracting `*sql.DB` and `*sql.Tx` is the right design but wasn't obvious until the transaction requirement forced it. A schema design session before coding would have landed here earlier.

### Provider abstraction earlier
The `OpenAIClient`, `PiClient`, `OpencodeClient` started as three independent structs before the `caller` interface unified them. The `subprocessClient` consolidation (eliminating 40+ lines of duplication) should have been the starting shape. Designing the interface before the implementation is the lesson.

### Version variable placement
`var version = "dev"` belongs in its own file (`version.go`) alongside build-info variables like `commit` and `buildDate`, not mixed into `main.go` with command registration. A small thing but it keeps `main.go` focused on wiring.

---

## Code Quality Metrics

| Phase | Files changed | Lines added | Issues found by review | Issues fixed |
|---|---|---|---|---|
| Phase 1 | 10 | +682 | — | — |
| Phase 2 | 12 | +832 | 45 | 45 |
| Phase 3 review | 13 | +626 | 28 | 28 |
| Phase 3 new | 9 | +457 | 19 (P4 review) | 19 |
| Phase 4 | 5 | +260 | — | — |
| **Total** | — | **~2857** | **92** | **92** |

---

## Security Posture

| Control | Status |
|---|---|
| API keys never in CLI flags or logs | ✅ `sanitizeErrorMsg` redacts keys in all error paths |
| Config file `chmod 600` + `O_NOFOLLOW` | ✅ Prevents symlink attacks on write |
| SQLite `chmod 600` after open | ✅ With `log.Printf` on failure |
| Prompt injection | ✅ `html.EscapeString` on all user input |
| LLM response size cap | ✅ 64KB `io.LimitReader` with explicit error on limit hit |
| SQL injection | ✅ Parameterised queries only, no `fmt.Sprintf` in SQL |
| Checksum verification in install | ✅ sha256 verified before `mv` to install dir |
| Rate-limit guard | ✅ Warns at >5 LLM calls/60s, atomic file update |

---

## Known Limitations / Future Work

| Item | Priority | Notes |
|---|---|---|
| `task config init` interactive wizard | Medium | Currently creates a static default file; full interactive setup would improve first-run UX |
| `task rm S-N` has no undo | Low | SQLite cascades delete tasks + subtasks; no soft-delete or recycle |
| Slug IDs are per-project but not user-facing searchable | Low | `T-7` in project A has no relation to `T-7` in project B; could be confusing with `--project` flag |
| `opencode run` JSON parsing relies on undocumented event format | Medium | If opencode changes its JSON output shape, `extractLastAssistantText` breaks silently |
| Homebrew tap needs a separate repo (`homebrew-task-cli`) | Low | Formula exists in main repo; needs moving to separate tap repo for `brew tap` to work |
| No `go test` coverage | High | Zero tests written — everything was manually verified; adding table-driven tests for `db`, `llm/parse`, and `ratelimit` would be the highest-value starting point |
| Windows support | Low | `syscall.O_NOFOLLOW` is POSIX-only; would need build tags to support Windows |

---

## Commit History

```
cff30cb chore: update Homebrew formula with real v0.1.0 checksums
3838c89 fix: address all 19 phase-4 review issues
e35286b feat: phase 4 — distribution, version embedding, GitHub Actions release
5d6d443 fix: address all 28 phase-3 review issues
21f7051 feat: phase 3 — Gemini provider, providers listing, config test, rate limit
006f44c fix: address all 45 review issues
39190d3 feat: phase 2 — retry, context timeout, export, rm, onboarding, security
e56bd6d docs: README, PHASES, TASKS, AGENTS
8d3317a feat: main.go — cobra command tree
5ff496d feat: UI renderer and cobra CLI command tree
d31b1a1 feat: LLM client layer — OpenAI-compat, pi subprocess, opencode subprocess
74addae feat: project scaffold — config, SQLite schema, db layer, project detection
```

---

## Final State

```
task-cli v0.1.0
├── 5 LLM providers  (deepseek, openai, gemini, pi, opencode)
├── 14 CLI commands  (init, story, list, show, status, done, start,
│                     add, rm, export, providers, config, version, help)
├── SQLite storage   (~/.task/tasks.db — projects → stories → tasks → subtasks)
├── Security         (chmod 600, O_NOFOLLOW, html escape, key redaction)
├── Distribution     (GitHub Actions CI/CD, install.sh, Homebrew formula)
└── Zero runtime deps (single binary, ~10MB)
```
