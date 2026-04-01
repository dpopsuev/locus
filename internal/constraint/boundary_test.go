package constraint

import (
	"testing"

	"github.com/dpopsuev/locus/internal/arch"
	"github.com/dpopsuev/locus/internal/port"
)

func TestCheckBoundaryRules_NoRules(t *testing.T) {
	edges := []arch.ArchEdge{{From: "a", To: "b"}}
	got := CheckBoundaryRules(edges, nil)
	if len(got) != 0 {
		t.Fatalf("expected 0 violations, got %d", len(got))
	}
}

func TestCheckBoundaryRules_NoEdges(t *testing.T) {
	rules := []port.BoundaryRule{{FromPattern: "*", ToPattern: "*", Allow: false}}
	got := CheckBoundaryRules(nil, rules)
	if len(got) != 0 {
		t.Fatalf("expected 0 violations, got %d", len(got))
	}
}

func TestCheckBoundaryRules_AllowedEdge(t *testing.T) {
	edges := []arch.ArchEdge{{From: "internal/api", To: "internal/core"}}
	rules := []port.BoundaryRule{
		{FromPattern: "internal/api", ToPattern: "internal/core", Allow: true},
	}
	got := CheckBoundaryRules(edges, rules)
	if len(got) != 0 {
		t.Fatalf("expected 0 violations for allowed edge, got %d", len(got))
	}
}

func TestCheckBoundaryRules_DisallowedEdge(t *testing.T) {
	edges := []arch.ArchEdge{{From: "internal/core", To: "internal/api"}}
	rules := []port.BoundaryRule{
		{FromPattern: "internal/core", ToPattern: "internal/api", Allow: false},
	}
	got := CheckBoundaryRules(edges, rules)
	if len(got) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(got))
	}
	if got[0].From != "internal/core" || got[0].To != "internal/api" {
		t.Errorf("violation = %s -> %s, want internal/core -> internal/api", got[0].From, got[0].To)
	}
	if got[0].Severity != "error" {
		t.Errorf("severity = %s, want error", got[0].Severity)
	}
	if got[0].Rule != "internal/core -> internal/api" {
		t.Errorf("rule = %q, want %q", got[0].Rule, "internal/core -> internal/api")
	}
}

func TestCheckBoundaryRules_GlobPattern(t *testing.T) {
	edges := []arch.ArchEdge{
		{From: "internal/api", To: "internal/db"},
		{From: "internal/core", To: "internal/db"},
		{From: "cmd/server", To: "internal/db"},
	}
	// Deny anything matching internal/* from accessing internal/db.
	rules := []port.BoundaryRule{
		{FromPattern: "internal/*", ToPattern: "internal/db", Allow: false},
	}
	got := CheckBoundaryRules(edges, rules)
	// internal/api and internal/core match "internal/*", but cmd/server does not.
	if len(got) != 2 {
		t.Fatalf("expected 2 violations, got %d", len(got))
	}
}

func TestCheckBoundaryRules_SubstringMatch(t *testing.T) {
	edges := []arch.ArchEdge{
		{From: "pkg/handler/auth", To: "pkg/database/postgres"},
	}
	// Deny handler from accessing database (substring).
	rules := []port.BoundaryRule{
		{FromPattern: "handler", ToPattern: "database", Allow: false},
	}
	got := CheckBoundaryRules(edges, rules)
	if len(got) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(got))
	}
}

func TestCheckBoundaryRules_WildcardFrom(t *testing.T) {
	edges := []arch.ArchEdge{
		{From: "anything", To: "internal/secret"},
		{From: "other", To: "internal/secret"},
	}
	// Deny all access to internal/secret.
	rules := []port.BoundaryRule{
		{FromPattern: "*", ToPattern: "internal/secret", Allow: false},
	}
	got := CheckBoundaryRules(edges, rules)
	if len(got) != 2 {
		t.Fatalf("expected 2 violations, got %d", len(got))
	}
}

func TestCheckBoundaryRules_MultipleRules(t *testing.T) {
	edges := []arch.ArchEdge{
		{From: "internal/api", To: "internal/core"},
		{From: "internal/core", To: "internal/api"},
	}
	rules := []port.BoundaryRule{
		{FromPattern: "internal/api", ToPattern: "internal/core", Allow: true},  // allowed
		{FromPattern: "internal/core", ToPattern: "internal/api", Allow: false}, // denied
	}
	got := CheckBoundaryRules(edges, rules)
	if len(got) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(got))
	}
	if got[0].From != "internal/core" {
		t.Errorf("from = %s, want internal/core", got[0].From)
	}
}

func TestMatchPattern(t *testing.T) {
	tests := []struct {
		component string
		pattern   string
		want      bool
	}{
		{"anything", "", true},
		{"anything", "*", true},
		{"internal/api", "internal/*", true},
		{"internal/api", "internal/api", true},
		{"cmd/server", "internal/*", false},
		{"pkg/handler/auth", "handler", true},
		{"pkg/core", "handler", false},
	}
	for _, tt := range tests {
		got := matchPattern(tt.component, tt.pattern)
		if got != tt.want {
			t.Errorf("matchPattern(%q, %q) = %v, want %v", tt.component, tt.pattern, got, tt.want)
		}
	}
}
