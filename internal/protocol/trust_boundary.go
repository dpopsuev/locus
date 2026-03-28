package protocol

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dpopsuev/locus/internal/arch"
)

// TrustZoneInfo describes which trust zone a component belongs to and why.
type TrustZoneInfo struct {
	Component string `json:"component"`
	Zone      string `json:"zone"`
	Reason    string `json:"reason"`
}

// TrustBoundaryReport holds trust boundary detection results.
type TrustBoundaryReport struct {
	Zones     []TrustZoneInfo `json:"zones"`
	Crossings int             `json:"boundary_crossings"`
	Summary   string          `json:"summary"`
}

// ComputeTrustBoundaries infers trust zones from package name patterns and edge targets,
// then counts boundary crossings between different zones.
func ComputeTrustBoundaries(services []arch.ArchService, edges []arch.ArchEdge) *TrustBoundaryReport {
	// Build a set of edge targets per component for boundary detection.
	edgeTargets := make(map[string]map[string]bool)
	for _, e := range edges {
		if edgeTargets[e.From] == nil {
			edgeTargets[e.From] = make(map[string]bool)
		}
		edgeTargets[e.From][e.To] = true
	}

	// Classify each component into a trust zone.
	zoneMap := make(map[string]string, len(services))
	zones := make([]TrustZoneInfo, 0, len(services))
	for i := range services {
		svc := &services[i]
		zone, reason := inferTrustZone(svc.Name, edgeTargets[svc.Name])
		zoneMap[svc.Name] = zone
		zones = append(zones, TrustZoneInfo{
			Component: svc.Name,
			Zone:      zone,
			Reason:    reason,
		})
	}

	sort.Slice(zones, func(i, j int) bool {
		if zones[i].Zone != zones[j].Zone {
			return zones[i].Zone < zones[j].Zone
		}
		return zones[i].Component < zones[j].Component
	})

	// Count boundary crossings: edges where from-zone != to-zone.
	crossings := 0
	for _, e := range edges {
		fromZone := zoneMap[e.From]
		toZone := zoneMap[e.To]
		if fromZone != "" && toZone != "" && fromZone != toZone {
			crossings++
		}
	}

	// Count zones.
	zoneCounts := make(map[string]int)
	for _, z := range zones {
		zoneCounts[z.Zone]++
	}

	summary := fmt.Sprintf("%d component(s) in %d zone(s), %d boundary crossing(s)",
		len(zones), len(zoneCounts), crossings)

	return &TrustBoundaryReport{
		Zones:     zones,
		Crossings: crossings,
		Summary:   summary,
	}
}

// inferTrustZone classifies a component name into a trust zone.
func inferTrustZone(name string, targets map[string]bool) (zone, reason string) {
	lower := strings.ToLower(name)

	switch {
	case strings.Contains(lower, "cmd") || strings.HasPrefix(lower, "cmd/"):
		return "entrypoint", "cmd package"

	case strings.Contains(lower, "internal"):
		return "internal", "internal package path"

	case containsAny(lower, "store", "repo", "db", "database", "persist", "cache"):
		return "data", "data/storage package"

	case hasBoundaryTarget(targets):
		return "boundary", "imports network/RPC packages"

	default:
		return "domain", "general domain logic"
	}
}

// containsAny returns true if s contains any of the given substrings.
func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// hasBoundaryTarget checks if any edge target suggests network/RPC usage.
func hasBoundaryTarget(targets map[string]bool) bool {
	boundaryPrefixes := []string{"net/http", "grpc", "net/rpc", "http", "api", "server", "handler"}
	for t := range targets {
		lower := strings.ToLower(t)
		for _, prefix := range boundaryPrefixes {
			if strings.Contains(lower, prefix) {
				return true
			}
		}
	}
	return false
}
