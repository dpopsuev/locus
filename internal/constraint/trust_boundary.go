package constraint

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dpopsuev/locus/internal/arch"
)

// Trust zone names.
const (
	zoneEntrypoint = "entrypoint"
	zoneBoundary   = "boundary"
	zoneDomain     = "domain"
	zoneData       = "data"
	zoneInfra      = "infra"
	zonePort       = "port"
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
// If desiredRoles is non-nil, components with explicit role assignments are classified
// by their desired role instead of name heuristics (BUG-25 + BUG-27).
func ComputeTrustBoundaries(services []arch.ArchService, edges []arch.ArchEdge, desiredRoles map[string]string) *TrustBoundaryReport {
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
		zone, reason := inferTrustZone(svc.Name, edgeTargets[svc.Name], desiredRoles)
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

// inferTrustZone classifies a component into a trust zone.
// Primary: uses hexa role from desiredRoles (auto-detected or manually set).
// Fallback: structural signals (same as hexa classification — no keywords).
func inferTrustZone(name string, targets map[string]bool, desiredRoles map[string]string) (zone, reason string) {
	// Use hexa role if available (auto-classified or desired_state override).
	if desiredRoles != nil {
		if role, ok := desiredRoles[name]; ok {
			return roleToZone(role), "hexa role: " + role
		}
	}

	// Structural fallback: boundary target detection (language-agnostic).
	if hasBoundaryTarget(targets) {
		return zoneBoundary, "imports boundary packages"
	}

	return zoneDomain, "default classification"
}

// roleToZone maps a hexagonal role to a trust zone.
func roleToZone(role string) string {
	switch strings.ToLower(role) {
	case "entrypoint", "entry":
		return zoneEntrypoint
	case "adapter", "boundary":
		return zoneBoundary
	case "infra", "infrastructure":
		return zoneInfra
	case "port":
		return zonePort
	case "data", "store", "repo":
		return zoneData
	default:
		return zoneDomain
	}
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
