package clinic

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dpopsuev/locus/internal/analysis"
	"github.com/dpopsuev/locus/internal/arch"
	"github.com/dpopsuev/locus/internal/port"
)

// classKindInterface is the Kind value for interfaces in analysis.ClassInfo.
const classKindInterface = "interface"

// HexaRole classifies a component's hexagonal architecture role.
type HexaRole string

const (
	HexaRoleDomain  HexaRole = "domain"
	HexaRolePort    HexaRole = "port"
	HexaRoleAdapter HexaRole = "adapter"
	HexaRoleInfra   HexaRole = "infra"
	HexaRoleApp     HexaRole = "app"
	HexaRoleEntry   HexaRole = "entrypoint"
)

// hexaRoleOrder defines sort priority for roles (lower = first).
var hexaRoleOrder = map[HexaRole]int{
	HexaRoleEntry:   0,
	HexaRoleAdapter: 1,
	HexaRoleInfra:   2,
	HexaRolePort:    3,
	HexaRoleApp:     4,
	HexaRoleDomain:  5,
}

// Adapter-matching keywords for service name classification.
var adapterKeywords = []string{
	"handler", "server", "client", "repository", "repo", "gateway",
	"driver", "adapter", "grpc", "http", "api", "db",
	"postgres", "mysql", "redis", "kafka", "rabbitmq", "mcp",
}

// Infra-matching keywords for service name classification.
var infraKeywords = []string{
	"config", "logging", "telemetry", "metrics", "middleware",
	"store", "storage", "persist", "cache",
}

// App-matching keywords for service name classification.
var appKeywords = []string{
	"app", "usecase", "orchestrat", "workflow",
}

// portInterfaceRatioThreshold is the minimum ratio of interfaces to total types
// for a package to be classified as a port.
const portInterfaceRatioThreshold = 0.5

// HexaComponent represents a classified component in a hexagonal architecture.
type HexaComponent struct {
	Name   string   `json:"name"`
	Role   HexaRole `json:"role"`
	Reason string   `json:"reason"`
}

// HexaViolation records a dependency that breaks hexagonal architecture rules.
type HexaViolation struct {
	From     string   `json:"from"`
	To       string   `json:"to"`
	FromRole HexaRole `json:"from_role"`
	ToRole   HexaRole `json:"to_role"`
	Rule     string   `json:"rule"`
	Severity string   `json:"severity"`
}

// HexaClassificationReport contains the classification of all components.
type HexaClassificationReport struct {
	Components []HexaComponent `json:"components"`
	Summary    string          `json:"summary"`
}

// HexaValidationReport contains classification, violations, and compliance score.
type HexaValidationReport struct {
	Classification []HexaComponent `json:"classification"`
	Violations     []HexaViolation `json:"violations"`
	Score          float64         `json:"score"`
	Summary        string          `json:"summary"`
}

// ComputeHexaClassification classifies each service into a hexagonal architecture role
// using heuristics based on name, outgoing edge protocols, and type composition.
func ComputeHexaClassification(
	services []arch.ArchService,
	edges []arch.ArchEdge,
	classes []analysis.ClassInfo,
) *HexaClassificationReport {
	// Build set of services that have outgoing external edges.
	externalSources := make(map[string]bool)
	for _, e := range edges {
		if e.Protocol == "external" {
			externalSources[e.From] = true
		}
	}

	// Build per-package interface and total type counts from classes.
	ifaceCounts := make(map[string]int)
	totalCounts := make(map[string]int)
	for _, c := range classes {
		totalCounts[c.Package]++
		if c.Kind == classKindInterface {
			ifaceCounts[c.Package]++
		}
	}

	components := make([]HexaComponent, 0, len(services))
	for i := range services {
		svc := &services[i]
		role, reason := classifyService(svc, externalSources, ifaceCounts, totalCounts)
		components = append(components, HexaComponent{
			Name:   svc.Name,
			Role:   role,
			Reason: reason,
		})
	}

	sort.Slice(components, func(i, j int) bool {
		oi, oj := hexaRoleOrder[components[i].Role], hexaRoleOrder[components[j].Role]
		if oi != oj {
			return oi < oj
		}
		return components[i].Name < components[j].Name
	})

	return &HexaClassificationReport{
		Components: components,
		Summary:    buildClassificationSummary(components),
	}
}

func classifyService(
	svc *arch.ArchService,
	externalSources map[string]bool,
	ifaceCounts, totalCounts map[string]int,
) (role HexaRole, reason string) {
	segment := lastPathSegment(svc.Name)

	// 1. Entrypoint: cmd/ prefix or root "."
	if strings.HasPrefix(svc.Name, "cmd/") || svc.Name == "." {
		return HexaRoleEntry, "command entrypoint"
	}

	// 2. Adapter: keyword match or external protocol edges
	if arch.ContainsAny(segment, adapterKeywords...) {
		return HexaRoleAdapter, "name contains adapter keyword"
	}
	if externalSources[svc.Name] {
		return HexaRoleAdapter, "has outgoing external edges"
	}

	// 3. Infra: keyword match
	if arch.ContainsAny(segment, infraKeywords...) {
		return HexaRoleInfra, "name contains infrastructure keyword"
	}

	// 4. Port: package has > 50% interfaces
	pkg := svc.Package
	if pkg == "" {
		pkg = svc.Name
	}
	total := totalCounts[pkg]
	ifaces := ifaceCounts[pkg]
	if total > 0 && float64(ifaces)/float64(total) > portInterfaceRatioThreshold {
		return HexaRolePort, "package is mostly interfaces"
	}

	// 5. App: keyword match
	if arch.ContainsAny(segment, appKeywords...) {
		return HexaRoleApp, "name contains application keyword"
	}

	// 6. Domain: default
	return HexaRoleDomain, "default classification"
}

// ResolveRoles returns the effective hexagonal role for each component.
// Auto-classified roles from ComputeHexaClassification are used as the base;
// manual overrides from DesiredState.Roles take precedence.
func ResolveRoles(classification *HexaClassificationReport, overrides map[string]string) map[string]HexaRole {
	roles := make(map[string]HexaRole, len(classification.Components))
	for _, c := range classification.Components {
		roles[c.Name] = c.Role
	}
	for comp, role := range overrides {
		roles[comp] = HexaRole(role)
	}
	return roles
}

// RoleMultiplier returns a scaling factor for thresholds based on hexagonal role.
// Values > 1.0 are more lenient (composition roots tolerate more).
// Values < 1.0 are stricter (domain should be focused).
func RoleMultiplier(role HexaRole) float64 {
	switch role {
	case HexaRoleEntry:
		return 2.0 // cmd/ naturally large
	case HexaRoleApp:
		return 1.5 // composition roots have high fan-out
	case HexaRoleAdapter:
		return 1.3 // adapters have integration complexity
	case HexaRoleInfra:
		return 1.2 // infra has config complexity
	case HexaRoleDomain:
		return 0.8 // domain should be focused
	default:
		return 1.0 // port, unknown
	}
}

// ComputeHexaViolations validates hexagonal architecture rules and returns
// a report with violations and a compliance score.
func ComputeHexaViolations(
	services []arch.ArchService,
	edges []arch.ArchEdge,
	classes []analysis.ClassInfo,
) *HexaValidationReport {
	classification := ComputeHexaClassification(services, edges, classes)

	roleMap := make(map[string]HexaRole, len(classification.Components))
	for _, c := range classification.Components {
		roleMap[c.Name] = c.Role
	}

	var violations []HexaViolation
	for _, e := range edges {
		fromRole, fromOK := roleMap[e.From]
		toRole, toOK := roleMap[e.To]
		if !fromOK || !toOK {
			continue
		}

		if rule, severity := checkViolation(fromRole, toRole); rule != "" {
			violations = append(violations, HexaViolation{
				From:     e.From,
				To:       e.To,
				FromRole: fromRole,
				ToRole:   toRole,
				Rule:     rule,
				Severity: severity,
			})
		}
	}

	// Sort: errors first, then by From name.
	sort.Slice(violations, func(i, j int) bool {
		if violations[i].Severity != violations[j].Severity {
			return violations[i].Severity == port.SeverityError
		}
		return violations[i].From < violations[j].From
	})

	totalEdges := len(edges)
	score := 100.0
	if totalEdges > 0 {
		score = float64(totalEdges-len(violations)) / float64(totalEdges) * 100
	}

	return &HexaValidationReport{
		Classification: classification.Components,
		Violations:     violations,
		Score:          score,
		Summary:        buildViolationSummary(score, violations),
	}
}

func checkViolation(from, to HexaRole) (rule, severity string) {
	switch {
	case from == HexaRoleDomain && to == HexaRoleAdapter:
		return "domain must not depend on adapter", port.SeverityError
	case from == HexaRoleDomain && to == HexaRoleInfra:
		return "domain must not depend on infrastructure", port.SeverityError
	case from == HexaRolePort && to == HexaRoleAdapter:
		return "port must not depend on adapter", port.SeverityError
	case from == HexaRolePort && to == HexaRoleInfra:
		return "port must not depend on infrastructure", port.SeverityError
	case from == HexaRoleDomain && to == HexaRoleApp:
		return "domain should not depend on application layer", port.SeverityWarning
	default:
		return "", ""
	}
}

func buildClassificationSummary(components []HexaComponent) string {
	counts := make(map[HexaRole]int)
	for _, c := range components {
		counts[c.Role]++
	}

	// Build parts in a stable order matching the role sort order.
	roles := []HexaRole{
		HexaRoleEntry, HexaRoleAdapter, HexaRoleInfra,
		HexaRolePort, HexaRoleApp, HexaRoleDomain,
	}
	var parts []string
	for _, r := range roles {
		if n := counts[r]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, r))
		}
	}

	return fmt.Sprintf("%d components classified: %s", len(components), strings.Join(parts, ", "))
}

func buildViolationSummary(score float64, violations []HexaViolation) string {
	if len(violations) == 0 {
		return "Hexagonal compliance: 100% — no violations"
	}

	errors, warnings := 0, 0
	for _, v := range violations {
		switch v.Severity {
		case port.SeverityError:
			errors++
		case port.SeverityWarning:
			warnings++
		}
	}

	return fmt.Sprintf("Hexagonal compliance: %.0f%% — %d error(s), %d warning(s)", score, errors, warnings)
}

func lastPathSegment(name string) string {
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		return name[idx+1:]
	}
	return name
}
