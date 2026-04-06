// Package git provides git operations used across Locus packages.
package git

import (
	"os/exec"
	"strings"
)

// ChangedFilesSince returns the list of changed files since a git ref.
func ChangedFilesSince(repoPath, since string) ([]string, error) {
	cmd := exec.Command("git", "diff", "--name-only", since)
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var files []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			files = append(files, line)
		}
	}
	return files, nil
}
