package analysis_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dpopsuev/locus/internal/analysis"
)

func TestGoASTCallGraph_OnLocus(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Skip("cannot resolve repo root")
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Skip("not in a Go repo")
	}

	a := analysis.NewGoASTDeep(root)
	if a == nil {
		t.Fatal("expected GoASTDeepAnalyzer for Go repo")
	}

	cg, err := a.CallGraph(root, analysis.CallGraphOpts{
		ExportedOnly: true,
		Depth:        3,
	})
	if err != nil {
		t.Fatalf("CallGraph: %v", err)
	}
	if len(cg.Nodes) == 0 {
		t.Error("expected at least one node")
	}
	if len(cg.Edges) == 0 {
		t.Error("expected at least one edge")
	}
	if cg.Layer != analysis.LayerGoAST {
		t.Errorf("layer = %q, want goast", cg.Layer)
	}
	t.Logf("GoAST CallGraph: %d nodes, %d edges", len(cg.Nodes), len(cg.Edges))
}

func TestGoASTCallGraph_WithEntry(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Skip("cannot resolve repo root")
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Skip("not in a Go repo")
	}

	a := analysis.NewGoASTDeep(root)
	if a == nil {
		t.Fatal("expected GoASTDeepAnalyzer")
	}

	cg, err := a.CallGraph(root, analysis.CallGraphOpts{
		Entry: "ScanAndBuild",
		Depth: 2,
	})
	if err != nil {
		t.Fatalf("CallGraph: %v", err)
	}
	if len(cg.Edges) == 0 {
		t.Error("expected edges from ScanAndBuild")
	}

	// Verify ScanAndBuild is in the graph.
	found := false
	for _, n := range cg.Nodes {
		if n.Name == "ScanAndBuild" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected ScanAndBuild node in graph")
	}
	t.Logf("ScanAndBuild CallGraph: %d nodes, %d edges", len(cg.Nodes), len(cg.Edges))
}

func TestGoASTDataFlowTrace(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Skip("cannot resolve repo root")
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Skip("not in a Go repo")
	}

	a := analysis.NewGoASTDeep(root)
	if a == nil {
		t.Fatal("expected GoASTDeepAnalyzer")
	}

	df, err := a.DataFlowTrace(root, "ScanAndBuild", 3)
	if err != nil {
		t.Fatalf("DataFlowTrace: %v", err)
	}
	if len(df.Nodes) == 0 {
		t.Error("expected at least one node")
	}
	if df.Layer != analysis.LayerGoAST {
		t.Errorf("layer = %q, want goast", df.Layer)
	}
	t.Logf("DataFlowTrace: %d nodes, %d edges", len(df.Nodes), len(df.Edges))
}

func TestGoASTDeep_NonGoRepo(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[project]\n"), 0o644)

	a := analysis.NewGoASTDeep(dir)
	if a != nil {
		t.Error("expected nil for non-Go repo")
	}
}

func TestGoASTFallbackIntegration(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Skip("cannot resolve repo root")
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Skip("not in a Go repo")
	}

	fb := analysis.NewDeepFallback(root)
	cg, err := fb.CallGraph(root, analysis.CallGraphOpts{Entry: "ScanAndBuild", Depth: 2})
	if err != nil {
		t.Fatalf("fallback CallGraph: %v", err)
	}
	if len(cg.Edges) == 0 {
		t.Error("expected edges from fallback")
	}
	t.Logf("Fallback CallGraph: %d edges, layer=%s", len(cg.Edges), cg.Layer)
}
