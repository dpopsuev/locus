package clinic

import (
	"testing"

	"github.com/dpopsuev/oculus"
)

func TestEnrichFeatureEnvy_WithTargets(t *testing.T) {
	report := &PatternScanReport{
		Detections: []PatternDetection{
			{
				PatternID: "feature_envy",
				Component: "pkg/handler",
				Evidence:  []string{"80% of call sites target pkg/repo"},
			},
		},
	}

	callEdges := []oculus.CallEdge{
		{Caller: "Process", CallerPkg: "pkg/handler", Callee: "Save", CalleePkg: "pkg/repo", CrossPkg: true},
		{Caller: "Process", CallerPkg: "pkg/handler", Callee: "Validate", CalleePkg: "pkg/repo", CrossPkg: true},
		{Caller: "Process", CallerPkg: "pkg/handler", Callee: "Log", CalleePkg: "pkg/util", CrossPkg: true},
		{Caller: "Init", CallerPkg: "pkg/handler", Callee: "NewDB", CalleePkg: "pkg/db", CrossPkg: true},
	}

	EnrichWithCallGraph(report, callEdges)

	d := report.Detections[0]
	if len(d.MoveTargets) == 0 {
		t.Fatal("expected move targets")
	}

	found := false
	for _, mt := range d.MoveTargets {
		if mt.Symbol == "Process" && mt.TargetPkg == "pkg/repo" {
			found = true
			if mt.CallPct < 0.6 {
				t.Errorf("expected CallPct > 0.6 for Process, got %f", mt.CallPct)
			}
		}
	}
	if !found {
		t.Error("expected Process as a move target")
	}
}

func TestEnrichFeatureEnvy_NoCallGraph(t *testing.T) {
	report := &PatternScanReport{
		Detections: []PatternDetection{
			{
				PatternID: "feature_envy",
				Component: "pkg/handler",
				Evidence:  []string{"80% of call sites target pkg/repo"},
			},
		},
	}

	EnrichWithCallGraph(report, nil)

	if len(report.Detections[0].MoveTargets) != 0 {
		t.Error("expected no move targets with nil call edges")
	}
}

func TestEnrichFeatureEnvy_NoFeatureEnvy(t *testing.T) {
	report := &PatternScanReport{
		Detections: []PatternDetection{
			{PatternID: "god_component", Component: "pkg/big"},
		},
	}

	// No god_component symbols in call edges → SplitSuggestion stays nil.
	callEdges := []oculus.CallEdge{
		{Caller: "X", CallerPkg: "other", Callee: "Y", CalleePkg: "other"},
	}

	EnrichWithCallGraph(report, callEdges)

	if len(report.Detections[0].MoveTargets) != 0 {
		t.Error("expected no move targets for non-feature-envy detection")
	}
}

func TestExtractEnvyTarget(t *testing.T) {
	tests := []struct {
		evidence []string
		want     string
	}{
		{[]string{"80% of call sites target pkg/repo"}, "pkg/repo"},
		{[]string{"no match here"}, ""},
		{nil, ""},
	}
	for _, tt := range tests {
		got := extractEnvyTarget(tt.evidence)
		if got != tt.want {
			t.Errorf("extractEnvyTarget(%v) = %q, want %q", tt.evidence, got, tt.want)
		}
	}
}

func TestSuggestSplit_TwoClusters(t *testing.T) {
	// 6 symbols: A1↔A2↔A3 (cluster A), B1↔B2↔B3 (cluster B).
	symbols := []string{"A1", "A2", "A3", "B1", "B2", "B3"}
	callEdges := []oculus.CallEdge{
		{Caller: "A1", Callee: "A2", CallerPkg: "pkg", CalleePkg: "pkg"},
		{Caller: "A2", Callee: "A3", CallerPkg: "pkg", CalleePkg: "pkg"},
		{Caller: "B1", Callee: "B2", CallerPkg: "pkg", CalleePkg: "pkg"},
		{Caller: "B2", Callee: "B3", CallerPkg: "pkg", CalleePkg: "pkg"},
	}

	result := SuggestSplit("pkg", symbols, callEdges)
	if result == nil {
		t.Fatal("expected split suggestion")
	}
	if len(result.Groups) != 2 {
		t.Errorf("expected 2 groups, got %d", len(result.Groups))
	}
}

func TestSuggestSplit_SingleCluster(t *testing.T) {
	symbols := []string{"A", "B", "C", "D"}
	callEdges := []oculus.CallEdge{
		{Caller: "A", Callee: "B", CallerPkg: "pkg", CalleePkg: "pkg"},
		{Caller: "B", Callee: "C", CallerPkg: "pkg", CalleePkg: "pkg"},
		{Caller: "C", Callee: "D", CallerPkg: "pkg", CalleePkg: "pkg"},
	}

	result := SuggestSplit("pkg", symbols, callEdges)
	if result != nil {
		t.Error("expected nil for single connected cluster")
	}
}

func TestSuggestSplit_NoEdges(t *testing.T) {
	result := SuggestSplit("pkg", []string{"A", "B", "C", "D"}, nil)
	if result != nil {
		t.Error("expected nil for nil call edges")
	}
}

func TestSuggestSplit_TooFewSymbols(t *testing.T) {
	result := SuggestSplit("pkg", []string{"A", "B"}, nil)
	if result != nil {
		t.Error("expected nil for <4 symbols")
	}
}
