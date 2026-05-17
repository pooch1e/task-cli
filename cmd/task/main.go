package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/joelkram/task-cli/internal/config"
	"github.com/joelkram/task-cli/internal/db"
	"github.com/joelkram/task-cli/internal/export"
	"github.com/joelkram/task-cli/internal/llm"
	"github.com/joelkram/task-cli/internal/ratelimit"
	"github.com/joelkram/task-cli/internal/ui"
	"github.com/spf13/cobra"
)

// version is set at build time via:
//
//	go build -ldflags "-X main.version=$(git describe --tags --always)"
//
// Falls back to "dev" when built without the flag.
var version = "dev"

func main() {
	if err := rootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

// ── root ──────────────────────────────────────────────────────────────────────

func rootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "task",
		Short: "Personal user story and task tracker",
		Long:  "Track user stories, tasks and subtasks for your projects.\nPowered by LLM story generation via DeepSeek, pi, or opencode.",
	}
	cmd.AddCommand(
		initCmd(),
		storyCmd(),
		listCmd(),
		showCmd(),
		statusCmd(),
		doneCmd(),
		startCmd(),
		addCmd(),
		rmCmd(),
		exportCmd(),
		providersCmd(),
		configCmd(),
		versionCmd(),
	)
	return cmd
}

// ── init ──────────────────────────────────────────────────────────────────────

func initCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialise task for the current project",
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := openAppContext()
			if err != nil {
				return err
			}
			defer app.Close()
			ui.Success(fmt.Sprintf("Project %q initialised  %s", app.Project.Name, app.Project.Path))
			return nil
		},
	}
}

// ── story ─────────────────────────────────────────────────────────────────────

func storyCmd() *cobra.Command {
	var dryRun bool
	var agent, model string

	cmd := &cobra.Command{
		Use:   "story <feature description>",
		Short: "Generate a user story and tasks via LLM",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			feature := args[0]

			cfg, err := config.LoadOrDefault()
			if err != nil {
				return err
			}
			if agent != "" {
				cfg.LLM.Provider = agent
			}
			if model != "" {
				cfg.LLM.Model = model
			}

			if dryRun {
				fmt.Println("── Dry run ──")
				fmt.Printf("Provider: %s   Model: %s\n\n", cfg.LLM.Provider, cfg.LLM.Model)
				fmt.Println(llm.BuildPrompt(llm.StoryRequest{Feature: feature, ProjectName: "preview"}))
				return nil
			}

			// First-run check: no config file and no API key for direct providers.
			if _, cfgErr := config.Load(); cfgErr == config.ErrNotFound &&
				cfg.LLM.Provider != config.ProviderPi &&
				cfg.LLM.Provider != config.ProviderOpencode &&
				os.Getenv("TASK_API_KEY") == "" {
				printOnboarding()
			}

			if err := cfg.Validate(); err != nil {
				ui.Error(err.Error())
				ui.Info("Set TASK_API_KEY=sk-... or run: task config init")
				return nil
			}

			app, err := openAppContext()
			if err != nil {
				return err
			}
			defer app.Close()

			ui.Info(fmt.Sprintf("Generating story for %q using %s/%s …", feature, cfg.LLM.Provider, cfg.LLM.Model))

			// Rate-limit guard: warn if the LLM is being called too frequently.
			if dir, err := config.Dir(); err == nil {
				if exceeded, rlErr := ratelimit.CheckAndRecord(dir); rlErr == nil && exceeded {
					ui.Warn("Rate limit: more than 5 LLM calls in the last 60s — check your usage.")
				}
			}

			client, err := llm.New(cfg)
			if err != nil {
				return fmt.Errorf("invalid provider: %w", err)
			}
			gen, err := client.GenerateStory(llm.StoryRequest{
				Feature:     feature,
				ProjectName: app.Project.Name,
			})
			if err != nil {
				return fmt.Errorf("LLM generation failed: %w", err)
			}

			story, err := saveGeneratedStory(app.DB, app.Project.ID, gen)
			if err != nil {
				return err
			}

			ui.Success(fmt.Sprintf("Created %s: %s", story.Slug, story.Title))
			view, err := app.DB.LoadStoryView(story.ID)
			if err != nil {
				return err
			}
			ui.PrintStory(view)
			return nil
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print the prompt without calling the LLM")
	cmd.Flags().StringVar(&agent, "agent", "", "LLM provider override: deepseek | openai | pi | opencode")
	cmd.Flags().StringVar(&model, "model", "", "Model override (e.g. github-copilot/claude-haiku-4.5)")
	return cmd
}

// ── list ──────────────────────────────────────────────────────────────────────

func listCmd() *cobra.Command {
	var projectName string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all stories and tasks for the current project",
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := openAppContext()
			if err != nil {
				return err
			}
			defer app.Close()

			if projectName != "" {
				p, err := app.DB.GetProject(projectName)
				if err != nil {
					return fmt.Errorf("project %q not found", projectName)
				}
				app.Project = p
			}

			views, err := app.DB.LoadProjectView(app.Project.ID)
			if err != nil {
				return err
			}

			ui.PrintProject(app.Project.Name, app.Project.Path)
			if len(views) == 0 {
				ui.Info("No stories yet. Run: task story \"<feature>\"")
				return nil
			}
			for _, v := range views {
				ui.PrintStory(v)
			}
			fmt.Println()
			return nil
		},
	}

	cmd.Flags().StringVarP(&projectName, "project", "p", "", "Project name (defaults to current repo)")
	return cmd
}

// ── show ──────────────────────────────────────────────────────────────────────

func showCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <S-N>",
		Short: "Show a story with full detail including acceptance criteria",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := openAppContext()
			if err != nil {
				return err
			}
			defer app.Close()

			s, err := app.DB.GetStoryBySlug(app.Project.ID, args[0])
			if err != nil {
				return fmt.Errorf("story %q not found in project %q", args[0], app.Project.Name)
			}
			view, err := app.DB.LoadStoryView(s.ID)
			if err != nil {
				return err
			}
			fmt.Println()
			ui.PrintStory(view)
			ui.PrintAcceptanceCriteria(s.AcceptanceCriteria)
			fmt.Println()
			return nil
		},
	}
}

// ── status ────────────────────────────────────────────────────────────────────

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show project progress summary",
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := openAppContext()
			if err != nil {
				return err
			}
			defer app.Close()
			stats, err := app.DB.GetProjectStats(app.Project.ID)
			if err != nil {
				return err
			}
			ui.PrintProject(app.Project.Name, app.Project.Path)
			ui.PrintStats(stats)
			return nil
		},
	}
}

// ── done / start ──────────────────────────────────────────────────────────────

func doneCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "done <T-N | S-N>",
		Short: "Mark a task or story as done",
		Args:  cobra.ExactArgs(1),
		RunE:  setStatus("done"),
	}
}

func startCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start <T-N>",
		Short: "Mark a task as in-progress",
		Args:  cobra.ExactArgs(1),
		RunE:  setStatus("in-progress"),
	}
}

// setStatus returns a RunE handler that transitions a slug to the given status.
func setStatus(status string) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		slug := args[0]
		if err := validateSlug(slug); err != nil {
			return err
		}
		app, err := openAppContext()
		if err != nil {
			return err
		}
		defer app.Close()

		switch slug[0] {
		case 'S':
			err = app.DB.SetStoryStatus(app.Project.ID, slug, status)
		case 'T':
			err = app.DB.SetTaskStatus(app.Project.ID, slug, status)
		default:
			return fmt.Errorf("unrecognised slug %q — use S-N for stories, T-N for tasks", slug)
		}
		if err != nil {
			return err
		}
		ui.Success(fmt.Sprintf("%s marked as %s", slug, status))
		return nil
	}
}

// ── add ───────────────────────────────────────────────────────────────────────

func addCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "add", Short: "Manually add a task or subtask"}
	cmd.AddCommand(
		addChildCmd(addChildConfig{
			use:       "task <title>",
			short:     "Add a task to a story",
			flagName:  "story",
			flagUsage: "Story slug (e.g. S-1)",
			lookup: func(app *AppContext, slug string) (int64, error) {
				s, err := app.DB.GetStoryBySlug(app.Project.ID, slug)
				if err != nil {
					return 0, fmt.Errorf("story %q not found", slug)
				}
				return s.ID, nil
			},
			create: func(app *AppContext, parentID int64, title string) (string, string, error) {
				t, err := app.DB.CreateTask(app.Project.ID, parentID, title)
				if err != nil {
					return "", "", err
				}
				return t.Slug, t.Title, nil
			},
		}),
		addChildCmd(addChildConfig{
			use:       "subtask <title>",
			short:     "Add a subtask to a task",
			flagName:  "task",
			flagUsage: "Task slug (e.g. T-1)",
			lookup: func(app *AppContext, slug string) (int64, error) {
				t, err := app.DB.GetTaskBySlug(app.Project.ID, slug)
				if err != nil {
					return 0, fmt.Errorf("task %q not found", slug)
				}
				return t.ID, nil
			},
			create: func(app *AppContext, parentID int64, title string) (string, string, error) {
				st, err := app.DB.CreateSubtask(parentID, title)
				if err != nil {
					return "", "", err
				}
				return st.Slug, st.Title, nil
			},
		}),
	)
	return cmd
}

// addChildConfig parameterises addChildCmd for both task and subtask creation.
type addChildConfig struct {
	use, short, flagName, flagUsage string
	lookup                          func(*AppContext, string) (int64, error)
	create                          func(*AppContext, int64, string) (string, string, error)
}

// addChildCmd builds a cobra sub-command that looks up a parent by slug and
// creates a child record, printing the new slug on success.
func addChildCmd(cfg addChildConfig) *cobra.Command {
	var parentSlug string
	cmd := &cobra.Command{
		Use:   cfg.use,
		Short: cfg.short,
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := openAppContext()
			if err != nil {
				return err
			}
			defer app.Close()

			parentID, err := cfg.lookup(app, parentSlug)
			if err != nil {
				return err
			}
			slug, title, err := cfg.create(app, parentID, args[0])
			if err != nil {
				return err
			}
			ui.Success(fmt.Sprintf("Created %s: %s", slug, title))
			return nil
		},
	}
	cmd.Flags().StringVar(&parentSlug, cfg.flagName, "", cfg.flagUsage)
	_ = cmd.MarkFlagRequired(cfg.flagName)
	return cmd
}

// ── rm ────────────────────────────────────────────────────────────────────────

func rmCmd() *cobra.Command {
	var yes bool

	cmd := &cobra.Command{
		Use:   "rm <S-N | T-N>",
		Short: "Remove a story or task (and all its children)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug := args[0]
			if err := validateSlug(slug); err != nil {
				return err
			}
			if !yes {
				if !ui.IsTTY() {
					return fmt.Errorf("non-interactive terminal: use --yes to confirm removal of %s", slug)
				}
				fmt.Printf("Remove %s and all its children? [y/N] ", slug)
				var confirm string
				if _, err := fmt.Scanln(&confirm); err != nil || (confirm != "y" && confirm != "Y") {
					ui.Info("Aborted.")
					return nil
				}
			}

			app, err := openAppContext()
			if err != nil {
				return err
			}
			defer app.Close()

			switch slug[0] {
			case 'S':
				err = app.DB.DeleteStory(app.Project.ID, slug)
			case 'T':
				err = app.DB.DeleteTask(app.Project.ID, slug)
			default:
				return fmt.Errorf("unrecognised slug %q — use S-N or T-N", slug)
			}
			if err != nil {
				return err
			}
			ui.Success(fmt.Sprintf("Removed %s", slug))
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation prompt")
	return cmd
}

// ── export ────────────────────────────────────────────────────────────────────

func exportCmd() *cobra.Command {
	var format, outFile string

	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export stories to markdown or JSON",
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := openAppContext()
			if err != nil {
				return err
			}
			defer app.Close()

			views, err := app.DB.LoadProjectView(app.Project.ID)
			if err != nil {
				return err
			}
			if len(views) == 0 {
				ui.Info("Nothing to export.")
				return nil
			}

			w := os.Stdout
			if outFile != "" {
				f, err := os.Create(outFile)
				if err != nil {
					return fmt.Errorf("creating output file: %w", err)
				}
				defer f.Close()
				w = f
			}

			switch format {
			case "json":
				err = export.ToJSON(w, app.Project, views)
			case "markdown":
				err = export.ToMarkdown(w, app.Project, views)
			default:
				return fmt.Errorf("unknown format %q — use markdown or json", format)
			}
			if err != nil {
				return err
			}
			if outFile != "" {
				ui.Success(fmt.Sprintf("Exported to %s", outFile))
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&format, "format", "f", "markdown", "Output format: markdown | json")
	cmd.Flags().StringVarP(&outFile, "out", "o", "", "Output file (defaults to stdout)")
	return cmd
}

// ── providers ────────────────────────────────────────────────────────────────

func providersCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "providers",
		Short: "List all supported LLM providers with cost and key information",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _ := config.LoadOrDefault()
			current := cfg.LLM.Provider

			fmt.Printf("\n  %-12s  %-32s  %-14s  %-20s  %s\n",
				"Provider", "Default model", "Cost/1K in", "Key required", "Notes")
			fmt.Printf("  %s\n", ui.Dimmed(strings.Repeat("-", 100)))

			for _, p := range config.KnownProviders {
				marker := "  "
				if p.Name == current {
					marker = "\u25b6 "
				}
				var cost string
				if p.CostPer1KInput == 0 {
					cost = "free"
				} else {
					cost = fmt.Sprintf("$%.5f", p.CostPer1KInput)
				}
				keyInfo := "none"
				if p.KeyRequired {
					if p.KeyEnvVar != "" && p.KeyEnvVar != "TASK_API_KEY" {
						keyInfo = p.KeyEnvVar + " or TASK_API_KEY"
					} else {
						keyInfo = "TASK_API_KEY"
					}
				}
				fmt.Printf("%s%-12s  %-32s  %-14s  %-20s  %s\n",
					marker, p.Name,
					truncate(p.DefaultModel, 32),
					cost, keyInfo, p.Notes,
				)
			}
			fmt.Printf("\n  \u25b6 = current provider (%s)\n\n", current)
			return nil
		},
	}
}

// ── config ────────────────────────────────────────────────────────────────────

func configCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "config", Short: "Manage task configuration"}
	cmd.AddCommand(configInitCmd(), configShowCmd(), configTestCmd())
	return cmd
}

func configInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Create default config at ~/.task/config.toml",
		RunE: func(cmd *cobra.Command, args []string) error {
			path, _ := config.Path()
			if _, err := os.Stat(path); err == nil {
				ui.Warn(fmt.Sprintf("Config already exists at %s", path))
				ui.Info("Edit it directly, or delete it and re-run to regenerate.")
				return nil
			}
			if err := config.Save(config.Default()); err != nil {
				return err
			}
			ui.Success(fmt.Sprintf("Config created at %s", path))
			ui.Info("Set your API key: edit the file or set TASK_API_KEY=sk-...")
			ui.Info("For free usage: set provider = \"pi\" and use --model github-copilot/claude-haiku-4.5")
			return nil
		},
	}
}

func configShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Print current config (API key redacted)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err == config.ErrNotFound {
				ui.Warn("No config file found. Run: task config init")
				return nil
			}
			if err != nil {
				return err
			}
			key := cfg.LLM.APIKey
			if len(key) > 8 {
				key = key[:8] + "…"
			}
			path, _ := config.Path()
			fmt.Printf("Config: %s\n\n", path)
			fmt.Printf("  provider:  %s\n", cfg.LLM.Provider)
			fmt.Printf("  model:     %s\n", cfg.LLM.Model)
			fmt.Printf("  base_url:  %s\n", cfg.LLM.BaseURL)
			fmt.Printf("  api_key:   %s\n", key)
			fmt.Printf("  db_path:   %s\n", cfg.Storage.DBPath)
			return nil
		},
	}
}

func configTestCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "test",
		Short: "Fire a test prompt to verify the current config is working",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err == config.ErrNotFound {
				ui.Warn("No config file. Run: task config init")
				return nil
			}
			if err != nil {
				return err
			}
			if err := cfg.Validate(); err != nil {
				ui.Error(err.Error())
				return nil
			}
			ui.Info(fmt.Sprintf("Testing %s / %s …", cfg.LLM.Provider, cfg.LLM.Model))
			client, err := llm.New(cfg)
			if err != nil {
				ui.Error(err.Error())
				return nil
			}
			if err := client.Ping(); err != nil {
				ui.Error(fmt.Sprintf("Connection failed: %s", err))
				return nil
			}
			ui.Success("Connection OK")
			return nil
		},
	}
}

// ── version ────────────────────────────────────────────────────────────────

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the task-cli version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("task-cli %s\n", version)
		},
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

// saveGeneratedStory converts a GeneratedStory to DB inputs and persists it
// atomically via db.PersistStory (single transaction).
func saveGeneratedStory(d *db.DB, projectID int64, gen *llm.GeneratedStory) (*db.Story, error) {
	acJSON, err := json.Marshal(gen.Story.AcceptanceCriteria)
	if err != nil {
		return nil, fmt.Errorf("marshaling acceptance criteria: %w", err)
	}

	tasks := make([]db.TaskInput, len(gen.Tasks))
	for i, t := range gen.Tasks {
		tasks[i] = db.TaskInput{Title: t.Title, Subtasks: t.Subtasks}
	}

	return d.PersistStory(projectID, gen.Story.Title, gen.Story.Description, string(acJSON), tasks)
}

// printOnboarding prints a first-run setup guide.
func printOnboarding() {
	ui.Warn("No config found and TASK_API_KEY is not set.")
	fmt.Println()
	fmt.Println("  Quick setup:")
	fmt.Println()
	fmt.Println("  Free via GitHub Copilot:")
	fmt.Println("    task story \"your feature\" --agent pi --model github-copilot/claude-haiku-4.5")
	fmt.Println()
	fmt.Println("  DeepSeek direct (~$0.00011/call):")
	fmt.Println("    task config init && export TASK_API_KEY=sk-...")
	fmt.Println()
}

// truncate shortens s to at most maxLen runes, appending … if truncated.
// maxLen must be ≥2; values below that return s unchanged.
func truncate(s string, maxLen int) string {
	if maxLen < 2 || len(s) <= maxLen {
		return s
	}
	return s[:maxLen-1] + "…"
}

// validateSlug returns an error if s is not a valid S-N or T-N slug.
func validateSlug(s string) error {
	if len(s) < 2 || (s[0] != 'S' && s[0] != 'T') || s[1] != '-' {
		return fmt.Errorf("invalid slug %q — expected S-N or T-N", s)
	}
	return nil
}
