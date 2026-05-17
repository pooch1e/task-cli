package project

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Detect returns the project name and root path for the current working
// directory by walking up to the nearest .git root.
// Returns an error if the working directory cannot be determined.
func Detect() (name string, root string, err error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", "", fmt.Errorf("getting working directory: %w", err)
	}

	// Prefer git rev-parse — handles worktrees and unusual repo layouts.
	if out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output(); err == nil {
		root = strings.TrimSpace(string(out))
		return filepath.Base(root), root, nil
	}

	// Manual walk as fallback when git is unavailable.
	dir := cwd
	for {
		if fi, err := os.Stat(filepath.Join(dir, ".git")); err == nil && fi.IsDir() {
			return filepath.Base(dir), dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	// Not inside a git repo — use cwd basename as project name.
	return filepath.Base(cwd), cwd, nil
}
