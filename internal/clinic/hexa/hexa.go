package hexa

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dpopsuev/locus/internal/arch"
	"github.com/dpopsuev/locus/internal/graph"
	"github.com/dpopsuev/locus/internal/model"
	"github.com/dpopsuev/locus/internal/oculus"
	"github.com/dpopsuev/locus/internal/port"
)

// ClassKindInterface is the Kind value for interfaces in oculus.ClassInfo.
const ClassKindInterface = "interface"

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

// Graph-based classification thresholds. Zero keyword lists, zero import patterns.
const (
	// portInterfaceRatio: if >50% of exported symbols are interfaces → port.
	portInterfaceRatio = 0.5
	// infraFanInRatio: if component is imported by >40% of all components → infra.
	infraFanInRatio = 0.4
	// compositionRootFanOut: fan-out >= this AND fan-in <= 1 → composition root (entrypoint/app).
	compositionRootFanOut = 10
	// infraTypeThreshold: packages with more than this many exported types are
	// likely domain core, not infrastructure utilities. Infrastructure packages
	// (loggers, config, caches) export 1-3 types; domain packages export many.
	infraTypeThreshold = 3
)

// HexaComponent represents a classified component in a hexagonal architecture.
type HexaComponent struct {
	Name   string   `json:"name"`
	Role   HexaRole `json:"role"`
	Reason string   `json:"reason"`
}

// HexaViolation records a dependency that breaks hexagonal architecture rules.
type HexaViolation struct {
	From     string        `json:"from"`
	To       string        `json:"to"`
	FromRole HexaRole      `json:"from_role"`
	ToRole   HexaRole      `json:"to_role"`
	Rule     string        `json:"rule"`
	Severity port.Severity `json:"severity"`
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
	Score          port.Score      `json:"score"`
	Summary        string          `json:"summary"`
}

// ComputeHexaClassification classifies each service into a hexagonal architecture role
// using structural graph signals. Zero keyword lists, zero import path patterns,
// language-agnostic. The graph tells us adapter/port/domain without knowing WHAT
// is imported — only WHETHER edges cross boundaries.
func ComputeHexaClassification(
	services []arch.ArchService,
	edges []arch.ArchEdge,
	classes []oculus.ClassInfo,
) *HexaClassificationReport {
	// Build structural signals from the graph.
	hasExternalEdge := make(map[string]bool)
	for _, e := range edges {
		if e.Protocol == "external" {
			hasExternalEdge[e.From] = true
		}
	}

	fanIn := graph.FanIn(edges)
	fanOut := graph.FanOut(edges)
	totalComponents := len(services)

	// Build per-package interface and total type counts from class oculus.
	ifaceCounts := make(map[string]int)
	totalCounts := make(map[string]int)
	for _, c := range classes {
		totalCounts[c.Package]++
		if c.Kind == ClassKindInterface {
			ifaceCounts[c.Package]++
		}
	}

	components := make([]HexaComponent, 0, len(services))
	for i := range services {
		svc := &services[i]
		role, reason := classifyService(svc, hasExternalEdge[svc.Name],
			fanIn[svc.Name], fanOut[svc.Name], totalComponents,
			ifaceCounts, totalCounts)
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

// classifyService uses pure structural graph signals. Priority cascade:
// entrypoint > port > adapter > infra > dead > domain.
// No keyword lists. No import path patterns. Language-agnostic.
func classifyService(
	svc *arch.ArchService,
	hasExternal bool,
	fi, fo, totalComponents int,
	ifaceCounts, totalCounts map[string]int,
) (role HexaRole, reason string) {
	// 1. Entrypoint: has "main" symbol OR composition root pattern (high fan-out, low fan-in).
	if hasMainSymbol(svc) {
		return HexaRoleEntry, "has main symbol"
	}
	if fo >= compositionRootFanOut && fi <= 1 {
		return HexaRoleEntry, fmt.Sprintf("composition root (fan-out=%d, fan-in=%d)", fo, fi)
	}

	// 2. Port: >50% of exported symbols are interfaces.
	ifaceRatio := interfaceRatio(svc)
	if ifaceRatio > portInterfaceRatio {
		return HexaRolePort, fmt.Sprintf("%.0f%% interfaces", ifaceRatio*100)
	}
	// Fallback: class-analysis-based interface ratio (for languages without symbol Kind).
	pkg := svc.Package
	if pkg == "" {
		pkg = svc.Name
	}
	if total := totalCounts[pkg]; total > 0 {
		classRatio := float64(ifaceCounts[pkg]) / float64(total)
		if classRatio > portInterfaceRatio {
			return HexaRolePort, fmt.Sprintf("%.0f%% interfaces (class analysis)", classRatio*100)
		}
	}

	// 3. Adapter: has external edges (crosses boundary).
	if hasExternal {
		return HexaRoleAdapter, "imports external dependencies"
	}

	// 4. Infra: imported by >40% of all components (widely used infrastructure).
	// Guard: packages with a rich type surface (>3 exported types) are domain
	// core, not infrastructure utilities. Infra packages export few types.
	if isInfraCandidate(fi, totalComponents) && isLowTypeSurface(svc, pkg, totalCounts) {
		return HexaRoleInfra, fmt.Sprintf("imported by %d/%d components (%.0f%%)", fi, totalComponents, float64(fi)/float64(totalComponents)*100)
	}

	// 5. Domain: default.
	return HexaRoleDomain, "default classification"
}

// isInfraCandidate returns true if the package has high enough fan-in to be
// considered for infra classification.
func isInfraCandidate(fanIn, totalComponents int) bool {
	return totalComponents > 2 && float64(fanIn)/float64(totalComponents) > infraFanInRatio
}

// isLowTypeSurface returns true if the package exports few types, indicating
// it's a utility/infra package rather than domain core.
func isLowTypeSurface(svc *arch.ArchService, pkg string, totalCounts map[string]int) bool {
	exportedTypes := totalCounts[svc.Name]
	if pkg != "" {
		exportedTypes = totalCounts[pkg]
	}
	// Fallback: if class analysis produced no results (no LSP/tree-sitter),
	// count exported struct/interface/type symbols from the scanner output.
	if exportedTypes == 0 {
		for i := range svc.Symbols {
			k := svc.Symbols[i].Kind
			if k == model.SymbolStruct || k == model.SymbolInterface || k == model.SymbolClass || k == model.SymbolEnum {
				exportedTypes++
			}
		}
	}
	return exportedTypes <= infraTypeThreshold
}

// hasMainSymbol checks if the component exports a "main" or "Main" symbol.
// Language-agnostic — works for Go (main), Python (__main__), Rust (main), etc.
func hasMainSymbol(svc *arch.ArchService) bool {
	for _, sym := range svc.Symbols {
		lower := strings.ToLower(sym.Name)
		if lower == "main" || lower == "__main__" {
			return true
		}
	}
	return false
}

// interfaceRatio returns the fraction of exported symbols that are interfaces.
// Uses model.Symbol.Kind metadata (populated by TSK-257).
func interfaceRatio(svc *arch.ArchService) float64 {
	if len(svc.Symbols) == 0 {
		return 0
	}
	count := 0
	for _, sym := range svc.Symbols {
		if sym.Kind == model.SymbolInterface {
			count++
		}
	}
	return float64(count) / float64(len(svc.Symbols))
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
	classes []oculus.ClassInfo,
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
		Score:          port.Score(score),
		Summary:        buildViolationSummary(score, violations),
	}
}

func checkViolation(from, to HexaRole) (rule string, severity port.Severity) {
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
