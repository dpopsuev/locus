package arch

import "sort"

// APISurface measures the public API size of a component.
type APISurface struct {
	Component     string `json:"component"`
	ExportedCount int    `json:"exported_count"`
}

// BoundaryCrossing flags an edge that crosses trust zone boundaries.
type BoundaryCrossing struct {
	From     string `json:"from"`
	To       string `json:"to"`
	FromZone string `json:"from_zone"`
	ToZone   string `json:"to_zone"`
}

// ComputeAPISurface returns the exported symbol count per component.
func ComputeAPISurface(m ArchModel) []APISurface {
	surfaces := make([]APISurface, 0, len(m.Services))
	for i := range m.Services {
		svc := &m.Services[i]
		surfaces = append(surfaces, APISurface{
			Component:     svc.Name,
			ExportedCount: len(svc.Symbols),
		})
	}
	sort.Slice(surfaces, func(i, j int) bool {
		return surfaces[i].ExportedCount > surfaces[j].ExportedCount
	})
	return surfaces
}

// DetectBoundaryCrossings finds edges that cross trust zone boundaries.
// If trusted is non-empty, only crossings into non-trusted zones are reported.
func DetectBoundaryCrossings(m ArchModel, trusted []string) []BoundaryCrossing {
	svcZone := make(map[string]string, len(m.Services))
	for i := range m.Services {
		svc := &m.Services[i]
		if svc.TrustZone != "" {
			svcZone[svc.Name] = svc.TrustZone
		}
	}

	trustedSet := make(map[string]bool, len(trusted))
	for _, t := range trusted {
		trustedSet[t] = true
	}

	crossings := make([]BoundaryCrossing, 0, len(m.Edges))
	for _, e := range m.Edges {
		fromZone := svcZone[e.From]
		toZone := svcZone[e.To]
		if fromZone == "" || toZone == "" || fromZone == toZone {
			continue
		}
		if len(trustedSet) > 0 && trustedSet[toZone] {
			continue
		}
		crossings = append(crossings, BoundaryCrossing{
			From:     e.From,
			To:       e.To,
			FromZone: fromZone,
			ToZone:   toZone,
		})
	}
	return crossings
}
