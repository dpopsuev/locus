package impact

import (
	"fmt"
	"sort"

	"github.com/dpopsuev/locus/internal/analysis"
	"github.com/dpopsuev/locus/internal/port"
)

// SymbolBlastReport holds the blast radius analysis for a single symbol,
// showing all direct and transitive callers plus affected packages.
type SymbolBlastReport struct {
	Symbol        string            `json:"symbol"`
	DirectCallers []port.CallerSite `json:"direct_callers"`
	TransCallers  []port.CallerSite `json:"transitive_callers"`
	AffectedPkgs  []string          `json:"affected_packages"`
	BlastRadius   int               `json:"blast_radius"`
	RiskLevel     string            `json:"risk_level"`
	Summary       string            `json:"summary"`
}

// ComputeSymbolBlastRadius computes direct and transitive callers for a symbol,
// then derives the blast radius as a percentage of total packages affected.
func ComputeSymbolBlastRadius(edges []analysis.CallEdge, symbol string, totalPkgs int) *SymbolBlastReport {
	report := &SymbolBlastReport{Symbol: symbol}

	// Build reverse adjacency: callee -> []CallEdge (who calls it).
	reverse := make(map[string][]analysis.CallEdge)
	for _, e := range edges {
		reverse[e.Callee] = append(reverse[e.Callee], e)
	}

	// Direct callers: edges where Callee == symbol.
	directEdges := reverse[symbol]
	directSet := make(map[string]bool, len(directEdges))
	for _, e := range directEdges {
		directSet[e.Caller] = true
		report.DirectCallers = append(report.DirectCallers, edgeToCallerSite(e))
	}

	// BFS from direct callers to find transitive callers.
	visited := make(map[string]bool, len(directSet))
	for caller := range directSet {
		visited[caller] = true
	}
	// The symbol itself should not appear as a transitive caller.
	visited[symbol] = true

	queue := make([]string, 0, len(directSet))
	for caller := range directSet {
		queue = append(queue, caller)
	}

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, e := range reverse[cur] {
			if !visited[e.Caller] {
				visited[e.Caller] = true
				report.TransCallers = append(report.TransCallers, edgeToCallerSite(e))
				queue = append(queue, e.Caller)
			}
		}
	}

	// Collect unique packages from all callers.
	pkgSet := make(map[string]bool)
	for _, c := range report.DirectCallers {
		if c.CallerPkg != "" {
			pkgSet[c.CallerPkg] = true
		}
	}
	for _, c := range report.TransCallers {
		if c.CallerPkg != "" {
			pkgSet[c.CallerPkg] = true
		}
	}
	for pkg := range pkgSet {
		report.AffectedPkgs = append(report.AffectedPkgs, pkg)
	}
	sort.Strings(report.AffectedPkgs)

	// Blast radius: affected packages as percentage of total.
	if totalPkgs > 0 {
		report.BlastRadius = len(report.AffectedPkgs) * 100 / totalPkgs
	}

	// Risk level thresholds match ComputeImpact.
	report.RiskLevel = classifyRisk(report.BlastRadius)

	report.Summary = fmt.Sprintf("%d direct, %d transitive caller(s) of %s across %d package(s) — %s risk",
		len(report.DirectCallers), len(report.TransCallers), symbol, len(report.AffectedPkgs), report.RiskLevel)

	return report
}

// edgeToCallerSite converts a CallEdge to a port.CallerSite.
func edgeToCallerSite(e analysis.CallEdge) port.CallerSite {
	return port.CallerSite{
		Caller:       e.Caller,
		CallerPkg:    e.CallerPkg,
		Line:         e.Line,
		File:         e.File,
		ReceiverType: e.ReceiverType,
	}
}
