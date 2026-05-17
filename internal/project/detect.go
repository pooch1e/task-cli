package project

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Detect returns the project name and root path for the current working directory.
// It walks up looking for a .git directory. Falls back to the cwd basename.
func Detect() (name string, root string) {
	cwd, err := os.Getwd()
	if err != nil {
		return "default", cwd
	}

	// Try git rev-parse for the canonical root first (handles worktrees etc.)
	if out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output(); err == nil {
		root = strings.TrimSpace(string(out))
		return filepath.Base(root), root
	}

	// Manual walk as fallback (no git installed)
	dir := cwd
	for {
		if fi, err := os.Stat(filepath.Join(dir, ".git")); err == nil && fi.IsDir() {
			return filepath.Base(dir), dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return filepath.Base(cwd), cwd
}
