package diagram

import (
	"strings"
	"testing"
	"time"

	"github.com/dpopsuev/locus/internal/analysis"
	"github.com/dpopsuev/locus/internal/arch"
	"github.com/dpopsuev/locus/internal/history"
)

func testReport() *arch.ContextReport {
	return &arch.ContextReport{
		ModulePath:     "github.com/example/project",
		Scanner:        "test",
		SuggestedDepth: 1,
		Architecture: arch.ArchModel{
			Title: "project",
			Services: []arch.ArchService{
				{Name: "cmd/app", Package: "github.com/example/project/cmd/app", Churn: 5, Symbols: []string{"main"}},
				{Name: "internal/core", Package: "github.com/example/project/internal/core", Churn: 20, Symbols: []string{"Run", "Config", "New"}},
				{Name: "internal/store", Package: "github.com/example/project/internal/store", Churn: 8, Symbols: []string{"DB", "Get", "Put"}},
				{Name: "internal/api", Package: "github.com/example/project/internal/api", Churn: 12, Symbols: []string{"Handler", "Router"}},
				{Name: "pkg/util", Package: "github.com/example/project/pkg/util", Churn: 2, Symbols: []string{"Must"}},
			},
			Edges: []arch.ArchEdge{
				{From: "cmd/app", To: "internal/core", Weight: 1, CallSites: 3, LOCSurface: 10},
				{From: "cmd/app", To: "internal/api", Weight: 1, CallSites: 2, LOCSurface: 5},
				{From: "internal/api", To: "internal/core", Weight: 2, CallSites: 8, LOCSurface: 25},
				{From: "internal/api", To: "internal/store", Weight: 1, CallSites: 4, LOCSurface: 12},
				{From: "internal/core", To: "internal/store", Weight: 1, CallSites: 5, LOCSurface: 15},
				{From: "internal/core", To: "pkg/util", Weight: 1, CallSites: 2, LOCSurface: 3},
			},
		},
		HotSpots: []arch.HotSpot{
			{Component: "internal/core", FanIn: 2, Churn: 20},
		},
		ImportDepth: arch.DepthMap{
			"cmd/app":        0,
			"internal/api":   1,
			"internal/core":  2,
			"internal/store": 3,
			"pkg/util":       3,
		},
		LayerViolations: []arch.LayerViolation{
			{From: "internal/store", To: "internal/api", FromLayer: "3", ToLayer: "1"},
		},
	}
}

func testHistory() []history.EntrySummary {
	base := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	return []history.EntrySummary{
		{Timestamp: base, HeadSHA: "aaa", Components: 3, Edges: 4},
		{Timestamp: base.AddDate(0, 0, 1), HeadSHA: "bbb", Components: 4, Edges: 5},
		{Timestamp: base.AddDate(0, 0, 2), HeadSHA: "ccc", Components: 5, Edges: 6},
	}
}

func TestRenderDependency(t *testing.T) {
	out, err := Render(Input{Report: testReport()}, Options{Type: "dependency"})
	if err != nil {
		t.Fatal(err)
	}
	assertMermaidType(t, out, "graph TD")
	assertContains(t, out, "cmd_app")
	assertContains(t, out, "internal_core")
	assertContains(t, out, "-->")
}

func TestRenderDependencyScoped(t *testing.T) {
	out, err := Render(Input{Report: testReport()}, Options{Type: "dependency", Scope: "internal/core"})
	if err != nil {
		t.Fatal(err)
	}
	assertMermaidType(t, out, "graph TD")
	assertContains(t, out, "internal_core")
	if strings.Contains(out, "internal_api") {
		t.Log("internal/api is a neighbor of internal/core, correctly included")
	}
}

func TestRenderC4(t *testing.T) {
	out, err := Render(Input{Report: testReport()}, Options{Type: "c4"})
	if err != nil {
		t.Fatal(err)
	}
	assertMermaidType(t, out, "C4Component")
	assertContains(t, out, "Container_Boundary")
	assertContains(t, out, "Component")
	assertContains(t, out, "Rel")
}

func TestRenderCoupling(t *testing.T) {
	out, err := Render(Input{Report: testReport()}, Options{Type: "coupling"})
	if err != nil {
		t.Fatal(err)
	}
	assertMermaidType(t, out, "sankey-beta")
	assertContains(t, out, "internal/api")
	assertContains(t, out, "internal/core")
}

func TestRenderCouplingTopN(t *testing.T) {
	out, err := Render(Input{Report: testReport()}, Options{Type: "coupling", TopN: 2})
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	dataLines := 0
	for _, l := range lines {
		if !strings.HasPrefix(l, "---") && !strings.HasPrefix(l, "config") && !strings.HasPrefix(l, "  ") && !strings.HasPrefix(l, "sankey") && !strings.HasPrefix(l, "%%") && l != "" {
			dataLines++
		}
	}
	if dataLines > 2 {
		t.Errorf("expected at most 2 data lines, got %d", dataLines)
	}
}

func TestRenderChurnBar(t *testing.T) {
	out, err := Render(Input{Report: testReport()}, Options{Type: "churn"})
	if err != nil {
		t.Fatal(err)
	}
	assertMermaidType(t, out, "xychart-beta")
	assertContains(t, out, "bar")
	assertContains(t, out, "internal/core")
}

func TestRenderChurnTimeline(t *testing.T) {
	out, err := Render(Input{Report: testReport(), History: testHistory()}, Options{Type: "churn"})
	if err != nil {
		t.Fatal(err)
	}
	assertMermaidType(t, out, "xychart-beta")
	assertContains(t, out, "line")
	assertContains(t, out, "Mar 01")
}

func TestRenderLayers(t *testing.T) {
	out, err := Render(Input{Report: testReport()}, Options{Type: "layers"})
	if err != nil {
		t.Fatal(err)
	}
	assertMermaidType(t, out, "block-beta")
	assertContains(t, out, "block:")
	assertContains(t, out, "cmd_app")
}

func TestRenderTree(t *testing.T) {
	out, err := Render(Input{Report: testReport()}, Options{Type: "tree"})
	if err != nil {
		t.Fatal(err)
	}
	assertMermaidType(t, out, "mindmap")
	assertContains(t, out, "root")
	assertContains(t, out, "project")
	assertContains(t, out, "internal")
}

func TestRenderUnknownType(t *testing.T) {
	_, err := Render(Input{Report: testReport()}, Options{Type: "unknown"})
	if err == nil {
		t.Fatal("expected error for unknown type")
	}
}

func TestTypes(t *testing.T) {
	types := Types()
	if len(types) != 15 {
		t.Fatalf("expected 15 types, got %d: %v", len(types), types)
	}
}

// --- Tier 3 diagram tests with mock DeepAnalyzer ---

type mockDeepAnalyzer struct{}

func (m *mockDeepAnalyzer) CallGraph(_ string, _ analysis.CallGraphOpts) (*analysis.CallGraph, error) {
	return &analysis.CallGraph{
		Nodes: []analysis.FuncNode{
			{Name: "main", Package: "cmd/app", Line: 10},
			{Name: "Run", Package: "internal/core", Line: 15},
			{Name: "Get", Package: "internal/store", Line: 20},
		},
		Edges: []analysis.CallEdge{
			{Caller: "main", Callee: "Run", CallerPkg: "cmd/app", CalleePkg: "internal/core", CrossPkg: true},
			{Caller: "Run", Callee: "Get", CallerPkg: "internal/core", CalleePkg: "internal/store", CrossPkg: true},
		},
		Layer: "mock",
	}, nil
}

func (m *mockDeepAnalyzer) DataFlowTrace(_, _ string, _ int) (*analysis.DataFlow, error) {
	return &analysis.DataFlow{
		Nodes: []analysis.DataFlowNode{
			{Name: "main", Kind: "entry"},
			{Name: "HandleRequest", Kind: "process", Pkg: "internal/api"},
			{Name: "SQL Database", Kind: "data_store"},
		},
		Edges: []analysis.DataFlowEdge{
			{From: "main", To: "HandleRequest"},
			{From: "HandleRequest", To: "SQL Database", Label: "Query"},
		},
		Boundaries: []analysis.TrustBoundary{
			{Name: "Auth Boundary", Nodes: []string{"HandleRequest"}},
		},
		Layer: "mock",
	}, nil
}

func (m *mockDeepAnalyzer) DetectStateMachines(_ string) ([]analysis.StateMachine, error) {
	return []analysis.StateMachine{
		{
			Name:    "OrderStatus",
			Package: "internal/order",
			States:  []string{"Pending", "Processing", "Shipped", "Delivered"},
			Transitions: []analysis.StateTransition{
				{From: "Pending", To: "Processing", Trigger: "confirm"},
				{From: "Processing", To: "Shipped", Trigger: "ship"},
				{From: "Shipped", To: "Delivered", Trigger: "deliver"},
			},
			Initial: "Pending",
		},
	}, nil
}

func TestRenderCallGraph(t *testing.T) {
	in := Input{Report: testReport(), DeepAnalyzer: &mockDeepAnalyzer{}}
	out, err := Render(in, Options{Type: "callgraph"})
	if err != nil {
		t.Fatal(err)
	}
	assertMermaidType(t, out, "flowchart TB")
	assertContains(t, out, "main")
	assertContains(t, out, "Run")
	assertContains(t, out, "-.->") // cross-package dotted edge
}

func TestRenderDataflow(t *testing.T) {
	in := Input{Report: testReport(), DeepAnalyzer: &mockDeepAnalyzer{}}
	out, err := Render(in, Options{Type: "dataflow", Entry: "main"})
	if err != nil {
		t.Fatal(err)
	}
	assertMermaidType(t, out, "flowchart LR")
	assertContains(t, out, "HandleRequest")
	assertContains(t, out, "SQL Database")
	assertContains(t, out, "Auth Boundary")
}

func TestRenderState(t *testing.T) {
	in := Input{Report: testReport(), DeepAnalyzer: &mockDeepAnalyzer{}}
	out, err := Render(in, Options{Type: "state"})
	if err != nil {
		t.Fatal(err)
	}
	assertMermaidType(t, out, "stateDiagram-v2")
	assertContains(t, out, "Pending")
	assertContains(t, out, "Delivered")
	assertContains(t, out, "[*] --> Pending")
}

func TestRenderCallGraphNoAnalyzer(t *testing.T) {
	in := Input{Report: testReport()}
	_, err := Render(in, Options{Type: "callgraph"})
	if err == nil {
		t.Fatal("expected error without DeepAnalyzer")
	}
}

func TestRenderDataflowNoAnalyzer(t *testing.T) {
	in := Input{Report: testReport()}
	_, err := Render(in, Options{Type: "dataflow"})
	if err == nil {
		t.Fatal("expected error without DeepAnalyzer")
	}
}

func TestRenderStateNoAnalyzer(t *testing.T) {
	in := Input{Report: testReport()}
	_, err := Render(in, Options{Type: "state"})
	if err == nil {
		t.Fatal("expected error without DeepAnalyzer")
	}
}

// --- assertions ---

func assertMermaidType(t *testing.T, out, prefix string) {
	t.Helper()
	if !strings.Contains(out, prefix) {
		t.Errorf("output missing Mermaid type prefix %q:\n%s", prefix, truncateTest(out, 500))
	}
}

func assertContains(t *testing.T, out, substr string) {
	t.Helper()
	if !strings.Contains(out, substr) {
		t.Errorf("output missing %q:\n%s", substr, truncateTest(out, 500))
	}
}

func truncateTest(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	return s[:limit] + "..."
}
