package export

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/joelkram/task-cli/internal/db"
)

// ToMarkdown writes all stories and their tasks as GitHub-style markdown
// to w. Each story becomes an h2 section; each task is a checklist item.
func ToMarkdown(w io.Writer, proj *db.Project, views []*db.StoryView) error {
	fmt.Fprintf(w, "# %s — Task Export\n\n", proj.Name)
	fmt.Fprintf(w, "_Generated: %s_\n\n---\n\n", time.Now().Format("2006-01-02 15:04"))

	for _, v := range views {
		s := v.Story
		doneIcon := "[ ]"
		if s.Status == "done" {
			doneIcon = "[x]"
		}
		fmt.Fprintf(w, "## %s %s — %s\n\n", doneIcon, s.Slug, s.Title)

		if s.Description != "" {
			fmt.Fprintf(w, "%s\n\n", s.Description)
		}

		// Acceptance criteria
		var criteria []string
		if err := json.Unmarshal([]byte(s.AcceptanceCriteria), &criteria); err == nil && len(criteria) > 0 {
			fmt.Fprintf(w, "**Acceptance Criteria**\n\n")
			for _, c := range criteria {
				fmt.Fprintf(w, "- %s\n", c)
			}
			fmt.Fprintln(w)
		}

		// Tasks
		if len(v.Tasks) > 0 {
			fmt.Fprintf(w, "**Tasks**\n\n")
			for _, t := range v.Tasks {
				check := checkBox(t.Status)
				fmt.Fprintf(w, "- %s `%s` %s\n", check, t.Slug, t.Title)
				for _, st := range v.Subtasks[t.ID] {
					fmt.Fprintf(w, "  - %s `%s` %s\n", checkBox(st.Status), st.Slug, st.Title)
				}
			}
			fmt.Fprintln(w)
		}

		fmt.Fprintln(w, "---")
		fmt.Fprintln(w)
	}
	return nil
}

// ToJSON writes a full structured dump of all stories to w.
func ToJSON(w io.Writer, proj *db.Project, views []*db.StoryView) error {
	type subtaskOut struct {
		Slug   string `json:"slug"`
		Title  string `json:"title"`
		Status string `json:"status"`
	}
	type taskOut struct {
		Slug     string       `json:"slug"`
		Title    string       `json:"title"`
		Status   string       `json:"status"`
		Subtasks []subtaskOut `json:"subtasks"`
	}
	type storyOut struct {
		Slug               string    `json:"slug"`
		Title              string    `json:"title"`
		Description        string    `json:"description"`
		AcceptanceCriteria []string  `json:"acceptance_criteria"`
		Status             string    `json:"status"`
		CreatedAt          time.Time `json:"created_at"`
		Tasks              []taskOut `json:"tasks"`
	}
	type output struct {
		Project   string     `json:"project"`
		ExportedAt time.Time `json:"exported_at"`
		Stories   []storyOut `json:"stories"`
	}

	out := output{
		Project:    proj.Name,
		ExportedAt: time.Now().UTC(),
	}

	for _, v := range views {
		var criteria []string
		_ = json.Unmarshal([]byte(v.Story.AcceptanceCriteria), &criteria)

		so := storyOut{
			Slug:               v.Story.Slug,
			Title:              v.Story.Title,
			Description:        v.Story.Description,
			AcceptanceCriteria: criteria,
			Status:             v.Story.Status,
			CreatedAt:          v.Story.CreatedAt,
		}
		for _, t := range v.Tasks {
			to := taskOut{Slug: t.Slug, Title: t.Title, Status: t.Status}
			for _, st := range v.Subtasks[t.ID] {
				to.Subtasks = append(to.Subtasks, subtaskOut{Slug: st.Slug, Title: st.Title, Status: st.Status})
			}
			so.Tasks = append(so.Tasks, to)
		}
		out.Stories = append(out.Stories, so)
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func checkBox(status string) string {
	if strings.EqualFold(status, "done") {
		return "[x]"
	}
	return "[ ]"
}
