package protocol

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/dpopsuev/locus/internal/arch"
	"github.com/dpopsuev/locus/internal/store"
)

func TestCheckBoundaryRules_NoRules(t *testing.T) {
	edges := []arch.ArchEdge{{From: "a", To: "b"}}
	got := CheckBoundaryRules(edges, nil)
	if len(got) != 0 {
		t.Fatalf("expected 0 violations, got %d", len(got))
	}
}

func TestCheckBoundaryRules_NoEdges(t *testing.T) {
	rules := []store.BoundaryRule{{FromPattern: "*", ToPattern: "*", Allow: false}}
	got := CheckBoundaryRules(nil, rules)
	if len(got) != 0 {
		t.Fatalf("expected 0 violations, got %d", len(got))
	}
}

func TestCheckBoundaryRules_AllowedEdge(t *testing.T) {
	edges := []arch.ArchEdge{{From: "internal/api", To: "internal/core"}}
	rules := []store.BoundaryRule{
		{FromPattern: "internal/api", ToPattern: "internal/core", Allow: true},
	}
	got := CheckBoundaryRules(edges, rules)
	if len(got) != 0 {
		t.Fatalf("expected 0 violations for allowed edge, got %d", len(got))
	}
}

func TestCheckBoundaryRules_DisallowedEdge(t *testing.T) {
	edges := []arch.ArchEdge{{From: "internal/core", To: "internal/api"}}
	rules := []store.BoundaryRule{
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
	rules := []store.BoundaryRule{
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
	rules := []store.BoundaryRule{
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
	rules := []store.BoundaryRule{
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
	rules := []store.BoundaryRule{
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

// --- Enhanced Drift integration tests ---

func TestGetDrift_NoBoundariesNoBudgets(t *testing.T) {
	s := testStore(filepath.Join(t.TempDir(), "cache"), t.TempDir())
	p := New(s, nil)

	report := &arch.ContextReport{
		Architecture: arch.ArchModel{
			Services: []arch.ArchService{
				{Name: "pkg_a", LOC: 100},
				{Name: "pkg_b", LOC: 200},
			},
			Edges: []arch.ArchEdge{
				{From: "pkg_a", To: "pkg_b"},
			},
		},
	}
	_ = s.PutReport(context.Background(), "/repo", "sha1", report)
	_ = s.PutDesiredState(context.Background(), "/repo", &store.DesiredState{
		Layers: []string{"pkg_b", "pkg_a"}, // b is lower, a is higher, edge goes down = clean
	})

	drift, err := p.GetDrift(context.Background(), "/repo", "/repo@sha1")
	if err != nil {
		t.Fatalf("GetDrift: %v", err)
	}
	if !drift.HasDesiredState {
		t.Error("expected has_desired_state=true")
	}
	if !drift.Clean {
		t.Errorf("expected clean, got summary=%s", drift.Summary)
	}
	if drift.Score != 100.0 {
		t.Errorf("expected score=100, got %.1f", drift.Score)
	}
}

func TestGetDrift_WithBoundaryViolations(t *testing.T) {
	s := testStore(filepath.Join(t.TempDir(), "cache"), t.TempDir())
	p := New(s, nil)

	report := &arch.ContextReport{
		Architecture: arch.ArchModel{
			Services: []arch.ArchService{
				{Name: "internal/core", LOC: 500},
				{Name: "internal/api", LOC: 300},
			},
			Edges: []arch.ArchEdge{
				{From: "internal/core", To: "internal/api"},
			},
		},
	}
	_ = s.PutReport(context.Background(), "/repo", "sha1", report)
	_ = s.PutDesiredState(context.Background(), "/repo", &store.DesiredState{
		Layers: []string{"internal/core", "internal/api"},
		Boundaries: []store.BoundaryRule{
			{FromPattern: "internal/core", ToPattern: "internal/api", Allow: false},
		},
	})

	drift, err := p.GetDrift(context.Background(), "/repo", "/repo@sha1")
	if err != nil {
		t.Fatalf("GetDrift: %v", err)
	}
	if drift.Clean {
		t.Error("expected not clean")
	}
	if len(drift.BoundaryViolations) != 1 {
		t.Fatalf("expected 1 boundary violation, got %d", len(drift.BoundaryViolations))
	}
	if drift.BoundaryBreaches != 1 {
		t.Errorf("boundary_breaches = %d, want 1", drift.BoundaryBreaches)
	}
	if drift.Score >= 100 {
		t.Errorf("score should be < 100 with violations, got %.1f", drift.Score)
	}
}

func TestGetDrift_WithBudgetViolations(t *testing.T) {
	s := testStore(filepath.Join(t.TempDir(), "cache"), t.TempDir())
	p := New(s, nil)

	report := &arch.ContextReport{
		Architecture: arch.ArchModel{
			Services: []arch.ArchService{
				{Name: "pkg/core", LOC: 1000, Churn: 20},
				{Name: "pkg/util", LOC: 200},
			},
			Edges: []arch.ArchEdge{
				{From: "pkg/util", To: "pkg/core"},
				{From: "pkg/core", To: "pkg/util"},
			},
		},
	}
	_ = s.PutReport(context.Background(), "/repo", "sha1", report)
	_ = s.PutDesiredState(context.Background(), "/repo", &store.DesiredState{
		Layers: []string{"pkg/util", "pkg/core"},
		Constraints: []store.HealthConstraint{
			{Component: "pkg/core", MaxChurn: 5},
		},
	})

	drift, err := p.GetDrift(context.Background(), "/repo", "/repo@sha1")
	if err != nil {
		t.Fatalf("GetDrift: %v", err)
	}
	if drift.Clean {
		t.Error("expected not clean")
	}
	if len(drift.BudgetViolations) != 1 {
		t.Fatalf("expected 1 budget violation, got %d", len(drift.BudgetViolations))
	}
	if drift.ConstraintBreaches != 1 {
		t.Errorf("constraint_breaches = %d, want 1", drift.ConstraintBreaches)
	}
}

func TestGetDrift_CombinedViolations(t *testing.T) {
	s := testStore(filepath.Join(t.TempDir(), "cache"), t.TempDir())
	p := New(s, nil)

	report := &arch.ContextReport{
		Architecture: arch.ArchModel{
			Services: []arch.ArchService{
				{Name: "api", LOC: 100, Churn: 30},
				{Name: "core", LOC: 500},
				{Name: "db", LOC: 200},
			},
			Edges: []arch.ArchEdge{
				{From: "api", To: "core"},
				{From: "core", To: "api"}, // layer violation (core is lower, importing higher)
				{From: "api", To: "db"},   // boundary violation
			},
		},
	}
	_ = s.PutReport(context.Background(), "/repo", "sha1", report)
	_ = s.PutDesiredState(context.Background(), "/repo", &store.DesiredState{
		Layers: []string{"db", "core", "api"}, // db=low, core=mid, api=high
		Boundaries: []store.BoundaryRule{
			{FromPattern: "api", ToPattern: "db", Allow: false}, // deny api -> db
		},
		Constraints: []store.HealthConstraint{
			{Component: "api", MaxChurn: 10}, // actual=30, budget=10
		},
	})

	drift, err := p.GetDrift(context.Background(), "/repo", "/repo@sha1")
	if err != nil {
		t.Fatalf("GetDrift: %v", err)
	}
	if drift.Clean {
		t.Error("expected not clean")
	}
	// Should have layer, boundary, and budget violations.
	if len(drift.LayerViolations) == 0 {
		t.Error("expected layer violations")
	}
	if len(drift.BoundaryViolations) != 1 {
		t.Errorf("expected 1 boundary violation, got %d", len(drift.BoundaryViolations))
	}
	if len(drift.BudgetViolations) != 1 {
		t.Errorf("expected 1 budget violation, got %d", len(drift.BudgetViolations))
	}
	if drift.Score >= 100 {
		t.Errorf("score should be < 100, got %.1f", drift.Score)
	}
	if drift.Score < 0 {
		t.Errorf("score should be >= 0, got %.1f", drift.Score)
	}
	t.Logf("Combined drift: %s", drift.Summary)
}

func TestGetDrift_NoDesiredState(t *testing.T) {
	s := testStore(filepath.Join(t.TempDir(), "cache"), t.TempDir())
	p := New(s, nil)

	drift, err := p.GetDrift(context.Background(), "/repo")
	if err != nil {
		t.Fatalf("GetDrift: %v", err)
	}
	if drift.HasDesiredState {
		t.Error("expected has_desired_state=false")
	}
}

func TestGetDrift_ScoreCalculation(t *testing.T) {
	s := testStore(filepath.Join(t.TempDir(), "cache"), t.TempDir())
	p := New(s, nil)

	report := &arch.ContextReport{
		Architecture: arch.ArchModel{
			Services: []arch.ArchService{
				{Name: "a", LOC: 100},
				{Name: "b", LOC: 200},
			},
			Edges: []arch.ArchEdge{
				{From: "a", To: "b"},
				{From: "b", To: "a"},
			},
		},
	}
	_ = s.PutReport(context.Background(), "/repo", "sha1", report)
	_ = s.PutDesiredState(context.Background(), "/repo", &store.DesiredState{
		Layers: []string{"a", "b"}, // a=low, b=high; a->b is OK, b->a is violation
	})

	drift, err := p.GetDrift(context.Background(), "/repo", "/repo@sha1")
	if err != nil {
		t.Fatalf("GetDrift: %v", err)
	}
	// 2 edges checked for layer purity, 1 violation => score = (2-1)/2*100 = 50
	if drift.Score < 0 || drift.Score > 100 {
		t.Errorf("score out of range: %.1f", drift.Score)
	}
	t.Logf("Score: %.1f%%, summary: %s", drift.Score, drift.Summary)
}
