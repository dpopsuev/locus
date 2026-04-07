package git

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// ComputeChurn shells out to git log and returns a map from short package path
// to file-change count over the last N days. Paths are relative to modPath
// within the repo at root.
func ComputeChurn(root string, days int, modPath string) map[string]int {
	if days <= 0 {
		return nil
	}
	sinceArg := fmt.Sprintf("--since=%d.days.ago", days)
	cmd := exec.Command("git", "log", "--format=", "--name-only", sinceArg)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	absRoot, _ := filepath.Abs(root)
	return aggregateChurn(string(out), absRoot)
}

// aggregateChurn parses git log --name-only output and returns churn counts
// per directory. Test files (_test.go) are excluded — their churn is expected
// and should not inflate smell thresholds (Shotgun Surgery, Unstable Interface).
func aggregateChurn(gitOutput, absRoot string) map[string]int {
	result := make(map[string]int)
	for _, line := range strings.Split(gitOutput, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasSuffix(line, "_test.go") {
			continue
		}
		dir := filepath.Dir(line)
		if dir == "." {
			continue
		}
		full := filepath.Join(absRoot, dir)
		rel, err := filepath.Rel(absRoot, full)
		if err != nil {
			continue
		}
		rel = filepath.ToSlash(rel)
		result[rel]++
	}
	return result
}
