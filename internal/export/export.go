package export

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/joelkram/task-cli/internal/db"
)

// ── output types (package-level so they are testable) ─────────────────────────

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

type exportPayload struct {
	Project    string    `json:"project"`
	ExportedAt time.Time `json:"exported_at"`
	Stories    []storyOut `json:"stories"`
}

// ── public functions ──────────────────────────────────────────────────────────

// ToMarkdown writes all stories and their tasks as GitHub-style markdown to w.
func ToMarkdown(w io.Writer, proj *db.Project, views []*db.StoryView) error {
	fmt.Fprintf(w, "# %s — Task Export\n\n", proj.Name)
	fmt.Fprintf(w, "_Generated: %s_\n\n---\n\n", time.Now().Format("2006-01-02 15:04"))

	for _, v := range views {
		s := v.Story
		fmt.Fprintf(w, "## %s %s — %s\n\n", checkBox(s.Status), s.Slug, s.Title)
		if s.Description != "" {
			fmt.Fprintf(w, "%s\n\n", s.Description)
		}

		criteria := parseCriteria(s.AcceptanceCriteria)
		if len(criteria) > 0 {
			fmt.Fprintln(w, "**Acceptance Criteria**\n")
			for _, c := range criteria {
				fmt.Fprintf(w, "- %s\n", c)
			}
			fmt.Fprintln(w)
		}

		if len(v.Tasks) > 0 {
			fmt.Fprintln(w, "**Tasks**\n")
			for _, t := range v.Tasks {
				fmt.Fprintf(w, "- %s `%s` %s\n", checkBox(t.Status), t.Slug, t.Title)
				for _, st := range v.Subtasks[t.ID] {
					fmt.Fprintf(w, "  - %s `%s` %s\n", checkBox(st.Status), st.Slug, st.Title)
				}
			}
			fmt.Fprintln(w)
		}

		fmt.Fprintln(w, "---\n")
	}
	return nil
}

// ToJSON writes a full structured dump of all stories to w.
func ToJSON(w io.Writer, proj *db.Project, views []*db.StoryView) error {
	payload := buildPayload(proj, views)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}

// ── private helpers ───────────────────────────────────────────────────────────

// buildPayload converts DB views into the serialisable export structure.
func buildPayload(proj *db.Project, views []*db.StoryView) exportPayload {
	out := exportPayload{
		Project:    proj.Name,
		ExportedAt: time.Now().UTC(),
	}
	for _, v := range views {
		so := storyOut{
			Slug:               v.Story.Slug,
			Title:              v.Story.Title,
			Description:        v.Story.Description,
			AcceptanceCriteria: parseCriteria(v.Story.AcceptanceCriteria),
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
	return out
}

// parseCriteria unmarshals a JSON acceptance-criteria string. Returns nil (not
// an empty slice) on empty or invalid input so callers can check len() safely.
func parseCriteria(raw string) []string {
	if raw == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

func checkBox(status string) string {
	if strings.EqualFold(status, "done") {
		return "[x]"
	}
	return "[ ]"
}
