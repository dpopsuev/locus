package solid

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dpopsuev/locus/internal/arch"
	"github.com/dpopsuev/locus/internal/clinic/hexa"
	"github.com/dpopsuev/locus/internal/graph"
	"github.com/dpopsuev/locus/internal/port"
	"github.com/dpopsuev/oculus"
)

// SOLIDPrinciple identifies one of the four SOLID principles detected.
type SOLIDPrinciple string

const (
	PrincipleSRP SOLIDPrinciple = "SRP"
	PrincipleOCP SOLIDPrinciple = "OCP"
	PrincipleISP SOLIDPrinciple = "ISP"
	PrincipleDIP SOLIDPrinciple = "DIP"
)

// SRP thresholds.
const (
	srpLOCError       = 1000
	srpFanOutError    = 8
	srpLOCWarning     = 500
	srpFanOutWarning  = 5
	srpDomainDivThres = 3
	srpSymbolThres    = 20
)

// SRP fan-in (blast radius) thresholds for severity escalation.
const (
	srpFanInMedium = 3
	srpFanInHigh   = 8
)

// ISP thresholds.
const (
	ispMethodsError   = 8
	ispMethodsWarning = 5
)

// OCP thresholds.
const (
	ocpCasesError   = 10
	ocpCasesWarning = 5
)

// solidPrincipleCount is the number of SOLID principles checked per component.
const solidPrincipleCount = 4

// SOLIDViolation records a single SOLID principle violation.
type SOLIDViolation struct {
	Principle  SOLIDPrinciple `json:"principle"`
	Component  string         `json:"component"`
	Detail     string         `json:"detail"`
	Severity   port.Severity  `json:"severity"`
	Suggestion string         `json:"suggestion"`
}

// SOLIDReport summarizes SOLID compliance across all principles.
type SOLIDReport struct {
	Violations  []SOLIDViolation `json:"violations"`
	ByPrinciple map[string]int   `json:"by_principle"`
	Score       port.Score       `json:"score"`
	Summary     string           `json:"summary"`
}

// IsAccepted checks if a violation is in the accepted list.
func IsAccepted(accepted []port.AcceptedViolation, component, principle string) bool {
	for _, a := range accepted {
		if a.Component == component && a.Principle == principle {
			return true
		}
	}
	return false
}

// ComputeSRPViolations detects Single Responsibility Principle violations
// based on LOC, fan-out count, and fan-out domain diversity.
// Thresholds are scaled by role multiplier; accepted violations are suppressed.
// Severity is weighted by blast radius (fan-in count):
//   - fan-in 0-2 → warning  (low blast radius, safe to refactor)
//   - fan-in 3-7 → error    (medium blast radius)
//   - fan-in 8+  → critical (high blast radius, many dependents break)
func ComputeSRPViolations(services []arch.ArchService, edges []arch.ArchEdge, roles map[string]hexa.HexaRole, accepted []port.AcceptedViolation) []SOLIDViolation {
	fanIn := graph.FanIn(edges)
	fanOut := graph.FanOut(edges)
	fanOutTargets := make(map[string]map[string]bool, len(services))
	for _, e := range edges {
		if fanOutTargets[e.From] == nil {
			fanOutTargets[e.From] = make(map[string]bool)
		}
		fanOutTargets[e.From][e.To] = true
	}

	var violations []SOLIDViolation

	for i := range services {
		svc := &services[i]

		if IsAccepted(accepted, svc.Name, string(PrincipleSRP)) {
			continue
		}

		// Entrypoints (composition roots) naturally aggregate many domains.
		// High fan-out and domain diversity is their job, not an SRP violation.
		if roles[svc.Name] == hexa.HexaRoleEntry {
			continue
		}

		fo := fanOut[svc.Name]
		fi := fanIn[svc.Name]
		mult := hexa.RoleMultiplier(roles[svc.Name])

		locError := int(float64(srpLOCError) * mult)
		locWarning := int(float64(srpLOCWarning) * mult)
		foError := int(float64(srpFanOutError) * mult)
		foWarning := int(float64(srpFanOutWarning) * mult)

		// Check LOC + fan-out thresholds.
		switch {
		case svc.LOC > locError && fo > foError:
			violations = append(violations, SOLIDViolation{
				Principle:  PrincipleSRP,
				Component:  svc.Name,
				Detail:     fmt.Sprintf("%s has %d LOC and fan-out %d (fan-in %d)", svc.Name, svc.LOC, fo, fi),
				Severity:   srpSeverityByFanIn(fi),
				Suggestion: "Split into focused packages by responsibility",
			})
		case svc.LOC > locWarning && fo > foWarning:
			violations = append(violations, SOLIDViolation{
				Principle:  PrincipleSRP,
				Component:  svc.Name,
				Detail:     fmt.Sprintf("%s has %d LOC and fan-out %d (fan-in %d)", svc.Name, svc.LOC, fo, fi),
				Severity:   srpSeverityByFanIn(fi),
				Suggestion: "Consider extracting related functionality",
			})
		}

		// Check fan-out domain diversity.
		diversity := countDomainDiversity(fanOutTargets[svc.Name])
		if diversity > srpDomainDivThres && len(svc.Symbols) > srpSymbolThres {
			violations = append(violations, SOLIDViolation{
				Principle:  PrincipleSRP,
				Component:  svc.Name,
				Detail:     fmt.Sprintf("%s touches %d domains with %d symbols (fan-in %d)", svc.Name, diversity, len(svc.Symbols), fi),
				Severity:   srpSeverityByFanIn(fi),
				Suggestion: "Component touches too many domains",
			})
		}
	}

	return violations
}

// srpSeverityByFanIn returns SRP severity based on fan-in (blast radius).
// A component with many dependents is riskier to refactor.
func srpSeverityByFanIn(fanIn int) port.Severity {
	switch {
	case fanIn >= srpFanInHigh:
		return port.SeverityCritical
	case fanIn >= srpFanInMedium:
		return port.SeverityError
	default:
		return port.SeverityWarning
	}
}

// countDomainDiversity counts the number of distinct domains among targets.
// A domain is the first path segment after the last "internal/" prefix,
// or the first segment of the path if "internal/" is absent.
func countDomainDiversity(targets map[string]bool) int {
	domains := make(map[string]bool, len(targets))
	for target := range targets {
		domains[extractDomain(target)] = true
	}
	return len(domains)
}

// extractDomain returns the domain segment from a target path.
func extractDomain(target string) string {
	// Find the last occurrence of "internal/".
	const internalPrefix = "internal/"
	idx := strings.LastIndex(target, internalPrefix)
	if idx >= 0 {
		after := target[idx+len(internalPrefix):]
		if slash := strings.IndexByte(after, '/'); slash >= 0 {
			return after[:slash]
		}
		return after
	}
	// No "internal/" — use first path segment.
	if slash := strings.IndexByte(target, '/'); slash >= 0 {
		return target[:slash]
	}
	return target
}

// ISP implementor-count thresholds for severity escalation.
const (
	ispImplCountMedium = 3
	ispImplCountHigh   = 6
)

// ComputeISPViolations detects Interface Segregation Principle violations
// based on interface method counts, weighted by implementor count.
// More implementors means changing the interface has higher impact:
//   - 1-2 implementors → warning
//   - 3-5 implementors → error
//   - 6+  implementors → critical
func ComputeISPViolations(classes []oculus.ClassInfo, impls []oculus.ImplEdge, accepted []port.AcceptedViolation) []SOLIDViolation {
	// Build implementor count per interface.
	implCount := make(map[string]int)
	for _, edge := range impls {
		if edge.Kind == "implements" {
			implCount[edge.To]++
		}
	}

	violations := make([]SOLIDViolation, 0, len(classes))

	for i := range classes {
		c := &classes[i]
		if c.Kind != hexa.ClassKindInterface {
			continue
		}

		if IsAccepted(accepted, c.Name, string(PrincipleISP)) {
			continue
		}

		methodCount := len(c.Methods)

		var baseSeverity port.Severity
		switch {
		case methodCount > ispMethodsError:
			baseSeverity = port.SeverityError
		case methodCount > ispMethodsWarning:
			baseSeverity = port.SeverityWarning
		default:
			continue
		}

		// Weight severity by implementor count.
		severity := ispSeverityByImplCount(baseSeverity, implCount[c.Name])

		detail := fmt.Sprintf("%s has %d methods (threshold: %d)", c.Name, methodCount, ispMethodsWarning)
		if n := implCount[c.Name]; n > 0 {
			detail = fmt.Sprintf("%s has %d methods, %d implementor(s)", c.Name, methodCount, n)
		}

		violations = append(violations, SOLIDViolation{
			Principle:  PrincipleISP,
			Component:  c.Name,
			Detail:     detail,
			Severity:   severity,
			Suggestion: "Split into smaller role-specific interfaces",
		})
	}

	return violations
}

// ispSeverityByImplCount adjusts ISP severity based on implementor count.
// A fat interface with many implementors is harder to refactor.
func ispSeverityByImplCount(baseSeverity port.Severity, implCount int) port.Severity {
	switch {
	case implCount >= ispImplCountHigh:
		return port.SeverityCritical
	case implCount >= ispImplCountMedium:
		if baseSeverity == port.SeverityWarning {
			return port.SeverityError
		}
		return port.SeverityError
	default:
		// 0-2 implementors: keep base but cap at warning when few.
		if implCount > 0 && baseSeverity == port.SeverityError {
			return port.SeverityWarning
		}
		return baseSeverity
	}
}

// ComputeOCPViolations detects Open/Closed Principle violations by finding
// type switch statements in Go source files under root.
func ComputeOCPViolations(root string, accepted []port.AcceptedViolation) []SOLIDViolation {
	if root == "" {
		return nil
	}

	var violations []SOLIDViolation

	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil //nolint:nilerr // skip unreadable entries
		}
		// Skip directories starting with ".".
		if info.IsDir() {
			base := filepath.Base(path)
			if strings.HasPrefix(base, ".") && path != root {
				return filepath.SkipDir
			}
			if base == "vendor" || base == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		// Skip non-Go files and test files.
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		found := findTypeSwitchViolations(path)
		for _, v := range found {
			if !IsAccepted(accepted, v.Component, string(PrincipleOCP)) {
				violations = append(violations, v)
			}
		}
		return nil
	})

	return violations
}

// findTypeSwitchViolations parses a single Go file and returns OCP violations
// for type switch statements exceeding the threshold.
func findTypeSwitchViolations(path string) []SOLIDViolation {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil
	}

	var violations []SOLIDViolation

	ast.Inspect(f, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSwitchStmt)
		if !ok {
			return true
		}

		cases := countTypeSwitchCases(ts)
		if cases <= ocpCasesWarning {
			return true
		}

		funcName := enclosingFuncName(f, fset, n)
		relPath := path

		severity := port.SeverityWarning
		if cases > ocpCasesError {
			severity = port.SeverityError
		}

		violations = append(violations, SOLIDViolation{
			Principle:  PrincipleOCP,
			Component:  relPath,
			Detail:     fmt.Sprintf("type switch in %s has %d cases", funcName, cases),
			Severity:   severity,
			Suggestion: "Consider replacing with interface dispatch",
		})

		return true
	})

	return violations
}

// countTypeSwitchCases counts the case clauses in a type switch statement.
func countTypeSwitchCases(ts *ast.TypeSwitchStmt) int {
	count := 0
	for _, stmt := range ts.Body.List {
		if _, ok := stmt.(*ast.CaseClause); ok {
			count++
		}
	}
	return count
}

// enclosingFuncName finds the name of the function containing the given AST node.
func enclosingFuncName(f *ast.File, fset *token.FileSet, target ast.Node) string {
	targetPos := fset.Position(target.Pos()).Offset

	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		start := fset.Position(fn.Pos()).Offset
		end := fset.Position(fn.End()).Offset
		if targetPos >= start && targetPos <= end {
			if fn.Recv != nil && len(fn.Recv.List) > 0 {
				return fmt.Sprintf("%s.%s", receiverTypeName(fn.Recv.List[0].Type), fn.Name.Name)
			}
			return fn.Name.Name
		}
	}
	return "<unknown>"
}

// receiverTypeName extracts the type name from a receiver expression.
func receiverTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		return receiverTypeName(t.X)
	case *ast.Ident:
		return t.Name
	default:
		return "<receiver>"
	}
}

// ComputeDIPViolations detects Dependency Inversion Principle violations
// by checking whether domain/app layers depend on adapter/infra concretions.
func ComputeDIPViolations(
	services []arch.ArchService,
	edges []arch.ArchEdge,
	hexaClassification *hexa.HexaClassificationReport,
	accepted []port.AcceptedViolation,
) []SOLIDViolation {
	if hexaClassification == nil {
		return nil
	}

	// Build role map from classification.
	roleMap := make(map[string]hexa.HexaRole, len(hexaClassification.Components))
	for _, c := range hexaClassification.Components {
		roleMap[c.Name] = c.Role
	}

	var violations []SOLIDViolation

	for _, e := range edges {
		if IsAccepted(accepted, e.From, string(PrincipleDIP)) {
			continue
		}

		fromRole := roleMap[e.From]
		toRole := roleMap[e.To]

		if fromRole == "" || toRole == "" {
			continue
		}

		switch {
		case fromRole == hexa.HexaRoleDomain && (toRole == hexa.HexaRoleAdapter || toRole == hexa.HexaRoleInfra):
			violations = append(violations, SOLIDViolation{
				Principle:  PrincipleDIP,
				Component:  e.From,
				Detail:     fmt.Sprintf("%s (%s) depends on %s (%s)", e.From, fromRole, e.To, toRole),
				Severity:   port.SeverityError,
				Suggestion: "Introduce an interface in the domain layer",
			})
		case fromRole == hexa.HexaRoleApp && toRole == hexa.HexaRoleAdapter:
			violations = append(violations, SOLIDViolation{
				Principle:  PrincipleDIP,
				Component:  e.From,
				Detail:     fmt.Sprintf("%s (%s) depends on %s (%s)", e.From, fromRole, e.To, toRole),
				Severity:   port.SeverityWarning,
				Suggestion: "Introduce an interface in the domain layer",
			})
		}
	}

	return violations
}

// ComputeSOLIDScan runs all four SOLID detectors and produces a unified report.
// Thresholds are scaled by role multiplier; accepted violations are suppressed.
func ComputeSOLIDScan(
	services []arch.ArchService,
	edges []arch.ArchEdge,
	classes []oculus.ClassInfo,
	impls []oculus.ImplEdge,
	hexaClassification *hexa.HexaClassificationReport,
	root string,
	roles map[string]hexa.HexaRole,
	accepted []port.AcceptedViolation,
) *SOLIDReport {
	var allViolations []SOLIDViolation
	allViolations = append(allViolations, ComputeSRPViolations(services, edges, roles, accepted)...)
	allViolations = append(allViolations, ComputeISPViolations(classes, impls, accepted)...)
	allViolations = append(allViolations, ComputeOCPViolations(root, accepted)...)
	allViolations = append(allViolations, ComputeDIPViolations(services, edges, hexaClassification, accepted)...)

	// Sort: errors first, then warnings, then by component name.
	sort.Slice(allViolations, func(i, j int) bool {
		si := severityRank(allViolations[i].Severity)
		sj := severityRank(allViolations[j].Severity)
		if si != sj {
			return si < sj
		}
		return allViolations[i].Component < allViolations[j].Component
	})

	// Build per-principle counts.
	byPrinciple := make(map[string]int)
	for _, v := range allViolations {
		byPrinciple[string(v.Principle)]++
	}

	// Score: percentage-based — violations / total checks.
	totalChecks := len(services) * solidPrincipleCount
	if totalChecks == 0 {
		totalChecks = 1
	}
	score := 100 - float64(len(allViolations))/float64(totalChecks)*100
	if score < 0 {
		score = 0
	}

	summary := buildSOLIDSummary(score, byPrinciple)

	return &SOLIDReport{
		Violations:  allViolations,
		ByPrinciple: byPrinciple,
		Score:       port.Score(score),
		Summary:     summary,
	}
}

// severityRank returns a sort rank for severity (lower = more severe = first).
func severityRank(severity port.Severity) int {
	switch severity {
	case port.SeverityCritical:
		return 0
	case port.SeverityError:
		return 1
	case port.SeverityWarning:
		return 2
	case port.SeverityInfo:
		return 3
	default:
		return 4
	}
}

// buildSOLIDSummary generates the human-readable summary line.
func buildSOLIDSummary(score float64, byPrinciple map[string]int) string {
	total := 0
	for _, v := range byPrinciple {
		total += v
	}

	if total == 0 {
		return "SOLID score: 100/100 — no violations detected"
	}

	parts := make([]string, 0, len(byPrinciple))
	// Fixed order for deterministic output.
	for _, p := range []SOLIDPrinciple{PrincipleSRP, PrincipleOCP, PrincipleISP, PrincipleDIP} {
		if count, ok := byPrinciple[string(p)]; ok && count > 0 {
			parts = append(parts, fmt.Sprintf("%s: %d", p, count))
		}
	}

	return fmt.Sprintf("SOLID score: %.0f/100 — %d violation(s): %s", score, total, strings.Join(parts, ", "))
}
