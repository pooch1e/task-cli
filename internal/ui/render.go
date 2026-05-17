package ui

import (
	"fmt"
	"strings"

	"github.com/joelkram/task-cli/internal/db"
)

// ANSI colour helpers — no external dependency needed.
const (
	reset  = "\033[0m"
	bold   = "\033[1m"
	dim    = "\033[2m"
	green  = "\033[32m"
	yellow = "\033[33m"
	cyan   = "\033[36m"
	red    = "\033[31m"
	blue   = "\033[34m"
	grey   = "\033[90m"
)

func color(c, s string) string { return c + s + reset }
func b(s string) string        { return color(bold, s) }
func dimS(s string) string     { return color(dim+grey, s) }

// statusIcon returns a coloured icon for a status string.
func statusIcon(status string) string {
	switch status {
	case "done":
		return color(green, "✓")
	case "in-progress":
		return color(yellow, "◎")
	case "archived":
		return color(grey, "✕")
	default: // todo / open
		return color(grey, "○")
	}
}

// PrintProject prints the project header line.
func PrintProject(name, path string) {
	fmt.Printf("\n%s  %s\n", b(color(cyan, "◆ "+name)), dimS(path))
	fmt.Println(dimS(strings.Repeat("─", 60)))
}

// PrintStory prints a story row with its tasks and subtasks.
func PrintStory(story *db.Story, tasks []*db.Task, subtaskMap map[int64][]*db.Subtask) {
	icon := statusIcon(story.Status)
	fmt.Printf("\n  %s  %s  %s\n",
		icon,
		b(color(blue, story.Slug)),
		b(story.Title),
	)
	if story.Description != "" {
		fmt.Printf("       %s\n", dimS(story.Description))
	}

	for _, t := range tasks {
		PrintTask(t, subtaskMap[t.ID])
	}

	if len(tasks) == 0 {
		fmt.Printf("       %s\n", dimS("no tasks yet"))
	}
}

// PrintTask prints a task row with its subtasks.
func PrintTask(task *db.Task, subtasks []*db.Subtask) {
	icon := statusIcon(task.Status)
	fmt.Printf("       %s  %s  %s\n",
		icon,
		color(yellow, task.Slug),
		task.Title,
	)
	for _, st := range subtasks {
		PrintSubtask(st)
	}
}

// PrintSubtask prints a subtask row.
func PrintSubtask(st *db.Subtask) {
	icon := statusIcon(st.Status)
	fmt.Printf("              %s  %s  %s\n",
		icon,
		color(grey, st.Slug),
		dimS(st.Title),
	)
}

// PrintStats prints a project progress summary with a bar.
func PrintStats(stats *db.ProjectStats) {
	fmt.Println()
	printBar("Stories", stats.DoneStories, stats.TotalStories, green)
	printBar("Tasks  ", stats.DoneTasks, stats.TotalTasks, yellow)
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
	width := 20
	filled := 0
	if total > 0 {
		filled = (done * width) / total
	}
	bar := color(clr, strings.Repeat("█", filled)) + dimS(strings.Repeat("░", width-filled))
	pct := 0
	if total > 0 {
		pct = (done * 100) / total
	}
	fmt.Printf("  %s  [%s] %s%d%%%s %s\n",
		b(label), bar,
		bold, pct, reset,
		dimS(fmt.Sprintf("%d/%d", done, total)),
	)
}

// Success / Error / Info helpers used by commands.
func Success(msg string) { fmt.Printf("%s %s\n", color(green, "✓"), msg) }
func Error(msg string)   { fmt.Printf("%s %s\n", color(red, "✗"), msg) }
func Info(msg string)    { fmt.Printf("%s %s\n", color(cyan, "ℹ"), msg) }
func Warn(msg string)    { fmt.Printf("%s %s\n", color(yellow, "⚠"), msg) }
