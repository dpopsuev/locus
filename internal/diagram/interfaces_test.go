package diagram

import (
	"strings"
	"testing"

	"github.com/dpopsuev/locus/internal/diagram/core"
	"github.com/dpopsuev/locus/internal/oculus"
)

func TestRenderInterfaces_WithImplementors(t *testing.T) {
	mock := &mockAnalyzer{
		classes: []oculus.ClassInfo{
			{Name: "Reader", Package: "io", Kind: "interface", Exported: true,
				Methods: []oculus.MethodInfo{
					{Name: "Read", Signature: "Read(p []byte) (int, error)", Exported: true},
				},
			},
			{Name: "FileReader", Package: "io", Kind: "struct", Exported: true,
				Fields: []oculus.FieldInfo{
					{Name: "path", Type: "string", Exported: false},
				},
				Methods: []oculus.MethodInfo{
					{Name: "Read", Signature: "Read(p []byte) (int, error)", Exported: true},
				},
			},
			{Name: "BufReader", Package: "io", Kind: "struct", Exported: true,
				Methods: []oculus.MethodInfo{
					{Name: "Read", Signature: "Read(p []byte) (int, error)", Exported: true},
				},
			},
			{Name: "Unrelated", Package: "io", Kind: "struct", Exported: true},
		},
		impls: []oculus.ImplEdge{
			{From: "FileReader", To: "Reader", Kind: "implements"},
			{From: "BufReader", To: "Reader", Kind: "implements"},
		},
	}

	in := core.Input{Analyzer: mock, Root: "/tmp/test"}
	out, err := Render(in, core.Options{Type: "interfaces"})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out, "classDiagram") {
		t.Error("expected classDiagram prefix")
	}
	if !strings.Contains(out, "<<interface>>") {
		t.Error("missing interface stereotype")
	}
	if !strings.Contains(out, "class Reader") {
		t.Error("missing Reader interface")
	}
	if !strings.Contains(out, "class FileReader") {
		t.Error("missing FileReader implementor")
	}
	if !strings.Contains(out, "class BufReader") {
		t.Error("missing BufReader implementor")
	}
	if strings.Contains(out, "class Unrelated") {
		t.Error("Unrelated struct should not appear in interfaces diagram")
	}
	if count := strings.Count(out, "..|>"); count != 2 {
		t.Errorf("expected 2 implements arrows, got %d", count)
	}
	if !strings.Contains(out, "FileReader ..|> Reader") {
		t.Error("missing FileReader ..|> Reader edge")
	}
	if !strings.Contains(out, "BufReader ..|> Reader") {
		t.Error("missing BufReader ..|> Reader edge")
	}
}

func TestRenderInterfaces_OrphanInterface(t *testing.T) {
	mock := &mockAnalyzer{
		classes: []oculus.ClassInfo{
			{Name: "Stringer", Package: "fmt", Kind: "interface", Exported: true,
				Methods: []oculus.MethodInfo{
					{Name: "String", Signature: "String() string", Exported: true},
				},
			},
		},
		impls: []oculus.ImplEdge{},
	}

	in := core.Input{Analyzer: mock, Root: "/tmp/test"}
	out, err := Render(in, core.Options{Type: "interfaces"})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out, "class Stringer") {
		t.Error("orphan interface should still be rendered")
	}
	if !strings.Contains(out, "<<interface>>") {
		t.Error("missing interface stereotype for orphan")
	}
	if strings.Contains(out, "..|>") {
		t.Error("no implements arrows expected for orphan interface")
	}
}

func TestRenderInterfaces_ScopeFiltering(t *testing.T) {
	mock := &mockAnalyzer{
		classes: []oculus.ClassInfo{
			{Name: "Writer", Package: "io", Kind: "interface", Exported: true,
				Methods: []oculus.MethodInfo{
					{Name: "Write", Signature: "Write(p []byte) (int, error)", Exported: true},
				},
			},
			{Name: "Logger", Package: "log", Kind: "interface", Exported: true,
				Methods: []oculus.MethodInfo{
					{Name: "Log", Signature: "Log(msg string)", Exported: true},
				},
			},
			{Name: "FileWriter", Package: "io", Kind: "struct", Exported: true,
				Methods: []oculus.MethodInfo{
					{Name: "Write", Signature: "Write(p []byte) (int, error)", Exported: true},
				},
			},
		},
		impls: []oculus.ImplEdge{
			{From: "FileWriter", To: "Writer", Kind: "implements"},
		},
	}

	in := core.Input{Analyzer: mock, Root: "/tmp/test"}
	out, err := Render(in, core.Options{Type: "interfaces", Scope: "io"})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out, "class Writer") {
		t.Error("Writer should appear (package io matches scope)")
	}
	if !strings.Contains(out, "class FileWriter") {
		t.Error("FileWriter should appear (package io matches scope)")
	}
	if strings.Contains(out, "class Logger") {
		t.Error("Logger should be filtered out (package log != scope io)")
	}
}

func TestRenderInterfaces_EmptyInput(t *testing.T) {
	mock := &mockAnalyzer{
		classes: []oculus.ClassInfo{
			{Name: "Foo", Package: "bar", Kind: "struct", Exported: true},
		},
		impls: []oculus.ImplEdge{},
	}

	in := core.Input{Analyzer: mock, Root: "/tmp/test"}
	_, err := Render(in, core.Options{Type: "interfaces"})
	if err == nil {
		t.Fatal("expected error when no interfaces found")
	}
	if !strings.Contains(err.Error(), "no interfaces found") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestRenderInterfaces_NilAnalyzer(t *testing.T) {
	in := core.Input{Root: "/tmp/test"}
	_, err := Render(in, core.Options{Type: "interfaces"})
	if err == nil {
		t.Fatal("expected error with nil analyzer")
	}
}
