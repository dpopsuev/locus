package history

import (
	"fmt"
	"strings"

	"github.com/dpopsuev/mos/moslib/arch"
)

type CodographDiff struct {
	AddedComponents   []string          `json:"added_components,omitempty"`
	RemovedComponents []string          `json:"removed_components,omitempty"`
	AddedEdges        []string          `json:"added_edges,omitempty"`
	RemovedEdges      []string          `json:"removed_edges,omitempty"`
	ChurnDeltas       []ChurnDelta      `json:"churn_deltas,omitempty"`
	Summary           string            `json:"summary"`
}

type ChurnDelta struct {
	Component string `json:"component"`
	OldChurn  int    `json:"old_churn"`
	NewChurn  int    `json:"new_churn"`
	Delta     int    `json:"delta"`
}

func DiffReports(old, new *arch.ContextReport) *CodographDiff {
	d := &CodographDiff{}

	oldComps := componentSet(old.Architecture.Services)
	newComps := componentSet(new.Architecture.Services)

	for name := range newComps {
		if !oldComps[name] {
			d.AddedComponents = append(d.AddedComponents, name)
		}
	}
	for name := range oldComps {
		if !newComps[name] {
			d.RemovedComponents = append(d.RemovedComponents, name)
		}
	}

	oldEdges := edgeSet(old.Architecture.Edges)
	newEdges := edgeSet(new.Architecture.Edges)

	for key := range newEdges {
		if !oldEdges[key] {
			d.AddedEdges = append(d.AddedEdges, key)
		}
	}
	for key := range oldEdges {
		if !newEdges[key] {
			d.RemovedEdges = append(d.RemovedEdges, key)
		}
	}

	oldChurn := churnMap(old.HotSpots)
	newChurn := churnMap(new.HotSpots)

	allChurnKeys := map[string]bool{}
	for k := range oldChurn {
		allChurnKeys[k] = true
	}
	for k := range newChurn {
		allChurnKeys[k] = true
	}
	for comp := range allChurnKeys {
		oc := oldChurn[comp]
		nc := newChurn[comp]
		if oc != nc {
			d.ChurnDeltas = append(d.ChurnDeltas, ChurnDelta{
				Component: comp,
				OldChurn:  oc,
				NewChurn:  nc,
				Delta:     nc - oc,
			})
		}
	}

	d.Summary = buildSummary(d)
	return d
}

func componentSet(svcs []arch.ArchService) map[string]bool {
	m := make(map[string]bool, len(svcs))
	for _, s := range svcs {
		m[s.Name] = true
	}
	return m
}

func edgeKey(e arch.ArchEdge) string {
	return e.From + "->" + e.To
}

func edgeSet(edges []arch.ArchEdge) map[string]bool {
	m := make(map[string]bool, len(edges))
	for _, e := range edges {
		m[edgeKey(e)] = true
	}
	return m
}

func churnMap(spots []arch.HotSpot) map[string]int {
	m := make(map[string]int, len(spots))
	for _, s := range spots {
		m[s.Component] = s.Churn
	}
	return m
}

func buildSummary(d *CodographDiff) string {
	var parts []string
	if n := len(d.AddedComponents); n > 0 {
		parts = append(parts, fmt.Sprintf("+%d components", n))
	}
	if n := len(d.RemovedComponents); n > 0 {
		parts = append(parts, fmt.Sprintf("-%d components", n))
	}
	if n := len(d.AddedEdges); n > 0 {
		parts = append(parts, fmt.Sprintf("+%d edges", n))
	}
	if n := len(d.RemovedEdges); n > 0 {
		parts = append(parts, fmt.Sprintf("-%d edges", n))
	}
	if n := len(d.ChurnDeltas); n > 0 {
		parts = append(parts, fmt.Sprintf("%d churn changes", n))
	}
	if len(parts) == 0 {
		return "no changes"
	}
	return strings.Join(parts, ", ")
}
