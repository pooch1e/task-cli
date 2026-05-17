package ui

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/joelkram/task-cli/internal/db"
)

// ANSI colour helpers — no external dependency.
const (
	reset  = "\033[0m"
	bold   = "\033[1m"
	dim    = "\033[2m"
	green  = "\033[32m"
	yellow = "\033[33m"
	cyan   = "\033[36m"
	red    = "\033[31m"
	grey   = "\033[90m"
	blue   = "\033[34m"
)

func color(c, s string) string { return c + s + reset }
func b(s string) string        { return color(bold, s) }
func dimS(s string) string     { return color(dim+grey, s) }

func statusIcon(status string) string {
	switch status {
	case "done":
		return color(green, "✓")
	case "in-progress":
		return color(yellow, "◎")
	case "archived":
		return color(grey, "✕")
	default:
		return color(grey, "○")
	}
}

// PrintProject prints the project header.
func PrintProject(name, path string) {
	fmt.Printf("\n%s  %s\n", b(color(cyan, "◆ "+name)), dimS(path))
	fmt.Println(dimS(strings.Repeat("─", 60)))
}

// PrintStory prints a story and all its tasks/subtasks from a StoryView.
func PrintStory(v *db.StoryView) {
	s := v.Story
	fmt.Printf("\n  %s  %s  %s\n", statusIcon(s.Status), b(color(blue, s.Slug)), b(s.Title))
	if s.Description != "" {
		fmt.Printf("       %s\n", dimS(s.Description))
	}
	if len(v.Tasks) == 0 {
		fmt.Printf("       %s\n", dimS("no tasks yet"))
		return
	}
	for _, t := range v.Tasks {
		printTask(t, v.Subtasks[t.ID])
	}
}

func printTask(t *db.Task, subtasks []*db.Subtask) {
	fmt.Printf("       %s  %s  %s\n", statusIcon(t.Status), color(yellow, t.Slug), t.Title)
	for _, st := range subtasks {
		fmt.Printf("              %s  %s  %s\n",
			statusIcon(st.Status), color(grey, st.Slug), dimS(st.Title))
	}
}

// PrintAcceptanceCriteria prints a story's acceptance criteria from JSON.
func PrintAcceptanceCriteria(raw string) {
	if raw == "" {
		return
	}
	var criteria []string
	if err := json.Unmarshal([]byte(raw), &criteria); err != nil || len(criteria) == 0 {
		return
	}
	fmt.Println()
	fmt.Println("  Acceptance criteria:")
	for _, c := range criteria {
		fmt.Printf("    · %s\n", c)
	}
}

// PrintStats prints a project progress summary with visual bars.
func PrintStats(stats *db.ProjectStats) {
	fmt.Println()
	printBar("Stories ", stats.DoneStories, stats.TotalStories, green)
	printBar("Tasks   ", stats.DoneTasks, stats.TotalTasks, yellow)
	if stats.TotalSubtasks > 0 {
		printBar("Subtasks", stats.DoneSubtasks, stats.TotalSubtasks, cyan)
	}
	fmt.Println()
}

func printBar(label string, done, total int, clr string) {
	if total == 0 {
		fmt.Printf("  %s  %s\n", b(label), dimS("none"))
		return
	}
	const width = 20
	filled := (done * width) / total
	bar := color(clr, strings.Repeat("█", filled)) + dimS(strings.Repeat("░", width-filled))
	pct := (done * 100) / total
	fmt.Printf("  %s  [%s] %s%d%%%s %s\n",
		b(label), bar, bold, pct, reset, dimS(fmt.Sprintf("%d/%d", done, total)))
}

// Success / Error / Info / Warn are logging helpers used by commands.
func Success(msg string) { fmt.Printf("%s %s\n", color(green, "✓"), msg) }
func Error(msg string)   { fmt.Printf("%s %s\n", color(red, "✗"), msg) }
func Info(msg string)    { fmt.Printf("%s %s\n", color(cyan, "ℹ"), msg) }
func Warn(msg string)    { fmt.Printf("%s %s\n", color(yellow, "⚠"), msg) }
