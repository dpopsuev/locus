package oculus

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTestGoFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestTreeSitterDeepCallGraph(t *testing.T) {
	dir := t.TempDir()
	writeTestGoFile(t, dir, "go.mod", "module testmod\ngo 1.21\n")
	writeTestGoFile(t, dir, "main.go", `package main

func main() {
	Run()
}

func Run() {
	Helper()
}

func Helper() {}
`)
	a, err := NewTreeSitterDeep(dir)
	if err != nil {
		t.Fatal(err)
	}
	cg, err := a.CallGraph(dir, CallGraphOpts{Entry: "main", Depth: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(cg.Edges) < 2 {
		t.Fatalf("expected at least 2 edges, got %d", len(cg.Edges))
	}
	if cg.Layer != "treesitter" {
		t.Fatalf("expected layer=treesitter, got %s", cg.Layer)
	}
	t.Logf("CallGraph: %d nodes, %d edges", len(cg.Nodes), len(cg.Edges))
}

func TestTreeSitterDeepDataFlowTrace(t *testing.T) {
	dir := t.TempDir()
	writeTestGoFile(t, dir, "go.mod", "module testmod\ngo 1.21\n")
	writeTestGoFile(t, dir, "main.go", `package main

func main() {
	Process()
}

func Process() {
	Save()
}

func Save() {}
`)
	a, err := NewTreeSitterDeep(dir)
	if err != nil {
		t.Fatal(err)
	}
	flow, err := a.DataFlowTrace(dir, "main", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(flow.Nodes) < 2 {
		t.Fatalf("expected at least 2 nodes, got %d", len(flow.Nodes))
	}
	if flow.Layer != "treesitter" {
		t.Fatalf("expected layer=treesitter, got %s", flow.Layer)
	}
	t.Logf("DataFlow: %d nodes, %d edges", len(flow.Nodes), len(flow.Edges))
}

func TestTreeSitterDeepStateMachines(t *testing.T) {
	dir := t.TempDir()
	writeTestGoFile(t, dir, "go.mod", "module testmod\ngo 1.21\n")
	writeTestGoFile(t, dir, "state.go", `package main

type Status int

const (
	StatusPending Status = iota
	StatusActive
	StatusDone
)

func transition(s Status) Status {
	switch s {
	case StatusPending:
		return StatusActive
	case StatusActive:
		return StatusDone
	}
	return s
}
`)
	a, err := NewTreeSitterDeep(dir)
	if err != nil {
		t.Fatal(err)
	}
	machines, err := a.DetectStateMachines(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(machines) == 0 {
		t.Fatal("expected at least one state machine")
	}
	sm := machines[0]
	if sm.Name != "Status" {
		t.Fatalf("expected state machine named Status, got %s", sm.Name)
	}
	if len(sm.States) != 3 {
		t.Fatalf("expected 3 states, got %d: %v", len(sm.States), sm.States)
	}
	t.Logf("StateMachine: %s with %d states, %d transitions", sm.Name, len(sm.States), len(sm.Transitions))
}

func TestDeepFallbackCallGraph(t *testing.T) {
	dir := t.TempDir()
	writeTestGoFile(t, dir, "go.mod", "module testmod\ngo 1.21\n")
	writeTestGoFile(t, dir, "main.go", `package main

func main() {
	Hello()
}

func Hello() {}
`)
	fb := NewDeepFallback(dir, nil)
	cg, err := fb.CallGraph(dir, CallGraphOpts{Entry: "main", Depth: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(cg.Edges) == 0 {
		t.Fatal("expected at least one edge from fallback")
	}
	t.Logf("Fallback CallGraph: %d edges, layer=%s", len(cg.Edges), cg.Layer)
}
