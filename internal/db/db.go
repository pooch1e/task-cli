package db

import (
	"database/sql"
	_ "embed"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schema string

// SELECT column lists — defined once so schema changes are a single edit.
const (
	storySelectCols = "id, project_id, slug, title, description, acceptance_criteria, status, created_at"
	taskSelectCols  = "id, story_id, slug, title, status, created_at, updated_at"
)

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

	if err := os.Chmod(path, 0600); err != nil {
		log.Printf("warning: could not set db file permissions: %v", err)
	}

	return &DB{conn}, nil
}

// ── Projects ──────────────────────────────────────────────────────────────────

type Project struct {
	ID        int64
	Name      string
	Path      string
	CreatedAt time.Time
}

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
	AcceptanceCriteria string // JSON array stored as text
	Status             string
	CreatedAt          time.Time
}

// TaskInput describes a task and its subtitles for bulk story persistence.
type TaskInput struct {
	Title    string
	Subtasks []string
}

func (db *DB) CreateStory(projectID int64, title, description, acceptanceCriteria string) (*Story, error) {
	if title == "" {
		return nil, fmt.Errorf("story title cannot be empty")
	}
	slug, err := db.nextStorySlug(projectID)
	if err != nil {
		return nil, err
	}

	res, err := db.Exec(
		`INSERT INTO stories (project_id, slug, title, description, acceptance_criteria) VALUES (?, ?, ?, ?, ?)`,
		projectID, slug, title, description, acceptanceCriteria,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return db.GetStoryByID(id)
}

// nextStorySlug derives the next slug by finding the maximum existing numeric
// suffix. Using MAX rather than COUNT means deletion + re-insertion never
// produces a colliding slug.
func (db *DB) nextStorySlug(projectID int64) (string, error) {
	var maxN sql.NullInt64
	err := db.QueryRow(
		`SELECT MAX(CAST(REPLACE(slug, 'S-', '') AS INTEGER))
         FROM stories WHERE project_id = ?`, projectID,
	).Scan(&maxN)
	if err != nil {
		return "", err
	}
	n := int64(0)
	if maxN.Valid {
		n = maxN.Int64
	}
	return fmt.Sprintf("S-%d", n+1), nil
}

// PersistStory writes a story, its tasks, and subtasks in a single transaction.
// All inserts succeed or all are rolled back — no orphaned records.
func (db *DB) PersistStory(projectID int64, title, description, acJSON string, tasks []TaskInput) (*Story, error) {
	if title == "" {
		return nil, fmt.Errorf("story title cannot be empty")
	}

	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("starting transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck — Rollback is a no-op after Commit

	slug, err := db.nextStorySlug(projectID)
	if err != nil {
		return nil, err
	}

	res, err := tx.Exec(
		`INSERT INTO stories (project_id, slug, title, description, acceptance_criteria) VALUES (?, ?, ?, ?, ?)`,
		projectID, slug, title, description, acJSON,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting story: %w", err)
	}
	storyID, _ := res.LastInsertId()

	for _, t := range tasks {
		taskSlug, err := db.nextTaskSlugTx(tx, projectID)
		if err != nil {
			return nil, err
		}
		tres, err := tx.Exec(
			`INSERT INTO tasks (story_id, slug, title) VALUES (?, ?, ?)`,
			storyID, taskSlug, t.Title,
		)
		if err != nil {
			return nil, fmt.Errorf("inserting task: %w", err)
		}
		taskID, _ := tres.LastInsertId()

		for i, st := range t.Subtasks {
			stSlug := fmt.Sprintf("%s.%d", taskSlug, i+1)
			if _, err := tx.Exec(
				`INSERT INTO subtasks (task_id, slug, title) VALUES (?, ?, ?)`,
				taskID, stSlug, st,
			); err != nil {
				return nil, fmt.Errorf("inserting subtask: %w", err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing story: %w", err)
	}

	return db.GetStoryByID(storyID)
}

// nextTaskSlugTx derives the next task slug within an open transaction.
func (db *DB) nextTaskSlugTx(tx *sql.Tx, projectID int64) (string, error) {
	var maxN sql.NullInt64
	err := tx.QueryRow(
		`SELECT MAX(CAST(REPLACE(t.slug, 'T-', '') AS INTEGER))
         FROM tasks t
         JOIN stories s ON s.id = t.story_id
         WHERE s.project_id = ?`, projectID,
	).Scan(&maxN)
	if err != nil {
		return "", err
	}
	n := int64(0)
	if maxN.Valid {
		n = maxN.Int64
	}
	return fmt.Sprintf("T-%d", n+1), nil
}

func (db *DB) GetStoryByID(id int64) (*Story, error) {
	return scanStory(db.QueryRow(`SELECT `+storySelectCols+` FROM stories WHERE id = ?`, id))
}

func (db *DB) GetStoryBySlug(projectID int64, slug string) (*Story, error) {
	return scanStory(db.QueryRow(
		`SELECT `+storySelectCols+` FROM stories WHERE project_id = ? AND slug = ?`, projectID, slug,
	))
}

func (db *DB) ListStories(projectID int64) ([]*Story, error) {
	rows, err := db.Query(`SELECT `+storySelectCols+` FROM stories WHERE project_id = ? ORDER BY id`, projectID)
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
		`UPDATE stories SET status = ? WHERE project_id = ? AND slug = ?`, status, projectID, slug,
	)
	return err
}

func (db *DB) DeleteStory(projectID int64, slug string) error {
	_, err := db.Exec(
		`DELETE FROM stories WHERE project_id = ? AND slug = ?`, projectID, slug,
	)
	return err
}

func scanStory(s interface{ Scan(...any) error }) (*Story, error) {
	st := &Story{}
	err := s.Scan(&st.ID, &st.ProjectID, &st.Slug, &st.Title,
		&st.Description, &st.AcceptanceCriteria, &st.Status, &st.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("story not found")
	}
	return st, err
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
	if title == "" {
		return nil, fmt.Errorf("task title cannot be empty")
	}
	var projectID int64
	if err := db.QueryRow(`SELECT project_id FROM stories WHERE id = ?`, storyID).Scan(&projectID); err != nil {
		return nil, fmt.Errorf("finding story for task: %w", err)
	}

	slug, err := db.nextTaskSlugDirect(projectID)
	if err != nil {
		return nil, err
	}

	res, err := db.Exec(`INSERT INTO tasks (story_id, slug, title) VALUES (?, ?, ?)`, storyID, slug, title)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return db.GetTaskByID(id)
}

// nextTaskSlugDirect derives the next task slug using a direct DB connection.
func (db *DB) nextTaskSlugDirect(projectID int64) (string, error) {
	var maxN sql.NullInt64
	err := db.QueryRow(
		`SELECT MAX(CAST(REPLACE(t.slug, 'T-', '') AS INTEGER))
         FROM tasks t
         JOIN stories s ON s.id = t.story_id
         WHERE s.project_id = ?`, projectID,
	).Scan(&maxN)
	if err != nil {
		return "", err
	}
	n := int64(0)
	if maxN.Valid {
		n = maxN.Int64
	}
	return fmt.Sprintf("T-%d", n+1), nil
}

func (db *DB) GetTaskByID(id int64) (*Task, error) {
	return scanTask(db.QueryRow(`SELECT `+taskSelectCols+` FROM tasks WHERE id = ?`, id))
}

func (db *DB) GetTaskBySlug(projectID int64, slug string) (*Task, error) {
	return scanTask(db.QueryRow(
		`SELECT t.`+taskSelectCols+`
         FROM tasks t JOIN stories s ON s.id = t.story_id
         WHERE s.project_id = ? AND t.slug = ?`, projectID, slug,
	))
}

func (db *DB) ListTasksForStory(storyID int64) ([]*Task, error) {
	rows, err := db.Query(
		`SELECT `+taskSelectCols+` FROM tasks WHERE story_id = ? ORDER BY id`, storyID,
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
         WHERE slug = ? AND story_id IN (SELECT id FROM stories WHERE project_id = ?)`,
		status, slug, projectID,
	)
	return err
}

func (db *DB) DeleteTask(projectID int64, slug string) error {
	_, err := db.Exec(
		`DELETE FROM tasks WHERE slug = ? AND story_id IN (SELECT id FROM stories WHERE project_id = ?)`,
		slug, projectID,
	)
	return err
}

func scanTask(s interface{ Scan(...any) error }) (*Task, error) {
	t := &Task{}
	err := s.Scan(&t.ID, &t.StoryID, &t.Slug, &t.Title, &t.Status, &t.CreatedAt, &t.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("task not found")
	}
	return t, err
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

// CreateSubtask derives the next subtask slug in a single query that fetches
// both the existing count and the parent task's slug.
func (db *DB) CreateSubtask(taskID int64, title string) (*Subtask, error) {
	if title == "" {
		return nil, fmt.Errorf("subtask title cannot be empty")
	}

	var taskSlug string
	var count int
	err := db.QueryRow(
		`SELECT t.slug, COUNT(su.id)
         FROM tasks t
         LEFT JOIN subtasks su ON su.task_id = t.id
         WHERE t.id = ?
         GROUP BY t.id, t.slug`,
		taskID,
	).Scan(&taskSlug, &count)
	if err != nil {
		return nil, fmt.Errorf("finding task for subtask: %w", err)
	}

	slug := fmt.Sprintf("%s.%d", taskSlug, count+1)
	res, err := db.Exec(`INSERT INTO subtasks (task_id, slug, title) VALUES (?, ?, ?)`, taskID, slug, title)
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
		`UPDATE subtasks SET status = ? WHERE task_id = ? AND slug = ?`, status, taskID, slug,
	)
	return err
}

// ── Story Views ───────────────────────────────────────────────────────────────

// StoryView bundles a story with its tasks and subtasks for display and export.
type StoryView struct {
	Story    *Story
	Tasks    []*Task
	Subtasks map[int64][]*Subtask // keyed by task ID
}

func (db *DB) LoadStoryView(storyID int64) (*StoryView, error) {
	s, err := db.GetStoryByID(storyID)
	if err != nil {
		return nil, err
	}
	tasks, err := db.ListTasksForStory(storyID)
	if err != nil {
		return nil, err
	}
	subtasks := make(map[int64][]*Subtask, len(tasks))
	for _, t := range tasks {
		if subtasks[t.ID], err = db.ListSubtasksForTask(t.ID); err != nil {
			return nil, err
		}
	}
	return &StoryView{Story: s, Tasks: tasks, Subtasks: subtasks}, nil
}

func (db *DB) LoadProjectView(projectID int64) ([]*StoryView, error) {
	stories, err := db.ListStories(projectID)
	if err != nil {
		return nil, err
	}
	views := make([]*StoryView, 0, len(stories))
	for _, s := range stories {
		v, err := db.LoadStoryView(s.ID)
		if err != nil {
			return nil, err
		}
		views = append(views, v)
	}
	return views, nil
}

// ── Stats ─────────────────────────────────────────────────────────────────────

type ProjectStats struct {
	TotalStories  int
	DoneStories   int
	TotalTasks    int
	DoneTasks     int
	TotalSubtasks int
	DoneSubtasks  int
}

// GetProjectStats fetches all counts in a single SQL query.
// COUNT(DISTINCT CASE WHEN ...) handles the LEFT JOIN fanout correctly —
// plain SUM would over-count rows multiplied by the subtask join.
func (db *DB) GetProjectStats(projectID int64) (*ProjectStats, error) {
	s := &ProjectStats{}
	err := db.QueryRow(`
		SELECT
			COUNT(DISTINCT st.id),
			COUNT(DISTINCT CASE WHEN st.status = 'done' THEN st.id END),
			COUNT(DISTINCT t.id),
			COUNT(DISTINCT CASE WHEN t.status  = 'done' THEN t.id  END),
			COUNT(DISTINCT su.id),
			COUNT(DISTINCT CASE WHEN su.status = 'done' THEN su.id END)
		FROM stories st
		LEFT JOIN tasks    t  ON t.story_id = st.id
		LEFT JOIN subtasks su ON su.task_id = t.id
		WHERE st.project_id = ?`, projectID,
	).Scan(
		&s.TotalStories, &s.DoneStories,
		&s.TotalTasks, &s.DoneTasks,
		&s.TotalSubtasks, &s.DoneSubtasks,
	)
	return s, err
}
