package impact

import (
	"fmt"
	"sort"

	"github.com/dpopsuev/locus/internal/arch"
	"github.com/dpopsuev/locus/internal/graph"
)

// IndependenceScore measures how well-focused and widely-used a component is.
// High independence = well-defined module. Low = candidate for merge.
type IndependenceScore struct {
	Component    string  `json:"component"`
	FanIn        int     `json:"fan_in"`
	Exports      int     `json:"exports"`
	LOC          int     `json:"loc"`
	Independence float64 `json:"independence"`
}

// ComputeIndependenceScores calculates independence = (fanIn × exports) / max(LOC, 1)
// for each component. Language-agnostic.
func ComputeIndependenceScores(services []arch.ArchService, fanIn graph.CountMap) []IndependenceScore {
	scores := make([]IndependenceScore, 0, len(services))
	for i := range services {
		svc := &services[i]
		fi := fanIn[svc.Name]
		exports := len(svc.Symbols)
		loc := svc.LOC
		if loc == 0 {
			loc = 1
		}
		independence := float64(fi*exports) / float64(loc)

		scores = append(scores, IndependenceScore{
			Component:    svc.Name,
			FanIn:        fi,
			Exports:      exports,
			LOC:          svc.LOC,
			Independence: independence,
		})
	}

	sort.Slice(scores, func(i, j int) bool {
		return scores[i].Independence > scores[j].Independence
	})
	return scores
}

// CohesionCluster represents packages always co-imported by the same consumers.
type CohesionCluster struct {
	Members     []string `json:"members"`
	CoImportPct float64  `json:"co_import_pct"`
	Consumers   int      `json:"consumers"`
}

// DetectCohesionClusters finds packages always imported together by the same consumers.
// Uses Jaccard similarity on consumer sets: pairs with similarity > threshold are clustered
// via ConnectedComponents.
func DetectCohesionClusters(edges []arch.ArchEdge, threshold float64) []CohesionCluster {
	if threshold <= 0 {
		threshold = 0.7
	}

	// Build consumer map: component → set of consumers (who imports it).
	consumers := make(map[string]map[string]bool)
	for _, e := range edges {
		if consumers[e.To] == nil {
			consumers[e.To] = make(map[string]bool)
		}
		consumers[e.To][e.From] = true
	}

	// Find pairs with high Jaccard similarity.
	components := make([]string, 0, len(consumers))
	for c := range consumers {
		components = append(components, c)
	}
	sort.Strings(components)

	type pair struct{ a, b string }
	var highSim []pair
	for i := 0; i < len(components); i++ {
		for j := i + 1; j < len(components); j++ {
			a, b := components[i], components[j]
			sim := jaccard(consumers[a], consumers[b])
			if sim >= threshold {
				highSim = append(highSim, pair{a, b})
			}
		}
	}

	if len(highSim) == 0 {
		return nil
	}

	// Build edges for ConnectedComponents.
	simEdges := make([]simEdge, len(highSim))
	for i, p := range highSim {
		simEdges[i] = simEdge{p.a, p.b}
	}

	groups := graph.ConnectedComponents(simEdges)

	clusters := make([]CohesionCluster, 0, len(groups))
	for _, g := range groups {
		if len(g) < 2 {
			continue
		}
		// Compute average co-import percentage.
		totalSim := 0.0
		pairs := 0
		for i := 0; i < len(g); i++ {
			for j := i + 1; j < len(g); j++ {
				totalSim += jaccard(consumers[g[i]], consumers[g[j]])
				pairs++
			}
		}
		avgSim := totalSim / float64(max(pairs, 1))

		// Count total unique consumers.
		allConsumers := make(map[string]bool)
		for _, m := range g {
			for c := range consumers[m] {
				allConsumers[c] = true
			}
		}

		clusters = append(clusters, CohesionCluster{
			Members:     g,
			CoImportPct: avgSim,
			Consumers:   len(allConsumers),
		})
	}

	sort.Slice(clusters, func(i, j int) bool {
		return len(clusters[i].Members) > len(clusters[j].Members)
	})
	return clusters
}

// ConsolidationReport combines independence scores and cohesion clusters
// with actionable merge/split suggestions.
type ConsolidationReport struct {
	IndependenceScores []IndependenceScore `json:"independence_scores"`
	Clusters           []CohesionCluster   `json:"clusters,omitempty"`
	Summary            string              `json:"summary"`
}

// ComputeConsolidation produces the full consolidation analysis.
func ComputeConsolidation(services []arch.ArchService, edges []arch.ArchEdge) *ConsolidationReport {
	fanIn := graph.FanIn(edges)
	scores := ComputeIndependenceScores(services, fanIn)
	clusters := DetectCohesionClusters(edges, 0.7)

	summary := fmt.Sprintf("%d components scored, %d cohesion cluster(s)",
		len(scores), len(clusters))

	return &ConsolidationReport{
		IndependenceScores: scores,
		Clusters:           clusters,
		Summary:            summary,
	}
}

// simEdge satisfies graph.Edge for ConnectedComponents.
type simEdge struct{ from, to string }

func (e simEdge) Source() string { return e.from }
func (e simEdge) Target() string { return e.to }

func jaccard(a, b map[string]bool) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 0
	}
	intersection := 0
	for k := range a {
		if b[k] {
			intersection++
		}
	}
	union := len(a) + len(b) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}
