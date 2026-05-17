package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/joelkram/task-cli/internal/db"
)

const progressBarWidth = 20

// ANSI colour codes — suppressed automatically on non-TTY stdout.
const (
	ansiReset  = "\033[0m"
	ansiBold   = "\033[1m"
	ansiDim    = "\033[2m"
	ansiGreen  = "\033[32m"
	ansiYellow = "\033[33m"
	ansiCyan   = "\033[36m"
	ansiRed    = "\033[31m"
	ansiGrey   = "\033[90m"
	ansiBlue   = "\033[34m"
)

// IsTTY reports whether stdout is an interactive terminal.
func IsTTY() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func color(code, s string) string {
	if !IsTTY() {
		return s
	}
	return code + s + ansiReset
}

// bold wraps s in ANSI bold.
func bold(s string) string { return color(ansiBold, s) }

// dimGrey wraps s in ANSI dim+grey.
func dimGrey(s string) string { return color(ansiDim+ansiGrey, s) }

// Dimmed is the exported form of dimGrey for use outside this package.
func Dimmed(s string) string { return dimGrey(s) }

func statusIcon(status string) string {
	switch status {
	case "done":
		return color(ansiGreen, "✓")
	case "in-progress":
		return color(ansiYellow, "◎")
	case "archived":
		return color(ansiGrey, "✕")
	default:
		return color(ansiGrey, "○")
	}
}

// PrintProject prints the project header.
func PrintProject(name, path string) {
	fmt.Printf("\n%s  %s\n", bold(color(ansiCyan, "◆ "+name)), dimGrey(path))
	fmt.Println(dimGrey(strings.Repeat("─", 60)))
}

// PrintStory prints a story and all its tasks/subtasks from a StoryView.
func PrintStory(v *db.StoryView) {
	s := v.Story
	fmt.Printf("\n  %s  %s  %s\n", statusIcon(s.Status), bold(color(ansiBlue, s.Slug)), bold(s.Title))
	if s.Description != "" {
		fmt.Printf("       %s\n", dimGrey(s.Description))
	}
	if len(v.Tasks) == 0 {
		fmt.Printf("       %s\n", dimGrey("no tasks yet"))
		return
	}
	for _, t := range v.Tasks {
		printTask(t, v.Subtasks[t.ID])
	}
}

func printTask(t *db.Task, subtasks []*db.Subtask) {
	fmt.Printf("       %s  %s  %s\n", statusIcon(t.Status), color(ansiYellow, t.Slug), t.Title)
	for _, st := range subtasks {
		fmt.Printf("              %s  %s  %s\n",
			statusIcon(st.Status), color(ansiGrey, st.Slug), dimGrey(st.Title))
	}
}

// PrintAcceptanceCriteria prints a story's acceptance criteria from a JSON string.
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
	printBar("Stories ", stats.DoneStories, stats.TotalStories, ansiGreen)
	printBar("Tasks   ", stats.DoneTasks, stats.TotalTasks, ansiYellow)
	if stats.TotalSubtasks > 0 {
		printBar("Subtasks", stats.DoneSubtasks, stats.TotalSubtasks, ansiCyan)
	}
	fmt.Println()
}

func printBar(label string, done, total int, clr string) {
	if total == 0 {
		fmt.Printf("  %s  %s\n", bold(label), dimGrey("none"))
		return
	}
	if done > total {
		done = total // guard: prevent negative bar width from integer quirks
	}
	filled := (done * progressBarWidth) / total
	bar := color(clr, strings.Repeat("█", filled)) +
		dimGrey(strings.Repeat("░", progressBarWidth-filled))
	pct := (done * 100) / total
	fmt.Printf("  %s  [%s] %s%d%%%s %s\n",
		bold(label), bar,
		ansiBold, pct, ansiReset,
		dimGrey(fmt.Sprintf("%d/%d", done, total)),
	)
}

// Success / Error / Info / Warn are command output helpers.
func Success(msg string) { fmt.Printf("%s %s\n", color(ansiGreen, "✓"), msg) }
func Error(msg string)   { fmt.Printf("%s %s\n", color(ansiRed, "✗"), msg) }
func Info(msg string)    { fmt.Printf("%s %s\n", color(ansiCyan, "ℹ"), msg) }
func Warn(msg string)    { fmt.Printf("%s %s\n", color(ansiYellow, "⚠"), msg) }
