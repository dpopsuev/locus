package analysis

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Convention represents a detected coding convention.
type Convention struct {
	Category string   `json:"category"` // naming, structure, style
	Pattern  string   `json:"pattern"`
	Examples []string `json:"examples,omitempty"`
	Count    int      `json:"count"`
}

// ConventionReport holds the result of convention detection.
type ConventionReport struct {
	Conventions []Convention `json:"conventions"`
	Total       int          `json:"total"`
}

// DetectConventions scans the project and detects coding conventions.
func DetectConventions(root string) (*ConventionReport, error) {
	report := &ConventionReport{}

	// Track what we find
	structureDirs := make(map[string]int)
	testPatterns := make(map[string][]string)
	configPatterns := make(map[string][]string)
	namingFiles := make(map[string]int)   // snake_case vs camelCase for files
	namingTypes := make(map[string]int)  // PascalCase for types (from filenames)

	structurePrefixes := []string{"cmd/", "internal/", "pkg/", "src/"}
	testSuffixes := []string{"_test.go", "test_", ".spec.ts", ".spec.js", "_test.py"}
	configNames := []string{".yaml", ".yml", ".toml", ".json", "config.", ".config"}

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info.IsDir() {
			// Skip hidden and vendor
			base := filepath.Base(path)
			if strings.HasPrefix(base, ".") || base == "vendor" || base == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}

		rel, _ := filepath.Rel(root, path)
		base := filepath.Base(path)

		// File structure patterns
		for _, prefix := range structurePrefixes {
			if strings.HasPrefix(rel, prefix) {
				structureDirs[prefix]++
				break
			}
		}

		// Test file patterns
		for _, suffix := range testSuffixes {
			if strings.HasSuffix(base, suffix) || strings.HasPrefix(base, suffix) {
				examples := testPatterns[suffix]
				examples = append(examples, rel)
				if len(examples) > 5 {
					examples = examples[:5]
				}
				testPatterns[suffix] = examples
				break
			}
		}

		// Config patterns
		for _, cfg := range configNames {
			if strings.Contains(base, cfg) || strings.HasSuffix(base, cfg) {
				examples := configPatterns[cfg]
				examples = append(examples, rel)
				if len(examples) > 5 {
					examples = examples[:5]
				}
				configPatterns[cfg] = examples
				break
			}
		}

		// Naming: file conventions (exclude test and config)
		ext := filepath.Ext(base)
		if ext == ".go" && !strings.HasSuffix(base, "_test.go") {
			name := strings.TrimSuffix(base, ext)
			if snakeCaseRegex.MatchString(name) {
				namingFiles["snake_case"]++
			} else if camelCaseRegex.MatchString(name) {
				namingFiles["camelCase"]++
			}
		}
		if ext == ".go" && strings.HasSuffix(base, "_test.go") {
			// Go: PascalCase for types often in exported files
			namingTypes["PascalCase"]++
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	// Build conventions from collected data
	for _, prefix := range structurePrefixes {
		if structureDirs[prefix] > 0 {
			report.Conventions = append(report.Conventions, Convention{
				Category: "structure",
				Pattern:  prefix + " directory layout",
				Count:    structureDirs[prefix],
				Examples:  []string{prefix + "..."},
			})
			report.Total += structureDirs[prefix]
		}
	}

	for pattern, examples := range testPatterns {
		if len(examples) > 0 {
			report.Conventions = append(report.Conventions, Convention{
				Category: "style",
				Pattern:  "test file: " + pattern,
				Count:    len(examples),
				Examples: examples,
			})
			report.Total += len(examples)
		}
	}

	for pattern, examples := range configPatterns {
		if len(examples) > 0 {
			report.Conventions = append(report.Conventions, Convention{
				Category: "structure",
				Pattern:  "config: " + pattern,
				Count:    len(examples),
				Examples: examples,
			})
			report.Total += len(examples)
		}
	}

	for naming, count := range namingFiles {
		if count > 0 {
			report.Conventions = append(report.Conventions, Convention{
				Category: "naming",
				Pattern:  "file naming: " + naming,
				Count:    count,
			})
			report.Total += count
		}
	}

	for naming, count := range namingTypes {
		if count > 0 {
			report.Conventions = append(report.Conventions, Convention{
				Category: "naming",
				Pattern:  "type naming: " + naming,
				Count:    count,
			})
			report.Total += count
		}
	}

	return report, nil
}

var (
	snakeCaseRegex = regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`)
	camelCaseRegex = regexp.MustCompile(`^[a-z][a-zA-Z0-9]*$`)
)
