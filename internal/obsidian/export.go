// Package obsidian writes task-cli stories and tasks as interconnected
// Markdown files in an Obsidian vault.
//
// Layout:
//
//	<vault>/<project>/
//	  S-1 Story Title/
//	    story.md
//	    tasks/
//	      T-1 Task Title.md
//	      T-2 Task Title.md
//	  S-2 Another Story/
//	    story.md
//	    tasks/
//	      T-3 Task Title.md
package obsidian

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/joelkram/task-cli/internal/db"
)

// Options controls export behaviour.
type Options struct {
	// Force overwrites existing files. Without it, existing files are left
	// untouched so manual edits in Obsidian are preserved.
	Force bool
}

// Export writes all stories and their tasks to the vault at vaultPath under a
// subdirectory named after projectName.
func Export(stories []*db.StoryView, vaultPath, projectName string, opts Options) error {
	if vaultPath == "" {
		return fmt.Errorf("vault path is not set — run `task obsidian set-vault <path>` first")
	}

	projectDir := filepath.Join(vaultPath, sanitizeName(projectName))
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		return fmt.Errorf("creating project dir: %w", err)
	}

	for _, sv := range stories {
		if err := writeStory(projectDir, sv, opts); err != nil {
			return fmt.Errorf("writing story %s: %w", sv.Story.Slug, err)
		}
	}
	return nil
}

// writeStory creates the story directory, story.md, and tasks/*.md.
func writeStory(projectDir string, sv *db.StoryView, opts Options) error {
	dirName := fmt.Sprintf("%s %s", sv.Story.Slug, sanitizeName(sv.Story.Title))
	storyDir := filepath.Join(projectDir, dirName)
	tasksDir := filepath.Join(storyDir, "tasks")

	if err := os.MkdirAll(tasksDir, 0755); err != nil {
		return fmt.Errorf("creating tasks dir: %w", err)
	}

	// story.md
	storyPath := filepath.Join(storyDir, "story.md")
	if opts.Force || !fileExists(storyPath) {
		content := buildStoryFile(sv)
		if err := os.WriteFile(storyPath, []byte(content), 0644); err != nil {
			return fmt.Errorf("writing story.md: %w", err)
		}
	}

	// tasks/<slug> <title>.md
	storyDirName := dirName // used for backlinks
	for _, t := range sv.Tasks {
		subtasks := sv.Subtasks[t.ID]
		taskPath := filepath.Join(tasksDir, taskFileName(t))
		if opts.Force || !fileExists(taskPath) {
			content := buildTaskFile(t, subtasks, storyDirName)
			if err := os.WriteFile(taskPath, []byte(content), 0644); err != nil {
				return fmt.Errorf("writing task %s: %w", t.Slug, err)
			}
		}
	}
	return nil
}

// ── File content builders ─────────────────────────────────────────────────────

func buildStoryFile(sv *db.StoryView) string {
	s := sv.Story
	var b strings.Builder

	// Frontmatter
	b.WriteString("---\n")
	b.WriteString(fmt.Sprintf("id: %s\n", s.Slug))
	b.WriteString(fmt.Sprintf("status: %s\n", s.Status))
	b.WriteString(fmt.Sprintf("created: %s\n", s.CreatedAt.Format(time.DateOnly)))
	b.WriteString("tags:\n  - task-cli\n  - story\n")
	b.WriteString("---\n\n")

	// Title
	b.WriteString(fmt.Sprintf("# %s %s\n\n", s.Slug, s.Title))

	// Description
	if s.Description != "" {
		b.WriteString("## Description\n\n")
		b.WriteString(s.Description)
		b.WriteString("\n\n")
	}

	// Acceptance criteria
	var ac []string
	if s.AcceptanceCriteria != "" && s.AcceptanceCriteria != "null" {
		if err := json.Unmarshal([]byte(s.AcceptanceCriteria), &ac); err == nil && len(ac) > 0 {
			b.WriteString("## Acceptance Criteria\n\n")
			for _, criterion := range ac {
				b.WriteString(fmt.Sprintf("- %s\n", criterion))
			}
			b.WriteString("\n")
		}
	}

	// Tasks
	if len(sv.Tasks) > 0 {
		b.WriteString("## Tasks\n\n")
		for _, t := range sv.Tasks {
			check := statusCheckbox(t.Status)
			// Wikilink: [[S-1 Story Title/tasks/T-1 Task Title]]
			link := taskWikilink(sv.Story, t)
			b.WriteString(fmt.Sprintf("- [%s] %s\n", check, link))
		}
		b.WriteString("\n")
	}

	return b.String()
}

func buildTaskFile(t *db.Task, subtasks []*db.Subtask, storyDirName string) string {
	var b strings.Builder

	// Frontmatter
	b.WriteString("---\n")
	b.WriteString(fmt.Sprintf("id: %s\n", t.Slug))
	b.WriteString(fmt.Sprintf("status: %s\n", t.Status))
	b.WriteString(fmt.Sprintf("created: %s\n", t.CreatedAt.Format(time.DateOnly)))
	b.WriteString("tags:\n  - task-cli\n  - task\n")
	b.WriteString("---\n\n")

	// Title
	b.WriteString(fmt.Sprintf("# %s %s\n\n", t.Slug, t.Title))

	// Back-link to story
	b.WriteString("## Story\n\n")
	b.WriteString(fmt.Sprintf("[[%s/story]]\n\n", storyDirName))

	// Status
	b.WriteString(fmt.Sprintf("**Status:** %s\n\n", t.Status))

	// Subtasks inline
	if len(subtasks) > 0 {
		b.WriteString("## Subtasks\n\n")
		for _, st := range subtasks {
			check := statusCheckbox(st.Status)
			b.WriteString(fmt.Sprintf("- [%s] `%s` %s\n", check, st.Slug, st.Title))
		}
		b.WriteString("\n")
	}

	// Notes section for manual editing in Obsidian
	b.WriteString("## Notes\n\n")

	return b.String()
}

// ── Helpers ───────────────────────────────────────────────────────────────────

var unsafeChars = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]`)

// sanitizeName strips characters that are invalid in file/directory names
// across common operating systems and trims surrounding whitespace.
func sanitizeName(name string) string {
	name = unsafeChars.ReplaceAllString(name, "")
	// Collapse multiple spaces
	name = strings.Join(strings.FieldsFunc(name, unicode.IsSpace), " ")
	// Obsidian treats [] specially in file names — replace with parens
	name = strings.ReplaceAll(name, "[", "(")
	name = strings.ReplaceAll(name, "]", ")")
	return strings.TrimSpace(name)
}

// taskFileName returns the filename (without directory) for a task file.
func taskFileName(t *db.Task) string {
	return fmt.Sprintf("%s %s.md", t.Slug, sanitizeName(t.Title))
}

// taskWikilink builds the Obsidian wikilink from a story root dir to a task file.
// Format: [[S-1 Story Title/tasks/T-1 Task Title]]
func taskWikilink(s *db.Story, t *db.Task) string {
	storyDir := fmt.Sprintf("%s %s", s.Slug, sanitizeName(s.Title))
	taskName := fmt.Sprintf("%s %s", t.Slug, sanitizeName(t.Title))
	return fmt.Sprintf("[[%s/tasks/%s]]", storyDir, taskName)
}

// statusCheckbox maps a status string to a Markdown checkbox character.
func statusCheckbox(status string) string {
	switch status {
	case "done":
		return "x"
	case "in-progress":
		return "/"
	default:
		return " "
	}
}

// fileExists reports whether path exists as a regular file.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
