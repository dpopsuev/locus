package protocol

import (
	"testing"

	"github.com/dpopsuev/locus/internal/analysis"
	"github.com/dpopsuev/locus/internal/arch"
)

func TestComputeSRPViolations_HighFanOut(t *testing.T) {
	services := []arch.ArchService{
		{Name: "internal/bigpkg", LOC: 1200, Symbols: make([]string, 5)},
	}
	// Create 10 outbound edges to trigger LOC>1000 && fan-out>8 → error.
	edges := make([]arch.ArchEdge, 10)
	for i := range edges {
		edges[i] = arch.ArchEdge{From: "internal/bigpkg", To: "internal/target" + string(rune('a'+i))}
	}

	violations := ComputeSRPViolations(services, edges)

	if len(violations) == 0 {
		t.Fatal("expected at least 1 SRP violation for LOC=1200, fan-out=10")
	}

	found := false
	for _, v := range violations {
		if v.Severity == SeverityError && v.Principle == PrincipleSRP {
			found = true
		}
	}
	if !found {
		t.Error("expected an error-severity SRP violation")
	}
}

func TestComputeSRPViolations_Warning(t *testing.T) {
	services := []arch.ArchService{
		{Name: "internal/midpkg", LOC: 600, Symbols: make([]string, 5)},
	}
	edges := make([]arch.ArchEdge, 6)
	for i := range edges {
		edges[i] = arch.ArchEdge{From: "internal/midpkg", To: "internal/dep" + string(rune('a'+i))}
	}

	violations := ComputeSRPViolations(services, edges)

	if len(violations) == 0 {
		t.Fatal("expected at least 1 SRP warning for LOC=600, fan-out=6")
	}

	for _, v := range violations {
		if v.Severity != SeverityWarning {
			t.Errorf("expected warning severity, got %s", v.Severity)
		}
	}
}

func TestComputeSRPViolations_Clean(t *testing.T) {
	services := []arch.ArchService{
		{Name: "internal/small", LOC: 100, Symbols: make([]string, 3)},
	}
	edges := []arch.ArchEdge{
		{From: "internal/small", To: "internal/a"},
		{From: "internal/small", To: "internal/b"},
	}

	violations := ComputeSRPViolations(services, edges)

	if len(violations) != 0 {
		t.Errorf("expected 0 violations for LOC=100, fan-out=2, got %d", len(violations))
	}
}

func TestComputeSRPViolations_DomainDiversity(t *testing.T) {
	services := []arch.ArchService{
		{Name: "internal/hub", LOC: 200, Symbols: make([]string, 25)},
	}
	// 4 distinct domains: store, arch, analysis, protocol.
	edges := []arch.ArchEdge{
		{From: "internal/hub", To: "internal/store/sql"},
		{From: "internal/hub", To: "internal/arch"},
		{From: "internal/hub", To: "internal/analysis"},
		{From: "internal/hub", To: "internal/protocol"},
	}

	violations := ComputeSRPViolations(services, edges)

	if len(violations) == 0 {
		t.Fatal("expected domain diversity violation for 4 domains and 25 symbols")
	}

	found := false
	for _, v := range violations {
		if v.Suggestion == "Component touches too many domains" {
			found = true
		}
	}
	if !found {
		t.Error("expected 'Component touches too many domains' suggestion")
	}
}

func TestComputeISPViolations_FatInterface(t *testing.T) {
	methods := make([]analysis.MethodInfo, 9)
	for i := range methods {
		methods[i] = analysis.MethodInfo{Name: "Method" + string(rune('A'+i)), Exported: true}
	}
	classes := []analysis.ClassInfo{
		{Name: "BigInterface", Package: "pkg", Kind: "interface", Methods: methods},
	}

	violations := ComputeISPViolations(classes, nil)

	if len(violations) == 0 {
		t.Fatal("expected ISP violation for interface with 9 methods")
	}

	v := violations[0]
	if v.Severity != SeverityError {
		t.Errorf("expected error severity for 9 methods (threshold 8), got %s", v.Severity)
	}
	if v.Principle != PrincipleISP {
		t.Errorf("expected ISP principle, got %s", v.Principle)
	}
}

func TestComputeISPViolations_WarningThreshold(t *testing.T) {
	methods := make([]analysis.MethodInfo, 6)
	for i := range methods {
		methods[i] = analysis.MethodInfo{Name: "Method" + string(rune('A'+i)), Exported: true}
	}
	classes := []analysis.ClassInfo{
		{Name: "MediumInterface", Package: "pkg", Kind: "interface", Methods: methods},
	}

	violations := ComputeISPViolations(classes, nil)

	if len(violations) == 0 {
		t.Fatal("expected ISP warning for interface with 6 methods")
	}

	v := violations[0]
	if v.Severity != SeverityWarning {
		t.Errorf("expected warning severity for 6 methods (threshold 5), got %s", v.Severity)
	}
}

func TestComputeISPViolations_SmallInterface(t *testing.T) {
	methods := make([]analysis.MethodInfo, 3)
	for i := range methods {
		methods[i] = analysis.MethodInfo{Name: "Method" + string(rune('A'+i)), Exported: true}
	}
	classes := []analysis.ClassInfo{
		{Name: "SmallInterface", Package: "pkg", Kind: "interface", Methods: methods},
	}

	violations := ComputeISPViolations(classes, nil)

	if len(violations) != 0 {
		t.Errorf("expected 0 violations for 3-method interface, got %d", len(violations))
	}
}

func TestComputeISPViolations_ImplementorNotFlagged(t *testing.T) {
	// BUG-19: implementor sub-check removed — Go compiler enforces interface satisfaction.
	// Only fat interfaces (>5 methods) should be flagged, not implementors.
	ifaceMethods := make([]analysis.MethodInfo, 4)
	for i := range ifaceMethods {
		ifaceMethods[i] = analysis.MethodInfo{Name: "Method" + string(rune('A'+i)), Exported: true}
	}

	classes := []analysis.ClassInfo{
		{Name: "MyInterface", Package: "pkg", Kind: "interface", Methods: ifaceMethods},
		{Name: "PartialImpl", Package: "pkg", Kind: "struct", Methods: make([]analysis.MethodInfo, 2)},
	}
	impls := []analysis.ImplEdge{
		{From: "PartialImpl", To: "MyInterface", Kind: "implements"},
	}

	violations := ComputeISPViolations(classes, impls)

	if len(violations) != 0 {
		t.Errorf("expected 0 violations (4-method interface is fine, implementors not checked), got %d", len(violations))
	}
}

func TestComputeOCPViolations_EmptyRoot(t *testing.T) {
	violations := ComputeOCPViolations("")

	if violations != nil {
		t.Errorf("expected nil for empty root, got %d violations", len(violations))
	}
}

func TestComputeDIPViolations_NilClassification(t *testing.T) {
	services := []arch.ArchService{{Name: "a"}}
	edges := []arch.ArchEdge{{From: "a", To: "b"}}

	violations := ComputeDIPViolations(services, edges, nil)

	if violations != nil {
		t.Errorf("expected nil for nil classification, got %d violations", len(violations))
	}
}

func TestComputeDIPViolations_DomainToAdapter(t *testing.T) {
	services := []arch.ArchService{
		{Name: "domain/user"},
		{Name: "adapter/http"},
	}
	edges := []arch.ArchEdge{
		{From: "domain/user", To: "adapter/http"},
	}
	classification := &HexaClassificationReport{
		Components: []HexaComponent{
			{Name: "domain/user", Role: HexaRoleDomain},
			{Name: "adapter/http", Role: HexaRoleAdapter},
		},
	}

	violations := ComputeDIPViolations(services, edges, classification)

	if len(violations) == 0 {
		t.Fatal("expected DIP violation for domain → adapter")
	}

	v := violations[0]
	if v.Severity != SeverityError {
		t.Errorf("expected error severity for domain → adapter, got %s", v.Severity)
	}
	if v.Principle != PrincipleDIP {
		t.Errorf("expected DIP principle, got %s", v.Principle)
	}
}

func TestComputeDIPViolations_AppToAdapter(t *testing.T) {
	services := []arch.ArchService{
		{Name: "app/service"},
		{Name: "adapter/db"},
	}
	edges := []arch.ArchEdge{
		{From: "app/service", To: "adapter/db"},
	}
	classification := &HexaClassificationReport{
		Components: []HexaComponent{
			{Name: "app/service", Role: HexaRoleApp},
			{Name: "adapter/db", Role: HexaRoleAdapter},
		},
	}

	violations := ComputeDIPViolations(services, edges, classification)

	if len(violations) == 0 {
		t.Fatal("expected DIP warning for app → adapter")
	}

	v := violations[0]
	if v.Severity != SeverityWarning {
		t.Errorf("expected warning severity for app → adapter, got %s", v.Severity)
	}
}

func TestComputeSOLIDScan_Score(t *testing.T) {
	// Setup: 1 SRP violation (warning) + 1 ISP violation (error) = 2 violations.
	// 1 service × 4 principles = 4 checks. Score = 100 - 2/4*100 = 50.
	services := []arch.ArchService{
		{Name: "internal/big", LOC: 600, Symbols: make([]string, 5)},
	}
	edges := make([]arch.ArchEdge, 6)
	for i := range edges {
		edges[i] = arch.ArchEdge{From: "internal/big", To: "internal/dep" + string(rune('a'+i))}
	}

	methods := make([]analysis.MethodInfo, 9)
	for i := range methods {
		methods[i] = analysis.MethodInfo{Name: "M" + string(rune('A'+i)), Exported: true}
	}
	classes := []analysis.ClassInfo{
		{Name: "FatIface", Package: "pkg", Kind: "interface", Methods: methods},
	}

	report := ComputeSOLIDScan(services, edges, classes, nil, nil, "")

	expectedViolations := 2
	if len(report.Violations) != expectedViolations {
		t.Fatalf("expected %d violations, got %d", expectedViolations, len(report.Violations))
	}

	expectedScore := 50.0 // 100 - 2/4*100
	if report.Score != expectedScore {
		t.Errorf("expected score %.0f, got %.0f", expectedScore, report.Score)
	}

	if report.ByPrinciple["SRP"] != 1 {
		t.Errorf("expected 1 SRP violation, got %d", report.ByPrinciple["SRP"])
	}
	if report.ByPrinciple["ISP"] != 1 {
		t.Errorf("expected 1 ISP violation, got %d", report.ByPrinciple["ISP"])
	}
}

func TestComputeSOLIDScan_PerfectScore(t *testing.T) {
	services := []arch.ArchService{
		{Name: "internal/clean", LOC: 50, Symbols: make([]string, 3)},
	}
	edges := []arch.ArchEdge{
		{From: "internal/clean", To: "internal/a"},
	}
	classes := []analysis.ClassInfo{
		{Name: "SmallIface", Package: "pkg", Kind: "interface", Methods: make([]analysis.MethodInfo, 2)},
	}

	report := ComputeSOLIDScan(services, edges, classes, nil, nil, "")

	if report.Score != 100 {
		t.Errorf("expected score 100, got %.0f", report.Score)
	}
	if report.Summary != "SOLID score: 100/100 — no violations detected" {
		t.Errorf("unexpected summary: %s", report.Summary)
	}
}

func TestComputeSOLIDScan_ScoreFloor(t *testing.T) {
	// 21+ violations should floor at 0.
	services := make([]arch.ArchService, 0)
	classes := make([]analysis.ClassInfo, 0, 25)
	for i := range 25 {
		methods := make([]analysis.MethodInfo, 10)
		classes = append(classes, analysis.ClassInfo{
			Name:    "Iface" + string(rune('A'+i)),
			Package: "pkg",
			Kind:    "interface",
			Methods: methods,
		})
	}

	report := ComputeSOLIDScan(services, nil, classes, nil, nil, "")

	if report.Score != 0 {
		t.Errorf("expected score 0 (floor), got %.0f", report.Score)
	}
}

func TestComputeSOLIDScan_SortOrder(t *testing.T) {
	// Mix of error and warning violations — errors should come first.
	methods9 := make([]analysis.MethodInfo, 9)
	methods6 := make([]analysis.MethodInfo, 6)
	classes := []analysis.ClassInfo{
		{Name: "ZWarning", Package: "pkg", Kind: "interface", Methods: methods6},
		{Name: "AError", Package: "pkg", Kind: "interface", Methods: methods9},
	}

	report := ComputeSOLIDScan(nil, nil, classes, nil, nil, "")

	if len(report.Violations) < 2 {
		t.Fatalf("expected at least 2 violations, got %d", len(report.Violations))
	}

	// First violation should be the error.
	if report.Violations[0].Severity != SeverityError {
		t.Errorf("expected first violation to be error, got %s", report.Violations[0].Severity)
	}
	// Second violation should be the warning.
	if report.Violations[1].Severity != SeverityWarning {
		t.Errorf("expected second violation to be warning, got %s", report.Violations[1].Severity)
	}
}

func TestExtractDomain(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"internal/store/sql", "store"},
		{"internal/store", "store"},
		{"internal/arch", "arch"},
		{"cmd/app", "cmd"},
		{"pkg/util", "pkg"},
		{"foo/internal/bar/baz", "bar"},
		{"single", "single"},
	}

	for _, tt := range tests {
		got := extractDomain(tt.input)
		if got != tt.want {
			t.Errorf("extractDomain(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
