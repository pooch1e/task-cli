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

// SELECT column lists — single source of truth for column order.
const (
	storySelectCols = "id, project_id, slug, title, description, acceptance_criteria, status, created_at"
	taskSelectCols  = "id, story_id, slug, title, status, created_at, updated_at"
)

// DB wraps a SQLite connection. The internal conn is not embedded to prevent
// callers from bypassing the custom query helpers and their error handling.
type DB struct {
	conn *sql.DB
}

// Close releases the database connection.
func (db *DB) Close() error { return db.conn.Close() }

// Begin starts a transaction. Exposed so callers can use db.Begin() when they
// need multi-step atomic operations not covered by the helper methods.
func (db *DB) Begin() (*sql.Tx, error) { return db.conn.Begin() }

// Open opens (or creates) the SQLite database at path, runs schema migrations,
// and sets file permissions to 0600.
func Open(path string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("creating db dir: %w", err)
	}

	conn, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, fmt.Errorf("opening db: %w", err)
	}

	conn.SetMaxOpenConns(1) // SQLite: single writer

	if err := conn.Ping(); err != nil {
		return nil, fmt.Errorf("pinging db: %w", err)
	}

	if _, err := conn.Exec(schema); err != nil {
		return nil, fmt.Errorf("running schema: %w", err)
	}

	if err := os.Chmod(path, 0600); err != nil {
		log.Printf("warning: could not set db file permissions: %v", err)
	}

	return &DB{conn: conn}, nil
}

// ── Projects ──────────────────────────────────────────────────────────────────

type Project struct {
	ID        int64
	Name      string
	Path      string
	CreatedAt time.Time
}

func (db *DB) UpsertProject(name, path string) (*Project, error) {
	_, err := db.conn.Exec(
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
	p := &Project{}
	err := db.conn.QueryRow(
		`SELECT id, name, path, created_at FROM projects WHERE name = ?`, name,
	).Scan(&p.ID, &p.Name, &p.Path, &p.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("project %q not found", name)
	}
	return p, err
}

func (db *DB) ListProjects() ([]*Project, error) {
	rows, err := db.conn.Query(`SELECT id, name, path, created_at FROM projects ORDER BY name`)
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

// ── Slug generation ───────────────────────────────────────────────────────────

// nextSlug derives the next available slug for a project-scoped sequence.
// It uses MAX(numeric suffix) so deletion never produces collisions.
// Accepts an optional *sql.Tx; pass nil to use the main connection.
//
//   prefix: "S" → "S-1", "S-2" …
//   table:  "stories" or "tasks"
//   joinSQL: optional JOIN clause to filter by project (for tasks)
func (db *DB) nextSlug(tx execer, prefix, table, slugCol string, projectID int64) (string, error) {
	// Tasks are spread across stories, so we join via stories to scope by project.
	// Stories are directly project-scoped.
	var query string
	if table == "stories" {
		query = fmt.Sprintf(
			`SELECT MAX(CAST(REPLACE(%s, '%s-', '') AS INTEGER))
             FROM %s WHERE project_id = ?`, slugCol, prefix, table)
	} else {
		// tasks: join through stories
		query = fmt.Sprintf(
			`SELECT MAX(CAST(REPLACE(t.%s, '%s-', '') AS INTEGER))
             FROM %s t JOIN stories s ON s.id = t.story_id
             WHERE s.project_id = ?`, slugCol, prefix, table)
	}

	var maxN sql.NullInt64
	if err := tx.QueryRow(query, projectID).Scan(&maxN); err != nil {
		return "", fmt.Errorf("computing next %s slug: %w", prefix, err)
	}
	n := int64(0)
	if maxN.Valid {
		n = maxN.Int64
	}
	return fmt.Sprintf("%s-%d", prefix, n+1), nil
}

// execer abstracts *sql.DB and *sql.Tx so nextSlug works with both.
type execer interface {
	QueryRow(query string, args ...any) *sql.Row
	Exec(query string, args ...any) (sql.Result, error)
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

// TaskInput describes a task and its subtasks for bulk story persistence.
type TaskInput struct {
	Title    string
	Subtasks []string
}

func (db *DB) CreateStory(projectID int64, title, description, acceptanceCriteria string) (*Story, error) {
	if title == "" {
		return nil, fmt.Errorf("story title cannot be empty")
	}
	slug, err := db.nextSlug(db.conn, "S", "stories", "slug", projectID)
	if err != nil {
		return nil, err
	}
	res, err := db.conn.Exec(
		`INSERT INTO stories (project_id, slug, title, description, acceptance_criteria) VALUES (?, ?, ?, ?, ?)`,
		projectID, slug, title, description, acceptanceCriteria,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return db.GetStoryByID(id)
}

// PersistStory writes a story, its tasks, and subtasks in a single transaction.
func (db *DB) PersistStory(projectID int64, title, description, acJSON string, tasks []TaskInput) (*Story, error) {
	if title == "" {
		return nil, fmt.Errorf("story title cannot be empty")
	}
	tx, err := db.conn.Begin()
	if err != nil {
		return nil, fmt.Errorf("starting transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	storySlug, err := db.nextSlug(tx, "S", "stories", "slug", projectID)
	if err != nil {
		return nil, err
	}
	res, err := tx.Exec(
		`INSERT INTO stories (project_id, slug, title, description, acceptance_criteria) VALUES (?, ?, ?, ?, ?)`,
		projectID, storySlug, title, description, acJSON,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting story: %w", err)
	}
	storyID, _ := res.LastInsertId()

	for _, t := range tasks {
		taskSlug, err := db.nextSlug(tx, "T", "tasks", "slug", projectID)
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

func (db *DB) GetStoryByID(id int64) (*Story, error) {
	s, err := scanStory(db.conn.QueryRow(`SELECT `+storySelectCols+` FROM stories WHERE id = ?`, id))
	if err != nil {
		return nil, fmt.Errorf("story id=%d: %w", id, err)
	}
	return s, nil
}

func (db *DB) GetStoryBySlug(projectID int64, slug string) (*Story, error) {
	s, err := scanStory(db.conn.QueryRow(
		`SELECT `+storySelectCols+` FROM stories WHERE project_id = ? AND slug = ?`, projectID, slug,
	))
	if err != nil {
		return nil, fmt.Errorf("story %q: %w", slug, err)
	}
	return s, nil
}

func (db *DB) ListStories(projectID int64) ([]*Story, error) {
	rows, err := db.conn.Query(
		`SELECT `+storySelectCols+` FROM stories WHERE project_id = ? ORDER BY id`, projectID,
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
	res, err := db.conn.Exec(
		`UPDATE stories SET status = ? WHERE project_id = ? AND slug = ?`, status, projectID, slug,
	)
	if err != nil {
		return err
	}
	return requireOneRow(res, "story", slug)
}

func (db *DB) DeleteStory(projectID int64, slug string) error {
	res, err := db.conn.Exec(
		`DELETE FROM stories WHERE project_id = ? AND slug = ?`, projectID, slug,
	)
	if err != nil {
		return err
	}
	return requireOneRow(res, "story", slug)
}

func scanStory(s interface{ Scan(...any) error }) (*Story, error) {
	st := &Story{}
	err := s.Scan(&st.ID, &st.ProjectID, &st.Slug, &st.Title,
		&st.Description, &st.AcceptanceCriteria, &st.Status, &st.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("not found")
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

// CreateTask creates a task under storyID within projectID.
// projectID is passed explicitly to avoid an extra lookup for slug generation.
func (db *DB) CreateTask(projectID, storyID int64, title string) (*Task, error) {
	if title == "" {
		return nil, fmt.Errorf("task title cannot be empty")
	}
	slug, err := db.nextSlug(db.conn, "T", "tasks", "slug", projectID)
	if err != nil {
		return nil, err
	}
	res, err := db.conn.Exec(`INSERT INTO tasks (story_id, slug, title) VALUES (?, ?, ?)`, storyID, slug, title)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return db.GetTaskByID(id)
}

func (db *DB) GetTaskByID(id int64) (*Task, error) {
	t, err := scanTask(db.conn.QueryRow(`SELECT `+taskSelectCols+` FROM tasks WHERE id = ?`, id))
	if err != nil {
		return nil, fmt.Errorf("task id=%d: %w", id, err)
	}
	return t, nil
}

func (db *DB) GetTaskBySlug(projectID int64, slug string) (*Task, error) {
	t, err := scanTask(db.conn.QueryRow(
		`SELECT t.`+taskSelectCols+`
         FROM tasks t JOIN stories s ON s.id = t.story_id
         WHERE s.project_id = ? AND t.slug = ?`, projectID, slug,
	))
	if err != nil {
		return nil, fmt.Errorf("task %q: %w", slug, err)
	}
	return t, nil
}

func (db *DB) ListTasksForStory(storyID int64) ([]*Task, error) {
	rows, err := db.conn.Query(
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
	res, err := db.conn.Exec(
		`UPDATE tasks SET status = ?, updated_at = datetime('now')
         WHERE slug = ? AND story_id IN (SELECT id FROM stories WHERE project_id = ?)`,
		status, slug, projectID,
	)
	if err != nil {
		return err
	}
	return requireOneRow(res, "task", slug)
}

func (db *DB) DeleteTask(projectID int64, slug string) error {
	res, err := db.conn.Exec(
		`DELETE FROM tasks WHERE slug = ? AND story_id IN (SELECT id FROM stories WHERE project_id = ?)`,
		slug, projectID,
	)
	if err != nil {
		return err
	}
	return requireOneRow(res, "task", slug)
}

func scanTask(s interface{ Scan(...any) error }) (*Task, error) {
	t := &Task{}
	err := s.Scan(&t.ID, &t.StoryID, &t.Slug, &t.Title, &t.Status, &t.CreatedAt, &t.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("not found")
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

// CreateSubtask derives the next subtask slug in a single query.
func (db *DB) CreateSubtask(taskID int64, title string) (*Subtask, error) {
	if title == "" {
		return nil, fmt.Errorf("subtask title cannot be empty")
	}
	var taskSlug string
	var count int
	err := db.conn.QueryRow(
		`SELECT t.slug, COUNT(su.id)
         FROM tasks t LEFT JOIN subtasks su ON su.task_id = t.id
         WHERE t.id = ? GROUP BY t.id, t.slug`, taskID,
	).Scan(&taskSlug, &count)
	if err != nil {
		return nil, fmt.Errorf("finding task for subtask: %w", err)
	}

	slug := fmt.Sprintf("%s.%d", taskSlug, count+1)
	res, err := db.conn.Exec(`INSERT INTO subtasks (task_id, slug, title) VALUES (?, ?, ?)`, taskID, slug, title)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()

	st := &Subtask{}
	return st, db.conn.QueryRow(
		`SELECT id, task_id, slug, title, status, created_at FROM subtasks WHERE id = ?`, id,
	).Scan(&st.ID, &st.TaskID, &st.Slug, &st.Title, &st.Status, &st.CreatedAt)
}

func (db *DB) ListSubtasksForTask(taskID int64) ([]*Subtask, error) {
	rows, err := db.conn.Query(
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
	_, err := db.conn.Exec(
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
// COUNT(DISTINCT CASE WHEN …) handles the LEFT JOIN fanout correctly.
func (db *DB) GetProjectStats(projectID int64) (*ProjectStats, error) {
	s := &ProjectStats{}
	err := db.conn.QueryRow(`
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

// ── helpers ───────────────────────────────────────────────────────────────────

// requireOneRow returns an error if the result affected zero rows.
func requireOneRow(res sql.Result, kind, slug string) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%s %q not found", kind, slug)
	}
	return nil
}
