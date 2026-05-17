package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/joelkram/task-cli/internal/config"
	"github.com/joelkram/task-cli/internal/db"
	"github.com/joelkram/task-cli/internal/export"
	"github.com/joelkram/task-cli/internal/llm"
	"github.com/joelkram/task-cli/internal/ui"
	"github.com/spf13/cobra"
)

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
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// First-run onboarding: detect missing config + missing env key
			// Skip for config subcommands which are meant to fix this state
			if cmd.Name() == "init" || cmd.Parent().Name() == "config" {
				return nil
			}
			_, cfgErr := config.Load()
			if cfgErr == config.ErrNotFound && os.Getenv("TASK_API_KEY") == "" {
				printOnboarding()
				// Don't block — allow commands to run with defaults
			}
			return nil
		},
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
		configCmd(),
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
			ui.Success(fmt.Sprintf("Project %q initialised  %s", app.Project.Name, app.Root))
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

			gen, err := llm.New(cfg).GenerateStory(llm.StoryRequest{
				Feature:     feature,
				ProjectName: app.Project.Name,
			})
			if err != nil {
				return fmt.Errorf("LLM generation failed: %w", err)
			}

			story, err := persistGeneratedStory(app.DB, app.Project.ID, gen)
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
				app.Root = p.Path
			}

			views, err := app.DB.LoadProjectView(app.Project.ID)
			if err != nil {
				return err
			}

			ui.PrintProject(app.Project.Name, app.Root)

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

			ui.PrintProject(app.Project.Name, app.Root)
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
		RunE:  setStatusCmd("done"),
	}
}

func startCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start <T-N>",
		Short: "Mark a task as in-progress",
		Args:  cobra.ExactArgs(1),
		RunE:  setStatusCmd("in-progress"),
	}
}

func setStatusCmd(status string) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		slug := args[0]
		if len(slug) < 2 {
			return fmt.Errorf("invalid slug %q — expected S-N or T-N", slug)
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
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Manually add a task or subtask",
	}
	cmd.AddCommand(addTaskCmd(), addSubtaskCmd())
	return cmd
}

func addTaskCmd() *cobra.Command {
	var storySlug string

	cmd := &cobra.Command{
		Use:   "task <title>",
		Short: "Add a task to a story",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := openAppContext()
			if err != nil {
				return err
			}
			defer app.Close()

			s, err := app.DB.GetStoryBySlug(app.Project.ID, storySlug)
			if err != nil {
				return fmt.Errorf("story %q not found", storySlug)
			}

			t, err := app.DB.CreateTask(s.ID, args[0])
			if err != nil {
				return err
			}

			ui.Success(fmt.Sprintf("Created %s: %s", t.Slug, t.Title))
			return nil
		},
	}

	cmd.Flags().StringVar(&storySlug, "story", "", "Story slug (e.g. S-1)")
	_ = cmd.MarkFlagRequired("story")
	return cmd
}

func addSubtaskCmd() *cobra.Command {
	var taskSlug string

	cmd := &cobra.Command{
		Use:   "subtask <title>",
		Short: "Add a subtask to a task",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := openAppContext()
			if err != nil {
				return err
			}
			defer app.Close()

			t, err := app.DB.GetTaskBySlug(app.Project.ID, taskSlug)
			if err != nil {
				return fmt.Errorf("task %q not found", taskSlug)
			}

			st, err := app.DB.CreateSubtask(t.ID, args[0])
			if err != nil {
				return err
			}

			ui.Success(fmt.Sprintf("Created %s: %s", st.Slug, st.Title))
			return nil
		},
	}

	cmd.Flags().StringVar(&taskSlug, "task", "", "Task slug (e.g. T-1)")
	_ = cmd.MarkFlagRequired("task")
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
			if len(slug) < 2 {
				return fmt.Errorf("invalid slug %q", slug)
			}

			if !yes {
				fmt.Printf("Remove %s and all its children? [y/N] ", slug)
				var confirm string
				fmt.Scanln(&confirm)
				if confirm != "y" && confirm != "Y" {
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
	var format string
	var outFile string

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
			default:
				err = export.ToMarkdown(w, app.Project, views)
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

// ── config ────────────────────────────────────────────────────────────────────

func configCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage task configuration",
	}
	cmd.AddCommand(configInitCmd(), configShowCmd())
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

			display := *cfg
			if display.LLM.APIKey != "" {
				n := len(display.LLM.APIKey)
				if n > 8 {
					n = 8
				}
				display.LLM.APIKey = display.LLM.APIKey[:n] + "…"
			}

			path, _ := config.Path()
			fmt.Printf("Config: %s\n\n", path)
			fmt.Printf("  provider:  %s\n", display.LLM.Provider)
			fmt.Printf("  model:     %s\n", display.LLM.Model)
			fmt.Printf("  base_url:  %s\n", display.LLM.BaseURL)
			fmt.Printf("  api_key:   %s\n", display.LLM.APIKey)
			fmt.Printf("  db_path:   %s\n", display.Storage.DBPath)
			return nil
		},
	}
}

// ── internal helpers ──────────────────────────────────────────────────────────

// persistGeneratedStory writes a GeneratedStory to the database and returns
// the created Story record.
func persistGeneratedStory(d *db.DB, projectID int64, gen *llm.GeneratedStory) (*db.Story, error) {
	acJSON, _ := json.Marshal(gen.Story.AcceptanceCriteria)

	story, err := d.CreateStory(projectID, gen.Story.Title, gen.Story.Description, string(acJSON))
	if err != nil {
		return nil, fmt.Errorf("saving story: %w", err)
	}

	for _, t := range gen.Tasks {
		task, err := d.CreateTask(story.ID, t.Title)
		if err != nil {
			return nil, fmt.Errorf("saving task: %w", err)
		}
		for _, st := range t.Subtasks {
			if _, err := d.CreateSubtask(task.ID, st); err != nil {
				return nil, fmt.Errorf("saving subtask: %w", err)
			}
		}
	}

	return story, nil
}

// printOnboarding prints a first-run guide when no config and no API key exist.
func printOnboarding() {
	ui.Warn("No config found and TASK_API_KEY is not set.")
	fmt.Println()
	fmt.Println("  Quick setup options:")
	fmt.Println()
	fmt.Println("  1. Free via GitHub Copilot (recommended):")
	fmt.Println("       task config init")
	fmt.Println("       # then edit ~/.task/config.toml: set provider = \"pi\"")
	fmt.Println("       task story \"your feature\" --agent pi --model github-copilot/claude-haiku-4.5")
	fmt.Println()
	fmt.Println("  2. DeepSeek direct (~$0.00011 per call):")
	fmt.Println("       task config init")
	fmt.Println("       export TASK_API_KEY=sk-...")
	fmt.Println()
}
