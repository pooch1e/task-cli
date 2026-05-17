package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/joelkram/task-cli/internal/config"
	"github.com/joelkram/task-cli/internal/db"
	"github.com/joelkram/task-cli/internal/llm"
	"github.com/joelkram/task-cli/internal/project"
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
		configCmd(),
	)

	return cmd
}

// ── shared helpers ────────────────────────────────────────────────────────────

func mustOpenDB() *db.DB {
	cfg, err := config.Load()
	if err == config.ErrNotFound {
		cfg = config.Default()
	} else if err != nil {
		ui.Error(err.Error())
		os.Exit(1)
	}

	d, err := db.Open(cfg.Storage.DBPath)
	if err != nil {
		ui.Error(fmt.Sprintf("opening database: %s", err))
		os.Exit(1)
	}
	return d
}

func mustProject(d *db.DB) (*db.Project, string) {
	name, root := project.Detect()
	p, err := d.UpsertProject(name, root)
	if err != nil {
		ui.Error(fmt.Sprintf("detecting project: %s", err))
		os.Exit(1)
	}
	return p, root
}

// ── init ──────────────────────────────────────────────────────────────────────

func initCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialise task for the current project",
		RunE: func(cmd *cobra.Command, args []string) error {
			d := mustOpenDB()
			defer d.Close()

			p, root := mustProject(d)
			ui.Success(fmt.Sprintf("Project %q initialised  %s", p.Name, root))
			return nil
		},
	}
}

// ── story ─────────────────────────────────────────────────────────────────────

func storyCmd() *cobra.Command {
	var dryRun bool
	var agent string
	var model string

	cmd := &cobra.Command{
		Use:   "story <feature description>",
		Short: "Generate a user story and tasks via LLM",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			feature := args[0]

			cfg, err := loadOrDefault()
			if err != nil {
				return err
			}

			// flag overrides
			if agent != "" {
				cfg.LLM.Provider = agent
			}
			if model != "" {
				cfg.LLM.Model = model
			}

			if dryRun {
				req := llm.StoryRequest{Feature: feature, ProjectName: "preview"}
				fmt.Println("── Dry run ──")
				fmt.Printf("Provider: %s   Model: %s\n\n", cfg.LLM.Provider, cfg.LLM.Model)
				fmt.Println(llm.BuildPrompt(req))
				return nil
			}

			if err := cfg.Validate(); err != nil {
				ui.Error(err.Error())
				ui.Info("Set TASK_API_KEY=sk-... or run: task config init")
				return nil
			}

			d := mustOpenDB()
			defer d.Close()

			p, _ := mustProject(d)

			ui.Info(fmt.Sprintf("Generating story for %q using %s/%s …", feature, cfg.LLM.Provider, cfg.LLM.Model))

			client := llm.New(cfg)
			gen, err := client.GenerateStory(llm.StoryRequest{
				Feature:     feature,
				ProjectName: p.Name,
			})
			if err != nil {
				return fmt.Errorf("LLM generation failed: %w", err)
			}

			// Persist to DB
			acJSON, _ := json.Marshal(gen.Story.AcceptanceCriteria)

			story, err := d.CreateStory(
				p.ID,
				gen.Story.Title,
				gen.Story.Description,
				string(acJSON),
			)
			if err != nil {
				return fmt.Errorf("saving story: %w", err)
			}

			for _, t := range gen.Tasks {
				task, err := d.CreateTask(story.ID, t.Title)
				if err != nil {
					return fmt.Errorf("saving task: %w", err)
				}
				for _, st := range t.Subtasks {
					if _, err := d.CreateSubtask(task.ID, st); err != nil {
						return fmt.Errorf("saving subtask: %w", err)
					}
				}
			}

			ui.Success(fmt.Sprintf("Created %s: %s", story.Slug, story.Title))

			tasks, _ := d.ListTasksForStory(story.ID)
			subtaskMap := map[int64][]*db.Subtask{}
			for _, t := range tasks {
				subtaskMap[t.ID], _ = d.ListSubtasksForTask(t.ID)
			}
			ui.PrintStory(story, tasks, subtaskMap)
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
			d := mustOpenDB()
			defer d.Close()

			var p *db.Project
			var root string

			if projectName != "" {
				var err error
				p, err = d.GetProject(projectName)
				if err != nil {
					return fmt.Errorf("project %q not found", projectName)
				}
				root = p.Path
			} else {
				p, root = mustProject(d)
			}

			stories, err := d.ListStories(p.ID)
			if err != nil {
				return err
			}

			ui.PrintProject(p.Name, root)

			if len(stories) == 0 {
				ui.Info("No stories yet. Run: task story \"<feature>\"")
				return nil
			}

			for _, s := range stories {
				tasks, _ := d.ListTasksForStory(s.ID)
				subtaskMap := map[int64][]*db.Subtask{}
				for _, t := range tasks {
					subtaskMap[t.ID], _ = d.ListSubtasksForTask(t.ID)
				}
				ui.PrintStory(s, tasks, subtaskMap)
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
		Short: "Show a story with full detail",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug := args[0]

			d := mustOpenDB()
			defer d.Close()

			p, _ := mustProject(d)

			s, err := d.GetStoryBySlug(p.ID, slug)
			if err != nil {
				return fmt.Errorf("story %q not found in project %q", slug, p.Name)
			}

			tasks, _ := d.ListTasksForStory(s.ID)
			subtaskMap := map[int64][]*db.Subtask{}
			for _, t := range tasks {
				subtaskMap[t.ID], _ = d.ListSubtasksForTask(t.ID)
			}

			fmt.Println()
			ui.PrintStory(s, tasks, subtaskMap)

			// Print acceptance criteria
			if s.AcceptanceCriteria != "" {
				fmt.Println()
				fmt.Println("  Acceptance criteria:")
				var criteria []string
				if err := json.Unmarshal([]byte(s.AcceptanceCriteria), &criteria); err == nil {
					for _, c := range criteria {
						fmt.Printf("    · %s\n", c)
					}
				}
			}
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
			d := mustOpenDB()
			defer d.Close()

			p, root := mustProject(d)
			stats, err := d.GetProjectStats(p.ID)
			if err != nil {
				return err
			}

			ui.PrintProject(p.Name, root)
			ui.PrintStats(stats)
			return nil
		},
	}
}

// ── done ──────────────────────────────────────────────────────────────────────

func doneCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "done <T-N | S-N>",
		Short: "Mark a task or story as done",
		Args:  cobra.ExactArgs(1),
		RunE:  setStatusCmd("done"),
	}
}

// ── start ─────────────────────────────────────────────────────────────────────

func startCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start <T-N>",
		Short: "Mark a task as in-progress",
		Args:  cobra.ExactArgs(1),
		RunE:  setStatusCmd("in-progress"),
	}
}

func setStatusCmd(status string) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		slug := args[0]

		d := mustOpenDB()
		defer d.Close()

		p, _ := mustProject(d)

		switch slug[0] {
		case 'S':
			if err := d.SetStoryStatus(p.ID, slug, status); err != nil {
				return err
			}
		case 'T':
			if err := d.SetTaskStatus(p.ID, slug, status); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unrecognised slug %q — use S-N for stories, T-N for tasks", slug)
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
			if storySlug == "" {
				return fmt.Errorf("--story required (e.g. --story S-1)")
			}
			title := args[0]

			d := mustOpenDB()
			defer d.Close()

			p, _ := mustProject(d)

			s, err := d.GetStoryBySlug(p.ID, storySlug)
			if err != nil {
				return fmt.Errorf("story %q not found", storySlug)
			}

			t, err := d.CreateTask(s.ID, title)
			if err != nil {
				return err
			}

			ui.Success(fmt.Sprintf("Created %s: %s", t.Slug, t.Title))
			return nil
		},
	}

	cmd.Flags().StringVar(&storySlug, "story", "", "Story slug (e.g. S-1) to add the task to")
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
			if taskSlug == "" {
				return fmt.Errorf("--task required (e.g. --task T-1)")
			}
			title := args[0]

			d := mustOpenDB()
			defer d.Close()

			p, _ := mustProject(d)

			t, err := d.GetTaskBySlug(p.ID, taskSlug)
			if err != nil {
				return fmt.Errorf("task %q not found", taskSlug)
			}

			st, err := d.CreateSubtask(t.ID, title)
			if err != nil {
				return err
			}

			ui.Success(fmt.Sprintf("Created %s: %s", st.Slug, st.Title))
			return nil
		},
	}

	cmd.Flags().StringVar(&taskSlug, "task", "", "Task slug (e.g. T-1) to add the subtask to")
	_ = cmd.MarkFlagRequired("task")
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

			// Don't overwrite existing config
			if _, err := os.Stat(path); err == nil {
				ui.Warn(fmt.Sprintf("Config already exists at %s", path))
				ui.Info("Edit it directly or delete it to regenerate.")
				return nil
			}

			cfg := config.Default()
			if err := config.Save(cfg); err != nil {
				return err
			}

			ui.Success(fmt.Sprintf("Config created at %s", path))
			ui.Info("Set your API key: edit the file or set TASK_API_KEY=sk-...")
			ui.Info("Default provider: deepseek (change to pi or opencode for free usage via Copilot)")
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

			// Redact key
			display := *cfg
			if display.LLM.APIKey != "" {
				display.LLM.APIKey = display.LLM.APIKey[:min(8, len(display.LLM.APIKey))] + "…"
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

// ── helpers ───────────────────────────────────────────────────────────────────

func loadOrDefault() (*config.Config, error) {
	cfg, err := config.Load()
	if err == config.ErrNotFound {
		return config.Default(), nil
	}
	return cfg, err
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
