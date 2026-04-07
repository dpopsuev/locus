package hexa

import (
	"fmt"
	"testing"

	"github.com/dpopsuev/locus/internal/arch"
	"github.com/dpopsuev/locus/internal/model"
	"github.com/dpopsuev/locus/internal/oculus"
)

func TestComputeHexaClassification_BasicRoles(t *testing.T) {
	services := []arch.ArchService{
		// Entrypoint: has "main" symbol.
		{Name: "cmd/server", Symbols: model.SymbolsFromNames("main")},
		// Adapter: has external edges (structural, not keyword-based).
		{Name: "internal/notify"},
		// Domain: default.
		{Name: "internal/domain"},
	}
	edges := []arch.ArchEdge{
		{From: "internal/notify", To: "github.com/aws/sns", Protocol: "external"},
	}

	report := ComputeHexaClassification(services, edges, nil)

	roleOf := make(map[string]HexaRole)
	for _, c := range report.Components {
		roleOf[c.Name] = c.Role
	}

	if roleOf["cmd/server"] != HexaRoleEntry {
		t.Errorf("cmd/server: expected entrypoint, got %s", roleOf["cmd/server"])
	}
	if roleOf["internal/notify"] != HexaRoleAdapter {
		t.Errorf("internal/notify: expected adapter (external edge), got %s", roleOf["internal/notify"])
	}
	if roleOf["internal/domain"] != HexaRoleDomain {
		t.Errorf("internal/domain: expected domain, got %s", roleOf["internal/domain"])
	}
}

func TestComputeHexaClassification_PortDetection(t *testing.T) {
	services := []arch.ArchService{
		{Name: "internal/ports", Package: "github.com/example/app/internal/ports",
			Symbols: []model.Symbol{
				{Name: "UserRepo", Kind: model.SymbolInterface, Exported: true},
				{Name: "EventBus", Kind: model.SymbolInterface, Exported: true},
				{Name: "Config", Kind: model.SymbolClass, Exported: true},
			}},
	}

	report := ComputeHexaClassification(services, nil, nil)

	if len(report.Components) != 1 {
		t.Fatalf("expected 1 component, got %d", len(report.Components))
	}
	if report.Components[0].Role != HexaRolePort {
		t.Errorf("expected port (66%% interfaces), got %s (reason: %s)", report.Components[0].Role, report.Components[0].Reason)
	}
}

func TestComputeHexaClassification_PortFallbackFromClasses(t *testing.T) {
	// When Symbol.Kind is not available, fall back to class oculus.
	services := []arch.ArchService{
		{Name: "internal/ports", Package: "github.com/example/app/internal/ports"},
	}
	classes := []oculus.ClassInfo{
		{Name: "UserRepo", Package: "github.com/example/app/internal/ports", Kind: "interface"},
		{Name: "EventBus", Package: "github.com/example/app/internal/ports", Kind: "interface"},
		{Name: "Config", Package: "github.com/example/app/internal/ports", Kind: "struct"},
	}

	report := ComputeHexaClassification(services, nil, classes)

	if report.Components[0].Role != HexaRolePort {
		t.Errorf("expected port from class analysis, got %s", report.Components[0].Role)
	}
}

func TestComputeHexaClassification_InfraHighFanIn(t *testing.T) {
	// Component imported by >40% of all components → infra.
	services := []arch.ArchService{
		{Name: "internal/logging"},
		{Name: "internal/core"},
		{Name: "internal/api"},
		{Name: "internal/worker"},
		{Name: "internal/scheduler"},
	}
	// logging imported by 3/5 = 60% → infra.
	edges := []arch.ArchEdge{
		{From: "internal/core", To: "internal/logging"},
		{From: "internal/api", To: "internal/logging"},
		{From: "internal/worker", To: "internal/logging"},
	}

	report := ComputeHexaClassification(services, edges, nil)

	roleOf := make(map[string]HexaRole)
	for _, c := range report.Components {
		roleOf[c.Name] = c.Role
	}

	if roleOf["internal/logging"] != HexaRoleInfra {
		t.Errorf("expected infra for logging (60%% fan-in), got %s", roleOf["internal/logging"])
	}
}

func TestComputeHexaClassification_CompositionRoot(t *testing.T) {
	// High fan-out, low fan-in → composition root (entrypoint).
	services := make([]arch.ArchService, 12)
	services[0] = arch.ArchService{Name: "cmd/app"}
	for i := 1; i < 12; i++ {
		services[i] = arch.ArchService{Name: fmt.Sprintf("pkg/%d", i)}
	}
	edges := make([]arch.ArchEdge, 0, 11)
	for i := 1; i < 12; i++ {
		edges = append(edges, arch.ArchEdge{From: "cmd/app", To: fmt.Sprintf("pkg/%d", i)})
	}

	report := ComputeHexaClassification(services, edges, nil)

	roleOf := make(map[string]HexaRole)
	for _, c := range report.Components {
		roleOf[c.Name] = c.Role
	}

	if roleOf["cmd/app"] != HexaRoleEntry {
		t.Errorf("expected entrypoint for cmd/app (fan-out=11, fan-in=0), got %s", roleOf["cmd/app"])
	}
}

func TestComputeHexaClassification_SortOrder(t *testing.T) {
	services := []arch.ArchService{
		{Name: "internal/domain"},
		{Name: "cmd/api", Symbols: model.SymbolsFromNames("main")},
		{Name: "internal/notify"},
	}
	edges := []arch.ArchEdge{
		{From: "internal/notify", To: "external/lib", Protocol: "external"},
	}

	report := ComputeHexaClassification(services, edges, nil)

	// Expected order: entrypoint first, adapter second, domain last.
	expected := []HexaRole{HexaRoleEntry, HexaRoleAdapter, HexaRoleDomain}
	for i, c := range report.Components {
		if c.Role != expected[i] {
			t.Errorf("position %d: expected %s, got %s (name: %s)", i, expected[i], c.Role, c.Name)
		}
	}
}

func TestComputeHexaClassification_ExternalEdgeAdapter(t *testing.T) {
	services := []arch.ArchService{
		{Name: "internal/notify"},
	}
	edges := []arch.ArchEdge{
		{From: "internal/notify", To: "github.com/aws/sns", Protocol: "external"},
	}

	report := ComputeHexaClassification(services, edges, nil)

	if report.Components[0].Role != HexaRoleAdapter {
		t.Errorf("expected adapter (external edge), got %s", report.Components[0].Role)
	}
}

func TestComputeHexaViolations_Clean(t *testing.T) {
	services := []arch.ArchService{
		{Name: "internal/handler"},
		{Name: "internal/domain"},
	}
	// handler → domain is adapter → domain — allowed.
	edges := []arch.ArchEdge{
		{From: "internal/handler", To: "external/http", Protocol: "external"}, // makes handler an adapter
		{From: "internal/handler", To: "internal/domain"},
	}

	report := ComputeHexaViolations(services, edges, nil)

	if len(report.Violations) != 0 {
		for _, v := range report.Violations {
			t.Logf("unexpected violation: %s(%s) → %s(%s): %s", v.From, v.FromRole, v.To, v.ToRole, v.Rule)
		}
		t.Errorf("expected 0 violations, got %d", len(report.Violations))
	}
}

func TestComputeHexaViolations_DomainImportsAdapter(t *testing.T) {
	services := []arch.ArchService{
		{Name: "internal/adapter"},
		{Name: "internal/domain"},
	}
	edges := []arch.ArchEdge{
		{From: "internal/adapter", To: "external/lib", Protocol: "external"}, // makes adapter structural
		{From: "internal/domain", To: "internal/adapter"},                    // domain → adapter = violation
	}

	report := ComputeHexaViolations(services, edges, nil)

	if len(report.Violations) == 0 {
		t.Error("expected violation: domain → adapter")
	}
	for _, v := range report.Violations {
		t.Logf("violation: %s(%s) → %s(%s): %s [%s]", v.From, v.FromRole, v.To, v.ToRole, v.Rule, v.Severity)
	}
}

func TestComputeHexaViolations_PortToAdapter(t *testing.T) {
	services := []arch.ArchService{
		{Name: "internal/ports", Symbols: []model.Symbol{
			{Name: "Repo", Kind: model.SymbolInterface, Exported: true},
			{Name: "Bus", Kind: model.SymbolInterface, Exported: true},
		}},
		{Name: "internal/adapter"},
	}
	edges := []arch.ArchEdge{
		{From: "internal/adapter", To: "external/lib", Protocol: "external"},
		{From: "internal/ports", To: "internal/adapter"}, // port → adapter = violation
	}

	report := ComputeHexaViolations(services, edges, nil)

	if len(report.Violations) == 0 {
		t.Error("expected violation: port → adapter")
	}
}

func TestComputeHexaViolations_MultipleRules(t *testing.T) {
	services := []arch.ArchService{
		{Name: "internal/domain"},
		{Name: "internal/adapter"},
		{Name: "internal/ports", Symbols: []model.Symbol{
			{Name: "Store", Kind: model.SymbolInterface, Exported: true},
		}},
	}
	edges := []arch.ArchEdge{
		{From: "internal/adapter", To: "external/db", Protocol: "external"},
		{From: "internal/domain", To: "internal/adapter"}, // violation
		{From: "internal/ports", To: "internal/adapter"},  // violation
	}

	report := ComputeHexaViolations(services, edges, nil)

	if len(report.Violations) < 2 {
		t.Errorf("expected >= 2 violations, got %d", len(report.Violations))
	}
	if float64(report.Score) >= 100 {
		t.Errorf("expected score < 100 with violations, got %.0f", report.Score)
	}
}

func TestComputeHexaViolations_Scoped(t *testing.T) {
	services := []arch.ArchService{
		{Name: "internal/domain"},
		{Name: "internal/adapter"},
		{Name: "pkg/lib"},
	}
	edges := []arch.ArchEdge{
		{From: "internal/adapter", To: "external/http", Protocol: "external"},
		{From: "internal/domain", To: "internal/adapter"},
		{From: "pkg/lib", To: "internal/adapter"},
	}

	report := ComputeHexaViolations(services, edges, nil)

	// Should detect violations for both domain → adapter and pkg/lib → adapter
	t.Logf("score: %.0f, violations: %d", report.Score, len(report.Violations))
}

func TestComputeHexaClassification_EmptyRepo(t *testing.T) {
	report := ComputeHexaClassification(nil, nil, nil)

	if len(report.Components) != 0 {
		t.Errorf("expected 0 components, got %d", len(report.Components))
	}
}
