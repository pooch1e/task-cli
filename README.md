# task-cli

Personal user story and task tracker — a Go CLI that uses LLMs to generate user stories and tasks, stored locally in SQLite.

## Install

```bash
make install   # builds and copies to ~/bin/task
```

Or build manually:
```bash
go build -o task ./cmd/task
```

## Quick Start

```bash
# First run — create config
task config init

# Initialise in a repo
cd ~/your-project
task init

# Generate a story with tasks via LLM
task story "add OAuth login"

# Use pi or opencode as LLM backend (free with GitHub Copilot)
task story "add OAuth login" --agent pi --model github-copilot/claude-haiku-4.5
task story "add OAuth login" --agent opencode --model github-copilot/gpt-5-mini

# Dry-run: see the prompt without calling the LLM
task story "add OAuth login" --dry-run

# Track progress
task list
task show S-1
task start T-2
task done T-2
task status

# Add tasks manually
task add task "Write tests" --story S-1
task add subtask "Unit test token refresh" --task T-3
```

## Configuration

Config lives at `~/.task/config.toml` (chmod 600):

```toml
[llm]
provider    = "deepseek"                        # deepseek | openai | pi | opencode
model       = "deepseek-chat"
base_url    = "https://api.deepseek.com/v1"
# api_key   = "sk-..."                          # or set TASK_API_KEY env var
max_tokens  = 1024
timeout_secs = 30

[storage]
db_path = "~/.task/tasks.db"
```

### Free usage via GitHub Copilot

Set `provider = "pi"` or `provider = "opencode"` and pass `--model github-copilot/claude-haiku-4.5` — no API key needed.

## Data

All data is stored in `~/.task/tasks.db` (SQLite, chmod 600). Open it with any SQLite viewer (TablePlus, DB Browser).

## Project detection

`task` auto-detects the project name from the nearest `.git` root directory. Override by passing `--project <name>` to `task list`.

## Commands

| Command | Description |
|---|---|
| `task init` | Initialise project in current directory |
| `task story "<desc>"` | Generate story + tasks via LLM |
| `task list` | List all stories and tasks |
| `task show S-N` | Show story detail with acceptance criteria |
| `task status` | Progress bars per story/task/subtask |
| `task start T-N` | Mark task in-progress |
| `task done T-N\|S-N` | Mark task or story done |
| `task add task "<title>" --story S-N` | Add a task manually |
| `task add subtask "<title>" --task T-N` | Add a subtask manually |
| `task config init` | Create default config file |
| `task config show` | Print config (API key redacted) |
