package clinic

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/dpopsuev/locus/internal/graph"
	"github.com/dpopsuev/oculus"
)

// Pattern IDs used by enrichment.
const (
	patternIDFeatureEnvy  = "feature_envy"
	patternIDGodComponent = "god_component"
	MaxMoveTargets        = 10 // max move targets per detection
)

// MoveTarget identifies a specific symbol that should be moved to resolve Feature Envy.
type MoveTarget struct {
	Symbol    string  `json:"symbol"`
	SourcePkg string  `json:"source_pkg"`
	TargetPkg string  `json:"target_pkg"`
	CallPct   float64 `json:"call_pct"`
}

// SplitGroup represents a suggested sub-package to extract from a God Component.
type SplitGroup struct {
	Name      string   `json:"name"`
	Symbols   []string `json:"symbols"`
	Cohesion  float64  `json:"cohesion"`
	Rationale string   `json:"rationale"`
}

// SplitSuggestion holds the analysis for splitting a God Component.
type SplitSuggestion struct {
	Component string       `json:"component"`
	Groups    []SplitGroup `json:"groups"`
	Summary   string       `json:"summary"`
}

// EnrichWithCallGraph post-processes a PatternScanReport to add per-symbol
// move targets for Feature Envy and split suggestions for God Component.
// No-op if callEdges is nil or empty.
func EnrichWithCallGraph(report *PatternScanReport, callEdges []oculus.CallEdge) {
	if len(callEdges) == 0 || report == nil {
		return
	}

	// Build per-service symbol list from call edges.
	svcSymbols := make(map[string][]string)
	seen := make(map[string]map[string]bool)
	for i := range callEdges {
		pkg := callEdges[i].CallerPkg
		sym := callEdges[i].Caller
		if seen[pkg] == nil {
			seen[pkg] = make(map[string]bool)
		}
		if !seen[pkg][sym] {
			seen[pkg][sym] = true
			svcSymbols[pkg] = append(svcSymbols[pkg], sym)
		}
	}

	for i := range report.Detections {
		d := &report.Detections[i]
		switch d.PatternID {
		case patternIDFeatureEnvy:
			target := extractEnvyTarget(d.Evidence)
			if target != "" {
				d.MoveTargets = enrichFeatureEnvy(d.Component, target, callEdges)
			}
		case patternIDGodComponent:
			syms := svcSymbols[d.Component]
			d.SplitSuggestion = SuggestSplit(d.Component, syms, callEdges)
		}
	}
}

// envyTargetRe matches "N% of call sites target <pkg>" in evidence strings.
var envyTargetRe = regexp.MustCompile(`of call sites target (\S+)`)

// extractEnvyTarget parses the target package from Feature Envy evidence.
func extractEnvyTarget(evidence []string) string {
	for _, e := range evidence {
		if m := envyTargetRe.FindStringSubmatch(e); len(m) > 1 {
			return m[1]
		}
	}
	return ""
}

// enrichFeatureEnvy identifies specific symbols that should be moved.
func enrichFeatureEnvy(component, targetPkg string, callEdges []oculus.CallEdge) []MoveTarget {
	type symbolStats struct {
		total      int
		toTarget   int
		targetPkgs map[string]int
	}

	stats := make(map[string]*symbolStats)
	for i := range callEdges {
		e := &callEdges[i]
		if e.CallerPkg != component || !e.CrossPkg {
			continue
		}
		s := stats[e.Caller]
		if s == nil {
			s = &symbolStats{targetPkgs: make(map[string]int)}
			stats[e.Caller] = s
		}
		s.total++
		s.targetPkgs[e.CalleePkg]++
		if e.CalleePkg == targetPkg {
			s.toTarget++
		}
	}

	var targets []MoveTarget
	for sym, s := range stats {
		if s.total == 0 {
			continue
		}
		pct := float64(s.toTarget) / float64(s.total)
		if pct > 0.5 {
			targets = append(targets, MoveTarget{
				Symbol:    sym,
				SourcePkg: component,
				TargetPkg: targetPkg,
				CallPct:   pct,
			})
		}
	}

	sort.Slice(targets, func(i, j int) bool {
		return targets[i].CallPct > targets[j].CallPct
	})
	if len(targets) > MaxMoveTargets {
		targets = targets[:MaxMoveTargets]
	}
	return targets
}

// SuggestSplit analyzes intra-component symbol coupling to suggest
// concrete file groupings for extracting sub-packages from a God Component.
// Returns nil if no meaningful split is possible.
func SuggestSplit(component string, symbols []string, callEdges []oculus.CallEdge) *SplitSuggestion {
	if len(symbols) < 4 {
		return nil
	}

	// Build intra-component adjacency graph.
	adj := make(map[string]map[string]bool)
	symSet := make(map[string]bool, len(symbols))
	for _, s := range symbols {
		symSet[s] = true
	}

	for i := range callEdges {
		e := &callEdges[i]
		if e.CallerPkg != component || e.CalleePkg != component {
			continue
		}
		if !symSet[e.Caller] || !symSet[e.Callee] || e.Caller == e.Callee {
			continue
		}
		if adj[e.Caller] == nil {
			adj[e.Caller] = make(map[string]bool)
		}
		if adj[e.Callee] == nil {
			adj[e.Callee] = make(map[string]bool)
		}
		adj[e.Caller][e.Callee] = true
		adj[e.Callee][e.Caller] = true
	}

	// Find connected components via BFS.
	visited := make(map[string]bool)
	var groups [][]string

	for _, sym := range symbols {
		if visited[sym] {
			continue
		}
		if adj[sym] == nil {
			visited[sym] = true
			continue // isolated symbol, skip
		}
		group := graph.BFSGroup(sym, adj, visited)
		if len(group) >= 2 {
			groups = append(groups, group)
		}
	}

	if len(groups) < 2 {
		return nil
	}

	// Build SplitGroups.
	splitGroups := make([]SplitGroup, 0, len(groups))
	for idx, g := range groups {
		name := suggestGroupName(g, idx)
		cohesion := graph.Cohesion(g, adj)
		splitGroups = append(splitGroups, SplitGroup{
			Name:      name,
			Symbols:   g,
			Cohesion:  cohesion,
			Rationale: fmt.Sprintf("%d symbols with %.0f%% internal coupling", len(g), cohesion*100),
		})
	}

	sort.Slice(splitGroups, func(i, j int) bool {
		return len(splitGroups[i].Symbols) > len(splitGroups[j].Symbols)
	})

	return &SplitSuggestion{
		Component: component,
		Groups:    splitGroups,
		Summary:   fmt.Sprintf("suggest splitting into %d sub-packages", len(splitGroups)),
	}
}

func suggestGroupName(symbols []string, idx int) string {
	if len(symbols) == 0 {
		return fmt.Sprintf("group_%d", idx+1)
	}
	// Find longest common lowercase prefix.
	prefix := strings.ToLower(symbols[0])
	for _, s := range symbols[1:] {
		lower := strings.ToLower(s)
		for prefix != "" && !strings.HasPrefix(lower, prefix) {
			prefix = prefix[:len(prefix)-1]
		}
	}
	// Trim trailing underscores or non-alpha.
	prefix = strings.TrimRight(prefix, "_")
	if len(prefix) >= 3 {
		return prefix
	}
	return fmt.Sprintf("group_%d", idx+1)
}
