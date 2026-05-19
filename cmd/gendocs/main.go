// gendocs generates man pages for task-cli using cobra/doc.
//
// Usage (run from repo root):
//
//	go run ./cmd/gendocs -o man/
//
// This is a build-time tool — it is not shipped in the binary.
// The Makefile `make man` target runs it and writes man/task-cli.1.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

func main() {
	outDir := flag.String("o", "man", "Output directory for man pages")
	flag.Parse()

	if err := os.MkdirAll(*outDir, 0755); err != nil {
		log.Fatalf("creating output dir: %v", err)
	}

	root := buildRoot()

	// cobra/doc.GenManTree writes one file per command.
	// For task-cli this produces task-cli.1 plus sub-pages for subcommands.
	header := &doc.GenManHeader{
		Title:   "TASK-CLI",
		Section: "1",
		Date:    func() *time.Time { t := time.Now(); return &t }(),
		Source:  "task-cli",
		Manual:  "task-cli Manual",
	}

	if err := doc.GenManTree(root, header, *outDir); err != nil {
		log.Fatalf("generating man pages: %v", err)
	}

	fmt.Printf("Man pages written to %s/\n", *outDir)
}

// buildRoot constructs the cobra command tree with all metadata but no RunE
// bodies. This mirrors cmd/task/main.go exactly in terms of Use/Short/Long/Flags.
func buildRoot() *cobra.Command {
	root := &cobra.Command{
		Use:   "task",
		Short: "Personal user story and task tracker",
		Long: `task-cli is a local-first user story and task tracker for software projects.

It uses LLMs to generate structured user stories (with acceptance criteria) and
break them into tasks and subtasks — stored in a local SQLite database so your
work is always private and offline-capable.

Data model:
  Project
   └── Story  (S-1, S-2 …)       — "As a user, I want to …"
        └── Task   (T-1, T-2 …)  — concrete implementation steps
             └── Subtask (T-1.1) — optional fine-grained checklist

Quick start (free, no API key required):
  task init
  task story "add OAuth login" --agent pi --model github-copilot/claude-haiku-4.5
  task list
  task start T-1
  task done  T-1

For full documentation run: man task-cli`,
	}

	root.AddCommand(
		initDocs(),
		storyDocs(),
		listDocs(),
		showDocs(),
		statusDocs(),
		doneDocs(),
		startDocs(),
		addDocs(),
		rmDocs(),
		exportDocs(),
		providersDocs(),
		configDocs(),
		obsidianDocs(),
		versionDocs(),
	)

	// Disable the default completion command so it doesn't appear in the man page.
	root.CompletionOptions.DisableDefaultCmd = true

	return root
}

// ── commands (metadata only — no RunE) ───────────────────────────────────────

func initDocs() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialise task for the current project",
		Long: `Registers the current repository as a task-cli project.

The project name is derived from the nearest .git root directory. Run this once
per repo before creating stories. It is safe to run multiple times.

  cd ~/my-project
  task init`,
	}
}

func storyDocs() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "story <feature description>",
		Short: "Generate a user story and tasks via LLM",
		Long: `Sends a feature description to an LLM and generates a structured user story
with acceptance criteria, tasks, and subtasks. The result is saved to the local
database immediately.

The story follows the format: "As a <user>, I want to <goal> so that <reason>."
Acceptance criteria use Given/When/Then scenarios.

Examples:
  # Paid provider (DeepSeek, ~$0.00011/call — set TASK_API_KEY first)
  task story "add OAuth login"

  # Free via GitHub Copilot (pi subprocess)
  task story "add OAuth login" --agent pi --model github-copilot/claude-haiku-4.5

  # Free via GitHub Copilot (opencode subprocess)
  task story "add OAuth login" --agent opencode --model github-copilot/gpt-5-mini

  # Preview the prompt without calling the LLM
  task story "add OAuth login" --dry-run`,
	}
	cmd.Flags().Bool("dry-run", false, "Print the prompt without calling the LLM")
	cmd.Flags().String("agent", "", "LLM provider override: deepseek | openai | pi | opencode")
	cmd.Flags().String("model", "", "Model override (e.g. github-copilot/claude-haiku-4.5)")
	return cmd
}

func listDocs() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all stories and tasks for the current project",
		Long: `Prints every story and its tasks for the current project in a tree view.
Story and task status is shown inline (open, in-progress, done).

The project is auto-detected from the nearest .git root. Use --project to
target a different project stored in the same database.

Examples:
  task list
  task list --project my-other-repo`,
	}
	cmd.Flags().StringP("project", "p", "", "Project name (defaults to current repo)")
	return cmd
}

func showDocs() *cobra.Command {
	return &cobra.Command{
		Use:   "show <S-N>",
		Short: "Show a story with full detail including acceptance criteria",
		Long: `Prints the full detail of a single story: title, description, tasks with
their subtasks, and the acceptance criteria (Given/When/Then scenarios).

Use 'task list' first to find the story slug (e.g. S-1, S-2).

Example:
  task show S-1`,
	}
}

func statusDocs() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show project progress summary",
		Long: `Displays counts and completion percentages for stories, tasks, and subtasks
in the current project.

Example:
  task status`,
	}
}

func doneDocs() *cobra.Command {
	return &cobra.Command{
		Use:   "done <T-N | S-N>",
		Short: "Mark a task or story as done",
		Long: `Marks a task or story as done.

Pass a task slug (T-N) to mark that task done.
Pass a story slug (S-N) to mark the whole story done.

Examples:
  task done T-3
  task done S-1`,
	}
}

func startDocs() *cobra.Command {
	return &cobra.Command{
		Use:   "start <T-N>",
		Short: "Mark a task as in-progress",
		Long: `Marks a task as in-progress. Use this when you begin working on a task so
the status shows in 'task list' and 'task status'.

Example:
  task start T-2`,
	}
}

func addDocs() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Manually add a task or subtask",
		Long: `Adds a task or subtask manually without calling the LLM.

Subcommands:
  task add task "<title>" --story S-N      Add a task to a story
  task add subtask "<title>" --task T-N    Add a subtask to a task

Examples:
  task add task "Write integration tests" --story S-2
  task add subtask "Test error paths" --task T-4`,
	}

	addTask := &cobra.Command{
		Use:   "task <title>",
		Short: "Add a task to a story",
		Long:  "Creates a new task under the given story.\n\nExample:\n  task add task \"Write integration tests\" --story S-2",
	}
	addTask.Flags().String("story", "", "Story slug (e.g. S-1)")

	addSubtask := &cobra.Command{
		Use:   "subtask <title>",
		Short: "Add a subtask to a task",
		Long:  "Creates a new subtask under the given task. Subtasks are displayed inline\nand tracked with slugs like T-1.1, T-1.2.\n\nExample:\n  task add subtask \"Test error paths\" --task T-4",
	}
	addSubtask.Flags().String("task", "", "Task slug (e.g. T-1)")

	cmd.AddCommand(addTask, addSubtask)
	return cmd
}

func rmDocs() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rm <S-N | T-N>",
		Short: "Remove a story or task (and all its children)",
		Long: `Permanently removes a story or task and all of its children.

Removing a story (S-N) also removes all its tasks and subtasks.
Removing a task (T-N) also removes all its subtasks.

You will be prompted for confirmation unless --yes is passed.

Examples:
  task rm S-2
  task rm T-5 --yes`,
	}
	cmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")
	return cmd
}

func exportDocs() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export stories to markdown or JSON",
		Long: `Exports all stories and tasks for the current project to a single file or
stdout. Useful for sharing, archiving, or feeding into other tools.

Formats:
  markdown  — Human-readable document with headers and checklists (default)
  json      — Machine-readable array of story objects

Examples:
  task export                              # markdown to stdout
  task export -f json                      # JSON to stdout
  task export -f markdown -o stories.md    # write to file`,
	}
	cmd.Flags().StringP("format", "f", "markdown", "Output format: markdown | json")
	cmd.Flags().StringP("out", "o", "", "Output file (defaults to stdout)")
	return cmd
}

func providersDocs() *cobra.Command {
	return &cobra.Command{
		Use:   "providers",
		Short: "List all supported LLM providers with cost and key information",
		Long: `Lists all LLM providers supported by task-cli, their default models, cost
per 1,000 input tokens, and which environment variable or API key they require.

Free providers (pi, opencode) use your GitHub Copilot subscription via a local
subprocess — no API key needed.

The active provider is marked with ▶.`,
	}
}

func configDocs() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage task configuration",
		Long: `Manage the task-cli configuration file at ~/.task/config.toml.

Subcommands:
  config init    Create the config file with defaults
  config show    Print the current config (API key redacted)
  config test    Fire a test prompt to verify the LLM connection`,
	}

	cmd.AddCommand(
		&cobra.Command{
			Use:   "init",
			Short: "Create default config at ~/.task/config.toml",
			Long: `Creates ~/.task/config.toml with sensible defaults (DeepSeek provider).

Edit the file to set your API key, or set TASK_API_KEY in your environment.
For free usage set provider = "pi" and use --model github-copilot/<model>.

Safe to run only once — does nothing if the file already exists.`,
		},
		&cobra.Command{
			Use:   "show",
			Short: "Print current config (API key redacted)",
			Long:  "Prints the active configuration. The API key is truncated for safety.\n\nRun 'task config init' first if no config file exists.",
		},
		&cobra.Command{
			Use:   "test",
			Short: "Fire a test prompt to verify the current config is working",
			Long:  "Sends a minimal test prompt to the configured LLM provider and reports\nwhether the connection and API key are valid.\n\nUseful after changing providers or rotating API keys.",
		},
	)
	return cmd
}

func obsidianDocs() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "obsidian",
		Short: "Obsidian vault integration",
		Long: `Exports stories and tasks as interconnected Markdown files into an Obsidian vault.

Each story becomes a directory containing story.md and a tasks/ subfolder.
Task files link back to their parent story; story files link to each task.
This creates a navigable graph in Obsidian's Graph View.

Workflow:
  task obsidian set-vault ~/Documents/MyVault    # one-time setup
  task obsidian export                           # write files
  # Open Obsidian — your project appears under <vault>/<project>/

Files are never overwritten unless --force is passed, so you can annotate
and edit the Markdown freely in Obsidian without losing your notes.`,
	}

	exportCmd := &cobra.Command{
		Use:   "export",
		Short: "Export stories and tasks as linked Markdown files to an Obsidian vault",
		Long: `Writes one directory per story under <vault>/<project>/. Each story
directory contains story.md and a tasks/ subdirectory with one file per task.

Existing files are skipped by default to preserve manual edits. Use --force to
overwrite them.

Vault layout:
  <vault>/<project>/
    S-1 Story Title/
      story.md
      tasks/
        T-1 Task Title.md
        T-2 Task Title.md

Examples:
  task obsidian export
  task obsidian export --vault ~/Documents/MyVault
  task obsidian export --force`,
	}
	exportCmd.Flags().String("vault", "", "Override the vault path from config")
	exportCmd.Flags().Bool("force", false, "Overwrite existing files (discards manual edits)")

	setVaultCmd := &cobra.Command{
		Use:   "set-vault <path>",
		Short: "Save the Obsidian vault path to ~/.task/config.toml",
		Long: `Writes the given directory path to ~/.task/config.toml as [obsidian] vault_path.
After setting this once, 'task obsidian export' needs no extra flags.

The path must be an existing directory (your Obsidian vault root).

Example:
  task obsidian set-vault ~/Documents/MyVault`,
	}

	cmd.AddCommand(exportCmd, setVaultCmd)
	return cmd
}

func versionDocs() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the task-cli version",
		Long:  "Prints the version string embedded at build time.\n\nExample:\n  task version",
	}
}
