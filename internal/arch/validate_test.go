package arch

import (
	"testing"
)

func TestValidateArchitecture_Identical(t *testing.T) {
	m := ArchModel{
		Services: []ArchService{{Name: "a"}, {Name: "b"}},
		Edges:    []ArchEdge{{From: "a", To: "b"}},
	}
	drift := ValidateArchitecture(m, m)
	if len(drift.MissingComponents) != 0 || len(drift.ExtraComponents) != 0 {
		t.Errorf("expected no component drift, got %+v", drift)
	}
	if len(drift.MissingEdges) != 0 || len(drift.ExtraEdges) != 0 {
		t.Errorf("expected no edge drift, got %+v", drift)
	}
}

func TestValidateArchitecture_Drift(t *testing.T) {
	desired := ArchModel{
		Services: []ArchService{{Name: "a"}, {Name: "b"}, {Name: "c"}},
		Edges:    []ArchEdge{{From: "a", To: "b"}, {From: "b", To: "c"}},
	}
	actual := ArchModel{
		Services: []ArchService{{Name: "a"}, {Name: "b"}, {Name: "d"}},
		Edges:    []ArchEdge{{From: "a", To: "b"}, {From: "a", To: "d"}},
	}
	drift := ValidateArchitecture(desired, actual)
	if len(drift.MissingComponents) != 1 || drift.MissingComponents[0] != "c" {
		t.Errorf("expected missing [c], got %v", drift.MissingComponents)
	}
	if len(drift.ExtraComponents) != 1 || drift.ExtraComponents[0] != "d" {
		t.Errorf("expected extra [d], got %v", drift.ExtraComponents)
	}
	if len(drift.MissingEdges) != 1 {
		t.Errorf("expected 1 missing edge, got %d", len(drift.MissingEdges))
	}
	if len(drift.ExtraEdges) != 1 {
		t.Errorf("expected 1 extra edge, got %d", len(drift.ExtraEdges))
	}
}

func TestParseDesiredState_Mermaid(t *testing.T) {
	input := `graph TD
    A["api"]
    B["store"]
    A --> B
`
	m, err := ParseDesiredState(input, "mermaid")
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Services) != 2 {
		t.Errorf("expected 2 services, got %d", len(m.Services))
	}
	if len(m.Edges) != 1 {
		t.Errorf("expected 1 edge, got %d", len(m.Edges))
	}
	if m.Edges[0].From != "api" || m.Edges[0].To != "store" {
		t.Errorf("expected api->store, got %s->%s", m.Edges[0].From, m.Edges[0].To)
	}
}

func TestParseDesiredState_JSON(t *testing.T) {
	input := `{"services":[{"Name":"x"},{"Name":"y"}],"edges":[{"From":"x","To":"y"}]}`
	m, err := ParseDesiredState(input, "json")
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Services) != 2 {
		t.Fatalf("expected 2 services, got %d", len(m.Services))
	}
	if len(m.Edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(m.Edges))
	}
}

func TestParseDesiredState_Empty(t *testing.T) {
	_, err := ParseDesiredState("", "mermaid")
	if err == nil {
		t.Error("expected error for empty input")
	}
}
