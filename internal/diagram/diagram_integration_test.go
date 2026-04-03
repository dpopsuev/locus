package diagram

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/dpopsuev/locus/internal/analysis"
	"github.com/dpopsuev/locus/internal/arch"
	"github.com/dpopsuev/locus/internal/clinic"
)

func integrationRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "..", "..")
}

func integrationScan(t *testing.T) *arch.ContextReport {
	t.Helper()
	root := integrationRoot(t)
	report, err := arch.ScanAndBuild(root, arch.ScanOpts{ExcludeTests: true})
	if err != nil {
		t.Fatalf("ScanAndBuild: %v", err)
	}
	return report
}

func TestIntegration_ClassDiagram(t *testing.T) {
	if testing.Short() {
		t.Skip("integration: skipping expensive scan in -short mode")
	}
	root := integrationRoot(t)
	report := integrationScan(t)
	fa := analysis.NewFallback(root)

	input := Input{Report: report, Analyzer: fa, Root: root}
	out, err := Render(input, Options{Type: "classes"})
	if err != nil {
		t.Fatalf("render classes: %v", err)
	}
	assertContains(t, out, "classDiagram")
}

func TestIntegration_InterfacesDiagram(t *testing.T) {
	if testing.Short() {
		t.Skip("integration: skipping expensive scan in -short mode")
	}
	root := integrationRoot(t)
	report := integrationScan(t)
	fa := analysis.NewFallback(root)

	input := Input{Report: report, Analyzer: fa, Root: root}
	out, err := Render(input, Options{Type: "interfaces"})
	if err != nil {
		t.Fatalf("render interfaces: %v", err)
	}
	assertContains(t, out, "classDiagram")
}

func TestIntegration_HexaDiagram(t *testing.T) {
	if testing.Short() {
		t.Skip("integration: skipping expensive scan in -short mode")
	}
	root := integrationRoot(t)
	report := integrationScan(t)
	fa := analysis.NewFallback(root)
	classes, _ := fa.Classes(root)
	hexaClass := clinic.ComputeHexaClassification(report.Architecture.Services, report.Architecture.Edges, classes)

	roles := make(map[string]string, len(hexaClass.Components))
	for _, c := range hexaClass.Components {
		roles[c.Name] = string(c.Role)
	}

	input := Input{Report: report, Analyzer: fa, Root: root, HexaRoles: roles}
	out, err := Render(input, Options{Type: "hexa"})
	if err != nil {
		t.Fatalf("render hexa: %v", err)
	}
	assertContains(t, out, "graph TD")
	assertContains(t, out, "subgraph")
}

func TestIntegration_CallgraphDiagram(t *testing.T) {
	if testing.Short() {
		t.Skip("integration: skipping expensive scan in -short mode")
	}
	root := integrationRoot(t)
	report := integrationScan(t)
	da := analysis.NewDeepFallback(root)

	input := Input{Report: report, DeepAnalyzer: da, Root: root}
	out, err := Render(input, Options{Type: "callgraph"})
	if err != nil {
		t.Fatalf("render callgraph: %v", err)
	}
	assertContains(t, out, "flowchart TB")
}
