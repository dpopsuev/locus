package impact

import (
	"errors"
	"fmt"
	"sort"

	"github.com/dpopsuev/locus/internal/arch"
)

// ErrComponentNotFound is returned when the target component doesn't exist.
var ErrComponentNotFound = errors.New("component not found")

// DependencyClass distinguishes how a consumer uses a dependency.
type DependencyClass string

const (
	// ClassBinary means the consumer imports but doesn't deeply use fields.
	ClassBinary DependencyClass = "binary"
	// ClassEnrichment means the consumer reads specific fields/values.
	ClassEnrichment DependencyClass = "enrichment"

	// enrichmentCallSiteRatio is the threshold for classifying enrichment.
	enrichmentCallSiteRatio = 2.0
	// enrichmentLOCSurface is the min LOC surface for enrichment classification.
	enrichmentLOCSurface = 5
	// enrichmentWeight is how much more enrichment consumers count vs binary.
	enrichmentWeight = 3
)

// LeverageConsumer describes one consumer of a target component.
type LeverageConsumer struct {
	Component string          `json:"component"`
	Class     DependencyClass `json:"class"`
	CallSites int             `json:"call_sites"`
	Reason    string          `json:"reason"`
}

// LeverageReport is the output of a leverage analysis.
type LeverageReport struct {
	Target         string             `json:"target"`
	TotalConsumers int                `json:"total_consumers"`
	Binary         int                `json:"binary_consumers"`
	Enrichment     int                `json:"enrichment_consumers"`
	Consumers      []LeverageConsumer `json:"consumers"`
	LeverageScore  int                `json:"leverage_score"`
	Summary        string             `json:"summary"`
}

// ComputeLeverage identifies all consumers of a target component and classifies
// each as binary (imports only) or enrichment (deeply reads data). Returns a
// leverage score indicating how much downstream improvement a target enhancement
// would create. Inverse of blast_radius: blast_radius = "what breaks",
// leverage = "what improves".
func ComputeLeverage(
	edges []arch.ArchEdge,
	services []arch.ArchService,
	target string,
) (*LeverageReport, error) {
	// Verify target exists.
	found := false
	for i := range services {
		if services[i].Name == target {
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("%w: %q", ErrComponentNotFound, target)
	}

	// Find all consumers: edges where To == target.
	consumers := make([]LeverageConsumer, 0, len(edges)/4)
	for i := range edges {
		e := &edges[i]
		if e.To != target {
			continue
		}
		class, reason := classifyConsumer(e)
		consumers = append(consumers, LeverageConsumer{
			Component: e.From,
			Class:     class,
			CallSites: e.CallSites,
			Reason:    reason,
		})
	}

	sort.Slice(consumers, func(i, j int) bool {
		if consumers[i].Class != consumers[j].Class {
			return consumers[i].Class == ClassEnrichment
		}
		return consumers[i].Component < consumers[j].Component
	})

	binary := 0
	enrichment := 0
	for i := range consumers {
		if consumers[i].Class == ClassEnrichment {
			enrichment++
		} else {
			binary++
		}
	}

	score := computeLeverageScore(enrichment, binary, len(services))

	summary := fmt.Sprintf("%s: %d consumer(s) (%d enrichment, %d binary), leverage score %d/100",
		target, len(consumers), enrichment, binary, score)

	return &LeverageReport{
		Target:         target,
		TotalConsumers: len(consumers),
		Binary:         binary,
		Enrichment:     enrichment,
		Consumers:      consumers,
		LeverageScore:  score,
		Summary:        summary,
	}, nil
}

func classifyConsumer(edge *arch.ArchEdge) (class DependencyClass, reason string) {
	if edge.CallSites > 0 {
		w := edge.Weight
		if w == 0 {
			w = 1
		}
		ratio := float64(edge.CallSites) / float64(w)
		if ratio >= enrichmentCallSiteRatio {
			return ClassEnrichment, fmt.Sprintf("high call density (%.1fx)", ratio)
		}
	}
	if edge.LOCSurface > enrichmentLOCSurface {
		return ClassEnrichment, fmt.Sprintf("high LOC surface (%d lines)", edge.LOCSurface)
	}
	if edge.Weight <= 1 {
		return ClassBinary, "single import"
	}
	return ClassEnrichment, fmt.Sprintf("multi-import (weight=%d)", edge.Weight)
}

func computeLeverageScore(enrichment, binary, totalComponents int) int {
	if totalComponents == 0 {
		return 0
	}
	weighted := enrichment*enrichmentWeight + binary
	maxPossible := totalComponents * enrichmentWeight
	score := weighted * 100 / maxPossible
	if score > 100 {
		score = 100
	}
	return score
}
