package main

import (
	"fmt"
	"os"

	"github.com/joelkram/task-cli/internal/config"
	"github.com/joelkram/task-cli/internal/db"
	"github.com/joelkram/task-cli/internal/project"
	"github.com/joelkram/task-cli/internal/ui"
)

// AppContext holds the shared state every command needs: config, database,
// and the current project. Create with openAppContext; close with Close.
type AppContext struct {
	Config  *config.Config
	DB      *db.DB
	Project *db.Project
	Root    string
}

// openAppContext loads config, opens the database, and detects the current project.
func openAppContext() (*AppContext, error) {
	cfg, err := config.LoadOrDefault()
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}

	d, err := db.Open(cfg.Storage.DBPath)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	name, root := project.Detect()
	p, err := d.UpsertProject(name, root)
	if err != nil {
		d.Close()
		return nil, fmt.Errorf("detecting project: %w", err)
	}

	return &AppContext{Config: cfg, DB: d, Project: p, Root: root}, nil
}

// Close releases the database connection.
func (a *AppContext) Close() {
	if a.DB != nil {
		a.DB.Close()
	}
}

// mustOpen is a convenience wrapper for commands that call os.Exit on failure.
// Use openAppContext directly in commands that return errors cleanly.
func mustOpen() *AppContext {
	ctx, err := openAppContext()
	if err != nil {
		ui.Error(err.Error())
		os.Exit(1)
	}
	return ctx
}
