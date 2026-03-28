package protocol

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/dpopsuev/locus/internal/analysis"
	"github.com/dpopsuev/locus/internal/arch"
	"github.com/dpopsuev/locus/internal/cursor"
	"github.com/dpopsuev/locus/internal/history"
	"github.com/dpopsuev/locus/internal/remote"
	"github.com/dpopsuev/locus/internal/store"
)

// Error messages used across protocol methods.
var (
	ErrComponentRequired     = errors.New("component is required")
	ErrBeforeSHARequired     = errors.New("before_sha is required")
	ErrURLRequired           = errors.New("url is required")
	ErrBothBranchesRequired  = errors.New("both branch_a and branch_b are required")
	ErrOldestOrStepsRequired = errors.New("either oldest_ref or steps is required")
)

const (
	errScanFailed     = "scan failed: %w"
	errNoCachedScan   = "no cached scan for SHA %s — run scan_local first"
	errNoCachedReport = "no cached report for cache_key %q — run scan_local first"
)

// Protocol encapsulates all Locus business logic.
// Both CLI and MCP are thin wrappers around this.
type Protocol struct {
	db         store.Store
	workspaces []string
}

// New creates a Protocol with the given store and workspace roots.
func New(s store.Store, workspaces []string) *Protocol {
	return &Protocol{db: s, workspaces: workspaces}
}

// ScanOpts controls a local scan.
type ScanOpts struct {
	Depth           int
	ChurnDays       int
	IncludeExternal bool
	IncludeTests    bool
	IncludeCoverage bool
	Budget          int
	Scanner         string
	GitDays         int
	Authors         bool
	Format          string // "json", "md", "mermaid", "summary" — rendering is caller's job
	Intent          string // architecture, coupling, health (default), full
	Since           string // git ref to diff against for incremental scan
}

// RemoteOpts controls a remote codograph.
type RemoteOpts struct {
	Ref       string
	Keep      bool
	Depth     int
	ChurnDays int
	Budget    int
	Intent    string
}

// BranchDiffResult wraps branch metadata with the diff.
type BranchDiffResult struct {
	BranchA string                 `json:"branch_a"`
	BranchB string                 `json:"branch_b"`
	Diff    *history.CodographDiff `json:"diff"`
}

// DepResult holds fan-in/fan-out edges for a component.
type DepResult struct {
	Component string     `json:"component"`
	FanIn     []JSONEdge `json:"fan_in"`
	FanOut    []JSONEdge `json:"fan_out"`
}

// JSONEdge is the JSON shape for an architecture edge.
type JSONEdge struct {
	From       string `json:"from"`
	To         string `json:"to"`
	Weight     int    `json:"weight,omitempty"`
	CallSites  int    `json:"call_sites,omitempty"`
	LOCSurface int    `json:"loc_surface,omitempty"`
	Protocol   string `json:"protocol,omitempty"`
}

// SuggestDepthResult holds the depth suggestion.
type SuggestDepthResult struct {
	SuggestedDepth int    `json:"suggested_depth"`
	Components     int    `json:"flat_components"`
	Reasoning      string `json:"reasoning"`
}

// ScanResult wraps a scan report with its cache key and SHA.
type ScanResult struct {
	Report   *arch.ContextReport `json:"report"`
	CacheKey string              `json:"cache_key"`
	SHA      string              `json:"sha"`
}

// RenderScanSummary returns a compact ~50 token summary of a scan result.
func RenderScanSummary(r *ScanResult, driftInfo string) string {
	report := r.Report
	summary := fmt.Sprintf("Scanned %s: %d components, %d edges, %d cycles, scanner=%s\ncache_key: %s",
		report.ModulePath,
		len(report.Architecture.Services),
		len(report.Architecture.Edges),
		len(report.Cycles),
		report.Scanner,
		r.CacheKey)
	if driftInfo != "" {
		summary += "\n" + driftInfo
	}
	return summary
}

// --- Operations ---

func (p *Protocol) ScanProject(ctx context.Context, path string, opts ScanOpts) (*ScanResult, error) {
	path = p.resolvePath(path)
	churnDays := opts.ChurnDays
	if churnDays == 0 {
		churnDays = 30
	}

	sha := p.db.ResolveHEAD(path)
	if cached, hit, err := p.db.GetReport(ctx,path, sha); err == nil && hit {
		return &ScanResult{Report: cached, CacheKey: path + "@" + sha, SHA: sha}, nil
	}

	report, err := arch.ScanAndBuild(path, arch.ScanOpts{
		ScannerOverride: opts.Scanner,
		ExcludeTests:    !opts.IncludeTests,
		IncludeExternal: opts.IncludeExternal,
		IncludeCoverage: opts.IncludeCoverage,
		Depth:           opts.Depth,
		ChurnDays:       churnDays,
		Budget:          opts.Budget,
		GitDays:         opts.GitDays,
		Authors:         opts.Authors,
		Intent:          arch.ScanIntent(opts.Intent),
		Since:           opts.Since,
	})
	if err != nil {
		return nil, fmt.Errorf(errScanFailed, err)
	}
	if sha != "" {
		p.db.PutReport(ctx, path, sha, report)
		_ = p.db.PutComponentMeta(ctx, path, sha, generateComponentMeta(report))
		abs, _ := filepath.Abs(path)
		_ = p.db.RecordScan(ctx, string(history.Local), abs, sha, report)
	}
	return &ScanResult{Report: report, CacheKey: path + "@" + sha, SHA: sha}, nil
}

func (p *Protocol) SuggestDepth(ctx context.Context, path string) (*SuggestDepthResult, error) {
	path = p.resolvePath(path)
	report, err := arch.ScanAndBuild(path, arch.ScanOpts{ExcludeTests: true})
	if err != nil {
		return nil, fmt.Errorf(errScanFailed, err)
	}
	r := &SuggestDepthResult{
		SuggestedDepth: report.SuggestedDepth,
		Components:     len(report.Architecture.Services),
	}
	if report.SuggestedDepth > 0 {
		r.Reasoning = fmt.Sprintf("Flat scan produces %d components. --depth %d reduces this while preserving meaningful grouping.",
			len(report.Architecture.Services), report.SuggestedDepth)
	} else {
		r.Reasoning = fmt.Sprintf("Flat scan produces %d components, which is already manageable.",
			len(report.Architecture.Services))
	}
	return r, nil
}

func (p *Protocol) GetHotSpots(ctx context.Context, path string, churnDays, topN int, cacheKey ...string) ([]arch.HotSpot, error) {
	path = p.resolvePath(path)
	report, err := p.getOrScan(path, cacheKey...)
	if err != nil {
		return nil, err
	}
	spots := make([]arch.HotSpot, len(report.HotSpots))
	copy(spots, report.HotSpots)
	sort.Slice(spots, func(i, j int) bool { return spots[i].Churn > spots[j].Churn })
	if topN <= 0 {
		topN = 10
	}
	if len(spots) > topN {
		spots = spots[:topN]
	}
	return spots, nil
}

func (p *Protocol) GetDependencies(ctx context.Context, path, component string, cacheKey ...string) (*DepResult, error) {
	path = p.resolvePath(path)
	if component == "" {
		return nil, ErrComponentRequired
	}
	report, err := p.getOrScan(path, cacheKey...)
	if err != nil {
		return nil, err
	}
	result := &DepResult{Component: component}
	for _, e := range report.Architecture.Edges {
		je := JSONEdge{From: e.From, To: e.To, Weight: e.Weight, CallSites: e.CallSites, LOCSurface: e.LOCSurface, Protocol: e.Protocol}
		if e.To == component {
			result.FanIn = append(result.FanIn, je)
		}
		if e.From == component {
			result.FanOut = append(result.FanOut, je)
		}
	}
	return result, nil
}

func (p *Protocol) GetCouplingTable(ctx context.Context, path, sortBy string, topN int, cacheKey ...string) (string, error) {
	path = p.resolvePath(path)
	report, err := p.getOrScan(path, cacheKey...)
	if err != nil {
		return "", err
	}
	if sortBy == "" {
		sortBy = "fan_in"
	}
	return arch.RenderCouplingTable(report, sortBy, topN), nil
}

func (p *Protocol) GetEdgeList(ctx context.Context, path, component string, cacheKey ...string) (string, error) {
	path = p.resolvePath(path)
	report, err := p.getOrScan(path, cacheKey...)
	if err != nil {
		return "", err
	}
	return arch.RenderEdgeList(report, component), nil
}

// CycleReport holds cycle detection results extracted from a cached scan.
type CycleReport struct {
	Cycles          []arch.Cycle          `json:"cycles"`
	ImportDepth     arch.DepthMap         `json:"import_depth"`
	LayerViolations []arch.LayerViolation `json:"layer_violations,omitempty"`
}

func (p *Protocol) GetCycles(ctx context.Context, path string, layers []string, cacheKey ...string) (*CycleReport, error) {
	path = p.resolvePath(path)
	report, err := p.getOrScan(path, cacheKey...)
	if err != nil {
		return nil, err
	}
	r := &CycleReport{
		Cycles:      report.Cycles,
		ImportDepth: report.ImportDepth,
	}
	if len(layers) > 0 {
		r.LayerViolations = arch.CheckLayerPurity(report.Architecture.Edges, layers)
	} else {
		r.LayerViolations = report.LayerViolations
	}
	return r, nil
}

// ViolationReport holds architecture violation detection results.
type ViolationReport struct {
	Layers     []string             `json:"layers"`
	Violations []arch.LayerViolation `json:"violations"`
	Cycles     []arch.Cycle         `json:"cycles,omitempty"`
	Summary    string               `json:"summary"`
}

func (p *Protocol) GetViolations(ctx context.Context, path string, layers []string, cacheKey ...string) (*ViolationReport, error) {
	path = p.resolvePath(path)
	report, err := p.getOrScan(path, cacheKey...)
	if err != nil {
		return nil, err
	}

	// Auto-detect layers from import depth if none provided.
	if len(layers) == 0 {
		layers = inferLayerOrder(report)
	}

	violations := arch.CheckLayerPurity(report.Architecture.Edges, layers)

	summary := fmt.Sprintf("%d layer(s), %d violation(s), %d cycle(s)",
		len(layers), len(violations), len(report.Cycles))
	if len(violations) == 0 {
		summary = fmt.Sprintf("Clean architecture: %d layer(s), 0 violations", len(layers))
	}

	return &ViolationReport{
		Layers:     layers,
		Violations: violations,
		Cycles:     report.Cycles,
		Summary:    summary,
	}, nil
}

// inferLayerOrder derives a layer ordering from import depth analysis.
// Components at depth 0 (no imports) are the bottom layer; higher depth = higher layer.
func inferLayerOrder(report *arch.ContextReport) []string {
	depths := report.ImportDepth
	if depths == nil {
		depths = arch.ComputeImportDepth(report.Architecture.Edges)
	}

	// Group components by depth.
	layerMap := make(map[int][]string)
	for _, svc := range report.Architecture.Services {
		d := depths[svc.Name]
		layerMap[d] = append(layerMap[d], svc.Name)
	}

	// Collect unique depth levels, sorted.
	depthLevels := make([]int, 0, len(layerMap))
	for d := range layerMap {
		depthLevels = append(depthLevels, d)
	}
	sort.Ints(depthLevels)

	// Flatten: bottom (depth 0) first, top (highest depth) last.
	var layers []string
	for _, d := range depthLevels {
		comps := layerMap[d]
		sort.Strings(comps)
		layers = append(layers, comps...)
	}
	return layers
}

// --- Desired state ---

func (p *Protocol) SetDesiredState(ctx context.Context, path string, ds *store.DesiredState) error {
	path = p.resolvePath(path)
	return p.db.PutDesiredState(ctx, path, ds)
}

func (p *Protocol) GetDesiredState(ctx context.Context, path string) (*store.DesiredState, error) {
	path = p.resolvePath(path)
	return p.db.GetDesiredState(ctx, path)
}

// DriftReport holds architecture drift analysis results.
type DriftReport struct {
	HasDesiredState    bool                  `json:"has_desired_state"`
	LayerViolations    []arch.LayerViolation `json:"layer_violations,omitempty"`
	BoundaryViolations []BoundaryViolation   `json:"boundary_violations,omitempty"`
	BudgetViolations   []BudgetViolation     `json:"budget_violations,omitempty"`
	BoundaryBreaches   int                   `json:"boundary_breaches"`
	ConstraintBreaches int                   `json:"constraint_breaches"`
	Score              float64               `json:"score"`
	Clean              bool                  `json:"clean"`
	Summary            string                `json:"summary"`
}

func (p *Protocol) GetDrift(ctx context.Context, path string, cacheKey ...string) (*DriftReport, error) {
	path = p.resolvePath(path)
	ds, err := p.db.GetDesiredState(ctx, path)
	if err != nil {
		return nil, err
	}
	if ds == nil {
		return &DriftReport{HasDesiredState: false, Summary: "No desired state configured. Use set_desired_state or suggest_architecture."}, nil
	}
	report, err := p.getOrScan(path, cacheKey...)
	if err != nil {
		return nil, err
	}

	// 1. Layer purity (existing).
	layerViolations := arch.CheckLayerPurity(report.Architecture.Edges, ds.Layers)

	// 2. Boundary rules (new).
	boundaryViolations := CheckBoundaryRules(report.Architecture.Edges, ds.Boundaries)

	// 3. Budget violations (new).
	var budgetViolations []BudgetViolation
	if len(ds.Constraints) > 0 {
		budgetReport := ComputeBudgetViolations(
			report.Architecture.Services,
			report.Architecture.Edges,
			ds.Constraints,
		)
		budgetViolations = budgetReport.Violations
	}

	// Compute compliance score.
	totalViolations := len(layerViolations) + len(boundaryViolations) + len(budgetViolations)
	totalChecks := len(report.Architecture.Edges) + len(ds.Boundaries) + countBudgetChecks(report.Architecture.Services, ds.Constraints)
	score := 100.0
	if totalChecks > 0 {
		score = float64(totalChecks-totalViolations) / float64(totalChecks) * 100
		if score < 0 {
			score = 0
		}
	}

	clean := totalViolations == 0

	// Build summary.
	var parts []string
	if len(layerViolations) > 0 {
		parts = append(parts, fmt.Sprintf("%d layer violation(s)", len(layerViolations)))
	}
	if len(boundaryViolations) > 0 {
		parts = append(parts, fmt.Sprintf("%d boundary violation(s)", len(boundaryViolations)))
	}
	if len(budgetViolations) > 0 {
		parts = append(parts, fmt.Sprintf("%d budget violation(s)", len(budgetViolations)))
	}
	summary := "Clean — no drift detected"
	if !clean {
		summary = strings.Join(parts, ", ") + fmt.Sprintf(" (score: %.1f%%)", score)
	}

	return &DriftReport{
		HasDesiredState:    true,
		LayerViolations:    layerViolations,
		BoundaryViolations: boundaryViolations,
		BudgetViolations:   budgetViolations,
		BoundaryBreaches:   len(boundaryViolations),
		ConstraintBreaches: len(budgetViolations),
		Score:              score,
		Clean:              clean,
		Summary:            summary,
	}, nil
}

// countBudgetChecks counts the number of budget checks that will be performed
// for a given set of services and constraints (used for score calculation).
func countBudgetChecks(services []arch.ArchService, constraints []store.HealthConstraint) int {
	svcMap := make(map[string]bool, len(services))
	for _, s := range services {
		svcMap[s.Name] = true
	}
	count := 0
	for _, c := range constraints {
		if !svcMap[c.Component] {
			continue
		}
		if c.MaxFanIn > 0 {
			count++
		}
		if c.MaxChurn > 0 {
			count++
		}
		if c.MaxNesting > 0 {
			count++
		}
	}
	return count
}

func (p *Protocol) SuggestArchitecture(ctx context.Context, path string, cacheKey ...string) (*store.DesiredState, error) {
	path = p.resolvePath(path)
	report, err := p.getOrScan(path, cacheKey...)
	if err != nil {
		return nil, err
	}
	layers := inferLayerOrder(report)
	return &store.DesiredState{Layers: layers}, nil
}

// StatusResult holds workspace status information.
type StatusResult struct {
	Version    string               `json:"version"`
	Workspaces []string             `json:"workspaces"`
	Projects   []store.ProjectInfo `json:"projects,omitempty"`
}

func (p *Protocol) Status(ctx context.Context) (*StatusResult, error) {
	projects, _ := p.db.ListProjects(ctx)
	return &StatusResult{
		Workspaces: p.workspaces,
		Projects:   projects,
	}, nil
}

// CheckDriftOnScan checks desired state against a scan report and returns a one-liner.
// Returns empty string if no desired state exists.
func (p *Protocol) CheckDriftOnScan(ctx context.Context, path string, report *arch.ContextReport) string {
	path = p.resolvePath(path)
	ds, err := p.db.GetDesiredState(ctx, path)
	if err != nil || ds == nil || len(ds.Layers) == 0 {
		return ""
	}
	violations := arch.CheckLayerPurity(report.Architecture.Edges, ds.Layers)
	if len(violations) == 0 {
		return "Architecture: clean"
	}
	return fmt.Sprintf("Architecture: %d violation(s)", len(violations))
}

// generateComponentMeta creates metadata for all components in a scan report.
func generateComponentMeta(report *arch.ContextReport) []store.ComponentMeta {
	depths := arch.ComputeImportDepth(report.Architecture.Edges)
	fanIn := make(map[string]int)
	for _, e := range report.Architecture.Edges {
		fanIn[e.To]++
	}

	meta := make([]store.ComponentMeta, 0, len(report.Architecture.Services))
	for _, s := range report.Architecture.Services {
		role := inferRole(s.Name)
		keywords := extractKeywords(s)
		health := "healthy"
		if fanIn[s.Name] >= arch.MinFanInHotSpot && s.Churn >= arch.MinChurnHotSpot {
			health = "sick"
		}
		meta = append(meta, store.ComponentMeta{
			Name:        s.Name,
			Role:        role,
			Keywords:    keywords,
			Description: fmt.Sprintf("%s with %d symbols, %d LOC", role, len(s.Symbols), s.LOC),
			Layer:       depths[s.Name],
			LOC:         s.LOC,
			FanIn:       fanIn[s.Name],
			Health:      health,
		})
	}
	return meta
}

func inferRole(name string) string {
	switch {
	case strings.HasPrefix(name, "cmd/"):
		return "entrypoint"
	case strings.HasPrefix(name, "internal/"):
		return "core"
	case strings.HasPrefix(name, "pkg/"):
		return "library"
	case strings.Contains(name, "test"):
		return "test"
	default:
		return "module"
	}
}

func extractKeywords(s arch.ArchService) []string {
	seen := make(map[string]bool)
	var keywords []string
	// Path segments as keywords.
	for _, seg := range strings.Split(s.Name, "/") {
		if seg != "" && !seen[seg] {
			seen[seg] = true
			keywords = append(keywords, seg)
		}
	}
	// First 10 exported symbol names.
	n := min(len(s.Symbols), 10)
	for _, sym := range s.Symbols[:n] {
		lower := strings.ToLower(sym)
		if !seen[lower] {
			seen[lower] = true
			keywords = append(keywords, lower)
		}
	}
	return keywords
}

// SearchComponents queries component metadata by keywords.
func (p *Protocol) SearchComponents(ctx context.Context, path, query string, cacheKey ...string) ([]store.ComponentMeta, error) {
	path = p.resolvePath(path)
	sha := p.db.ResolveHEAD(path)
	return p.db.SearchComponents(ctx, path, sha, query)
}

// CallerSite represents a single call site for a symbol.
type CallerSite struct {
	Caller       string `json:"caller"`
	CallerPkg    string `json:"caller_pkg"`
	Line         int    `json:"line,omitempty"`
	File         string `json:"file,omitempty"`
	ReceiverType string `json:"receiver_type,omitempty"`
}

// CallersReport holds all call sites for a given symbol.
type CallersReport struct {
	Symbol  string       `json:"symbol"`
	Callers []CallerSite `json:"callers"`
	Summary string       `json:"summary"`
}

func (p *Protocol) GetInterfaceMetrics(ctx context.Context, path string, cacheKey ...string) (*InterfaceMetricsReport, error) {
	path = p.resolvePath(path)
	fa := analysis.NewFallback(path)
	classes, err := fa.Classes(path)
	if err != nil {
		return nil, fmt.Errorf("classes: %w", err)
	}
	impls, err := fa.Implements(path)
	if err != nil {
		return nil, fmt.Errorf("implements: %w", err)
	}
	return ComputeInterfaceMetrics(classes, impls), nil
}

func (p *Protocol) GetCallers(ctx context.Context, path, symbol string, cacheKey ...string) (*CallersReport, error) {
	path = p.resolvePath(path)
	if symbol == "" {
		return nil, ErrComponentRequired
	}

	da := analysis.NewDeepFallback(path)
	cg, err := da.CallGraph(path, analysis.CallGraphOpts{Depth: analysis.DefaultCallGraphDepth})
	if err != nil {
		return nil, fmt.Errorf("call graph: %w", err)
	}

	var callers []CallerSite
	for _, edge := range cg.Edges {
		if edge.Callee == symbol {
			callers = append(callers, CallerSite{
				Caller:       edge.Caller,
				CallerPkg:    edge.CallerPkg,
				Line:         edge.Line,
				File:         edge.File,
				ReceiverType: edge.ReceiverType,
			})
		}
	}

	summary := fmt.Sprintf("%d caller(s) of %s", len(callers), symbol)
	return &CallersReport{Symbol: symbol, Callers: callers, Summary: summary}, nil
}

// --- Cross-repo comparison ---

// CrossRepoReport holds comparison results between two repos.
type CrossRepoReport struct {
	Overlap    []string `json:"overlap"`
	OnlyInA    []string `json:"only_in_a"`
	OnlyInB    []string `json:"only_in_b"`
	NewCycles  int      `json:"new_cycles_if_merged"`
	Summary    string   `json:"summary"`
}

func (p *Protocol) GetCrossRepo(ctx context.Context, pathA, pathB string, cacheKeyA, cacheKeyB string) (*CrossRepoReport, error) {
	reportA, err := p.getOrScan(p.resolvePath(pathA), cacheKeyA)
	if err != nil {
		return nil, fmt.Errorf("repo A: %w", err)
	}
	reportB, err := p.getOrScan(p.resolvePath(pathB), cacheKeyB)
	if err != nil {
		return nil, fmt.Errorf("repo B: %w", err)
	}

	setA := make(map[string]bool)
	for _, s := range reportA.Architecture.Services {
		setA[s.Name] = true
	}
	setB := make(map[string]bool)
	for _, s := range reportB.Architecture.Services {
		setB[s.Name] = true
	}

	var overlap, onlyA, onlyB []string
	for n := range setA {
		if setB[n] {
			overlap = append(overlap, n)
		} else {
			onlyA = append(onlyA, n)
		}
	}
	for n := range setB {
		if !setA[n] {
			onlyB = append(onlyB, n)
		}
	}
	sort.Strings(overlap)
	sort.Strings(onlyA)
	sort.Strings(onlyB)

	// Simulate merge: combine edges and detect new cycles.
	allEdges := append(reportA.Architecture.Edges, reportB.Architecture.Edges...)
	mergedCycles := arch.DetectCycles(allEdges)
	existingCycles := len(reportA.Cycles) + len(reportB.Cycles)
	newCycles := max(len(mergedCycles)-existingCycles, 0)

	summary := fmt.Sprintf("%d shared, %d only-A, %d only-B, %d new cycles if merged",
		len(overlap), len(onlyA), len(onlyB), newCycles)

	return &CrossRepoReport{
		Overlap: overlap, OnlyInA: onlyA, OnlyInB: onlyB,
		NewCycles: newCycles, Summary: summary,
	}, nil
}

// --- Analysis presets ---

const (
	PresetArchReview = "architecture_review"
	PresetHealthCheck = "health_check"
	PresetOnboarding  = "onboarding"
	PresetPrePR       = "pre_pr"
)

func (p *Protocol) RunPreset(ctx context.Context, path, preset string, cacheKey ...string) (string, error) {
	path = p.resolvePath(path)
	report, err := p.getOrScan(path, cacheKey...)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	switch preset {
	case PresetArchReview:
		fmt.Fprintf(&b, "# Architecture Review: %s\n\n", report.ModulePath)
		fmt.Fprintf(&b, "%d components, %d edges, %d cycles\n\n", len(report.Architecture.Services), len(report.Architecture.Edges), len(report.Cycles))
		spots := report.HotSpots
		if len(spots) > 5 {
			spots = spots[:5]
		}
		if len(spots) > 0 {
			b.WriteString("## Hot Spots\n")
			for _, s := range spots {
				fmt.Fprintf(&b, "- %s (churn:%d, fan-in:%d)\n", s.Component, s.Churn, s.FanIn)
			}
		}
		if len(report.Cycles) > 0 {
			b.WriteString("\n## Cycles\n")
			for i, c := range report.Cycles {
				if i >= 3 {
					fmt.Fprintf(&b, "... and %d more\n", len(report.Cycles)-3)
					break
				}
				fmt.Fprintf(&b, "- %s\n", strings.Join(c, " → "))
			}
		}

	case PresetHealthCheck:
		fmt.Fprintf(&b, "# Health Check: %s\n\n", report.ModulePath)
		spots := report.HotSpots
		if len(spots) > 5 {
			spots = spots[:5]
		}
		for _, s := range spots {
			fmt.Fprintf(&b, "- %s (churn:%d, fan-in:%d)\n", s.Component, s.Churn, s.FanIn)
		}
		if len(spots) == 0 {
			b.WriteString("No hot spots detected.\n")
		}

	case PresetOnboarding:
		fmt.Fprintf(&b, "# Onboarding: %s\n\n", report.ModulePath)
		fmt.Fprintf(&b, "%d components, scanner=%s\n\n", len(report.Architecture.Services), report.Scanner)
		b.WriteString("## Top Components\n")
		n := min(len(report.Architecture.Services), 10)
		for _, s := range report.Architecture.Services[:n] {
			fmt.Fprintf(&b, "- %s (%d LOC)\n", s.Name, s.LOC)
		}

	case PresetPrePR:
		fmt.Fprintf(&b, "# Pre-PR Review: %s\n\n", report.ModulePath)
		fmt.Fprintf(&b, "%d components, %d cycles, %d violations\n",
			len(report.Architecture.Services), len(report.Cycles), len(report.LayerViolations))

	default:
		return "", fmt.Errorf("unknown preset %q (valid: %s, %s, %s, %s)",
			preset, PresetArchReview, PresetHealthCheck, PresetOnboarding, PresetPrePR)
	}
	return b.String(), nil
}

// --- Component drill-down ---

// ComponentDetail holds single-component analysis data.
type ComponentDetail struct {
	Name      string   `json:"name"`
	LOC       int      `json:"loc"`
	Symbols   []string `json:"symbols,omitempty"`
	Imports   []string `json:"imports,omitempty"`
	Importers []string `json:"importers,omitempty"`
	Churn     int      `json:"churn"`
	Health    string   `json:"health"`
}

func (p *Protocol) GetComponentDetail(ctx context.Context, path, name string, cacheKey ...string) (*ComponentDetail, error) {
	path = p.resolvePath(path)
	if name == "" {
		return nil, ErrComponentRequired
	}
	report, err := p.getOrScan(path, cacheKey...)
	if err != nil {
		return nil, err
	}

	var svc *arch.ArchService
	for i := range report.Architecture.Services {
		if report.Architecture.Services[i].Name == name {
			svc = &report.Architecture.Services[i]
			break
		}
	}
	if svc == nil {
		return nil, fmt.Errorf("component %q not found", name)
	}

	var imports, importers []string
	for _, e := range report.Architecture.Edges {
		if e.From == name {
			imports = append(imports, e.To)
		}
		if e.To == name {
			importers = append(importers, e.From)
		}
	}

	syms := svc.Symbols
	if len(syms) > 20 {
		syms = syms[:20]
	}

	fi := 0
	for _, e := range report.Architecture.Edges {
		if e.To == name {
			fi++
		}
	}
	health := "healthy"
	if fi >= arch.MinFanInHotSpot && svc.Churn >= arch.MinChurnHotSpot {
		health = "sick"
	}

	return &ComponentDetail{
		Name: name, LOC: svc.LOC, Symbols: syms,
		Imports: imports, Importers: importers,
		Churn: svc.Churn, Health: health,
	}, nil
}

// --- Natural language query ---

// QueryResult holds the answer to a natural language architecture question.
type QueryResult struct {
	Query      string `json:"query"`
	Action     string `json:"resolved_action"`
	Answer     any    `json:"answer"`
}

func (p *Protocol) AnswerQuery(ctx context.Context, path, query string, cacheKey ...string) (*QueryResult, error) {
	path = p.resolvePath(path)
	q := strings.ToLower(query)

	type pattern struct {
		keywords []string
		action   string
	}
	patterns := []pattern{
		{[]string{"risk", "hot"}, "coupling view=hot_spots"},
		{[]string{"cycle", "circular"}, "cycles"},
		{[]string{"depend", "import", "who uses"}, "deps"},
		{[]string{"violat", "layer"}, "violations"},
		{[]string{"change", "diff", "what changed"}, "scan_diff"},
		{[]string{"overview", "architect"}, "preset=architecture_review"},
		{[]string{"health", "status"}, "preset=health_check"},
		{[]string{"onboard", "getting started"}, "preset=onboarding"},
	}

	for _, pat := range patterns {
		for _, kw := range pat.keywords {
			if strings.Contains(q, kw) {
				switch {
				case strings.HasPrefix(pat.action, "coupling"):
					report, err := p.getOrScan(path, cacheKey...)
					if err != nil {
						return nil, err
					}
					return &QueryResult{Query: query, Action: pat.action, Answer: report.HotSpots}, nil
				case pat.action == "cycles":
					r, err := p.GetCycles(ctx, path, nil, cacheKey...)
					if err != nil {
						return nil, err
					}
					return &QueryResult{Query: query, Action: pat.action, Answer: r}, nil
				case pat.action == "violations":
					r, err := p.GetViolations(ctx, path, nil, cacheKey...)
					if err != nil {
						return nil, err
					}
					return &QueryResult{Query: query, Action: pat.action, Answer: r}, nil
				default:
					return &QueryResult{
						Query:  query,
						Action: pat.action,
						Answer: fmt.Sprintf("Suggested action: analysis %s", pat.action),
					}, nil
				}
			}
		}
	}

	return &QueryResult{
		Query:  query,
		Action: "none",
		Answer: "No matching pattern. Try: riskiest, cycles, violations, health, overview, what changed",
	}, nil
}

// GenerateHints returns follow-up action suggestions based on analysis findings.
func GenerateHints(report *arch.ContextReport) []string {
	var hints []string
	if len(report.Cycles) > 0 {
		hints = append(hints, fmt.Sprintf("Found %d cycle(s) — try: analysis action=violations", len(report.Cycles)))
	}
	if len(report.HotSpots) > 0 {
		hints = append(hints, fmt.Sprintf("Found %d hot spot(s) — try: analysis action=component component=%s", len(report.HotSpots), report.HotSpots[0].Component))
	}
	if len(report.LayerViolations) > 0 {
		hints = append(hints, fmt.Sprintf("Found %d layer violation(s) — try: render_diagram type=layers", len(report.LayerViolations)))
	}
	return hints
}

// ScanDiffReport holds structural differences between two cached scans.
type ScanDiffReport struct {
	BeforeSHA         string   `json:"before_sha"`
	AfterSHA          string   `json:"after_sha"`
	AddedComponents   []string `json:"added_components,omitempty"`
	RemovedComponents []string `json:"removed_components,omitempty"`
	AddedEdges        int      `json:"added_edges"`
	RemovedEdges      int      `json:"removed_edges"`
	LOCBefore         int      `json:"loc_before"`
	LOCAfter          int      `json:"loc_after"`
	LOCDelta          int      `json:"loc_delta"`
	Summary           string   `json:"summary"`
}

func (p *Protocol) GetScanDiff(ctx context.Context, path, beforeSHA, afterSHA string) (*ScanDiffReport, error) {
	path = p.resolvePath(path)

	if afterSHA == "" {
		afterSHA = p.db.ResolveHEAD(path)
	}
	if beforeSHA == "" {
		return nil, ErrBeforeSHARequired
	}

	before, hit, err := p.db.GetReport(ctx,path, beforeSHA)
	if err != nil || !hit {
		return nil, fmt.Errorf(errNoCachedScan, beforeSHA)
	}
	after, hit, err := p.db.GetReport(ctx,path, afterSHA)
	if err != nil || !hit {
		return nil, fmt.Errorf(errNoCachedScan, afterSHA)
	}

	beforeSet := make(map[string]bool)
	for _, s := range before.Architecture.Services {
		beforeSet[s.Name] = true
	}
	afterSet := make(map[string]bool)
	for _, s := range after.Architecture.Services {
		afterSet[s.Name] = true
	}

	var added, removed []string
	for name := range afterSet {
		if !beforeSet[name] {
			added = append(added, name)
		}
	}
	for name := range beforeSet {
		if !afterSet[name] {
			removed = append(removed, name)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)

	beforeEdges := make(map[[2]string]bool)
	for _, e := range before.Architecture.Edges {
		beforeEdges[[2]string{e.From, e.To}] = true
	}
	afterEdges := make(map[[2]string]bool)
	for _, e := range after.Architecture.Edges {
		afterEdges[[2]string{e.From, e.To}] = true
	}
	addedEdges, removedEdges := 0, 0
	for e := range afterEdges {
		if !beforeEdges[e] {
			addedEdges++
		}
	}
	for e := range beforeEdges {
		if !afterEdges[e] {
			removedEdges++
		}
	}

	locBefore, locAfter := 0, 0
	for _, s := range before.Architecture.Services {
		locBefore += s.LOC
	}
	for _, s := range after.Architecture.Services {
		locAfter += s.LOC
	}

	summary := fmt.Sprintf("%d→%d components (%+d), %d→%d edges (%+d), %d→%d LOC (%+d)",
		len(before.Architecture.Services), len(after.Architecture.Services), len(added)-len(removed),
		len(before.Architecture.Edges), len(after.Architecture.Edges), addedEdges-removedEdges,
		locBefore, locAfter, locAfter-locBefore)

	return &ScanDiffReport{
		BeforeSHA:         beforeSHA,
		AfterSHA:          afterSHA,
		AddedComponents:   added,
		RemovedComponents: removed,
		AddedEdges:        addedEdges,
		RemovedEdges:      removedEdges,
		LOCBefore:         locBefore,
		LOCAfter:          locAfter,
		LOCDelta:          locAfter - locBefore,
		Summary:           summary,
	}, nil
}

// CoverageReport holds per-component coverage data.
type CoverageReport struct {
	Coverage       []arch.CoverageResult `json:"coverage"`
	BelowThreshold []arch.CoverageResult `json:"below_threshold,omitempty"`
}

func (p *Protocol) GetCoverage(ctx context.Context, path string, threshold float64) (*CoverageReport, error) {
	path = p.resolvePath(path)
	cov, err := arch.RunGoCoverage(path, arch.DetectProjectPath(path))
	if err != nil {
		return nil, err
	}
	r := &CoverageReport{Coverage: cov}
	if threshold > 0 {
		for _, c := range cov {
			if c.CoveragePct < threshold {
				r.BelowThreshold = append(r.BelowThreshold, c)
			}
		}
	}
	sort.Slice(r.Coverage, func(i, j int) bool { return r.Coverage[i].Component < r.Coverage[j].Component })
	return r, nil
}

// APISurfaceReport holds API surface and boundary crossing data.
type APISurfaceReport struct {
	Surfaces  []arch.APISurface       `json:"surfaces"`
	Crossings []arch.BoundaryCrossing `json:"crossings,omitempty"`
}

func (p *Protocol) GetAPISurface(ctx context.Context, path string, trusted []string, cacheKey ...string) (*APISurfaceReport, error) {
	path = p.resolvePath(path)
	report, err := p.getOrScan(path, cacheKey...)
	if err != nil {
		return nil, err
	}
	return &APISurfaceReport{
		Surfaces:  report.APISurfaces,
		Crossings: report.BoundaryCrossings,
	}, nil
}

func (p *Protocol) ValidateArchitecture(ctx context.Context, path, desiredState, format string) (*arch.ArchDrift, error) {
	path = p.resolvePath(path)
	report, err := p.getOrScan(path)
	if err != nil {
		return nil, err
	}
	desired, err := arch.ParseDesiredState(desiredState, format)
	if err != nil {
		return nil, fmt.Errorf("parse desired state: %w", err)
	}
	return arch.ValidateArchitecture(*desired, report.Architecture), nil
}

// RemoteResult wraps a remote scan report with its cache key.
type RemoteResult struct {
	Report   *arch.ContextReport `json:"report"`
	CacheKey string              `json:"cache_key"`
	RefSHA   string              `json:"ref_sha"`
}

func (p *Protocol) CodographRemote(ctx context.Context, url string, opts RemoteOpts) (*RemoteResult, error) {
	if url == "" {
		return nil, ErrURLRequired
	}
	result, err := remote.Codograph(ctx, url, remote.Opts{
		Ref:       opts.Ref,
		Keep:      opts.Keep,
		Depth:     opts.Depth,
		ChurnDays: opts.ChurnDays,
		Budget:    opts.Budget,
		Intent:    opts.Intent,
	})
	if err != nil {
		return nil, fmt.Errorf("remote codography: %w", err)
	}
	cacheKey := remote.CacheKey(url, result.RefSHA)
	remoteProject := "remote:" + remote.NormalizeURL(url)
	_ = p.db.PutReport(ctx, remoteProject, result.RefSHA, result.Report)
	_ = p.db.RecordScan(ctx, string(history.Remote), remote.NormalizeURL(url), result.RefSHA, result.Report)
	return &RemoteResult{
		Report:   result.Report,
		CacheKey: cacheKey,
		RefSHA:   result.RefSHA,
	}, nil
}

func (p *Protocol) GetHistory(ctx context.Context, path string, last int) ([]history.EntrySummary, error) {
	path = p.resolvePath(path)
	abs, _ := filepath.Abs(path)
	if last <= 0 {
		last = 10
	}
	entries, err := p.db.ListHistory(ctx, abs, last)
	if err != nil {
		return nil, err
	}
	// Convert store.HistoryEntry to history.EntrySummary for backward compat.
	summaries := make([]history.EntrySummary, len(entries))
	for i, e := range entries {
		summaries[i] = history.EntrySummary{
			Timestamp:  e.Timestamp,
			HeadSHA:    e.SHA,
			Source:     history.Source(e.Source),
			RepoPath:   e.RepoPath,
			Components: e.Components,
			Edges:      e.Edges,
		}
	}
	return summaries, nil
}

func (p *Protocol) DiffCodographs(ctx context.Context, path string) (*history.CodographDiff, error) {
	path = p.resolvePath(path)
	abs, _ := filepath.Abs(path)
	prev, err := p.db.GetHistoryReport(ctx, abs, -2)
	if err != nil {
		return nil, fmt.Errorf("get previous codograph: %w", err)
	}
	latest, err := p.db.GetHistoryReport(ctx, abs, -1)
	if err != nil {
		return nil, fmt.Errorf("get latest codograph: %w", err)
	}
	return history.DiffReports(prev, latest), nil
}

func (p *Protocol) DiffBranches(ctx context.Context, path, branchA, branchB string) (*BranchDiffResult, error) {
	path = p.resolvePath(path)
	if branchA == "" || branchB == "" {
		return nil, ErrBothBranchesRequired
	}
	reportA, err := p.scanBranch(path, branchA)
	if err != nil {
		return nil, fmt.Errorf("scan branch %s: %w", branchA, err)
	}
	reportB, err := p.scanBranch(path, branchB)
	if err != nil {
		return nil, fmt.Errorf("scan branch %s: %w", branchB, err)
	}
	return &BranchDiffResult{
		BranchA: branchA,
		BranchB: branchB,
		Diff:    history.DiffReports(reportA, reportB),
	}, nil
}

func (p *Protocol) GetRules(ctx context.Context, path string) ([]cursor.Rule, error) {
	path = p.resolvePath(path)
	return cursor.ReadRules(path)
}

func (p *Protocol) GetSkills(ctx context.Context, path string) ([]cursor.Skill, error) {
	path = p.resolvePath(path)
	return cursor.ReadSkills(path)
}

func (p *Protocol) GetConventions(ctx context.Context, path string) (*analysis.ConventionReport, error) {
	path = p.resolvePath(path)
	return analysis.DetectConventions(path)
}

func (p *Protocol) GetImpact(ctx context.Context, path, component string, cacheKey ...string) (*ImpactResult, error) {
	path = p.resolvePath(path)
	if component == "" {
		return nil, ErrComponentRequired
	}
	report, err := p.getOrScan(path, cacheKey...)
	if err != nil {
		return nil, err
	}
	return ComputeImpact(
		report.Architecture.Edges,
		report.Architecture.Services,
		component,
	)
}

func (p *Protocol) GetGaps(ctx context.Context, path string) (*GapReport, error) {
	path = p.resolvePath(path)
	report, err := p.getOrScan(path)
	if err != nil {
		return nil, err
	}
	return DetectGaps(report, path)
}

func (p *Protocol) GetBudgets(ctx context.Context, path string, cacheKey ...string) (*BudgetReport, error) {
	path = p.resolvePath(path)
	report, err := p.getOrScan(path, cacheKey...)
	if err != nil {
		return nil, err
	}
	desired, _ := p.db.GetDesiredState(ctx, path)
	if desired == nil || len(desired.Constraints) == 0 {
		return &BudgetReport{Summary: "no budgets defined"}, nil
	}
	return ComputeBudgetViolations(report.Architecture.Services, report.Architecture.Edges, desired.Constraints), nil
}

func (p *Protocol) GetBlastRadius(ctx context.Context, path string, files []string, since string, cacheKey ...string) (*BlastRadiusReport, error) {
	path = p.resolvePath(path)
	report, err := p.getOrScan(path, cacheKey...)
	if err != nil {
		return nil, err
	}
	return ComputeBlastRadius(
		report.Architecture.Edges,
		report.Architecture.Services,
		report.ModulePath,
		path,
		files,
		since,
	)
}

func (p *Protocol) GetImportDirection(ctx context.Context, path string, cacheKey ...string) (*ImportDirectionReport, error) {
	path = p.resolvePath(path)
	report, err := p.getOrScan(path, cacheKey...)
	if err != nil {
		return nil, err
	}
	return ComputeImportDirection(report.Architecture.Edges, report.ImportDepth), nil
}

func (p *Protocol) GetModuleDependencies(_ context.Context, path string, _ ...string) (*DependencyReport, error) {
	path = p.resolvePath(path)
	goModPath := filepath.Join(path, "go.mod")
	return ComputeDependencies(goModPath)
}

func (p *Protocol) GetTrustBoundaries(ctx context.Context, path string, cacheKey ...string) (*TrustBoundaryReport, error) {
	path = p.resolvePath(path)
	report, err := p.getOrScan(path, cacheKey...)
	if err != nil {
		return nil, err
	}
	return ComputeTrustBoundaries(report.Architecture.Services, report.Architecture.Edges), nil
}

// Workspaces returns the configured workspace root paths.
func (p *Protocol) Workspaces() []string {
	return p.workspaces
}

// --- helpers ---

// GetCachedReport retrieves a report stored under a cache key (e.g. from scan_remote).
func (p *Protocol) GetCachedReport(cacheKey string) (*arch.ContextReport, error) {
	if idx := strings.LastIndex(cacheKey, "@"); idx >= 0 {
		path := cacheKey[:idx]
		sha := cacheKey[idx+1:]
		if report, hit, err := p.db.GetReport(context.Background(), path, sha); err == nil && hit {
			return report, nil
		}
	}
	return nil, fmt.Errorf(errNoCachedReport, cacheKey)
}

func (p *Protocol) getOrScan(path string, cacheKeys ...string) (*arch.ContextReport, error) {
	// If a cache key is provided, resolve from cache directly.
	for _, ck := range cacheKeys {
		if ck == "" {
			continue
		}
		if idx := strings.LastIndex(ck, "@"); idx >= 0 {
			ckPath := ck[:idx]
			sha := ck[idx+1:]
			if report, hit, err := p.db.GetReport(context.Background(), ckPath, sha); err == nil && hit {
				return report, nil
			}
		}
		return nil, fmt.Errorf(errNoCachedReport, ck)
	}

	sha := p.db.ResolveHEAD(path)
	if cached, hit, _ := p.db.GetReport(context.Background(), path, sha); hit {
		return cached, nil
	}
	r, err := arch.ScanAndBuild(path, arch.ScanOpts{ExcludeTests: true, ChurnDays: 30})
	if err != nil {
		return nil, fmt.Errorf(errScanFailed, err)
	}
	if sha != "" {
		p.db.PutReport(context.Background(), path, sha, r)
	}
	return r, nil
}

func (p *Protocol) scanBranch(repoPath, ref string) (*arch.ContextReport, error) {
	sha, err := p.db.ResolveBranch(repoPath, ref)
	if err != nil {
		return nil, err
	}
	if cached, hit, _ := p.db.GetReport(context.Background(), repoPath, sha); hit {
		return cached, nil
	}
	currentBranch := getCurrentBranch(repoPath)
	if err := checkoutRef(repoPath, ref); err != nil {
		return nil, fmt.Errorf("checkout %s: %w", ref, err)
	}
	defer func() {
		if currentBranch != "" {
			checkoutRef(repoPath, currentBranch)
		}
	}()
	report, err := arch.ScanAndBuild(repoPath, arch.ScanOpts{ExcludeTests: true, ChurnDays: 30})
	if err != nil {
		return nil, err
	}
	p.db.PutReport(context.Background(), repoPath, sha, report)
	return report, nil
}

func (p *Protocol) resolvePath(path string) string {
	if path == "" {
		if len(p.workspaces) > 0 {
			return p.workspaces[0]
		}
		return "."
	}

	abs, err := filepath.Abs(path)
	if err == nil {
		if _, serr := os.Stat(abs); serr == nil {
			return abs
		}
	}

	for _, ws := range p.workspaces {
		candidate := filepath.Join(ws, path)
		if _, serr := os.Stat(candidate); serr == nil {
			return candidate
		}
	}

	if abs != "" {
		return abs
	}
	return path
}

func getCurrentBranch(dir string) string {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func checkoutRef(dir, ref string) error {
	cmd := exec.Command("git", "checkout", ref)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git checkout %s: %s: %w", ref, string(out), err)
	}
	return nil
}

// --- Health ---

type HealthResult struct {
	OK     bool          `json:"ok"`
	Checks []HealthCheck `json:"checks"`
}

type HealthCheck struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
}

func (p *Protocol) Health(_ context.Context) *HealthResult {
	r := &HealthResult{OK: true}

	// Health checks for filesystem-backed stores.
	if fs, ok := p.db.(*store.FilesystemStore); ok {
		r.Checks = append(r.Checks, checkDir("cache_dir", fs.CacheRoot()))
		r.Checks = append(r.Checks, checkDir("history_dir", fs.HistoryDir()))
	}
	r.Checks = append(r.Checks, checkGit())
	for _, ws := range p.workspaces {
		r.Checks = append(r.Checks, checkDir("workspace:"+ws, ws))
	}

	for i := range r.Checks {
		if !r.Checks[i].OK {
			r.OK = false
		}
	}
	return r
}

func checkDir(name, path string) HealthCheck {
	if path == "" {
		return HealthCheck{Name: name, OK: false, Detail: "path is empty"}
	}
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		if err := os.MkdirAll(path, 0o755); err != nil {
			return HealthCheck{Name: name, OK: false, Detail: fmt.Sprintf("does not exist and cannot create: %v", err)}
		}
		return HealthCheck{Name: name, OK: true, Detail: path + " (created)"}
	}
	if err != nil {
		return HealthCheck{Name: name, OK: false, Detail: err.Error()}
	}
	if !info.IsDir() {
		return HealthCheck{Name: name, OK: false, Detail: path + " is not a directory"}
	}
	return HealthCheck{Name: name, OK: true, Detail: path}
}

func checkGit() HealthCheck {
	cmd := exec.Command("git", "--version")
	out, err := cmd.Output()
	if err != nil {
		return HealthCheck{Name: "git", OK: false, Detail: "git not found on PATH"}
	}
	return HealthCheck{Name: "git", OK: true, Detail: strings.TrimSpace(string(out))}
}

// --- Evolution ---

// EvolutionOpts controls an architecture evolution scan.
type EvolutionOpts struct {
	Path      string `json:"path"`
	OldestRef string `json:"oldest_ref,omitempty"`
	NewestRef string `json:"newest_ref,omitempty"`
	Steps     int    `json:"steps,omitempty"`
	Stride    int    `json:"stride,omitempty"`
	Depth     int    `json:"depth,omitempty"`
}

// EvolutionResult is the timeline of architecture snapshots.
type EvolutionResult struct {
	Path    string          `json:"path"`
	Steps   []EvolutionStep `json:"steps"`
	Summary string          `json:"summary"`
}

// EvolutionStep is a single point in the evolution timeline.
type EvolutionStep struct {
	Index      int                    `json:"index"`
	SHA        string                 `json:"sha"`
	ShortSHA   string                 `json:"short_sha"`
	Message    string                 `json:"message"`
	Date       string                 `json:"date"`
	Components int                    `json:"components"`
	Edges      int                    `json:"edges"`
	TotalLOC   int                    `json:"total_loc"`
	Diff       *history.CodographDiff `json:"diff,omitempty"`
}

// CommitMeta holds metadata for a single git commit.
type CommitMeta struct {
	SHA     string
	Message string
	Date    string
}

// listCommits enumerates commits in a range or the last N commits.
// Range mode (oldest != ""): git log --reverse oldest^..newest (inclusive both ends).
// Steps mode (limit > 0): git log --reverse -n limit newest.
func listCommits(repoPath, oldest, newest string, limit int) ([]CommitMeta, error) {
	if newest == "" {
		newest = "HEAD"
	}
	args := []string{"log", "--reverse", "--format=%H||%aI||%s"}
	if oldest != "" {
		args = append(args, oldest+"^.."+newest)
	} else if limit > 0 {
		args = append(args, "-n", strconv.Itoa(limit), newest)
	} else {
		return nil, ErrOldestOrStepsRequired
	}

	cmd := exec.Command("git", args...)
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git log: %w", err)
	}

	var commits []CommitMeta
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, "||", 3)
		if len(parts) < 3 {
			continue
		}
		commits = append(commits, CommitMeta{
			SHA:     parts[0],
			Message: parts[2],
			Date:    parts[1][:10], // YYYY-MM-DD from ISO 8601
		})
	}
	return commits, nil
}

// sampleCommits picks every stride-th commit, always including the first and last.
func sampleCommits(commits []CommitMeta, stride int) []CommitMeta {
	if stride <= 1 || len(commits) <= 2 {
		return commits
	}
	var sampled []CommitMeta
	for i := 0; i < len(commits); i += stride {
		sampled = append(sampled, commits[i])
	}
	if sampled[len(sampled)-1].SHA != commits[len(commits)-1].SHA {
		sampled = append(sampled, commits[len(commits)-1])
	}
	return sampled
}

func totalLOC(report *arch.ContextReport) int {
	total := 0
	for _, svc := range report.Architecture.Services {
		total += svc.LOC
	}
	return total
}

// Evolution scans architecture at multiple commits to show structural growth.
func (p *Protocol) Evolution(ctx context.Context, opts EvolutionOpts) (*EvolutionResult, error) {
	path := p.resolvePath(opts.Path)

	commits, err := listCommits(path, opts.OldestRef, opts.NewestRef, opts.Steps)
	if err != nil {
		return nil, fmt.Errorf("enumerate commits: %w", err)
	}
	if len(commits) == 0 {
		return nil, fmt.Errorf("no commits found in range")
	}

	commits = sampleCommits(commits, opts.Stride)

	currentBranch := getCurrentBranch(path)
	needsRestore := false
	defer func() {
		if needsRestore && currentBranch != "" {
			checkoutRef(path, currentBranch)
		}
	}()

	var steps []EvolutionStep
	var prevReport *arch.ContextReport

	for i, commit := range commits {
		report, cached, cacheErr := p.db.GetReport(ctx,path, commit.SHA)
		if cacheErr != nil || !cached {
			if !needsRestore {
				needsRestore = true
			}
			if err := checkoutRef(path, commit.SHA); err != nil {
				return nil, fmt.Errorf("checkout %s: %w", commit.SHA[:8], err)
			}
			report, err = arch.ScanAndBuild(path, arch.ScanOpts{
				ExcludeTests: true,
				ChurnDays:    30,
				Depth:        opts.Depth,
			})
			if err != nil {
				return nil, fmt.Errorf("scan %s: %w", commit.SHA[:8], err)
			}
			_ = p.db.PutReport(ctx,path, commit.SHA, report)
		}

		step := EvolutionStep{
			Index:      i,
			SHA:        commit.SHA,
			ShortSHA:   commit.SHA[:7],
			Message:    commit.Message,
			Date:       commit.Date,
			Components: len(report.Architecture.Services),
			Edges:      len(report.Architecture.Edges),
			TotalLOC:   totalLOC(report),
		}
		if prevReport != nil {
			step.Diff = history.DiffReports(prevReport, report)
		}
		steps = append(steps, step)
		prevReport = report
	}

	result := &EvolutionResult{
		Path:  path,
		Steps: steps,
	}
	result.Summary = buildEvolutionSummary(steps)
	return result, nil
}

func buildEvolutionSummary(steps []EvolutionStep) string {
	if len(steps) == 0 {
		return "no steps"
	}
	first := steps[0]
	last := steps[len(steps)-1]

	pct := func(old, new int) string {
		if old == 0 {
			if new == 0 {
				return "0%"
			}
			return "new"
		}
		return fmt.Sprintf("%+.0f%%", float64(new-old)/float64(old)*100)
	}

	return fmt.Sprintf("Growth: %d -> %d components (%s), %d -> %d edges (%s), %d -> %d LOC (%s)",
		first.Components, last.Components, pct(first.Components, last.Components),
		first.Edges, last.Edges, pct(first.Edges, last.Edges),
		first.TotalLOC, last.TotalLOC, pct(first.TotalLOC, last.TotalLOC),
	)
}

// RenderEvolutionTable renders the evolution result as a markdown table.
func RenderEvolutionTable(r *EvolutionResult) string {
	var b strings.Builder
	basename := filepath.Base(r.Path)
	strideInfo := ""
	if len(r.Steps) > 0 {
		strideInfo = fmt.Sprintf("%d steps", len(r.Steps))
	}
	fmt.Fprintf(&b, "## Architecture Evolution: %s (%s)\n\n", basename, strideInfo)
	fmt.Fprintln(&b, "| # | SHA | Date | Message | Pkgs | Edges | LOC | Delta |")
	fmt.Fprintln(&b, "|---|---------|------------|----------------------|------|-------|------|------------------------|")

	for _, s := range r.Steps {
		delta := "(basis)"
		if s.Diff != nil {
			delta = s.Diff.Summary
		}
		const maxCommitMsg = 40
		msg := s.Message
		if len(msg) > maxCommitMsg {
			msg = msg[:maxCommitMsg-3] + "..."
		}
		fmt.Fprintf(&b, "| %d | %s | %s | %s | %d | %d | %d | %s |\n",
			s.Index, s.ShortSHA, s.Date, msg,
			s.Components, s.Edges, s.TotalLOC, delta)
	}

	fmt.Fprintf(&b, "\n%s\n", r.Summary)
	return b.String()
}
