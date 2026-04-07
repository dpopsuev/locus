package git

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// CoverageResult holds per-component test coverage data.
type CoverageResult struct {
	Component   string  `json:"component"`
	CoveragePct float64 `json:"coverage_pct"`
}

// RunGoCoverage executes `go test -coverprofile` in root and parses the output.
// Returns nil without error for non-Go repos or if go test fails (best-effort).
func RunGoCoverage(root, modPath string) ([]CoverageResult, error) {
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		return nil, nil
	}

	tmp, err := os.CreateTemp("", "locus-cover-*.out")
	if err != nil {
		return nil, fmt.Errorf("create temp: %w", err)
	}
	coverFile := tmp.Name()
	tmp.Close()
	defer os.Remove(coverFile)

	cmd := exec.Command("go", "test", "-coverprofile="+coverFile, "-count=1", "./...") //nolint:gosec // coverFile is a controlled temp path
	cmd.Dir = root
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return nil, nil
	}

	return parseCoverProfile(coverFile, modPath)
}

// parseCoverProfile reads a Go coverage profile and aggregates by package.
func parseCoverProfile(path, modPath string) ([]CoverageResult, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	type accum struct {
		covered int
		total   int
	}
	pkgs := make(map[string]*accum)

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "mode:") {
			continue
		}
		// Format: file:startLine.startCol,endLine.endCol numStatements count
		parts := strings.Fields(line)
		if len(parts) != 3 {
			continue
		}
		colonIdx := strings.LastIndex(parts[0], ":")
		if colonIdx < 0 {
			continue
		}
		filePath := parts[0][:colonIdx]
		slashIdx := strings.LastIndex(filePath, "/")
		pkg := filePath
		if slashIdx >= 0 {
			pkg = filePath[:slashIdx]
		}

		numStmts, _ := strconv.Atoi(parts[1])
		count, _ := strconv.Atoi(parts[2])

		a := pkgs[pkg]
		if a == nil {
			a = &accum{}
			pkgs[pkg] = a
		}
		a.total += numStmts
		if count > 0 {
			a.covered += numStmts
		}
	}

	results := make([]CoverageResult, 0, len(pkgs))
	for pkg, a := range pkgs {
		component := pkg
		if modPath != "" && strings.HasPrefix(pkg, modPath+"/") {
			component = strings.TrimPrefix(pkg, modPath+"/")
		} else if pkg == modPath {
			component = "."
		}
		var pct float64
		if a.total > 0 {
			pct = float64(a.covered) / float64(a.total) * 100
		}
		results = append(results, CoverageResult{
			Component:   component,
			CoveragePct: pct,
		})
	}
	return results, nil
}
