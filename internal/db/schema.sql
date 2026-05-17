-- projects: one row per tracked repo / named project
CREATE TABLE IF NOT EXISTS projects (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT    NOT NULL UNIQUE,
    path       TEXT,
    created_at DATETIME DEFAULT (datetime('now'))
);

-- stories: user stories, scoped to a project (slug S-1, S-2 … per project)
CREATE TABLE IF NOT EXISTS stories (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id          INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    slug                TEXT    NOT NULL,        -- "S-1"
    title               TEXT    NOT NULL,
    description         TEXT,
    acceptance_criteria TEXT,                    -- JSON array
    status              TEXT    NOT NULL DEFAULT 'open',  -- open | done | archived
    created_at          DATETIME DEFAULT (datetime('now'))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_stories_project_slug ON stories(project_id, slug);

-- tasks: implementation tasks under a story (slug T-1, T-2 … per project)
CREATE TABLE IF NOT EXISTS tasks (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    story_id   INTEGER NOT NULL REFERENCES stories(id) ON DELETE CASCADE,
    slug       TEXT    NOT NULL,                 -- "T-1"
    title      TEXT    NOT NULL,
    status     TEXT    NOT NULL DEFAULT 'todo',  -- todo | in-progress | done
    created_at DATETIME DEFAULT (datetime('now')),
    updated_at DATETIME DEFAULT (datetime('now'))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_tasks_story_slug ON tasks(story_id, slug);

-- subtasks: granular steps under a task (slug T-1.1, T-1.2 … per task)
CREATE TABLE IF NOT EXISTS subtasks (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id    INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    slug       TEXT    NOT NULL,                 -- "T-1.1"
    title      TEXT    NOT NULL,
    status     TEXT    NOT NULL DEFAULT 'todo',
    created_at DATETIME DEFAULT (datetime('now'))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_subtasks_task_slug ON subtasks(task_id, slug);
