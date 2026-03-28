package protocol

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dpopsuev/locus/internal/analysis"
	"github.com/dpopsuev/locus/internal/arch"
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

// Score penalty per violation.
const solidViolationPenalty = 5

// SOLIDViolation records a single SOLID principle violation.
type SOLIDViolation struct {
	Principle  SOLIDPrinciple `json:"principle"`
	Component  string         `json:"component"`
	Detail     string         `json:"detail"`
	Severity   string         `json:"severity"`
	Suggestion string         `json:"suggestion"`
}

// SOLIDReport summarizes SOLID compliance across all principles.
type SOLIDReport struct {
	Violations  []SOLIDViolation `json:"violations"`
	ByPrinciple map[string]int   `json:"by_principle"`
	Score       float64          `json:"score"`
	Summary     string           `json:"summary"`
}

// ComputeSRPViolations detects Single Responsibility Principle violations
// based on LOC, fan-out count, and fan-out domain diversity.
func ComputeSRPViolations(services []arch.ArchService, edges []arch.ArchEdge) []SOLIDViolation {
	// Build fan-out: count of outbound edges per service.
	fanOut := make(map[string]int, len(services))
	fanOutTargets := make(map[string]map[string]bool, len(services))
	for _, e := range edges {
		fanOut[e.From]++
		if fanOutTargets[e.From] == nil {
			fanOutTargets[e.From] = make(map[string]bool)
		}
		fanOutTargets[e.From][e.To] = true
	}

	var violations []SOLIDViolation

	for i := range services {
		svc := &services[i]
		fo := fanOut[svc.Name]

		// Check LOC + fan-out thresholds.
		switch {
		case svc.LOC > srpLOCError && fo > srpFanOutError:
			violations = append(violations, SOLIDViolation{
				Principle:  PrincipleSRP,
				Component:  svc.Name,
				Detail:     fmt.Sprintf("%s has %d LOC and fan-out %d", svc.Name, svc.LOC, fo),
				Severity:   SeverityError,
				Suggestion: "Split into focused packages by responsibility",
			})
		case svc.LOC > srpLOCWarning && fo > srpFanOutWarning:
			violations = append(violations, SOLIDViolation{
				Principle:  PrincipleSRP,
				Component:  svc.Name,
				Detail:     fmt.Sprintf("%s has %d LOC and fan-out %d", svc.Name, svc.LOC, fo),
				Severity:   SeverityWarning,
				Suggestion: "Consider extracting related functionality",
			})
		}

		// Check fan-out domain diversity.
		diversity := countDomainDiversity(fanOutTargets[svc.Name])
		if diversity > srpDomainDivThres && len(svc.Symbols) > srpSymbolThres {
			violations = append(violations, SOLIDViolation{
				Principle:  PrincipleSRP,
				Component:  svc.Name,
				Detail:     fmt.Sprintf("%s touches %d domains with %d symbols", svc.Name, diversity, len(svc.Symbols)),
				Severity:   SeverityWarning,
				Suggestion: "Component touches too many domains",
			})
		}
	}

	return violations
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

// ComputeISPViolations detects Interface Segregation Principle violations
// based on interface method counts and implementor completeness.
func ComputeISPViolations(classes []analysis.ClassInfo, impls []analysis.ImplEdge) []SOLIDViolation {
	// Index classes by name for lookup.
	classMap := make(map[string]*analysis.ClassInfo, len(classes))
	for i := range classes {
		classMap[classes[i].Name] = &classes[i]
	}

	var violations []SOLIDViolation

	for i := range classes {
		c := &classes[i]
		if c.Kind != classKindInterface {
			continue
		}

		methodCount := len(c.Methods)

		switch {
		case methodCount > ispMethodsError:
			violations = append(violations, SOLIDViolation{
				Principle:  PrincipleISP,
				Component:  c.Name,
				Detail:     fmt.Sprintf("%s has %d methods (threshold: %d)", c.Name, methodCount, ispMethodsError),
				Severity:   SeverityError,
				Suggestion: "Split into smaller role-specific interfaces",
			})
		case methodCount > ispMethodsWarning:
			violations = append(violations, SOLIDViolation{
				Principle:  PrincipleISP,
				Component:  c.Name,
				Detail:     fmt.Sprintf("%s has %d methods (threshold: %d)", c.Name, methodCount, ispMethodsWarning),
				Severity:   SeverityWarning,
				Suggestion: "Split into smaller role-specific interfaces",
			})
		}

		// Check implementors for incomplete coverage.
		for _, edge := range impls {
			if edge.To != c.Name {
				continue
			}
			impl, ok := classMap[edge.From]
			if !ok {
				continue
			}
			if len(impl.Methods) < methodCount {
				violations = append(violations, SOLIDViolation{
					Principle:  PrincipleISP,
					Component:  edge.From,
					Detail:     fmt.Sprintf("%s may have empty methods for %s", edge.From, c.Name),
					Severity:   SeverityWarning,
					Suggestion: "Split into smaller role-specific interfaces",
				})
			}
		}
	}

	return violations
}

// ComputeOCPViolations detects Open/Closed Principle violations by finding
// type switch statements in Go source files under root.
func ComputeOCPViolations(root string) []SOLIDViolation {
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
		violations = append(violations, found...)
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

		severity := SeverityWarning
		if cases > ocpCasesError {
			severity = SeverityError
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
	hexaClassification *HexaClassificationReport,
) []SOLIDViolation {
	if hexaClassification == nil {
		return nil
	}

	// Build role map from classification.
	roleMap := make(map[string]HexaRole, len(hexaClassification.Components))
	for _, c := range hexaClassification.Components {
		roleMap[c.Name] = c.Role
	}

	var violations []SOLIDViolation

	for _, e := range edges {
		fromRole := roleMap[e.From]
		toRole := roleMap[e.To]

		if fromRole == "" || toRole == "" {
			continue
		}

		switch {
		case fromRole == HexaRoleDomain && (toRole == HexaRoleAdapter || toRole == HexaRoleInfra):
			violations = append(violations, SOLIDViolation{
				Principle:  PrincipleDIP,
				Component:  e.From,
				Detail:     fmt.Sprintf("%s (%s) depends on %s (%s)", e.From, fromRole, e.To, toRole),
				Severity:   SeverityError,
				Suggestion: "Introduce an interface in the domain layer",
			})
		case fromRole == HexaRoleApp && toRole == HexaRoleAdapter:
			violations = append(violations, SOLIDViolation{
				Principle:  PrincipleDIP,
				Component:  e.From,
				Detail:     fmt.Sprintf("%s (%s) depends on %s (%s)", e.From, fromRole, e.To, toRole),
				Severity:   SeverityWarning,
				Suggestion: "Introduce an interface in the domain layer",
			})
		}
	}

	return violations
}

// ComputeSOLIDScan runs all four SOLID detectors and produces a unified report.
func ComputeSOLIDScan(
	services []arch.ArchService,
	edges []arch.ArchEdge,
	classes []analysis.ClassInfo,
	impls []analysis.ImplEdge,
	hexaClassification *HexaClassificationReport,
	root string,
) *SOLIDReport {
	var allViolations []SOLIDViolation
	allViolations = append(allViolations, ComputeSRPViolations(services, edges)...)
	allViolations = append(allViolations, ComputeISPViolations(classes, impls)...)
	allViolations = append(allViolations, ComputeOCPViolations(root)...)
	allViolations = append(allViolations, ComputeDIPViolations(services, edges, hexaClassification)...)

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

	// Score: each violation costs solidViolationPenalty points, minimum 0.
	score := float64(100 - len(allViolations)*solidViolationPenalty)
	if score < 0 {
		score = 0
	}

	summary := buildSOLIDSummary(score, byPrinciple)

	return &SOLIDReport{
		Violations:  allViolations,
		ByPrinciple: byPrinciple,
		Score:       score,
		Summary:     summary,
	}
}

// severityRank returns a sort rank for severity (lower = more severe = first).
func severityRank(severity string) int {
	switch severity {
	case SeverityError:
		return 0
	case SeverityWarning:
		return 1
	case SeverityInfo:
		return 2
	default:
		return 3
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
