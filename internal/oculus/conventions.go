package oculus

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/dpopsuev/locus/internal/oculus/lang"
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

// conventionData holds intermediate data collected during the filesystem walk.
type conventionData struct {
	structureDirs  map[string]int
	testPatterns   map[string][]string
	configPatterns map[string][]string
	namingFiles    map[string]int
	namingTypes    map[string]int
}

var (
	structurePrefixes = []string{"cmd/", "internal/", "pkg/", "src/"}
	testSuffixes      = []string{"_test.go", "test_", ".spec.ts", ".spec.js", "_test.py"}
	configNames       = []string{".yaml", ".yml", ".toml", ".json", "config.", ".config"}
)

// DetectConventions scans the project and detects coding conventions.
func DetectConventions(root string) (*ConventionReport, error) {
	data := &conventionData{
		structureDirs:  make(map[string]int),
		testPatterns:   make(map[string][]string),
		configPatterns: make(map[string][]string),
		namingFiles:    make(map[string]int),
		namingTypes:    make(map[string]int),
	}

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			if lang.ShouldSkipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		rel, _ := filepath.Rel(root, path)
		base := filepath.Base(path)
		data.classifyFile(rel, base)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return data.buildReport(), nil
}

// classifyFile categorizes a single file into convention patterns.
func (d *conventionData) classifyFile(rel, base string) {
	// File structure patterns
	for _, prefix := range structurePrefixes {
		if strings.HasPrefix(rel, prefix) {
			d.structureDirs[prefix]++
			break
		}
	}

	// Test file patterns
	for _, suffix := range testSuffixes {
		if !strings.HasSuffix(base, suffix) && !strings.HasPrefix(base, suffix) {
			continue
		}
		examples := d.testPatterns[suffix]
		examples = append(examples, rel)
		if len(examples) > 5 {
			examples = examples[:5]
		}
		d.testPatterns[suffix] = examples
		break
	}

	// Config patterns
	for _, cfg := range configNames {
		if !strings.Contains(base, cfg) && !strings.HasSuffix(base, cfg) {
			continue
		}
		examples := d.configPatterns[cfg]
		examples = append(examples, rel)
		if len(examples) > 5 {
			examples = examples[:5]
		}
		d.configPatterns[cfg] = examples
		break
	}

	// Naming: file conventions
	ext := filepath.Ext(base)
	if ext != extGo {
		return
	}
	if strings.HasSuffix(base, "_test.go") {
		d.namingTypes["PascalCase"]++
		return
	}
	name := strings.TrimSuffix(base, ext)
	if snakeCaseRegex.MatchString(name) {
		d.namingFiles["snake_case"]++
	} else if camelCaseRegex.MatchString(name) {
		d.namingFiles["camelCase"]++
	}
}

// buildReport converts collected data into a ConventionReport.
func (d *conventionData) buildReport() *ConventionReport {
	report := &ConventionReport{}

	for _, prefix := range structurePrefixes {
		if d.structureDirs[prefix] > 0 {
			report.Conventions = append(report.Conventions, Convention{
				Category: "structure",
				Pattern:  prefix + " directory layout",
				Count:    d.structureDirs[prefix],
				Examples: []string{prefix + "..."},
			})
			report.Total += d.structureDirs[prefix]
		}
	}

	addPatternConventions(report, "style", "test file: ", d.testPatterns)
	addPatternConventions(report, "structure", "config: ", d.configPatterns)
	addCountConventions(report, "naming", "file naming: ", d.namingFiles)
	addCountConventions(report, "naming", "type naming: ", d.namingTypes)

	return report
}

// addPatternConventions adds conventions from pattern-to-examples maps.
func addPatternConventions(report *ConventionReport, category, prefix string, patterns map[string][]string) {
	for pattern, examples := range patterns {
		if len(examples) > 0 {
			report.Conventions = append(report.Conventions, Convention{
				Category: category,
				Pattern:  prefix + pattern,
				Count:    len(examples),
				Examples: examples,
			})
			report.Total += len(examples)
		}
	}
}

// addCountConventions adds conventions from name-to-count maps.
func addCountConventions(report *ConventionReport, category, prefix string, counts map[string]int) {
	for name, count := range counts {
		if count > 0 {
			report.Conventions = append(report.Conventions, Convention{
				Category: category,
				Pattern:  prefix + name,
				Count:    count,
			})
			report.Total += count
		}
	}
}

var (
	snakeCaseRegex = regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`)
	camelCaseRegex = regexp.MustCompile(`^[a-z][a-zA-Z0-9]*$`)
)
