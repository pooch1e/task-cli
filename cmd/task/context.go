package main

import (
	"fmt"

	"github.com/joelkram/task-cli/internal/config"
	"github.com/joelkram/task-cli/internal/db"
	"github.com/joelkram/task-cli/internal/project"
)

// AppContext holds the shared state every command needs.
// Create with openAppContext; always defer Close.
// Project.Path holds the detected repo root — no separate Root field.
type AppContext struct {
	Config  *config.Config
	DB      *db.DB
	Project *db.Project
}

// openAppContext loads config, opens the database, and detects the current
// project. Returns an error rather than calling os.Exit so cobra surfaces it.
func openAppContext() (*AppContext, error) {
	cfg, err := config.LoadOrDefault()
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}

	d, err := db.Open(cfg.Storage.DBPath)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	name, root, err := project.Detect()
	if err != nil {
		d.Close()
		return nil, fmt.Errorf("detecting project: %w", err)
	}

	p, err := d.UpsertProject(name, root)
	if err != nil {
		d.Close()
		return nil, fmt.Errorf("upserting project %q: %w", name, err)
	}

	return &AppContext{Config: cfg, DB: d, Project: p}, nil
}

// Close releases the database connection.
func (a *AppContext) Close() {
	if a.DB != nil {
		a.DB.Close()
	}
}
