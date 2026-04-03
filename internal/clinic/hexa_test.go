package clinic

import (
	"testing"

	"github.com/dpopsuev/locus/internal/analysis"
	"github.com/dpopsuev/locus/internal/arch"
	"github.com/dpopsuev/locus/internal/port"
)

func TestComputeHexaClassification_BasicRoles(t *testing.T) {
	services := []arch.ArchService{
		{Name: "cmd/server"},
		{Name: "internal/http/handler"},
		{Name: "internal/domain"},
	}

	report := ComputeHexaClassification(services, nil, nil)

	if len(report.Components) != 3 {
		t.Fatalf("expected 3 components, got %d", len(report.Components))
	}

	roleOf := make(map[string]HexaRole)
	for _, c := range report.Components {
		roleOf[c.Name] = c.Role
	}

	if roleOf["cmd/server"] != HexaRoleEntry {
		t.Errorf("cmd/server: expected entrypoint, got %s", roleOf["cmd/server"])
	}
	if roleOf["internal/http/handler"] != HexaRoleAdapter {
		t.Errorf("internal/http/handler: expected adapter, got %s", roleOf["internal/http/handler"])
	}
	if roleOf["internal/domain"] != HexaRoleDomain {
		t.Errorf("internal/domain: expected domain, got %s", roleOf["internal/domain"])
	}
}

func TestComputeHexaClassification_PortDetection(t *testing.T) {
	services := []arch.ArchService{
		{Name: "internal/ports", Package: "github.com/example/app/internal/ports"},
	}
	classes := []analysis.ClassInfo{
		{Name: "UserRepo", Package: "github.com/example/app/internal/ports", Kind: "interface"},
		{Name: "EventBus", Package: "github.com/example/app/internal/ports", Kind: "interface"},
		{Name: "Config", Package: "github.com/example/app/internal/ports", Kind: "struct"},
	}

	report := ComputeHexaClassification(services, nil, classes)

	if len(report.Components) != 1 {
		t.Fatalf("expected 1 component, got %d", len(report.Components))
	}
	if report.Components[0].Role != HexaRolePort {
		t.Errorf("expected port, got %s (reason: %s)", report.Components[0].Role, report.Components[0].Reason)
	}
}

func TestComputeHexaClassification_SortOrder(t *testing.T) {
	services := []arch.ArchService{
		{Name: "internal/domain/user"},
		{Name: "cmd/api"},
		{Name: "internal/handler"},
		{Name: "internal/config"},
	}

	report := ComputeHexaClassification(services, nil, nil)

	expected := []HexaRole{HexaRoleEntry, HexaRoleAdapter, HexaRoleInfra, HexaRoleDomain}
	for i, c := range report.Components {
		if c.Role != expected[i] {
			t.Errorf("position %d: expected role %s, got %s (name: %s)", i, expected[i], c.Role, c.Name)
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

	if len(report.Components) != 1 {
		t.Fatalf("expected 1 component, got %d", len(report.Components))
	}
	if report.Components[0].Role != HexaRoleAdapter {
		t.Errorf("expected adapter (external edge), got %s", report.Components[0].Role)
	}
}

func TestComputeHexaViolations_Clean(t *testing.T) {
	services := []arch.ArchService{
		{Name: "internal/handler"},
		{Name: "internal/domain"},
		{Name: "internal/app"},
	}
	edges := []arch.ArchEdge{
		{From: "internal/handler", To: "internal/domain"},
		{From: "internal/app", To: "internal/domain"},
	}

	report := ComputeHexaViolations(services, edges, nil)

	if len(report.Violations) != 0 {
		t.Errorf("expected 0 violations, got %d", len(report.Violations))
		for _, v := range report.Violations {
			t.Logf("  %s (%s) -> %s (%s): %s", v.From, v.FromRole, v.To, v.ToRole, v.Rule)
		}
	}
	if report.Score != 100.0 {
		t.Errorf("expected score 100.0, got %.1f", report.Score)
	}
}

func TestComputeHexaViolations_DomainImportsAdapter(t *testing.T) {
	services := []arch.ArchService{
		{Name: "internal/domain"},
		{Name: "internal/handler"},
	}
	edges := []arch.ArchEdge{
		{From: "internal/domain", To: "internal/handler"},
	}

	report := ComputeHexaViolations(services, edges, nil)

	if len(report.Violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(report.Violations))
	}

	v := report.Violations[0]
	if v.FromRole != HexaRoleDomain {
		t.Errorf("expected from_role domain, got %s", v.FromRole)
	}
	if v.ToRole != HexaRoleAdapter {
		t.Errorf("expected to_role adapter, got %s", v.ToRole)
	}
	if v.Severity != port.SeverityError {
		t.Errorf("expected error severity, got %s", v.Severity)
	}
	if v.Rule != "domain must not depend on adapter" {
		t.Errorf("unexpected rule: %s", v.Rule)
	}
}

func TestComputeHexaViolations_EmptyInput(t *testing.T) {
	report := ComputeHexaViolations(nil, nil, nil)

	if len(report.Violations) != 0 {
		t.Errorf("expected 0 violations, got %d", len(report.Violations))
	}
	if report.Score != 100.0 {
		t.Errorf("expected score 100.0, got %.1f", report.Score)
	}
	if report.Summary != "Hexagonal compliance: 100% — no violations" {
		t.Errorf("unexpected summary: %s", report.Summary)
	}
}

func TestComputeHexaViolations_MultipleRules(t *testing.T) {
	services := []arch.ArchService{
		{Name: "internal/domain"},
		{Name: "internal/handler"},
		{Name: "internal/config"},
		{Name: "internal/app"},
	}
	edges := []arch.ArchEdge{
		{From: "internal/domain", To: "internal/handler"}, // domain -> adapter = error
		{From: "internal/domain", To: "internal/config"},  // domain -> infra = error
		{From: "internal/domain", To: "internal/app"},     // domain -> app = warning
		{From: "internal/app", To: "internal/domain"},     // app -> domain = OK
	}

	report := ComputeHexaViolations(services, edges, nil)

	if len(report.Violations) != 3 {
		t.Fatalf("expected 3 violations, got %d", len(report.Violations))
	}

	// Verify sort order: errors before warnings.
	errorsSeen := 0
	warningStartIdx := -1
	for i, v := range report.Violations {
		if v.Severity == port.SeverityError {
			errorsSeen++
		}
		if v.Severity == port.SeverityWarning && warningStartIdx == -1 {
			warningStartIdx = i
		}
	}
	if errorsSeen != 2 {
		t.Errorf("expected 2 errors, got %d", errorsSeen)
	}
	if warningStartIdx != -1 && warningStartIdx < errorsSeen {
		t.Error("warnings appeared before errors in sorted output")
	}

	// Score: (4 - 3) / 4 * 100 = 25%
	expectedScore := 25.0
	if float64(report.Score) != expectedScore {
		t.Errorf("expected score %.1f, got %.1f", expectedScore, report.Score)
	}
}

func TestResolveRoles_ManualOverride(t *testing.T) {
	classification := &HexaClassificationReport{
		Components: []HexaComponent{
			{Name: "internal/driver", Role: HexaRoleAdapter, Reason: "name contains adapter keyword"},
			{Name: "internal/domain", Role: HexaRoleDomain, Reason: "default classification"},
		},
	}

	overrides := map[string]string{
		"internal/driver": "port",
	}

	roles := ResolveRoles(classification, overrides)

	if roles["internal/driver"] != HexaRolePort {
		t.Errorf("expected 'port' for internal/driver (override), got %q", roles["internal/driver"])
	}
	if roles["internal/domain"] != HexaRoleDomain {
		t.Errorf("expected 'domain' for internal/domain, got %q", roles["internal/domain"])
	}
}

func TestResolveRoles_NoOverride(t *testing.T) {
	classification := &HexaClassificationReport{
		Components: []HexaComponent{
			{Name: "internal/handler", Role: HexaRoleAdapter, Reason: "name contains adapter keyword"},
			{Name: "internal/domain", Role: HexaRoleDomain, Reason: "default classification"},
			{Name: "cmd/server", Role: HexaRoleEntry, Reason: "command entrypoint"},
		},
	}

	roles := ResolveRoles(classification, nil)

	if len(roles) != 3 {
		t.Fatalf("expected 3 roles, got %d", len(roles))
	}
	if roles["internal/handler"] != HexaRoleAdapter {
		t.Errorf("expected adapter, got %q", roles["internal/handler"])
	}
	if roles["internal/domain"] != HexaRoleDomain {
		t.Errorf("expected domain, got %q", roles["internal/domain"])
	}
	if roles["cmd/server"] != HexaRoleEntry {
		t.Errorf("expected entrypoint, got %q", roles["cmd/server"])
	}
}

func TestComputeHexaViolations_PortToAdapter(t *testing.T) {
	services := []arch.ArchService{
		{Name: "internal/ports", Package: "github.com/example/app/internal/ports"},
		{Name: "internal/handler"},
	}
	classes := []analysis.ClassInfo{
		{Name: "Repo", Package: "github.com/example/app/internal/ports", Kind: "interface"},
	}
	edges := []arch.ArchEdge{
		{From: "internal/ports", To: "internal/handler"},
	}

	report := ComputeHexaViolations(services, edges, classes)

	if len(report.Violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(report.Violations))
	}
	v := report.Violations[0]
	if v.Rule != "port must not depend on adapter" {
		t.Errorf("unexpected rule: %s", v.Rule)
	}
	if v.Severity != port.SeverityError {
		t.Errorf("expected error severity, got %s", v.Severity)
	}
}
