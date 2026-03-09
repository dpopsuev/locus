package triage

import (
	"testing"
)

func seedRegistry() *Registry {
	r := New()
	r.Register(ToolMeta{
		Name:        "scan_project",
		Description: "Full architecture scan",
		Keywords:    []string{"scan", "architecture", "overview", "structure", "codebase"},
		Categories:  []string{"architecture", "onboarding"},
		DefaultArgs: map[string]any{"format": "summary"},
		Rationale:   map[string]string{"architecture": "Full codebase overview"},
		Priority:    1,
	})
	r.Register(ToolMeta{
		Name:        "get_hot_spots",
		Description: "High fan-in + high churn components",
		Keywords:    []string{"perf", "bottleneck", "hot", "slow", "risk", "churn"},
		Categories:  []string{"performance", "refactoring"},
		DefaultArgs: map[string]any{"top_n": 10},
		Rationale:   map[string]string{"performance": "High fan-in + high churn = likely bottleneck"},
		Priority:    1,
	})
	r.Register(ToolMeta{
		Name:        "get_coupling_table",
		Description: "Coupling metrics table",
		Keywords:    []string{"coupling", "fan", "blast", "depend"},
		Categories:  []string{"performance", "architecture", "dependencies"},
		DefaultArgs: map[string]any{"sort_by": "fan_in"},
		Rationale:   map[string]string{"performance": "Most depended-on = highest blast radius"},
		Priority:    2,
	})
	r.Register(ToolMeta{
		Name:        "get_cycles",
		Description: "Circular dependency detection",
		Keywords:    []string{"cycle", "circular", "loop", "deadlock"},
		Categories:  []string{"architecture", "dependencies"},
		Rationale:   map[string]string{"dependencies": "Circular deps cause build/deploy issues"},
		Priority:    1,
	})
	r.Register(ToolMeta{
		Name:        "get_api_surface",
		Description: "API surface and trust boundaries",
		Keywords:    []string{"api", "surface", "export", "boundary", "trust", "attack"},
		Categories:  []string{"security", "architecture"},
		Rationale:   map[string]string{"security": "Large API surface = larger attack surface"},
		Priority:    1,
	})
	return r
}

func TestTriage_KeywordMatch(t *testing.T) {
	r := seedRegistry()
	res := r.Triage("find performance bottlenecks", "")
	if res.Category != "performance" {
		t.Fatalf("expected category=performance, got %q", res.Category)
	}
	if len(res.Tools) == 0 {
		t.Fatal("expected at least one tool match")
	}
	if res.Tools[0].Name != "get_hot_spots" {
		t.Errorf("expected first tool=get_hot_spots, got %q", res.Tools[0].Name)
	}
	if res.Confidence <= 0 {
		t.Errorf("expected positive confidence, got %f", res.Confidence)
	}
	t.Logf("result: category=%s confidence=%.2f tools=%d", res.Category, res.Confidence, len(res.Tools))
}

func TestTriage_CategoryGrouping(t *testing.T) {
	r := seedRegistry()
	res := r.Triage("circular dependency cycle", "")
	if res.Category != "dependencies" && res.Category != "architecture" {
		t.Fatalf("expected category=dependencies or architecture, got %q", res.Category)
	}
	found := false
	for _, tm := range res.Tools {
		if tm.Name == "get_cycles" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected get_cycles in results")
	}
}

func TestTriage_Fallback(t *testing.T) {
	r := seedRegistry()
	res := r.Triage("tell me about quantum physics", "")
	if res.Category != "general" {
		t.Fatalf("expected fallback category=general, got %q", res.Category)
	}
	if len(res.Tools) != 1 || res.Tools[0].Name != "scan_project" {
		t.Fatalf("expected fallback to scan_project, got %v", res.Tools)
	}
	if res.Confidence != 0 {
		t.Errorf("expected zero confidence for fallback, got %f", res.Confidence)
	}
}

func TestTriage_PathInjection(t *testing.T) {
	r := seedRegistry()
	res := r.Triage("hot spots", "/my/repo")
	if len(res.Tools) == 0 {
		t.Fatal("expected tool matches")
	}
	for _, tm := range res.Tools {
		p, ok := tm.Params["path"]
		if !ok || p != "/my/repo" {
			t.Errorf("tool %s: expected path=/my/repo, got %v", tm.Name, tm.Params)
		}
	}
}

func TestTriage_PrefixMatch(t *testing.T) {
	r := seedRegistry()
	res := r.Triage("perf issues", "")
	if res.Category != "performance" {
		t.Fatalf("expected performance category from prefix match, got %q", res.Category)
	}
}

func TestTriage_EmptyIntent(t *testing.T) {
	r := seedRegistry()
	res := r.Triage("", "")
	if res.Category != "general" {
		t.Fatalf("expected fallback on empty intent, got %q", res.Category)
	}
}

func TestList(t *testing.T) {
	r := seedRegistry()
	tools := r.List()
	if len(tools) != 5 {
		t.Fatalf("expected 5 tools, got %d", len(tools))
	}
}

func TestByCategory(t *testing.T) {
	r := seedRegistry()
	perf := r.ByCategory("performance")
	if len(perf) != 2 {
		t.Fatalf("expected 2 performance tools, got %d", len(perf))
	}
	names := make(map[string]bool)
	for _, t := range perf {
		names[t.Name] = true
	}
	if !names["get_hot_spots"] || !names["get_coupling_table"] {
		t.Errorf("expected get_hot_spots and get_coupling_table, got %v", names)
	}
}

func TestTriage_MultiToolChain(t *testing.T) {
	r := seedRegistry()
	res := r.Triage("slow bottleneck coupling blast radius", "")
	if len(res.Tools) < 2 {
		t.Fatalf("expected multi-tool chain for broad performance query, got %d tools", len(res.Tools))
	}
	if res.Category != "performance" {
		t.Errorf("expected performance category, got %q", res.Category)
	}
	t.Logf("chain: %d tools in category=%s", len(res.Tools), res.Category)
	for _, tm := range res.Tools {
		t.Logf("  - %s: %s", tm.Name, tm.Reason)
	}
}
