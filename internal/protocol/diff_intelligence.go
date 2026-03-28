package protocol

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dpopsuev/locus/internal/analysis"
)

// SemanticChange describes a symbol in a changed package and its caller impact.
type SemanticChange struct {
	Symbol        string `json:"symbol"`
	Package       string `json:"package"`
	ChangeType    string `json:"change_type"`
	Description   string `json:"description"`
	AffectedCount int    `json:"affected_count"`
	Severity      string `json:"severity"`
}

// DiffIntelligenceReport holds semantic analysis of changed files against a call graph.
type DiffIntelligenceReport struct {
	Since           string           `json:"since"`
	ChangedFiles    []string         `json:"changed_files"`
	ChangedPkgs     []string         `json:"changed_packages"`
	SemanticChanges []SemanticChange `json:"semantic_changes"`
	Summary         string           `json:"summary"`
}

// ComputeDiffIntelligence maps changed files to packages, then identifies symbols
// in those packages that have callers in the call graph. Pure function: accepts
// data, returns report.
func ComputeDiffIntelligence(changedFiles []string, modulePath string, graph *analysis.CallGraph) *DiffIntelligenceReport {
	report := &DiffIntelligenceReport{
		ChangedFiles: changedFiles,
	}

	if len(changedFiles) == 0 || graph == nil {
		report.Summary = "0 files changed across 0 packages, 0 symbols with callers affected"
		return report
	}

	// Map changed files to packages (strip module prefix, take directory).
	pkgSet := make(map[string]bool)
	for _, f := range changedFiles {
		rel := strings.TrimPrefix(f, modulePath+"/")
		dir := filepath.Dir(rel)
		if dir == "." {
			dir = "(root)"
		} else {
			dir = filepath.ToSlash(dir)
		}
		pkgSet[dir] = true
	}

	changedPkgs := make([]string, 0, len(pkgSet))
	for pkg := range pkgSet {
		changedPkgs = append(changedPkgs, pkg)
	}
	sort.Strings(changedPkgs)
	report.ChangedPkgs = changedPkgs

	// Build a caller count per symbol from call graph edges.
	callerCount := make(map[string]int)
	for _, edge := range graph.Edges {
		callerCount[edge.Callee]++
	}

	// For each symbol whose package matches a changed package, emit a SemanticChange.
	var changes []SemanticChange
	for _, node := range graph.Nodes {
		if !pkgSet[node.Package] {
			continue
		}
		count := callerCount[node.Name]
		if count == 0 {
			continue
		}
		changes = append(changes, SemanticChange{
			Symbol:        node.Name,
			Package:       node.Package,
			ChangeType:    "modified",
			Description:   fmt.Sprintf("Symbol %s in changed package %s has %d callers that may need review", node.Name, node.Package, count),
			AffectedCount: count,
			Severity:      callerSeverity(count),
		})
	}

	// Sort by affected count descending.
	sort.Slice(changes, func(i, j int) bool {
		return changes[i].AffectedCount > changes[j].AffectedCount
	})
	report.SemanticChanges = changes

	report.Summary = fmt.Sprintf("%d files changed across %d packages, %d symbols with callers affected",
		len(changedFiles), len(changedPkgs), len(changes))

	return report
}

// callerSeverity maps caller count to a severity level.
func callerSeverity(count int) string {
	switch {
	case count > 10:
		return "critical"
	case count > 3:
		return "high"
	case count > 0:
		return "medium"
	default:
		return "low"
	}
}
