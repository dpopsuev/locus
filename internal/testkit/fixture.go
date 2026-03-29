package testkit

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
)

// BuildFixture writes a set of files to a directory.
// The map keys are relative paths, values are file contents.
func BuildFixture(dir string, files map[string]string) error {
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	for _, rel := range paths {
		abs := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", filepath.Dir(abs), err)
		}
		if err := os.WriteFile(abs, []byte(files[rel]), 0o600); err != nil {
			return fmt.Errorf("write %s: %w", rel, err)
		}
	}
	return nil
}

// InitGitRepo initializes a git repository in dir with an initial commit.
func InitGitRepo(dir string) error {
	cmds := [][]string{
		{"git", "init"},
		{"git", "add", "-A"},
		{"git", "commit", "-m", "init", "--allow-empty"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...) //nolint:gosec // trusted test-only command
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=testkit",
			"GIT_AUTHOR_EMAIL=testkit@test",
			"GIT_COMMITTER_NAME=testkit",
			"GIT_COMMITTER_EMAIL=testkit@test",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("%s: %s: %w", args[0], string(out), err)
		}
	}
	return nil
}
