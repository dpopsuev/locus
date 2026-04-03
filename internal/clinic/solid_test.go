package clinic

import (
	"testing"

	"github.com/dpopsuev/locus/internal/analysis"
	"github.com/dpopsuev/locus/internal/arch"
	"github.com/dpopsuev/locus/internal/model"
	"github.com/dpopsuev/locus/internal/port"
)

func TestComputeSRPViolations_HighFanOut(t *testing.T) {
	services := []arch.ArchService{
		{Name: "internal/bigpkg", LOC: 1200, Symbols: make([]model.Symbol, 5)},
	}
	// Create 10 outbound edges to trigger LOC>1000 && fan-out>8.
	// Fan-in=0 (no edges TO bigpkg) → warning severity (low blast radius).
	edges := make([]arch.ArchEdge, 10)
	for i := range edges {
		edges[i] = arch.ArchEdge{From: "internal/bigpkg", To: "internal/target" + string(rune('a'+i))}
	}

	violations := ComputeSRPViolations(services, edges, nil, nil)

	if len(violations) == 0 {
		t.Fatal("expected at least 1 SRP violation for LOC=1200, fan-out=10")
	}

	found := false
	for _, v := range violations {
		if v.Severity == port.SeverityWarning && v.Principle == PrincipleSRP {
			found = true
		}
	}
	if !found {
		t.Error("expected a warning-severity SRP violation (fan-in=0, low blast radius)")
	}
}

func TestComputeSRPViolations_Warning(t *testing.T) {
	services := []arch.ArchService{
		{Name: "internal/midpkg", LOC: 600, Symbols: make([]model.Symbol, 5)},
	}
	edges := make([]arch.ArchEdge, 6)
	for i := range edges {
		edges[i] = arch.ArchEdge{From: "internal/midpkg", To: "internal/dep" + string(rune('a'+i))}
	}

	violations := ComputeSRPViolations(services, edges, nil, nil)

	if len(violations) == 0 {
		t.Fatal("expected at least 1 SRP warning for LOC=600, fan-out=6")
	}

	for _, v := range violations {
		if v.Severity != port.SeverityWarning {
			t.Errorf("expected warning severity, got %s", v.Severity)
		}
	}
}

func TestComputeSRPViolations_Clean(t *testing.T) {
	services := []arch.ArchService{
		{Name: "internal/small", LOC: 100, Symbols: make([]model.Symbol, 3)},
	}
	edges := []arch.ArchEdge{
		{From: "internal/small", To: "internal/a"},
		{From: "internal/small", To: "internal/b"},
	}

	violations := ComputeSRPViolations(services, edges, nil, nil)

	if len(violations) != 0 {
		t.Errorf("expected 0 violations for LOC=100, fan-out=2, got %d", len(violations))
	}
}

func TestComputeSRPViolations_DomainDiversity(t *testing.T) {
	services := []arch.ArchService{
		{Name: "internal/hub", LOC: 200, Symbols: make([]model.Symbol, 25)},
	}
	// 4 distinct domains: store, arch, analysis, protocol.
	edges := []arch.ArchEdge{
		{From: "internal/hub", To: "internal/store/sql"},
		{From: "internal/hub", To: "internal/arch"},
		{From: "internal/hub", To: "internal/analysis"},
		{From: "internal/hub", To: "internal/protocol"},
	}

	violations := ComputeSRPViolations(services, edges, nil, nil)

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

	violations := ComputeISPViolations(classes, nil, nil)

	if len(violations) == 0 {
		t.Fatal("expected ISP violation for interface with 9 methods")
	}

	v := violations[0]
	if v.Severity != port.SeverityError {
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

	violations := ComputeISPViolations(classes, nil, nil)

	if len(violations) == 0 {
		t.Fatal("expected ISP warning for interface with 6 methods")
	}

	v := violations[0]
	if v.Severity != port.SeverityWarning {
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

	violations := ComputeISPViolations(classes, nil, nil)

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

	violations := ComputeISPViolations(classes, impls, nil)

	if len(violations) != 0 {
		t.Errorf("expected 0 violations (4-method interface is fine, implementors not checked), got %d", len(violations))
	}
}

func TestComputeOCPViolations_EmptyRoot(t *testing.T) {
	violations := ComputeOCPViolations("", nil)

	if violations != nil {
		t.Errorf("expected nil for empty root, got %d violations", len(violations))
	}
}

func TestComputeDIPViolations_NilClassification(t *testing.T) {
	services := []arch.ArchService{{Name: "a"}}
	edges := []arch.ArchEdge{{From: "a", To: "b"}}

	violations := ComputeDIPViolations(services, edges, nil, nil)

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

	violations := ComputeDIPViolations(services, edges, classification, nil)

	if len(violations) == 0 {
		t.Fatal("expected DIP violation for domain → adapter")
	}

	v := violations[0]
	if v.Severity != port.SeverityError {
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

	violations := ComputeDIPViolations(services, edges, classification, nil)

	if len(violations) == 0 {
		t.Fatal("expected DIP warning for app → adapter")
	}

	v := violations[0]
	if v.Severity != port.SeverityWarning {
		t.Errorf("expected warning severity for app → adapter, got %s", v.Severity)
	}
}

func TestComputeSOLIDScan_Score(t *testing.T) {
	// Setup: 1 SRP violation (warning) + 1 ISP violation (error) = 2 violations.
	// 1 service × 4 principles = 4 checks. Score = 100 - 2/4*100 = 50.
	services := []arch.ArchService{
		{Name: "internal/big", LOC: 600, Symbols: make([]model.Symbol, 5)},
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

	report := ComputeSOLIDScan(services, edges, classes, nil, nil, "", nil, nil)

	expectedViolations := 2
	if len(report.Violations) != expectedViolations {
		t.Fatalf("expected %d violations, got %d", expectedViolations, len(report.Violations))
	}

	expectedScore := 50.0 // 100 - 2/4*100
	if float64(report.Score) != expectedScore {
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
		{Name: "internal/clean", LOC: 50, Symbols: make([]model.Symbol, 3)},
	}
	edges := []arch.ArchEdge{
		{From: "internal/clean", To: "internal/a"},
	}
	classes := []analysis.ClassInfo{
		{Name: "SmallIface", Package: "pkg", Kind: "interface", Methods: make([]analysis.MethodInfo, 2)},
	}

	report := ComputeSOLIDScan(services, edges, classes, nil, nil, "", nil, nil)

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

	report := ComputeSOLIDScan(services, nil, classes, nil, nil, "", nil, nil)

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

	report := ComputeSOLIDScan(nil, nil, classes, nil, nil, "", nil, nil)

	if len(report.Violations) < 2 {
		t.Fatalf("expected at least 2 violations, got %d", len(report.Violations))
	}

	// First violation should be the error.
	if report.Violations[0].Severity != port.SeverityError {
		t.Errorf("expected first violation to be error, got %s", report.Violations[0].Severity)
	}
	// Second violation should be the warning.
	if report.Violations[1].Severity != port.SeverityWarning {
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

// --- ISP severity by implementor count (TSK-178) ---

func TestISPSeverity_FewImplementors(t *testing.T) {
	// Fat interface (9 methods, base=error) with 2 implementors → warning.
	methods := make([]analysis.MethodInfo, 9)
	for i := range methods {
		methods[i] = analysis.MethodInfo{Name: "Method" + string(rune('A'+i)), Exported: true}
	}
	classes := []analysis.ClassInfo{
		{Name: "BigIface", Package: "pkg", Kind: "interface", Methods: methods},
	}
	impls := []analysis.ImplEdge{
		{From: "ImplA", To: "BigIface", Kind: "implements"},
		{From: "ImplB", To: "BigIface", Kind: "implements"},
	}

	violations := ComputeISPViolations(classes, impls, nil)

	if len(violations) == 0 {
		t.Fatal("expected ISP violation for 9-method interface")
	}
	v := violations[0]
	if v.Severity != port.SeverityWarning {
		t.Errorf("expected warning severity for 2 implementors, got %s", v.Severity)
	}
	if v.Principle != PrincipleISP {
		t.Errorf("expected ISP principle, got %s", v.Principle)
	}
}

func TestISPSeverity_ManyImplementors(t *testing.T) {
	// Fat interface (9 methods) with 6+ implementors → critical.
	methods := make([]analysis.MethodInfo, 9)
	for i := range methods {
		methods[i] = analysis.MethodInfo{Name: "Method" + string(rune('A'+i)), Exported: true}
	}
	classes := []analysis.ClassInfo{
		{Name: "WideIface", Package: "pkg", Kind: "interface", Methods: methods},
	}
	impls := make([]analysis.ImplEdge, 7)
	for i := range impls {
		impls[i] = analysis.ImplEdge{
			From: "Impl" + string(rune('A'+i)),
			To:   "WideIface",
			Kind: "implements",
		}
	}

	violations := ComputeISPViolations(classes, impls, nil)

	if len(violations) == 0 {
		t.Fatal("expected ISP violation for 9-method interface with 7 implementors")
	}
	v := violations[0]
	if v.Severity != port.SeverityCritical {
		t.Errorf("expected critical severity for 7 implementors, got %s", v.Severity)
	}
}

// --- SRP severity by blast radius / fan-in (TSK-179) ---

func TestSRPSeverity_LowFanIn(t *testing.T) {
	// God component (LOC=1200, fan-out=10) with fan-in=1 → warning.
	services := []arch.ArchService{
		{Name: "internal/godpkg", LOC: 1200, Symbols: make([]model.Symbol, 5)},
	}
	edges := make([]arch.ArchEdge, 10)
	for i := range edges {
		edges[i] = arch.ArchEdge{From: "internal/godpkg", To: "internal/dep" + string(rune('a'+i))}
	}
	// Add 1 inbound edge (fan-in=1, low blast radius).
	edges = append(edges, arch.ArchEdge{From: "internal/caller", To: "internal/godpkg"})

	violations := ComputeSRPViolations(services, edges, nil, nil)

	if len(violations) == 0 {
		t.Fatal("expected SRP violation for LOC=1200, fan-out=10")
	}
	v := violations[0]
	if v.Severity != port.SeverityWarning {
		t.Errorf("expected warning severity for fan-in=1, got %s", v.Severity)
	}
}

func TestSRPSeverity_HighFanIn(t *testing.T) {
	// God component (LOC=1200, fan-out=10) with fan-in=10 → critical.
	services := []arch.ArchService{
		{Name: "internal/godpkg", LOC: 1200, Symbols: make([]model.Symbol, 5)},
	}
	edges := make([]arch.ArchEdge, 10)
	for i := range edges {
		edges[i] = arch.ArchEdge{From: "internal/godpkg", To: "internal/dep" + string(rune('a'+i))}
	}
	// Add 10 inbound edges (fan-in=10, high blast radius).
	for i := range 10 {
		edges = append(edges, arch.ArchEdge{From: "internal/caller" + string(rune('a'+i)), To: "internal/godpkg"})
	}

	violations := ComputeSRPViolations(services, edges, nil, nil)

	if len(violations) == 0 {
		t.Fatal("expected SRP violation for LOC=1200, fan-out=10")
	}
	v := violations[0]
	if v.Severity != port.SeverityCritical {
		t.Errorf("expected critical severity for fan-in=10, got %s", v.Severity)
	}
}

// --- TSK-176: Role multiplier tests ---

func TestRoleMultiplier(t *testing.T) {
	tests := []struct {
		role HexaRole
		want float64
	}{
		{HexaRoleEntry, 2.0},
		{HexaRoleApp, 1.5},
		{HexaRoleAdapter, 1.3},
		{HexaRoleInfra, 1.2},
		{HexaRoleDomain, 0.8},
		{HexaRolePort, 1.0},
	}
	for _, tt := range tests {
		got := RoleMultiplier(tt.role)
		if got != tt.want {
			t.Errorf("RoleMultiplier(%q) = %f, want %f", tt.role, got, tt.want)
		}
	}
	// Unknown role should return 1.0.
	if got := RoleMultiplier("unknown"); got != 1.0 {
		t.Errorf("RoleMultiplier(unknown) = %f, want 1.0", got)
	}
}

func TestSRPWithRoleMultiplier_AppLenient(t *testing.T) {
	// LOC=1400 with role=app (multiplier=1.5): error threshold = 1500, warning threshold = 750.
	// LOC=1400 < 1500 error threshold but > 750 warning threshold.
	// Fan-out=10: warning threshold = int(5*1.5)=7, error threshold = int(8*1.5)=12.
	// 10 > 7 → warning condition met. So: warning, NOT error.
	services := []arch.ArchService{
		{Name: "app/orchestrator", LOC: 1400, Symbols: make([]model.Symbol, 5)},
	}
	edges := make([]arch.ArchEdge, 10)
	for i := range edges {
		edges[i] = arch.ArchEdge{From: "app/orchestrator", To: "internal/dep" + string(rune('a'+i))}
	}
	roles := map[string]HexaRole{"app/orchestrator": HexaRoleApp}

	violations := ComputeSRPViolations(services, edges, roles, nil)

	// Should NOT trigger error-level violation (LOC=1400 < 1500 error threshold).
	for _, v := range violations {
		if v.Principle == PrincipleSRP {
			// With fan-in=0 (no inbound edges), severity from srpSeverityByFanIn is warning.
			// The violation should be warning level (not error/critical) because LOC < error threshold.
			if v.Severity == port.SeverityCritical {
				t.Errorf("app role with LOC=1400 should NOT be critical (error threshold=1500)")
			}
		}
	}
}

func TestSRPWithDomainRole_Stricter(t *testing.T) {
	// LOC=900 with role=domain (multiplier=0.8): error threshold = 800, warning threshold = 400.
	// Fan-out=9: error threshold = int(8*0.8)=6.
	// LOC=900 > 800 AND fan-out=9 > 6 → should trigger a violation.
	services := []arch.ArchService{
		{Name: "domain/model", LOC: 900, Symbols: make([]model.Symbol, 5)},
	}
	edges := make([]arch.ArchEdge, 9)
	for i := range edges {
		edges[i] = arch.ArchEdge{From: "domain/model", To: "internal/dep" + string(rune('a'+i))}
	}
	roles := map[string]HexaRole{"domain/model": HexaRoleDomain}

	violations := ComputeSRPViolations(services, edges, roles, nil)

	if len(violations) == 0 {
		t.Fatal("domain role with LOC=900 should trigger SRP violation (threshold=800)")
	}
	found := false
	for _, v := range violations {
		if v.Principle == PrincipleSRP {
			found = true
		}
	}
	if !found {
		t.Error("expected SRP violation for domain component with LOC=900")
	}
}

// --- TSK-177: Accepted violation tests ---

func TestIsAccepted(t *testing.T) {
	accepted := []port.AcceptedViolation{
		{Component: "app/main", Principle: "SRP", Reason: "composition root"},
		{Component: "internal/hub", Principle: "god_component", Reason: "known monolith"},
	}

	if !IsAccepted(accepted, "app/main", "SRP") {
		t.Error("expected app/main + SRP to be accepted")
	}
	if !IsAccepted(accepted, "internal/hub", "god_component") {
		t.Error("expected internal/hub + god_component to be accepted")
	}
	if IsAccepted(accepted, "app/main", "DIP") {
		t.Error("expected app/main + DIP to NOT be accepted")
	}
	if IsAccepted(accepted, "other/pkg", "SRP") {
		t.Error("expected other/pkg + SRP to NOT be accepted")
	}
	if IsAccepted(nil, "app/main", "SRP") {
		t.Error("expected nil accepted list to return false")
	}
}

func TestSRPWithAccepted(t *testing.T) {
	// Same setup as TestComputeSRPViolations_HighFanOut — should trigger SRP violation.
	services := []arch.ArchService{
		{Name: "app/big", LOC: 1200, Symbols: make([]model.Symbol, 5)},
	}
	edges := make([]arch.ArchEdge, 10)
	for i := range edges {
		edges[i] = arch.ArchEdge{From: "app/big", To: "internal/target" + string(rune('a'+i))}
	}

	// Without accepted: should have violations.
	violations := ComputeSRPViolations(services, edges, nil, nil)
	if len(violations) == 0 {
		t.Fatal("expected SRP violation without accepted list")
	}

	// With accepted: should suppress.
	accepted := []port.AcceptedViolation{
		{Component: "app/big", Principle: "SRP", Reason: "composition root"},
	}
	violations = ComputeSRPViolations(services, edges, nil, accepted)
	if len(violations) != 0 {
		t.Errorf("expected 0 violations with accepted SRP for app/big, got %d", len(violations))
	}
}
