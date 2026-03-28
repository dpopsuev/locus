package protocol

import (
	"testing"

	"github.com/dpopsuev/locus/internal/analysis"
)

func TestComputeDiffIntelligence_SingleFileWithCallers(t *testing.T) {
	graph := &analysis.CallGraph{
		Nodes: []analysis.FuncNode{
			{Name: "DoWork", Package: "internal/core"},
			{Name: "Helper", Package: "internal/core"},
			{Name: "HandleRequest", Package: "internal/api"},
		},
		Edges: []analysis.CallEdge{
			{Caller: "HandleRequest", Callee: "DoWork", CallerPkg: "internal/api", CalleePkg: "internal/core"},
			{Caller: "Helper", Callee: "DoWork", CallerPkg: "internal/core", CalleePkg: "internal/core"},
		},
	}

	report := ComputeDiffIntelligence(
		[]string{"internal/core/work.go"},
		"",
		graph,
	)

	if len(report.ChangedPkgs) != 1 || report.ChangedPkgs[0] != "internal/core" {
		t.Fatalf("expected [internal/core], got %v", report.ChangedPkgs)
	}
	if len(report.SemanticChanges) != 1 {
		t.Fatalf("expected 1 semantic change, got %d", len(report.SemanticChanges))
	}
	sc := report.SemanticChanges[0]
	if sc.Symbol != "DoWork" {
		t.Errorf("symbol = %s, want DoWork", sc.Symbol)
	}
	if sc.AffectedCount != 2 {
		t.Errorf("affected_count = %d, want 2", sc.AffectedCount)
	}
	if sc.Severity != "medium" {
		t.Errorf("severity = %s, want medium", sc.Severity)
	}
	if sc.ChangeType != "modified" {
		t.Errorf("change_type = %s, want modified", sc.ChangeType)
	}
}

func TestComputeDiffIntelligence_NoExportedSymbols(t *testing.T) {
	// Symbols in the graph are in a different package than the changed file.
	graph := &analysis.CallGraph{
		Nodes: []analysis.FuncNode{
			{Name: "Unrelated", Package: "internal/other"},
		},
		Edges: []analysis.CallEdge{
			{Caller: "Main", Callee: "Unrelated", CallerPkg: "cmd", CalleePkg: "internal/other"},
		},
	}

	report := ComputeDiffIntelligence(
		[]string{"internal/util/helpers.go"},
		"",
		graph,
	)

	if len(report.ChangedPkgs) != 1 || report.ChangedPkgs[0] != "internal/util" {
		t.Fatalf("expected [internal/util], got %v", report.ChangedPkgs)
	}
	if len(report.SemanticChanges) != 0 {
		t.Fatalf("expected 0 semantic changes, got %d", len(report.SemanticChanges))
	}
}

func TestComputeDiffIntelligence_HighFanInCritical(t *testing.T) {
	nodes := []analysis.FuncNode{
		{Name: "Log", Package: "pkg/logger"},
	}
	edges := make([]analysis.CallEdge, 0, 15)
	for i := range 15 {
		edges = append(edges, analysis.CallEdge{
			Caller:    "Caller" + string(rune('A'+i)),
			Callee:    "Log",
			CallerPkg: "pkg/consumers",
			CalleePkg: "pkg/logger",
		})
	}
	graph := &analysis.CallGraph{Nodes: nodes, Edges: edges}

	report := ComputeDiffIntelligence(
		[]string{"pkg/logger/log.go"},
		"",
		graph,
	)

	if len(report.SemanticChanges) != 1 {
		t.Fatalf("expected 1 semantic change, got %d", len(report.SemanticChanges))
	}
	sc := report.SemanticChanges[0]
	if sc.AffectedCount != 15 {
		t.Errorf("affected_count = %d, want 15", sc.AffectedCount)
	}
	if sc.Severity != "critical" {
		t.Errorf("severity = %s, want critical", sc.Severity)
	}
}

func TestComputeDiffIntelligence_NoChangedFiles(t *testing.T) {
	graph := &analysis.CallGraph{
		Nodes: []analysis.FuncNode{{Name: "Foo", Package: "pkg"}},
		Edges: []analysis.CallEdge{{Caller: "Bar", Callee: "Foo"}},
	}

	report := ComputeDiffIntelligence(nil, "", graph)

	if len(report.ChangedFiles) != 0 {
		t.Fatalf("expected 0 changed files, got %d", len(report.ChangedFiles))
	}
	if len(report.ChangedPkgs) != 0 {
		t.Fatalf("expected 0 changed packages, got %d", len(report.ChangedPkgs))
	}
	if len(report.SemanticChanges) != 0 {
		t.Fatalf("expected 0 semantic changes, got %d", len(report.SemanticChanges))
	}
	if report.Summary != "0 files changed across 0 packages, 0 symbols with callers affected" {
		t.Errorf("unexpected summary: %s", report.Summary)
	}
}

func TestComputeDiffIntelligence_MultiplePackages(t *testing.T) {
	graph := &analysis.CallGraph{
		Nodes: []analysis.FuncNode{
			{Name: "Parse", Package: "internal/parser"},
			{Name: "Validate", Package: "internal/validator"},
			{Name: "Transform", Package: "internal/parser"},
		},
		Edges: []analysis.CallEdge{
			{Caller: "Run", Callee: "Parse", CallerPkg: "cmd", CalleePkg: "internal/parser"},
			{Caller: "Run", Callee: "Validate", CallerPkg: "cmd", CalleePkg: "internal/validator"},
			{Caller: "Parse", Callee: "Transform", CallerPkg: "internal/parser", CalleePkg: "internal/parser"},
			{Caller: "Validate", Callee: "Transform", CallerPkg: "internal/validator", CalleePkg: "internal/parser"},
		},
	}

	report := ComputeDiffIntelligence(
		[]string{
			"internal/parser/parse.go",
			"internal/validator/check.go",
		},
		"",
		graph,
	)

	if len(report.ChangedPkgs) != 2 {
		t.Fatalf("expected 2 changed packages, got %d: %v", len(report.ChangedPkgs), report.ChangedPkgs)
	}

	// Both packages have symbols with callers.
	if len(report.SemanticChanges) < 2 {
		t.Fatalf("expected at least 2 semantic changes, got %d", len(report.SemanticChanges))
	}

	// Should be sorted by affected count descending.
	for i := 1; i < len(report.SemanticChanges); i++ {
		if report.SemanticChanges[i].AffectedCount > report.SemanticChanges[i-1].AffectedCount {
			t.Errorf("semantic changes not sorted by affected_count descending at index %d", i)
		}
	}

	// Verify summary mentions correct counts.
	expected := "2 files changed across 2 packages"
	if report.Summary[:len(expected)] != expected {
		t.Errorf("summary prefix = %q, want %q", report.Summary[:len(expected)], expected)
	}
}

func TestComputeDiffIntelligence_ModulePathStripped(t *testing.T) {
	graph := &analysis.CallGraph{
		Nodes: []analysis.FuncNode{
			{Name: "Init", Package: "internal/app"},
		},
		Edges: []analysis.CallEdge{
			{Caller: "Main", Callee: "Init", CallerPkg: "cmd", CalleePkg: "internal/app"},
		},
	}

	report := ComputeDiffIntelligence(
		[]string{"github.com/example/myproject/internal/app/init.go"},
		"github.com/example/myproject",
		graph,
	)

	if len(report.ChangedPkgs) != 1 || report.ChangedPkgs[0] != "internal/app" {
		t.Fatalf("expected [internal/app] after stripping module path, got %v", report.ChangedPkgs)
	}
	if len(report.SemanticChanges) != 1 {
		t.Fatalf("expected 1 semantic change, got %d", len(report.SemanticChanges))
	}
}

func TestComputeDiffIntelligence_NilGraph(t *testing.T) {
	report := ComputeDiffIntelligence([]string{"some/file.go"}, "", nil)

	if len(report.ChangedFiles) != 1 {
		t.Fatalf("expected 1 changed file, got %d", len(report.ChangedFiles))
	}
	if len(report.SemanticChanges) != 0 {
		t.Fatalf("expected 0 semantic changes with nil graph, got %d", len(report.SemanticChanges))
	}
}

func TestComputeDiffIntelligence_ZeroCallersExcluded(t *testing.T) {
	// Symbol is in a changed package but has zero callers — should NOT appear.
	graph := &analysis.CallGraph{
		Nodes: []analysis.FuncNode{
			{Name: "Unused", Package: "internal/core"},
			{Name: "Used", Package: "internal/core"},
		},
		Edges: []analysis.CallEdge{
			{Caller: "Main", Callee: "Used", CallerPkg: "cmd", CalleePkg: "internal/core"},
		},
	}

	report := ComputeDiffIntelligence(
		[]string{"internal/core/thing.go"},
		"",
		graph,
	)

	if len(report.SemanticChanges) != 1 {
		t.Fatalf("expected 1 semantic change (Unused should be excluded), got %d", len(report.SemanticChanges))
	}
	if report.SemanticChanges[0].Symbol != "Used" {
		t.Errorf("symbol = %s, want Used", report.SemanticChanges[0].Symbol)
	}
}

func TestCallerSeverity(t *testing.T) {
	tests := []struct {
		count int
		want  string
	}{
		{0, "low"},
		{1, "medium"},
		{3, "medium"},
		{4, "high"},
		{10, "high"},
		{11, "critical"},
		{100, "critical"},
	}
	for _, tt := range tests {
		got := callerSeverity(tt.count)
		if got != tt.want {
			t.Errorf("callerSeverity(%d) = %s, want %s", tt.count, got, tt.want)
		}
	}
}

func TestComputeDiffIntelligence_RootPackage(t *testing.T) {
	graph := &analysis.CallGraph{
		Nodes: []analysis.FuncNode{
			{Name: "Main", Package: "(root)"},
		},
		Edges: []analysis.CallEdge{
			{Caller: "Test", Callee: "Main", CallerPkg: "(root)", CalleePkg: "(root)"},
		},
	}

	report := ComputeDiffIntelligence(
		[]string{"main.go"},
		"",
		graph,
	)

	if len(report.ChangedPkgs) != 1 || report.ChangedPkgs[0] != "(root)" {
		t.Fatalf("expected [(root)] for root-level file, got %v", report.ChangedPkgs)
	}
	if len(report.SemanticChanges) != 1 {
		t.Fatalf("expected 1 semantic change, got %d", len(report.SemanticChanges))
	}
}
