package obsidian

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/joelkram/task-cli/internal/db"
)

// ── fixtures ──────────────────────────────────────────────────────────────────

func makeStoryView() *db.StoryView {
	now := time.Date(2026, 5, 19, 0, 0, 0, 0, time.UTC)
	return &db.StoryView{
		Story: &db.Story{
			ID:                 1,
			Slug:               "S-1",
			Title:              "User can log in",
			Description:        "As a user, I want to log in so I can access my account.",
			AcceptanceCriteria: `["Given valid credentials, when I submit the form, then I am redirected to the dashboard"]`,
			Status:             "open",
			CreatedAt:          now,
		},
		Tasks: []*db.Task{
			{ID: 1, StoryID: 1, Slug: "T-1", Title: "Implement OAuth flow", Status: "done", CreatedAt: now, UpdatedAt: now},
			{ID: 2, StoryID: 1, Slug: "T-2", Title: "Write login tests", Status: "todo", CreatedAt: now, UpdatedAt: now},
		},
		Subtasks: map[int64][]*db.Subtask{
			1: {
				{ID: 1, TaskID: 1, Slug: "T-1.1", Title: "Register OAuth app", Status: "done"},
				{ID: 2, TaskID: 1, Slug: "T-1.2", Title: "Handle callback", Status: "done"},
			},
			2: nil,
		},
	}
}

// ── sanitizeName ──────────────────────────────────────────────────────────────

func TestSanitizeName(t *testing.T) {
	cases := []struct {
		input, want string
	}{
		{"Hello World", "Hello World"},
		{"foo/bar", "foobar"},
		{"foo:bar?baz", "foobarbaz"},
		{"  trim  ", "trim"},
		{"multiple   spaces", "multiple spaces"},
		{"[brackets]", "(brackets)"},
	}
	for _, c := range cases {
		got := sanitizeName(c.input)
		if got != c.want {
			t.Errorf("sanitizeName(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

// ── statusCheckbox ────────────────────────────────────────────────────────────

func TestStatusCheckbox(t *testing.T) {
	if statusCheckbox("done") != "x" {
		t.Error("done should be x")
	}
	if statusCheckbox("in-progress") != "/" {
		t.Error("in-progress should be /")
	}
	if statusCheckbox("todo") != " " {
		t.Error("todo should be space")
	}
}

// ── buildStoryFile ────────────────────────────────────────────────────────────

func TestBuildStoryFile(t *testing.T) {
	sv := makeStoryView()
	content := buildStoryFile(sv)

	checks := []string{
		"id: S-1",
		"status: open",
		"# S-1 User can log in",
		"## Description",
		"As a user",
		"## Acceptance Criteria",
		"Given valid credentials",
		"## Tasks",
		"- [x] [[S-1 User can log in/tasks/T-1 Implement OAuth flow]]",
		"- [ ] [[S-1 User can log in/tasks/T-2 Write login tests]]",
	}
	for _, want := range checks {
		if !strings.Contains(content, want) {
			t.Errorf("story.md missing %q\ncontent:\n%s", want, content)
		}
	}
}

// ── buildTaskFile ─────────────────────────────────────────────────────────────

func TestBuildTaskFile_WithSubtasks(t *testing.T) {
	sv := makeStoryView()
	task := sv.Tasks[0]
	subtasks := sv.Subtasks[task.ID]
	content := buildTaskFile(task, subtasks, "S-1 User can log in")

	checks := []string{
		"id: T-1",
		"status: done",
		"# T-1 Implement OAuth flow",
		"[[S-1 User can log in/story]]",
		"## Subtasks",
		"- [x] `T-1.1` Register OAuth app",
		"- [x] `T-1.2` Handle callback",
		"## Notes",
	}
	for _, want := range checks {
		if !strings.Contains(content, want) {
			t.Errorf("task file missing %q\ncontent:\n%s", want, content)
		}
	}
}

func TestBuildTaskFile_NoSubtasks(t *testing.T) {
	sv := makeStoryView()
	task := sv.Tasks[1]
	content := buildTaskFile(task, nil, "S-1 User can log in")

	if strings.Contains(content, "## Subtasks") {
		t.Error("task with no subtasks should not have Subtasks section")
	}
	if !strings.Contains(content, "## Notes") {
		t.Error("task file should always have Notes section")
	}
}

// ── Export (integration) ──────────────────────────────────────────────────────

func TestExport_WritesExpectedFiles(t *testing.T) {
	vault := t.TempDir()
	sv := makeStoryView()

	if err := Export([]*db.StoryView{sv}, vault, "my-project", Options{}); err != nil {
		t.Fatalf("Export: %v", err)
	}

	storyDir := filepath.Join(vault, "my-project", "S-1 User can log in")
	storyMD := filepath.Join(storyDir, "story.md")
	taskMD1 := filepath.Join(storyDir, "tasks", "T-1 Implement OAuth flow.md")
	taskMD2 := filepath.Join(storyDir, "tasks", "T-2 Write login tests.md")

	for _, path := range []string{storyMD, taskMD1, taskMD2} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected file missing: %s", path)
		}
	}
}

func TestExport_SkipsExistingWithoutForce(t *testing.T) {
	vault := t.TempDir()
	sv := makeStoryView()

	// First export
	if err := Export([]*db.StoryView{sv}, vault, "proj", Options{}); err != nil {
		t.Fatal(err)
	}

	// Corrupt story.md
	storyMD := filepath.Join(vault, "proj", "S-1 User can log in", "story.md")
	sentinel := "manual edit sentinel"
	if err := os.WriteFile(storyMD, []byte(sentinel), 0644); err != nil {
		t.Fatal(err)
	}

	// Second export without force — should not overwrite
	if err := Export([]*db.StoryView{sv}, vault, "proj", Options{Force: false}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(storyMD)
	if string(got) != sentinel {
		t.Error("Export without --force should not overwrite existing files")
	}
}

func TestExport_ForceOverwrites(t *testing.T) {
	vault := t.TempDir()
	sv := makeStoryView()

	if err := Export([]*db.StoryView{sv}, vault, "proj", Options{}); err != nil {
		t.Fatal(err)
	}

	storyMD := filepath.Join(vault, "proj", "S-1 User can log in", "story.md")
	if err := os.WriteFile(storyMD, []byte("corrupted"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := Export([]*db.StoryView{sv}, vault, "proj", Options{Force: true}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(storyMD)
	if string(got) == "corrupted" {
		t.Error("Export with --force should overwrite existing files")
	}
}

func TestExport_EmptyVaultPath(t *testing.T) {
	err := Export(nil, "", "proj", Options{})
	if err == nil {
		t.Error("expected error for empty vault path")
	}
}
