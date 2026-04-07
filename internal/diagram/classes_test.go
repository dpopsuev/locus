package diagram

import (
	"strings"
	"testing"

	"github.com/dpopsuev/locus/internal/analysis"
	"github.com/dpopsuev/locus/internal/diagram/core"
)

type mockAnalyzer struct {
	classes []analysis.ClassInfo
	impls   []analysis.ImplEdge
	refs    []analysis.FieldRef
	calls   []analysis.Call
	entries []analysis.EntryPoint
	nesting []analysis.NestingResult
}

func (m *mockAnalyzer) Classes(root string) ([]analysis.ClassInfo, error)   { return m.classes, nil }
func (m *mockAnalyzer) Implements(root string) ([]analysis.ImplEdge, error) { return m.impls, nil }
func (m *mockAnalyzer) FieldRefs(root string) ([]analysis.FieldRef, error)  { return m.refs, nil }
func (m *mockAnalyzer) CallChain(root, entry string, depth int) ([]analysis.Call, error) {
	return m.calls, nil
}
func (m *mockAnalyzer) EntryPoints(root string) ([]analysis.EntryPoint, error) { return m.entries, nil }
func (m *mockAnalyzer) NestingDepth(root string) ([]analysis.NestingResult, error) {
	return m.nesting, nil
}

func TestRenderClasses(t *testing.T) {
	mock := &mockAnalyzer{
		classes: []analysis.ClassInfo{
			{Name: "Server", Package: "main", Kind: "struct", Exported: true,
				Fields: []analysis.FieldInfo{
					{Name: "Addr", Type: "string", Exported: true},
				},
				Methods: []analysis.MethodInfo{
					{Name: "Start", Signature: "Start()", Exported: true},
				},
			},
			{Name: "Handler", Package: "main", Kind: "interface", Exported: true,
				Methods: []analysis.MethodInfo{
					{Name: "Handle", Signature: "Handle(req Request)", Exported: true},
				},
			},
		},
		impls: []analysis.ImplEdge{
			{From: "Server", To: "Handler", Kind: "implements"},
		},
	}

	in := core.Input{Analyzer: mock, Root: "/tmp/test"}
	out, err := Render(in, core.Options{Type: "classes"})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out, "classDiagram") {
		t.Error("expected classDiagram")
	}
	if !strings.Contains(out, "class Server") {
		t.Error("missing Server class")
	}
	if !strings.Contains(out, "<<interface>>") {
		t.Error("missing interface stereotype")
	}
	if !strings.Contains(out, "..|>") {
		t.Error("missing implements arrow")
	}
}

func TestRenderSequence(t *testing.T) {
	mock := &mockAnalyzer{
		calls: []analysis.Call{
			{Caller: "main", Callee: "Start", Package: "cmd"},
			{Caller: "Start", Callee: "Listen", Package: "net"},
		},
		entries: []analysis.EntryPoint{
			{Name: "main", Kind: "main"},
		},
	}

	in := core.Input{Analyzer: mock, Root: "/tmp/test"}
	out, err := Render(in, core.Options{Type: "sequence", Entry: "main"})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out, "sequenceDiagram") {
		t.Error("expected sequenceDiagram")
	}
	if !strings.Contains(out, "participant main") {
		t.Error("missing main participant")
	}
	if !strings.Contains(out, "main->>Start") {
		t.Error("missing main->Start message")
	}
}

func TestRenderER(t *testing.T) {
	mock := &mockAnalyzer{
		classes: []analysis.ClassInfo{
			{Name: "User", Package: "models", Kind: "struct",
				Fields: []analysis.FieldInfo{
					{Name: "ID", Type: "int"},
					{Name: "Profile", Type: "*Profile"},
				},
			},
			{Name: "Profile", Package: "models", Kind: "struct",
				Fields: []analysis.FieldInfo{
					{Name: "Bio", Type: "string"},
				},
			},
		},
		refs: []analysis.FieldRef{
			{Owner: "User", Field: "Profile", RefType: "Profile"},
		},
	}

	in := core.Input{Analyzer: mock, Root: "/tmp/test"}
	out, err := Render(in, core.Options{Type: "er"})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out, "erDiagram") {
		t.Error("expected erDiagram")
	}
	if !strings.Contains(out, "User") {
		t.Error("missing User entity")
	}
	if !strings.Contains(out, "Profile") {
		t.Error("missing Profile entity")
	}
	if !strings.Contains(out, "||--o{") {
		t.Error("missing relationship")
	}
}

func TestRenderSequence_AutoEntry(t *testing.T) {
	mock := &mockAnalyzer{
		entries: []analysis.EntryPoint{
			{Name: "main", Kind: "main"},
		},
		calls: []analysis.Call{
			{Caller: "main", Callee: "run"},
		},
	}

	in := core.Input{Analyzer: mock, Root: "/tmp/test"}
	out, err := Render(in, core.Options{Type: "sequence"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "main") {
		t.Error("auto-detected entry not used")
	}
}
