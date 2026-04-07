package diagram

import (
	"strings"
	"testing"

	"github.com/dpopsuev/locus/internal/diagram/core"
	"github.com/dpopsuev/locus/internal/oculus"
)

type mockAnalyzer struct {
	classes []oculus.ClassInfo
	impls   []oculus.ImplEdge
	refs    []oculus.FieldRef
	calls   []oculus.Call
	entries []oculus.EntryPoint
	nesting []oculus.NestingResult
}

func (m *mockAnalyzer) Classes(root string) ([]oculus.ClassInfo, error)   { return m.classes, nil }
func (m *mockAnalyzer) Implements(root string) ([]oculus.ImplEdge, error) { return m.impls, nil }
func (m *mockAnalyzer) FieldRefs(root string) ([]oculus.FieldRef, error)  { return m.refs, nil }
func (m *mockAnalyzer) CallChain(root, entry string, depth int) ([]oculus.Call, error) {
	return m.calls, nil
}
func (m *mockAnalyzer) EntryPoints(root string) ([]oculus.EntryPoint, error) { return m.entries, nil }
func (m *mockAnalyzer) NestingDepth(root string) ([]oculus.NestingResult, error) {
	return m.nesting, nil
}

func TestRenderClasses(t *testing.T) {
	mock := &mockAnalyzer{
		classes: []oculus.ClassInfo{
			{Name: "Server", Package: "main", Kind: "struct", Exported: true,
				Fields: []oculus.FieldInfo{
					{Name: "Addr", Type: "string", Exported: true},
				},
				Methods: []oculus.MethodInfo{
					{Name: "Start", Signature: "Start()", Exported: true},
				},
			},
			{Name: "Handler", Package: "main", Kind: "interface", Exported: true,
				Methods: []oculus.MethodInfo{
					{Name: "Handle", Signature: "Handle(req Request)", Exported: true},
				},
			},
		},
		impls: []oculus.ImplEdge{
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
		calls: []oculus.Call{
			{Caller: "main", Callee: "Start", Package: "cmd"},
			{Caller: "Start", Callee: "Listen", Package: "net"},
		},
		entries: []oculus.EntryPoint{
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
		classes: []oculus.ClassInfo{
			{Name: "User", Package: "models", Kind: "struct",
				Fields: []oculus.FieldInfo{
					{Name: "ID", Type: "int"},
					{Name: "Profile", Type: "*Profile"},
				},
			},
			{Name: "Profile", Package: "models", Kind: "struct",
				Fields: []oculus.FieldInfo{
					{Name: "Bio", Type: "string"},
				},
			},
		},
		refs: []oculus.FieldRef{
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
		entries: []oculus.EntryPoint{
			{Name: "main", Kind: "main"},
		},
		calls: []oculus.Call{
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
