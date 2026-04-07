package constraint

import (
	"testing"

	"github.com/dpopsuev/oculus"
)

func TestComputeInterfaceMetrics_Basic(t *testing.T) {
	classes := []oculus.ClassInfo{
		{Name: "Reader", Package: "io", Kind: "interface", Methods: []oculus.MethodInfo{
			{Name: "Read", Signature: "Read(p []byte) (int, error)", Exported: true},
		}},
		{Name: "Writer", Package: "io", Kind: "interface", Methods: []oculus.MethodInfo{
			{Name: "Write", Signature: "Write(p []byte) (int, error)", Exported: true},
		}},
		{Name: "Closer", Package: "io", Kind: "interface", Methods: []oculus.MethodInfo{
			{Name: "Close", Signature: "Close() error", Exported: true},
		}},
		{Name: "MyStruct", Package: "pkg", Kind: "struct"},
		{Name: "OtherStruct", Package: "pkg", Kind: "struct"},
	}
	impls := []oculus.ImplEdge{
		{From: "MyStruct", To: "Reader", Kind: "implements"},
		{From: "MyStruct", To: "Writer", Kind: "implements"},
		{From: "OtherStruct", To: "Reader", Kind: "implements"},
		{From: "MyStruct", To: "MyStruct", Kind: "embeds"}, // should be ignored
	}

	report := ComputeInterfaceMetrics(classes, impls)

	if len(report.Interfaces) != 3 {
		t.Fatalf("expected 3 interfaces, got %d", len(report.Interfaces))
	}

	// Interfaces are sorted by name: Closer, Reader, Writer.
	if report.Interfaces[0].Name != "Closer" {
		t.Errorf("expected first interface Closer, got %s", report.Interfaces[0].Name)
	}
	if !report.Interfaces[0].IsOrphan {
		t.Error("Closer should be orphan")
	}
	if len(report.Interfaces[0].Implementors) != 0 {
		t.Errorf("Closer should have 0 implementors, got %d", len(report.Interfaces[0].Implementors))
	}

	// Reader has 2 implementors.
	reader := report.Interfaces[1]
	if reader.Name != "Reader" {
		t.Errorf("expected Reader, got %s", reader.Name)
	}
	if len(reader.Implementors) != 2 {
		t.Errorf("Reader should have 2 implementors, got %d", len(reader.Implementors))
	}
	if reader.IsOrphan {
		t.Error("Reader should not be orphan")
	}

	// Writer has 1 implementor.
	writer := report.Interfaces[2]
	if writer.Name != "Writer" {
		t.Errorf("expected Writer, got %s", writer.Name)
	}
	if len(writer.Implementors) != 1 {
		t.Errorf("Writer should have 1 implementor, got %d", len(writer.Implementors))
	}

	// Aggregates.
	if report.TotalOrphans != 1 {
		t.Errorf("expected 1 orphan, got %d", report.TotalOrphans)
	}
	if report.AvgSize != 1.0 {
		t.Errorf("expected avg size 1.0, got %f", report.AvgSize)
	}
	if report.LargestIface != "Closer" && report.LargestIface != "Reader" && report.LargestIface != "Writer" {
		t.Errorf("unexpected largest interface: %s", report.LargestIface)
	}
	if report.Summary == "" {
		t.Error("summary should not be empty")
	}
}

func TestComputeInterfaceMetrics_Empty(t *testing.T) {
	report := ComputeInterfaceMetrics(nil, nil)

	if len(report.Interfaces) != 0 {
		t.Errorf("expected 0 interfaces, got %d", len(report.Interfaces))
	}
	if report.TotalOrphans != 0 {
		t.Errorf("expected 0 orphans, got %d", report.TotalOrphans)
	}
	if report.AvgSize != 0 {
		t.Errorf("expected avg size 0, got %f", report.AvgSize)
	}
}

func TestComputeInterfaceMetrics_LargestInterface(t *testing.T) {
	classes := []oculus.ClassInfo{
		{Name: "Small", Package: "pkg", Kind: "interface", Methods: []oculus.MethodInfo{
			{Name: "A", Signature: "A()", Exported: true},
		}},
		{Name: "Large", Package: "pkg", Kind: "interface", Methods: []oculus.MethodInfo{
			{Name: "A", Signature: "A()", Exported: true},
			{Name: "B", Signature: "B()", Exported: true},
			{Name: "C", Signature: "C()", Exported: true},
		}},
	}

	report := ComputeInterfaceMetrics(classes, nil)

	if report.LargestIface != "Large" {
		t.Errorf("expected largest interface Large, got %s", report.LargestIface)
	}
	if report.TotalOrphans != 2 {
		t.Errorf("expected 2 orphans, got %d", report.TotalOrphans)
	}
	// Avg = (1+3)/2 = 2.0
	if report.AvgSize != 2.0 {
		t.Errorf("expected avg size 2.0, got %f", report.AvgSize)
	}
}

func TestComputeInterfaceMetrics_AllImplemented(t *testing.T) {
	classes := []oculus.ClassInfo{
		{Name: "Doer", Package: "pkg", Kind: "interface", Methods: []oculus.MethodInfo{
			{Name: "Do", Signature: "Do()", Exported: true},
		}},
	}
	impls := []oculus.ImplEdge{
		{From: "Worker", To: "Doer", Kind: "implements"},
	}

	report := ComputeInterfaceMetrics(classes, impls)

	if report.TotalOrphans != 0 {
		t.Errorf("expected 0 orphans, got %d", report.TotalOrphans)
	}
	if report.Interfaces[0].IsOrphan {
		t.Error("Doer should not be orphan")
	}
	if len(report.Interfaces[0].Implementors) != 1 {
		t.Errorf("expected 1 implementor, got %d", len(report.Interfaces[0].Implementors))
	}
	if report.Interfaces[0].Implementors[0] != "Worker" {
		t.Errorf("expected implementor Worker, got %s", report.Interfaces[0].Implementors[0])
	}
}
