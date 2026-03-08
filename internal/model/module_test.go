package model_test

import (
	"encoding/json"
	"testing"

	"github.com/dpopsuev/locus/internal/model"
)

func TestNewModule(t *testing.T) {
	p := model.NewProject("github.com/example/foo")
	if p.Path != "github.com/example/foo" {
		t.Errorf("path = %q, want github.com/example/foo", p.Path)
	}
	if len(p.Namespaces) != 0 {
		t.Errorf("namespaces = %d, want 0", len(p.Namespaces))
	}
}

func TestAddPackage(t *testing.T) {
	p := model.NewProject("mod")
	p.AddNamespace(model.NewNamespace("a", "mod/a"))
	p.AddNamespace(model.NewNamespace("b", "mod/b"))
	if len(p.Namespaces) != 2 {
		t.Fatalf("namespaces = %d, want 2", len(p.Namespaces))
	}
	if p.Namespaces[0].Name != "a" {
		t.Errorf("namespaces[0].name = %q, want a", p.Namespaces[0].Name)
	}
}

func TestAddFileAndSymbol(t *testing.T) {
	ns := model.NewNamespace("pkg", "mod/pkg")
	ns.AddFile(model.NewFile("pkg/foo.go", "pkg"))
	ns.AddSymbol(&model.Symbol{Name: "Foo", Kind: model.SymbolFunction, Exported: true})
	ns.AddSymbol(&model.Symbol{Name: "bar", Kind: model.SymbolVariable, Exported: false})

	if len(ns.Files) != 1 {
		t.Errorf("files = %d, want 1", len(ns.Files))
	}
	if len(ns.Symbols) != 2 {
		t.Errorf("symbols = %d, want 2", len(ns.Symbols))
	}
}

func TestImportGraphAddEdge(t *testing.T) {
	g := model.NewDependencyGraph()
	g.AddEdge("a", "b", false)
	g.AddEdge("a", "c", true)
	g.AddEdge("a", "b", false) // duplicate

	if len(g.Edges) != 2 {
		t.Fatalf("edges = %d, want 2", len(g.Edges))
	}
}

func TestImportGraphEdgesFrom(t *testing.T) {
	g := model.NewDependencyGraph()
	g.AddEdge("a", "b", false)
	g.AddEdge("a", "c", true)
	g.AddEdge("b", "c", false)

	edges := g.EdgesFrom("a")
	if len(edges) != 2 {
		t.Errorf("EdgesFrom(a) = %d, want 2", len(edges))
	}

	edges = g.EdgesFrom("b")
	if len(edges) != 1 {
		t.Errorf("EdgesFrom(b) = %d, want 1", len(edges))
	}

	edges = g.EdgesFrom("z")
	if len(edges) != 0 {
		t.Errorf("EdgesFrom(z) = %d, want 0", len(edges))
	}
}

func TestSymbolKindString(t *testing.T) {
	tests := []struct {
		kind model.SymbolKind
		want string
	}{
		{model.SymbolFunction, "function"},
		{model.SymbolStruct, "struct"},
		{model.SymbolInterface, "interface"},
		{model.SymbolConstant, "constant"},
		{model.SymbolVariable, "variable"},
	}
	for _, tt := range tests {
		if got := tt.kind.String(); got != tt.want {
			t.Errorf("%d.String() = %q, want %q", tt.kind, got, tt.want)
		}
	}
}

func TestSymbolKindJSON(t *testing.T) {
	s := model.Symbol{Name: "Foo", Kind: model.SymbolFunction, Exported: true}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got model.Symbol
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Kind != model.SymbolFunction {
		t.Errorf("kind = %v, want SymbolFunction", got.Kind)
	}
	if got.Name != "Foo" {
		t.Errorf("name = %q, want Foo", got.Name)
	}
}

func TestModuleJSONRoundTrip(t *testing.T) {
	proj := model.NewProject("mod")
	ns := model.NewNamespace("pkg", "mod/pkg")
	ns.AddFile(model.NewFile("pkg/main.go", "pkg"))
	ns.AddSymbol(&model.Symbol{Name: "Run", Kind: model.SymbolFunction, Exported: true})
	proj.AddNamespace(ns)
	proj.DependencyGraph = model.NewDependencyGraph()
	proj.DependencyGraph.AddEdge("mod/pkg", "fmt", true)

	data, err := json.Marshal(proj)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got model.Project
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Path != "mod" {
		t.Errorf("path = %q, want mod", got.Path)
	}
	if len(got.Namespaces) != 1 {
		t.Fatalf("namespaces = %d, want 1", len(got.Namespaces))
	}
	if got.Namespaces[0].ImportPath != "mod/pkg" {
		t.Errorf("import_path = %q, want mod/pkg", got.Namespaces[0].ImportPath)
	}
	if got.DependencyGraph == nil || len(got.DependencyGraph.Edges) != 1 {
		t.Errorf("dependency graph edges = %v, want 1 edge", got.DependencyGraph)
	}
}
