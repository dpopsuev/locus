package impact

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dpopsuev/locus/internal/arch"
)

// FileMove describes a hypothetical component move or deletion.
type FileMove struct {
	From string `json:"from"`
	To   string `json:"to,omitempty"` // empty = deletion
}

// EdgeDiff describes a single edge addition or removal.
type EdgeDiff struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// MetricDelta shows a metric change for a component.
type MetricDelta struct {
	Component string `json:"component"`
	Before    int    `json:"before"`
	After     int    `json:"after"`
}

// GraphDelta holds the diff between before and after graph states.
type GraphDelta struct {
	RemovedEdges     []EdgeDiff    `json:"removed_edges,omitempty"`
	AddedEdges       []EdgeDiff    `json:"added_edges,omitempty"`
	ComponentsBefore int           `json:"components_before"`
	ComponentsAfter  int           `json:"components_after"`
	EdgesBefore      int           `json:"edges_before"`
	EdgesAfter       int           `json:"edges_after"`
	NewCycles        []arch.Cycle  `json:"new_cycles,omitempty"`
	RemovedCycles    []arch.Cycle  `json:"removed_cycles,omitempty"`
	FanInDelta       []MetricDelta `json:"fan_in_delta,omitempty"`
	Summary          string        `json:"summary"`
}

// ComputeWhatIf applies hypothetical mutations to a dependency graph and
// returns the delta between the original and mutated states.
func ComputeWhatIf(
	services []arch.ArchService,
	edges []arch.ArchEdge,
	cycles []arch.Cycle,
	moves []FileMove,
) (*GraphDelta, error) {
	if len(moves) == 0 {
		return &GraphDelta{
			ComponentsBefore: len(services),
			ComponentsAfter:  len(services),
			EdgesBefore:      len(edges),
			EdgesAfter:       len(edges),
			Summary:          "no moves specified",
		}, nil
	}

	beforeFanIn := arch.ComputeFanIn(edges)
	beforeEdgeSet := edgeSet(edges)

	// Clone services and edges for mutation.
	mutServices := make([]arch.ArchService, len(services))
	copy(mutServices, services)
	mutEdges := make([]arch.ArchEdge, len(edges))
	copy(mutEdges, edges)

	// Apply each move.
	for _, m := range moves {
		if m.To == "" {
			mutServices, mutEdges = applyDelete(mutServices, mutEdges, m.From)
		} else {
			mutServices, mutEdges = applyRename(mutServices, mutEdges, m.From, m.To)
		}
	}

	// Deduplicate edges after renames (merge duplicate From/To pairs).
	mutEdges = deduplicateEdges(mutEdges)

	// Remove self-loops created by merges.
	mutEdges = removeSelfLoops(mutEdges)

	afterFanIn := arch.ComputeFanIn(mutEdges)
	afterEdgeSet := edgeSet(mutEdges)

	// Diff edges.
	var removed, added []EdgeDiff
	for e := range beforeEdgeSet {
		if !afterEdgeSet[e] {
			removed = append(removed, EdgeDiff{From: e.from, To: e.to})
		}
	}
	for e := range afterEdgeSet {
		if !beforeEdgeSet[e] {
			added = append(added, EdgeDiff{From: e.from, To: e.to})
		}
	}
	sortEdgeDiffs(removed)
	sortEdgeDiffs(added)

	// Diff cycles.
	afterCycles := arch.DetectCycles(mutEdges)
	newCycles, removedCycles := diffCycles(cycles, afterCycles)

	// Diff fan-in.
	fanInDelta := diffFanIn(beforeFanIn, afterFanIn)

	summary := buildSummary(len(services), len(mutServices), len(removed), len(added), len(newCycles), len(removedCycles))

	return &GraphDelta{
		RemovedEdges:     removed,
		AddedEdges:       added,
		ComponentsBefore: len(services),
		ComponentsAfter:  len(mutServices),
		EdgesBefore:      len(edges),
		EdgesAfter:       len(mutEdges),
		NewCycles:        newCycles,
		RemovedCycles:    removedCycles,
		FanInDelta:       fanInDelta,
		Summary:          summary,
	}, nil
}

func applyDelete(services []arch.ArchService, edges []arch.ArchEdge, name string) ([]arch.ArchService, []arch.ArchEdge) {
	var newServices []arch.ArchService
	for i := range services {
		if services[i].Name != name {
			newServices = append(newServices, services[i])
		}
	}
	var newEdges []arch.ArchEdge
	for _, e := range edges {
		if e.From != name && e.To != name {
			newEdges = append(newEdges, e)
		}
	}
	return newServices, newEdges
}

func applyRename(services []arch.ArchService, edges []arch.ArchEdge, from, to string) ([]arch.ArchService, []arch.ArchEdge) {
	// Rename service.
	found := false
	for i := range services {
		if services[i].Name == from {
			services[i].Name = to
			found = true
			break
		}
	}
	// If target already exists and source was found, merge (remove duplicate).
	if found {
		services = mergeServices(services, to)
	}

	// Rename edges.
	for i := range edges {
		if edges[i].From == from {
			edges[i].From = to
		}
		if edges[i].To == from {
			edges[i].To = to
		}
	}
	return services, edges
}

// mergeServices deduplicates services with the same name by merging LOC and Symbols.
func mergeServices(services []arch.ArchService, name string) []arch.ArchService {
	var merged *arch.ArchService
	var result []arch.ArchService
	for i := range services {
		if services[i].Name == name {
			if merged == nil {
				merged = &services[i]
				result = append(result, services[i])
			} else {
				merged.LOC += services[i].LOC
				merged.Symbols = append(merged.Symbols, services[i].Symbols...)
				// Update the already-appended entry.
				result[len(result)-1] = *merged
			}
		} else {
			result = append(result, services[i])
		}
	}
	return result
}

func deduplicateEdges(edges []arch.ArchEdge) []arch.ArchEdge {
	type key struct{ from, to string }
	merged := make(map[key]*arch.ArchEdge)
	var order []key
	for i := range edges {
		k := key{edges[i].From, edges[i].To}
		if existing, ok := merged[k]; ok {
			existing.CallSites += edges[i].CallSites
			existing.LOCSurface += edges[i].LOCSurface
			if edges[i].Weight > existing.Weight {
				existing.Weight = edges[i].Weight
			}
		} else {
			e := edges[i]
			merged[k] = &e
			order = append(order, k)
		}
	}
	result := make([]arch.ArchEdge, 0, len(order))
	for _, k := range order {
		result = append(result, *merged[k])
	}
	return result
}

func removeSelfLoops(edges []arch.ArchEdge) []arch.ArchEdge {
	var result []arch.ArchEdge
	for _, e := range edges {
		if e.From != e.To {
			result = append(result, e)
		}
	}
	return result
}

type edgeKey struct{ from, to string }

func edgeSet(edges []arch.ArchEdge) map[edgeKey]bool {
	s := make(map[edgeKey]bool, len(edges))
	for _, e := range edges {
		s[edgeKey{e.From, e.To}] = true
	}
	return s
}

func diffFanIn(before, after map[string]int) []MetricDelta {
	all := make(map[string]bool)
	for k := range before {
		all[k] = true
	}
	for k := range after {
		all[k] = true
	}

	var deltas []MetricDelta
	for k := range all {
		b, a := before[k], after[k]
		if b != a {
			deltas = append(deltas, MetricDelta{Component: k, Before: b, After: a})
		}
	}
	sort.Slice(deltas, func(i, j int) bool {
		return deltas[i].Component < deltas[j].Component
	})
	return deltas
}

func diffCycles(before, after []arch.Cycle) (newCycles, removedCycles []arch.Cycle) {
	beforeSet := cycleSet(before)
	afterSet := cycleSet(after)

	for key, cycle := range afterSet {
		if _, ok := beforeSet[key]; !ok {
			newCycles = append(newCycles, cycle)
		}
	}
	for key, cycle := range beforeSet {
		if _, ok := afterSet[key]; !ok {
			removedCycles = append(removedCycles, cycle)
		}
	}
	return newCycles, removedCycles
}

func cycleSet(cycles []arch.Cycle) map[string]arch.Cycle {
	s := make(map[string]arch.Cycle, len(cycles))
	for _, c := range cycles {
		sorted := make([]string, len(c))
		copy(sorted, c)
		sort.Strings(sorted)
		key := strings.Join(sorted, "→")
		s[key] = c
	}
	return s
}

func sortEdgeDiffs(diffs []EdgeDiff) {
	sort.Slice(diffs, func(i, j int) bool {
		if diffs[i].From != diffs[j].From {
			return diffs[i].From < diffs[j].From
		}
		return diffs[i].To < diffs[j].To
	})
}

func buildSummary(compBefore, compAfter, removedEdges, addedEdges, newCycles, removedCyclesCount int) string {
	parts := []string{
		fmt.Sprintf("components: %d→%d", compBefore, compAfter),
	}
	if removedEdges > 0 {
		parts = append(parts, fmt.Sprintf("%d edge(s) removed", removedEdges))
	}
	if addedEdges > 0 {
		parts = append(parts, fmt.Sprintf("%d edge(s) added", addedEdges))
	}
	if newCycles > 0 {
		parts = append(parts, fmt.Sprintf("%d new cycle(s)", newCycles))
	}
	if removedCyclesCount > 0 {
		parts = append(parts, fmt.Sprintf("%d cycle(s) broken", removedCyclesCount))
	}
	return strings.Join(parts, ", ")
}
