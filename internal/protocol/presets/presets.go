// Package presets implements preset report renderers for Locus analysis.
// Each preset is a Strategy that renders a specific view of a ContextReport.
package presets

import (
	"context"
	"fmt"
	"strings"

	"github.com/dpopsuev/locus/internal/arch"
	"github.com/dpopsuev/locus/internal/clinic"
	clinichexa "github.com/dpopsuev/locus/internal/clinic/hexa"
	clinicnaming "github.com/dpopsuev/locus/internal/clinic/naming"
	clinicsolid "github.com/dpopsuev/locus/internal/clinic/solid"
	"github.com/dpopsuev/locus/internal/constraint"
	"github.com/dpopsuev/locus/internal/port"
	"github.com/dpopsuev/locus/internal/survey"
	"github.com/dpopsuev/oculus"
	"github.com/dpopsuev/oculus/lang"
	"github.com/dpopsuev/oculus/lsp"
)

// Preset name constants.
const (
	ArchReview  = "architecture_review"
	HealthCheck = "health_check"
	Onboarding  = "onboarding"
	PrePR       = "pre_pr"
	Normative   = "normative"
	PreRefactor = "pre_refactor"
	FullClinic  = "full_clinic"
	CodeHealth  = "code_health"
)

// ErrUnknown is returned when an unknown preset name is requested.
var ErrUnknown = fmt.Errorf("unknown preset")

// Deps holds the dependencies that stateful presets need.
// Avoids importing the Protocol type (breaks cycle).
type Deps struct {
	Pool         lsp.Pool
	DesiredState func(ctx context.Context, path string) (*port.DesiredState, error)
}

// Run dispatches to the appropriate preset renderer.
func Run(ctx context.Context, report *arch.ContextReport, path, preset string, deps Deps) (string, error) {
	var b strings.Builder
	switch preset {
	case ArchReview:
		archReview(&b, report)
	case HealthCheck:
		healthCheck(&b, report)
	case Onboarding:
		onboarding(&b, report)
	case PrePR:
		prePR(&b, report)
	case Normative:
		normative(ctx, &b, path, report, deps)
	case PreRefactor:
		preRefactor(&b, path, report)
	case FullClinic:
		fullClinic(ctx, &b, path, report, deps)
	case CodeHealth:
		codeHealth(&b, path, report, deps)
	default:
		return "", fmt.Errorf("%w %q (valid: %s, %s, %s, %s, %s, %s, %s, %s)",
			ErrUnknown, preset, ArchReview, HealthCheck, Onboarding, PrePR,
			Normative, PreRefactor, FullClinic, CodeHealth)
	}
	return b.String(), nil
}

// Names returns all valid preset names.
func Names() []string {
	return []string{ArchReview, HealthCheck, Onboarding, PrePR, Normative, PreRefactor, FullClinic, CodeHealth}
}

func archReview(b *strings.Builder, report *arch.ContextReport) {
	fmt.Fprintf(b, "# Architecture Review: %s\n\n", report.ModulePath)
	fmt.Fprintf(b, "%d components, %d edges, %d cycles\n\n", len(report.Architecture.Services), len(report.Architecture.Edges), len(report.Cycles))
	spots := report.HotSpots
	if len(spots) > 5 {
		spots = spots[:5]
	}
	if len(spots) > 0 {
		b.WriteString("## Hot Spots\n")
		for _, s := range spots {
			fmt.Fprintf(b, "- %s (churn:%d, fan-in:%d)\n", s.Component, s.Churn, s.FanIn)
		}
	}
	if len(report.Cycles) > 0 {
		b.WriteString("\n## Cycles\n")
		for i, c := range report.Cycles {
			if i >= 3 {
				fmt.Fprintf(b, "... and %d more\n", len(report.Cycles)-3)
				break
			}
			fmt.Fprintf(b, "- %s\n", strings.Join(c, " → "))
		}
	}
}

func healthCheck(b *strings.Builder, report *arch.ContextReport) {
	fmt.Fprintf(b, "# Health Check: %s\n\n", report.ModulePath)
	spots := report.HotSpots
	if len(spots) > 5 {
		spots = spots[:5]
	}
	for _, s := range spots {
		fmt.Fprintf(b, "- %s (churn:%d, fan-in:%d)\n", s.Component, s.Churn, s.FanIn)
	}
	if len(spots) == 0 {
		b.WriteString("No hot spots detected.\n")
	}
}

func onboarding(b *strings.Builder, report *arch.ContextReport) {
	fmt.Fprintf(b, "# Onboarding: %s\n\n", report.ModulePath)
	fmt.Fprintf(b, "%d components, scanner=%s\n\n", len(report.Architecture.Services), report.Scanner)
	b.WriteString("## Top Components\n")
	n := min(len(report.Architecture.Services), 10)
	for i := range report.Architecture.Services[:n] {
		fmt.Fprintf(b, "- %s (%d LOC)\n", report.Architecture.Services[i].Name, report.Architecture.Services[i].LOC)
	}
}

func prePR(b *strings.Builder, report *arch.ContextReport) {
	fmt.Fprintf(b, "# Pre-PR Review: %s\n\n", report.ModulePath)
	fmt.Fprintf(b, "%d components, %d cycles, %d violations\n",
		len(report.Architecture.Services), len(report.Cycles), len(report.LayerViolations))
}

func normative(ctx context.Context, b *strings.Builder, path string, report *arch.ContextReport, deps Deps) {
	fmt.Fprintf(b, "# Normative Analysis: %s\n\n", report.ModulePath)

	idReport := constraint.ComputeImportDirection(report.Architecture.Edges, report.ImportDepth)
	fmt.Fprintf(b, "## Import Direction\n%s\n\n", idReport.Summary)
	for i, v := range idReport.Violations {
		if i >= 5 {
			fmt.Fprintf(b, "... and %d more\n", len(idReport.Violations)-5)
			break
		}
		fmt.Fprintf(b, "- [%s] %s → %s (depth %d→%d)\n", v.Severity, v.From, v.To, v.FromDepth, v.ToDepth)
	}

	tbReport := constraint.ComputeTrustBoundaries(report.Architecture.Services, report.Architecture.Edges, desiredRoles(ctx, path, deps))
	fmt.Fprintf(b, "\n## Trust Boundaries\n%s\n", tbReport.Summary)

	if desired, _ := deps.DesiredState(ctx, path); desired != nil && len(desired.Constraints) > 0 {
		budgetReport := constraint.ComputeBudgetViolations(report.Architecture.Services, report.Architecture.Edges, desired.Constraints)
		fmt.Fprintf(b, "\n## Budgets\n%s\n", budgetReport.Summary)
	}
}

func preRefactor(b *strings.Builder, path string, report *arch.ContextReport) {
	fmt.Fprintf(b, "# Pre-Refactor Analysis: %s\n\n", report.ModulePath)

	spots := report.HotSpots
	if len(spots) > 10 {
		spots = spots[:10]
	}
	b.WriteString("## Hot Spots (churn × coupling)\n")
	for _, s := range spots {
		fmt.Fprintf(b, "- %s (churn:%d, fan-in:%d)\n", s.Component, s.Churn, s.FanIn)
	}

	da := oculus.NewFallback(path, nil)
	if classes, err := da.Classes(path); err == nil {
		impls, _ := da.Implements(path)
		imReport := constraint.ComputeInterfaceMetrics(classes, impls)
		fmt.Fprintf(b, "\n## Interfaces\n%s\n", imReport.Summary)
		for _, iface := range imReport.Interfaces {
			if iface.IsOrphan {
				fmt.Fprintf(b, "- ORPHAN: %s (%d methods, 0 implementors)\n", iface.Name, iface.MethodCount)
			}
		}
	}

	idReport := constraint.ComputeImportDirection(report.Architecture.Edges, report.ImportDepth)
	if len(idReport.Violations) > 0 {
		fmt.Fprintf(b, "\n## Import Direction\n%s\n", idReport.Summary)
	}
}

func fullClinic(ctx context.Context, b *strings.Builder, path string, report *arch.ContextReport, deps Deps) {
	fmt.Fprintf(b, "# Full Clinic: %s\n\n", report.ModulePath)
	fmt.Fprintf(b, "%d components, %d edges\n\n", len(report.Architecture.Services), len(report.Architecture.Edges))

	fmt.Fprintf(b, "## Architecture\n")
	fmt.Fprintf(b, "- Cycles: %d\n", len(report.Cycles))
	fmt.Fprintf(b, "- Layer violations: %d\n", len(report.LayerViolations))

	idReport := constraint.ComputeImportDirection(report.Architecture.Edges, report.ImportDepth)
	fmt.Fprintf(b, "- Import direction: %s\n", idReport.Summary)

	tbReport := constraint.ComputeTrustBoundaries(report.Architecture.Services, report.Architecture.Edges, desiredRoles(ctx, path, deps))
	fmt.Fprintf(b, "- Trust zones: %s\n", tbReport.Summary)

	spots := report.HotSpots
	if len(spots) > 5 {
		spots = spots[:5]
	}
	if len(spots) > 0 {
		fmt.Fprintf(b, "\n## Hot Spots\n")
		for _, s := range spots {
			fmt.Fprintf(b, "- %s (churn:%d, fan-in:%d)\n", s.Component, s.Churn, s.FanIn)
		}
	}

	fa := oculus.NewFallback(path, deps.Pool)
	if classes, err := fa.Classes(path); err == nil {
		impls, _ := fa.Implements(path)
		imReport := constraint.ComputeInterfaceMetrics(classes, impls)
		fmt.Fprintf(b, "\n## Interfaces\n%s\n", imReport.Summary)
	}

	if desired, _ := deps.DesiredState(ctx, path); desired != nil && len(desired.Constraints) > 0 {
		budgetReport := constraint.ComputeBudgetViolations(report.Architecture.Services, report.Architecture.Edges, desired.Constraints)
		fmt.Fprintf(b, "\n## Budgets\n%s\n", budgetReport.Summary)
	}

	// Code Health Clinic pillars
	if classes, err := fa.Classes(path); err == nil {
		impls, _ := fa.Implements(path)

		hexaClass := clinichexa.ComputeHexaClassification(report.Architecture.Services, report.Architecture.Edges, classes)
		desired, _ := deps.DesiredState(ctx, path)
		roles, accepted := ResolveRolesAndAccepted(hexaClass, desired)

		patternReport := clinic.ComputePatternScan(report.Architecture.Services, report.Architecture.Edges, report.Cycles, classes, impls, roles, accepted)
		fmt.Fprintf(b, "\n## Patterns & Smells\n%s\n", patternReport.Summary)

		hexaReport := clinichexa.ComputeHexaViolations(report.Architecture.Services, report.Architecture.Edges, classes)
		fmt.Fprintf(b, "\n## Hexagonal Architecture\n%s\n", hexaReport.Summary)

		solidReport := clinicsolid.ComputeSOLIDScan(report.Architecture.Services, report.Architecture.Edges, classes, impls, hexaClass, path, roles, accepted)
		fmt.Fprintf(b, "\n## SOLID Principles\n%s\n", solidReport.Summary)
	}

	rules := RulesFromServices(report.Architecture.Services)
	sqReport := clinicnaming.ComputeSymbolQuality(report.Architecture.Services, report.Architecture.Edges, rules)
	fmt.Fprintf(b, "\n## Symbol Quality\n%s\n", sqReport.Summary)
}

func codeHealth(b *strings.Builder, path string, report *arch.ContextReport, deps Deps) {
	fmt.Fprintf(b, "# Code Health Clinic: %s\n\n", report.ModulePath)
	fmt.Fprintf(b, "%d components, %d edges\n\n", len(report.Architecture.Services), len(report.Architecture.Edges))

	fa := oculus.NewFallback(path, deps.Pool)
	classes, _ := fa.Classes(path)
	impls, _ := fa.Implements(path)

	hexaClass := clinichexa.ComputeHexaClassification(report.Architecture.Services, report.Architecture.Edges, classes)

	patternReport := clinic.ComputePatternScan(report.Architecture.Services, report.Architecture.Edges, report.Cycles, classes, impls, nil, nil)
	fmt.Fprintf(b, "## Patterns & Smells\n%s\n\n", patternReport.Summary)

	hexaReport := clinichexa.ComputeHexaViolations(report.Architecture.Services, report.Architecture.Edges, classes)
	fmt.Fprintf(b, "## Hexagonal Architecture\n%s\n\n", hexaReport.Summary)

	solidReport := clinicsolid.ComputeSOLIDScan(report.Architecture.Services, report.Architecture.Edges, classes, impls, hexaClass, path, nil, nil)
	fmt.Fprintf(b, "## SOLID Principles\n%s\n\n", solidReport.Summary)

	rules := RulesFromServices(report.Architecture.Services)
	sqReport := clinicnaming.ComputeSymbolQuality(report.Architecture.Services, report.Architecture.Edges, rules)
	fmt.Fprintf(b, "## Symbol Quality\n%s\n", sqReport.Summary)
}

// --- Helpers exported for use by parent protocol package ---

// ResolveRolesAndAccepted resolves hexa roles and accepted violations from a classification report and desired state.
func ResolveRolesAndAccepted(hexaClass *clinichexa.HexaClassificationReport, desired *port.DesiredState) (map[string]clinichexa.HexaRole, []port.AcceptedViolation) {
	var roles map[string]clinichexa.HexaRole
	var accepted []port.AcceptedViolation

	if hexaClass != nil {
		var overrides map[string]string
		if desired != nil {
			overrides = desired.Roles
		}
		roles = clinichexa.ResolveRoles(hexaClass, overrides)
	}
	if desired != nil {
		accepted = desired.Accepted
	}
	return roles, accepted
}

// RulesFromServices returns language rules from the first service's language.
func RulesFromServices(services []arch.ArchService) lang.Rules {
	if len(services) == 0 {
		return nil
	}
	if ls := survey.GetLanguageSupport(services[0].Language); ls != nil && ls.Rules != nil {
		return ls.Rules
	}
	return nil
}

func desiredRoles(ctx context.Context, path string, deps Deps) map[string]string {
	desired, err := deps.DesiredState(ctx, path)
	if err != nil || desired == nil {
		return nil
	}
	return desired.Roles
}
