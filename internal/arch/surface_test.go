package arch

import "testing"

func TestComputeAPISurface(t *testing.T) {
	m := ArchModel{
		Services: []ArchService{
			{Name: "small", Symbols: []string{"A"}},
			{Name: "big", Symbols: []string{"X", "Y", "Z", "W"}},
			{Name: "empty"},
		},
	}
	surfaces := ComputeAPISurface(m)
	if len(surfaces) != 3 {
		t.Fatalf("expected 3 surfaces, got %d", len(surfaces))
	}
	if surfaces[0].Component != "big" || surfaces[0].ExportedCount != 4 {
		t.Errorf("first should be big(4), got %s(%d)", surfaces[0].Component, surfaces[0].ExportedCount)
	}
	if surfaces[2].Component != "empty" || surfaces[2].ExportedCount != 0 {
		t.Errorf("last should be empty(0), got %s(%d)", surfaces[2].Component, surfaces[2].ExportedCount)
	}
}

func TestDetectBoundaryCrossings(t *testing.T) {
	m := ArchModel{
		Services: []ArchService{
			{Name: "api", TrustZone: "public"},
			{Name: "auth", TrustZone: "internal"},
			{Name: "store", TrustZone: "internal"},
			{Name: "lib"},
		},
		Edges: []ArchEdge{
			{From: "api", To: "auth"},
			{From: "auth", To: "store"},
			{From: "api", To: "lib"},
		},
	}

	crossings := DetectBoundaryCrossings(m, nil)
	if len(crossings) != 1 {
		t.Fatalf("expected 1 crossing, got %d: %v", len(crossings), crossings)
	}
	if crossings[0].From != "api" || crossings[0].To != "auth" {
		t.Errorf("expected api->auth, got %s->%s", crossings[0].From, crossings[0].To)
	}
}

func TestDetectBoundaryCrossings_Trusted(t *testing.T) {
	m := ArchModel{
		Services: []ArchService{
			{Name: "api", TrustZone: "public"},
			{Name: "auth", TrustZone: "internal"},
		},
		Edges: []ArchEdge{
			{From: "api", To: "auth"},
		},
	}

	crossings := DetectBoundaryCrossings(m, []string{"internal"})
	if len(crossings) != 0 {
		t.Errorf("expected 0 crossings with trusted internal, got %d", len(crossings))
	}
}
