package db

import (
	"database/sql"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schema string

// DB wraps sql.DB with task-specific helpers.
type DB struct {
	*sql.DB
}

// Open opens (or creates) the SQLite database at path, runs migrations, and
// sets file permissions to 0600.
func Open(path string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("creating db dir: %w", err)
	}

	conn, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, fmt.Errorf("opening db: %w", err)
	}

	conn.SetMaxOpenConns(1) // SQLite is single-writer

	if err := conn.Ping(); err != nil {
		return nil, fmt.Errorf("pinging db: %w", err)
	}

	if _, err := conn.Exec(schema); err != nil {
		return nil, fmt.Errorf("running schema: %w", err)
	}

	// Lock down file permissions after creation
	_ = os.Chmod(path, 0600)

	return &DB{conn}, nil
}

// ── Projects ──────────────────────────────────────────────────────────────────

type Project struct {
	ID        int64
	Name      string
	Path      string
	CreatedAt time.Time
}

// UpsertProject returns the project by name, creating it if it doesn't exist.
func (db *DB) UpsertProject(name, path string) (*Project, error) {
	_, err := db.Exec(
		`INSERT INTO projects (name, path) VALUES (?, ?)
         ON CONFLICT(name) DO UPDATE SET path = excluded.path`,
		name, path,
	)
	if err != nil {
		return nil, err
	}
	return db.GetProject(name)
}

func (db *DB) GetProject(name string) (*Project, error) {
	row := db.QueryRow(`SELECT id, name, path, created_at FROM projects WHERE name = ?`, name)
	p := &Project{}
	return p, row.Scan(&p.ID, &p.Name, &p.Path, &p.CreatedAt)
}

func (db *DB) ListProjects() ([]*Project, error) {
	rows, err := db.Query(`SELECT id, name, path, created_at FROM projects ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*Project
	for rows.Next() {
		p := &Project{}
		if err := rows.Scan(&p.ID, &p.Name, &p.Path, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ── Stories ───────────────────────────────────────────────────────────────────

type Story struct {
	ID                 int64
	ProjectID          int64
	Slug               string
	Title              string
	Description        string
	AcceptanceCriteria string // JSON array
	Status             string
	CreatedAt          time.Time
}

func (db *DB) CreateStory(projectID int64, title, description, acceptanceCriteria string) (*Story, error) {
	slug, err := db.nextStorySlug(projectID)
	if err != nil {
		return nil, err
	}

	res, err := db.Exec(
		`INSERT INTO stories (project_id, slug, title, description, acceptance_criteria)
         VALUES (?, ?, ?, ?, ?)`,
		projectID, slug, title, description, acceptanceCriteria,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return db.GetStoryByID(id)
}

func (db *DB) nextStorySlug(projectID int64) (string, error) {
	var count int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM stories WHERE project_id = ?`, projectID,
	).Scan(&count)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("S-%d", count+1), nil
}

func (db *DB) GetStoryByID(id int64) (*Story, error) {
	row := db.QueryRow(
		`SELECT id, project_id, slug, title, description, acceptance_criteria, status, created_at
         FROM stories WHERE id = ?`, id,
	)
	return scanStory(row)
}

func (db *DB) GetStoryBySlug(projectID int64, slug string) (*Story, error) {
	row := db.QueryRow(
		`SELECT id, project_id, slug, title, description, acceptance_criteria, status, created_at
         FROM stories WHERE project_id = ? AND slug = ?`, projectID, slug,
	)
	return scanStory(row)
}

func (db *DB) ListStories(projectID int64) ([]*Story, error) {
	rows, err := db.Query(
		`SELECT id, project_id, slug, title, description, acceptance_criteria, status, created_at
         FROM stories WHERE project_id = ? ORDER BY id`, projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*Story
	for rows.Next() {
		s, err := scanStory(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (db *DB) SetStoryStatus(projectID int64, slug, status string) error {
	_, err := db.Exec(
		`UPDATE stories SET status = ? WHERE project_id = ? AND slug = ?`,
		status, projectID, slug,
	)
	return err
}

func scanStory(s interface{ Scan(...any) error }) (*Story, error) {
	st := &Story{}
	return st, s.Scan(
		&st.ID, &st.ProjectID, &st.Slug, &st.Title,
		&st.Description, &st.AcceptanceCriteria, &st.Status, &st.CreatedAt,
	)
}

// ── Tasks ─────────────────────────────────────────────────────────────────────

type Task struct {
	ID        int64
	StoryID   int64
	Slug      string
	Title     string
	Status    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (db *DB) CreateTask(storyID int64, title string) (*Task, error) {
	slug, err := db.nextTaskSlug(storyID)
	if err != nil {
		return nil, err
	}

	res, err := db.Exec(
		`INSERT INTO tasks (story_id, slug, title) VALUES (?, ?, ?)`,
		storyID, slug, title,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return db.GetTaskByID(id)
}

func (db *DB) nextTaskSlug(storyID int64) (string, error) {
	// Task slugs are global per-project. Derive project from story.
	var projectID int64
	if err := db.QueryRow(`SELECT project_id FROM stories WHERE id = ?`, storyID).Scan(&projectID); err != nil {
		return "", err
	}
	var count int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM tasks t
         JOIN stories s ON s.id = t.story_id
         WHERE s.project_id = ?`, projectID,
	).Scan(&count)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("T-%d", count+1), nil
}

func (db *DB) GetTaskByID(id int64) (*Task, error) {
	row := db.QueryRow(
		`SELECT id, story_id, slug, title, status, created_at, updated_at FROM tasks WHERE id = ?`, id,
	)
	return scanTask(row)
}

func (db *DB) GetTaskBySlug(projectID int64, slug string) (*Task, error) {
	row := db.QueryRow(
		`SELECT t.id, t.story_id, t.slug, t.title, t.status, t.created_at, t.updated_at
         FROM tasks t
         JOIN stories s ON s.id = t.story_id
         WHERE s.project_id = ? AND t.slug = ?`, projectID, slug,
	)
	return scanTask(row)
}

func (db *DB) ListTasksForStory(storyID int64) ([]*Task, error) {
	rows, err := db.Query(
		`SELECT id, story_id, slug, title, status, created_at, updated_at
         FROM tasks WHERE story_id = ? ORDER BY id`, storyID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (db *DB) SetTaskStatus(projectID int64, slug, status string) error {
	_, err := db.Exec(
		`UPDATE tasks SET status = ?, updated_at = datetime('now')
         WHERE slug = ? AND story_id IN (
             SELECT id FROM stories WHERE project_id = ?
         )`,
		status, slug, projectID,
	)
	return err
}

func scanTask(s interface{ Scan(...any) error }) (*Task, error) {
	t := &Task{}
	return t, s.Scan(&t.ID, &t.StoryID, &t.Slug, &t.Title, &t.Status, &t.CreatedAt, &t.UpdatedAt)
}

// ── Subtasks ──────────────────────────────────────────────────────────────────

type Subtask struct {
	ID        int64
	TaskID    int64
	Slug      string
	Title     string
	Status    string
	CreatedAt time.Time
}

func (db *DB) CreateSubtask(taskID int64, title string) (*Subtask, error) {
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM subtasks WHERE task_id = ?`, taskID).Scan(&count); err != nil {
		return nil, err
	}

	// derive parent task slug to build e.g. T-1.3
	var taskSlug string
	if err := db.QueryRow(`SELECT slug FROM tasks WHERE id = ?`, taskID).Scan(&taskSlug); err != nil {
		return nil, err
	}
	slug := fmt.Sprintf("%s.%d", taskSlug, count+1)

	res, err := db.Exec(
		`INSERT INTO subtasks (task_id, slug, title) VALUES (?, ?, ?)`,
		taskID, slug, title,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()

	st := &Subtask{}
	return st, db.QueryRow(
		`SELECT id, task_id, slug, title, status, created_at FROM subtasks WHERE id = ?`, id,
	).Scan(&st.ID, &st.TaskID, &st.Slug, &st.Title, &st.Status, &st.CreatedAt)
}

func (db *DB) ListSubtasksForTask(taskID int64) ([]*Subtask, error) {
	rows, err := db.Query(
		`SELECT id, task_id, slug, title, status, created_at FROM subtasks WHERE task_id = ? ORDER BY id`, taskID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*Subtask
	for rows.Next() {
		st := &Subtask{}
		if err := rows.Scan(&st.ID, &st.TaskID, &st.Slug, &st.Title, &st.Status, &st.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

func (db *DB) SetSubtaskStatus(taskID int64, slug, status string) error {
	_, err := db.Exec(
		`UPDATE subtasks SET status = ? WHERE task_id = ? AND slug = ?`,
		status, taskID, slug,
	)
	return err
}

// ── Stats ─────────────────────────────────────────────────────────────────────

type ProjectStats struct {
	TotalStories   int
	DoneStories    int
	TotalTasks     int
	DoneTasks      int
	TotalSubtasks  int
	DoneSubtasks   int
}

func (db *DB) GetProjectStats(projectID int64) (*ProjectStats, error) {
	s := &ProjectStats{}

	db.QueryRow(`SELECT COUNT(*) FROM stories WHERE project_id = ?`, projectID).Scan(&s.TotalStories)
	db.QueryRow(`SELECT COUNT(*) FROM stories WHERE project_id = ? AND status = 'done'`, projectID).Scan(&s.DoneStories)
	db.QueryRow(`SELECT COUNT(*) FROM tasks t JOIN stories st ON st.id = t.story_id WHERE st.project_id = ?`, projectID).Scan(&s.TotalTasks)
	db.QueryRow(`SELECT COUNT(*) FROM tasks t JOIN stories st ON st.id = t.story_id WHERE st.project_id = ? AND t.status = 'done'`, projectID).Scan(&s.DoneTasks)
	db.QueryRow(`SELECT COUNT(*) FROM subtasks su JOIN tasks t ON t.id = su.task_id JOIN stories st ON st.id = t.story_id WHERE st.project_id = ?`, projectID).Scan(&s.TotalSubtasks)
	db.QueryRow(`SELECT COUNT(*) FROM subtasks su JOIN tasks t ON t.id = su.task_id JOIN stories st ON st.id = t.story_id WHERE st.project_id = ? AND su.status = 'done'`, projectID).Scan(&s.DoneSubtasks)

	return s, nil
}
