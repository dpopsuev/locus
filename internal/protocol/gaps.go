package protocol

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/dpopsuev/locus/internal/arch"
)

// GapEntry represents knowledge gaps for a single component.
type GapEntry struct {
	Component string   `json:"component"`
	Gaps      []string `json:"gaps"`     // "no tests", "no docs", "low coverage"
	Severity  string   `json:"severity"` // info, warning, critical
}

// GapReport holds the result of gap detection.
type GapReport struct {
	Entries           []GapEntry `json:"entries"`
	TotalGaps         int        `json:"total_gaps"`
	ComponentsScanned int         `json:"components_scanned"`
}

// DetectGaps identifies undocumented or under-tested components.
func DetectGaps(report *arch.ContextReport, root string) (*GapReport, error) {
	r := &GapReport{ComponentsScanned: len(report.Architecture.Services)}

	// Compute fan-in per component
	fanIn := make(map[string]int)
	for _, e := range report.Architecture.Edges {
		fanIn[e.To]++
	}

	for _, svc := range report.Architecture.Services {
		comp := svc.Name
		dir := componentDir(root, report.ModulePath, comp)
		// Skip components outside the project root (e.g. GOPATH module cache). BUG-9.
		if !strings.HasPrefix(dir, root) {
			continue
		}
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			continue
		}
		entry := GapEntry{Component: comp}

		hasTests := hasTestFiles(dir)
		hasDocs := hasDocFiles(dir)

		if !hasTests {
			entry.Gaps = append(entry.Gaps, "no tests")
		}
		if !hasDocs {
			entry.Gaps = append(entry.Gaps, "no docs")
		}

		if len(entry.Gaps) == 0 {
			continue
		}

		// Severity: high fan-in without tests = critical
		fi := fanIn[comp]
		if !hasTests && fi >= 3 {
			entry.Severity = "critical"
		} else if !hasDocs {
			entry.Severity = "warning"
		} else {
			entry.Severity = "info"
		}

		r.Entries = append(r.Entries, entry)
		r.TotalGaps += len(entry.Gaps)
	}

	return r, nil
}

func componentDir(root, modPath, component string) string {
	if component == "." || component == "" {
		return root
	}
	return filepath.Join(root, component)
}

func hasTestFiles(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, "_test.go") ||
			(strings.HasPrefix(name, "test_") && strings.HasSuffix(name, ".py")) ||
			strings.HasSuffix(name, "_test.py") ||
			strings.HasSuffix(name, ".spec.ts") ||
			strings.HasSuffix(name, ".spec.js") {
			return true
		}
	}
	return false
}

func hasDocFiles(dir string) bool {
	docNames := []string{"README.md", "README", "README.txt", "doc.go", "DOC.md"}
	for _, name := range docNames {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	return false
}
