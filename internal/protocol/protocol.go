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
	"github.com/dpopsuev/locus/internal/clinic"
	"github.com/dpopsuev/locus/internal/constraint"
	"github.com/dpopsuev/locus/internal/cursor"
	"github.com/dpopsuev/locus/internal/graph"
	"github.com/dpopsuev/locus/internal/history"
	"github.com/dpopsuev/locus/internal/impact"
	"github.com/dpopsuev/locus/internal/port"
	"github.com/dpopsuev/locus/internal/remote"
)

// Error messages used across protocol methods.
var (
	ErrComponentRequired     = errors.New("component is required")
	ErrBeforeSHARequired     = errors.New("before_sha is required")
	ErrURLRequired           = errors.New("url is required")
	ErrBothBranchesRequired  = errors.New("both branch_a and branch_b are required")
	ErrOldestOrStepsRequired = errors.New("either oldest_ref or steps is required")
	ErrUnknownPreset         = errors.New("unknown preset")
	ErrComponentNotFound     = errors.New("component not found")
	ErrNoCommitsFound        = errors.New("no commits found in range")
)

// Sentinel errors for scan failures.
var (
	ErrScanFailed     = errors.New("scan failed")
	ErrNoCachedScan   = errors.New("no cached scan — run scan_local first")
	ErrNoCachedReport = errors.New("no cached report — run scan_local first")
)

// ProtocolStore is the subset of store.Store that Protocol requires.
// Defined here to invert the dependency direction: protocol depends on its own
// interface, not on the concrete store package's composite interface.
type ProtocolStore interface {
	port.ReportStore
	port.HistoryStore
	port.GitResolver
	port.DesiredStateStore
	PutComponentMeta(ctx context.Context, project, sha string, meta []port.ComponentMeta) error
	SearchComponents(ctx context.Context, project, sha, query string) ([]port.ComponentMeta, error)
	ListProjects(ctx context.Context) ([]port.ProjectInfo, error)
}

// HealthCheckable is implemented by stores that expose filesystem paths for health checks.
type HealthCheckable interface {
	CacheRoot() string
	HistoryDir() string
}

// Protocol encapsulates all Locus business logic.
// Both CLI and MCP are thin wrappers around this.
type Protocol struct {
	db         ProtocolStore
	workspaces []string
}

// New creates a Protocol with the given store and workspace roots.
func New(s ProtocolStore, workspaces []string) *Protocol {
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
	if cached, hit, err := p.db.GetReport(ctx, path, sha); err == nil && hit {
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
		return nil, fmt.Errorf("%w: %w", ErrScanFailed, err)
	}
	if sha != "" {
		_ = p.db.PutReport(ctx, path, sha, report)
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
		return nil, fmt.Errorf("%w: %w", ErrScanFailed, err)
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
	Cycles          []graph.Cycle          `json:"cycles"`
	ImportDepth     graph.DepthMap         `json:"import_depth"`
	LayerViolations []graph.LayerViolation `json:"layer_violations,omitempty"`
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
		r.LayerViolations = graph.CheckLayerPurity(report.Architecture.Edges, layers)
	} else {
		r.LayerViolations = report.LayerViolations
	}
	return r, nil
}

// ViolationReport holds architecture violation detection results.
type ViolationReport struct {
	Layers     []string               `json:"layers"`
	Violations []graph.LayerViolation `json:"violations"`
	Cycles     []graph.Cycle          `json:"cycles,omitempty"`
	Summary    string                 `json:"summary"`
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

	violations := graph.CheckLayerPurity(report.Architecture.Edges, layers)

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
		depths = graph.ImportDepth(report.Architecture.Edges)
	}

	// Group components by depth.
	layerMap := make(map[int][]string)
	for i := range report.Architecture.Services {
		d := depths[report.Architecture.Services[i].Name]
		layerMap[d] = append(layerMap[d], report.Architecture.Services[i].Name)
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

func (p *Protocol) SetDesiredState(ctx context.Context, path string, ds *port.DesiredState) error {
	path = p.resolvePath(path)
	return p.db.PutDesiredState(ctx, path, ds)
}

func (p *Protocol) GetDesiredState(ctx context.Context, path string) (*port.DesiredState, error) {
	path = p.resolvePath(path)
	return p.db.GetDesiredState(ctx, path)
}

// AcceptViolation appends an accepted violation to the project's desired state
// and persists it. If no desired state exists yet, one is created.
func (p *Protocol) AcceptViolation(ctx context.Context, path string, av port.AcceptedViolation) error {
	path = p.resolvePath(path)
	ds, err := p.db.GetDesiredState(ctx, path)
	if err != nil {
		return err
	}
	if ds == nil {
		ds = &port.DesiredState{}
	}
	ds.Accepted = append(ds.Accepted, av)
	return p.db.PutDesiredState(ctx, path, ds)
}

// DriftReport holds architecture drift analysis results.
type DriftReport struct {
	HasDesiredState    bool                           `json:"has_desired_state"`
	LayerViolations    []graph.LayerViolation         `json:"layer_violations,omitempty"`
	BoundaryViolations []constraint.BoundaryViolation `json:"boundary_violations,omitempty"`
	BudgetViolations   []constraint.BudgetViolation   `json:"budget_violations,omitempty"`
	BoundaryBreaches   int                            `json:"boundary_breaches"`
	ConstraintBreaches int                            `json:"constraint_breaches"`
	Score              port.Score                     `json:"score"`
	Clean              bool                           `json:"clean"`
	Summary            string                         `json:"summary"`
}

func (p *Protocol) GetDrift(ctx context.Context, path string, cacheKey ...string) (*DriftReport, error) {
	path = p.resolvePath(path)
	ds, err := p.db.GetDesiredState(ctx, path)
	if err != nil {
		return nil, err
	}
	report, err := p.getOrScan(path, cacheKey...)
	if err != nil {
		if ds == nil {
			return &DriftReport{HasDesiredState: false, Summary: "No desired state configured and scan unavailable."}, nil
		}
		return nil, err
	}

	// Auto-bootstrap: infer layers from the graph when no desired state exists.
	if ds == nil {
		layers := inferLayerOrder(report)
		ds = &port.DesiredState{Layers: layers}
	}

	// 1. Layer purity (existing).
	layerViolations := graph.CheckLayerPurity(report.Architecture.Edges, ds.Layers)

	// 2. Boundary rules (new).
	boundaryViolations := constraint.CheckBoundaryRules(report.Architecture.Edges, ds.Boundaries)

	// 3. Budget violations (new).
	var budgetViolations []constraint.BudgetViolation
	if len(ds.Constraints) > 0 {
		budgetReport := constraint.ComputeBudgetViolations(
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
		Score:              port.Score(score),
		Clean:              clean,
		Summary:            summary,
	}, nil
}

// countBudgetChecks counts the number of budget checks that will be performed
// for a given set of services and constraints (used for score calculation).
func countBudgetChecks(services []arch.ArchService, constraints []port.HealthConstraint) int {
	svcMap := make(map[string]bool, len(services))
	for i := range services {
		svcMap[services[i].Name] = true
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

func (p *Protocol) SuggestArchitecture(ctx context.Context, path string, cacheKey ...string) (*port.DesiredState, error) {
	path = p.resolvePath(path)
	report, err := p.getOrScan(path, cacheKey...)
	if err != nil {
		return nil, err
	}
	layers := inferLayerOrder(report)
	return &port.DesiredState{Layers: layers}, nil
}

// StatusResult holds workspace status information.
type StatusResult struct {
	Version    string             `json:"version"`
	Workspaces []string           `json:"workspaces"`
	Projects   []port.ProjectInfo `json:"projects,omitempty"`
}

func (p *Protocol) Status(ctx context.Context) (*StatusResult, error) {
	projects, _ := p.db.ListProjects(ctx)
	return &StatusResult{
		Workspaces: p.workspaces,
		Projects:   projects,
	}, nil
}

// FlushCache invalidates cached scan results for a project, forcing a fresh scan
// on the next request. If path is empty, flushes all projects.
func (p *Protocol) FlushCache(ctx context.Context, path string) (int, error) {
	if path != "" {
		path = p.resolvePath(path)
		return 1, p.db.Invalidate(ctx, path)
	}
	// Flush all projects.
	projects, err := p.db.ListProjects(ctx)
	if err != nil {
		return 0, err
	}
	for _, proj := range projects {
		_ = p.db.Invalidate(ctx, proj.Path)
	}
	return len(projects), nil
}

// CheckDriftOnScan checks desired state against a scan report and returns a one-liner.
// Returns empty string if no desired state exists.
func (p *Protocol) CheckDriftOnScan(ctx context.Context, path string, report *arch.ContextReport) string {
	path = p.resolvePath(path)
	ds, err := p.db.GetDesiredState(ctx, path)
	if err != nil || ds == nil || len(ds.Layers) == 0 {
		return ""
	}
	violations := graph.CheckLayerPurity(report.Architecture.Edges, ds.Layers)
	if len(violations) == 0 {
		return "Architecture: clean"
	}
	return fmt.Sprintf("Architecture: %d violation(s)", len(violations))
}

// generateComponentMeta creates metadata for all components in a scan report.
func generateComponentMeta(report *arch.ContextReport) []port.ComponentMeta {
	depths := graph.ImportDepth(report.Architecture.Edges)
	fanIn := graph.FanIn(report.Architecture.Edges)

	meta := make([]port.ComponentMeta, 0, len(report.Architecture.Services))
	for i := range report.Architecture.Services {
		s := &report.Architecture.Services[i]
		role := inferRole(s.Name)
		keywords := extractKeywords(*s)
		health := "healthy"
		if fanIn[s.Name] >= arch.MinFanInHotSpot && s.Churn >= arch.MinChurnHotSpot {
			health = "sick"
		}
		meta = append(meta, port.ComponentMeta{
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
		lower := strings.ToLower(sym.Name)
		if !seen[lower] {
			seen[lower] = true
			keywords = append(keywords, lower)
		}
	}
	return keywords
}

// SearchComponents queries component metadata by keywords.
func (p *Protocol) SearchComponents(ctx context.Context, path, query string, cacheKey ...string) ([]port.ComponentMeta, error) {
	path = p.resolvePath(path)
	sha := p.db.ResolveHEAD(path)
	return p.db.SearchComponents(ctx, path, sha, query)
}

// SymbolMatch is a single symbol search result.
type SymbolMatch struct {
	Symbol    string `json:"symbol"`
	Kind      string `json:"kind"`
	Component string `json:"component"`
	File      string `json:"file,omitempty"`
	Line      int    `json:"line,omitempty"`
}

// SymbolSearchReport holds symbol search results.
type SymbolSearchReport struct {
	Query   string        `json:"query"`
	Matches []SymbolMatch `json:"matches"`
	Summary string        `json:"summary"`
}

func (p *Protocol) SearchSymbols(_ context.Context, path, pattern string, cacheKey ...string) (*SymbolSearchReport, error) {
	path = p.resolvePath(path)
	report, err := p.getOrScan(path, cacheKey...)
	if err != nil {
		return nil, err
	}

	lower := strings.ToLower(pattern)
	var matches []SymbolMatch
	for i := range report.Architecture.Services {
		svc := &report.Architecture.Services[i]
		for _, sym := range svc.Symbols {
			if strings.Contains(strings.ToLower(sym.Name), lower) {
				matches = append(matches, SymbolMatch{
					Symbol:    sym.Name,
					Kind:      sym.Kind.String(),
					Component: svc.Name,
					File:      sym.File,
					Line:      sym.Line,
				})
			}
		}
	}

	summary := fmt.Sprintf("%d symbol(s) matching %q", len(matches), pattern)
	return &SymbolSearchReport{Query: pattern, Matches: matches, Summary: summary}, nil
}

// CallerSite is a type alias for port.CallerSite, kept for backward compatibility.
type CallerSite = port.CallerSite

// CallersReport holds all call sites for a given symbol.
type CallersReport struct {
	Symbol  string       `json:"symbol"`
	Callers []CallerSite `json:"callers"`
	Summary string       `json:"summary"`
}

// CalleesReport holds all functions called by a given symbol.
type CalleesReport struct {
	Symbol  string       `json:"symbol"`
	Callees []CallerSite `json:"callees"`
	Summary string       `json:"summary"`
}

func (p *Protocol) GetCallees(_ context.Context, path, symbol string, cacheKey ...string) (*CalleesReport, error) {
	path = p.resolvePath(path)
	if symbol == "" {
		return nil, ErrComponentRequired
	}
	da := analysis.CachedDeepFallback(path)
	cg, err := da.CallGraph(path, analysis.CallGraphOpts{Depth: analysis.DefaultCallGraphDepth})
	if err != nil {
		return nil, fmt.Errorf("call graph: %w", err)
	}

	var callees []CallerSite
	for _, e := range cg.Edges {
		if e.Caller == symbol {
			callees = append(callees, CallerSite{
				Caller:       e.Callee,
				CallerPkg:    e.CalleePkg,
				Line:         e.Line,
				File:         e.File,
				ReceiverType: e.ReceiverType,
			})
		}
	}

	return &CalleesReport{
		Symbol:  symbol,
		Callees: callees,
		Summary: fmt.Sprintf("%s calls %d function(s)", symbol, len(callees)),
	}, nil
}

// CallPathReport holds the shortest call chain between two symbols.
type CallPathReport struct {
	From    string   `json:"from"`
	To      string   `json:"to"`
	Path    []string `json:"path,omitempty"`
	Found   bool     `json:"found"`
	Summary string   `json:"summary"`
}

func (p *Protocol) GetCallPath(_ context.Context, path, from, to string, cacheKey ...string) (*CallPathReport, error) {
	path = p.resolvePath(path)
	da := analysis.CachedDeepFallback(path)
	cg, err := da.CallGraph(path, analysis.CallGraphOpts{Depth: analysis.DefaultCallGraphDepth})
	if err != nil {
		return nil, fmt.Errorf("call graph: %w", err)
	}

	// Build edges compatible with graph.ShortestPath.
	edges := make([]callEdge, len(cg.Edges))
	for i, e := range cg.Edges {
		edges[i] = callEdge{e.Caller, e.Callee}
	}

	result, found := graph.ShortestPath(edges, from, to)

	summary := fmt.Sprintf("no path from %s to %s", from, to)
	if found {
		summary = fmt.Sprintf("%s → %s: %d hops", from, to, len(result)-1)
	}

	return &CallPathReport{From: from, To: to, Path: result, Found: found, Summary: summary}, nil
}

// callEdge satisfies graph.Edge for call graph path queries.
type callEdge struct{ caller, callee string }

func (e callEdge) Source() string { return e.caller }
func (e callEdge) Target() string { return e.callee }

func (p *Protocol) GetInterfaceMetrics(ctx context.Context, path string, cacheKey ...string) (*constraint.InterfaceMetricsReport, error) {
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
	return constraint.ComputeInterfaceMetrics(classes, impls), nil
}

func (p *Protocol) GetSymbolBlastRadius(ctx context.Context, path, symbol string, cacheKey ...string) (*impact.SymbolBlastReport, error) {
	path = p.resolvePath(path)
	if symbol == "" {
		return nil, ErrComponentRequired
	}
	da := analysis.CachedDeepFallback(path)
	cg, err := da.CallGraph(path, analysis.CallGraphOpts{Depth: analysis.DefaultCallGraphDepth})
	if err != nil {
		return nil, fmt.Errorf("call graph: %w", err)
	}
	// Count unique packages from nodes for totalPkgs.
	pkgs := make(map[string]bool)
	for _, n := range cg.Nodes {
		pkgs[n.Package] = true
	}
	return impact.ComputeSymbolBlastRadius(cg.Edges, symbol, len(pkgs)), nil
}

func (p *Protocol) GetDiffIntelligence(ctx context.Context, path, since string, cacheKey ...string) (*DiffIntelligenceReport, error) {
	path = p.resolvePath(path)
	if since == "" {
		since = "HEAD~1"
	}
	report, err := p.getOrScan(path, cacheKey...)
	if err != nil {
		return nil, err
	}
	// Get changed files.
	changedFiles, err := changedFilesSince(path, since)
	if err != nil {
		return nil, fmt.Errorf("git diff: %w", err)
	}
	// Build call graph for symbol-level analysis.
	da := analysis.CachedDeepFallback(path)
	cg, err := da.CallGraph(path, analysis.CallGraphOpts{Depth: analysis.DefaultCallGraphDepth})
	if err != nil {
		return nil, fmt.Errorf("call graph: %w", err)
	}
	return ComputeDiffIntelligence(changedFiles, report.ModulePath, cg), nil
}

func (p *Protocol) GetCallers(ctx context.Context, path, symbol string, cacheKey ...string) (*CallersReport, error) {
	path = p.resolvePath(path)
	if symbol == "" {
		return nil, ErrComponentRequired
	}

	da := analysis.CachedDeepFallback(path)
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
	Overlap   []string `json:"overlap"`
	OnlyInA   []string `json:"only_in_a"`
	OnlyInB   []string `json:"only_in_b"`
	NewCycles int      `json:"new_cycles_if_merged"`
	Summary   string   `json:"summary"`
}

func (p *Protocol) GetCrossRepo(ctx context.Context, pathA, pathB, cacheKeyA, cacheKeyB string) (*CrossRepoReport, error) {
	reportA, err := p.getOrScan(p.resolvePath(pathA), cacheKeyA)
	if err != nil {
		return nil, fmt.Errorf("repo A: %w", err)
	}
	reportB, err := p.getOrScan(p.resolvePath(pathB), cacheKeyB)
	if err != nil {
		return nil, fmt.Errorf("repo B: %w", err)
	}

	setA := make(map[string]bool)
	for i := range reportA.Architecture.Services {
		setA[reportA.Architecture.Services[i].Name] = true
	}
	setB := make(map[string]bool)
	for i := range reportB.Architecture.Services {
		setB[reportB.Architecture.Services[i].Name] = true
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
	allEdges := make([]arch.ArchEdge, 0, len(reportA.Architecture.Edges)+len(reportB.Architecture.Edges))
	allEdges = append(allEdges, reportA.Architecture.Edges...)
	allEdges = append(allEdges, reportB.Architecture.Edges...)
	mergedCycles := graph.DetectCycles(allEdges)
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
	PresetArchReview  = "architecture_review"
	PresetHealthCheck = "health_check"
	PresetOnboarding  = "onboarding"
	PresetPrePR       = "pre_pr"
	PresetNormative   = "normative"
	PresetPreRefactor = "pre_refactor"
	PresetFullClinic  = "full_clinic"
	PresetCodeHealth  = "code_health"
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
		renderPresetArchReview(&b, report)
	case PresetHealthCheck:
		renderPresetHealthCheck(&b, report)
	case PresetOnboarding:
		renderPresetOnboarding(&b, report)
	case PresetPrePR:
		renderPresetPrePR(&b, report)
	case PresetNormative:
		p.renderPresetNormative(ctx, &b, path, report)
	case PresetPreRefactor:
		renderPresetPreRefactor(&b, path, report)
	case PresetFullClinic:
		p.renderPresetFullClinic(ctx, &b, path, report)
	case PresetCodeHealth:
		p.renderPresetCodeHealth(&b, path, report)
	default:
		return "", fmt.Errorf("%w %q (valid: %s, %s, %s, %s, %s, %s, %s, %s)",
			ErrUnknownPreset, preset, PresetArchReview, PresetHealthCheck, PresetOnboarding, PresetPrePR,
			PresetNormative, PresetPreRefactor, PresetFullClinic, PresetCodeHealth)
	}
	return b.String(), nil
}

func renderPresetArchReview(b *strings.Builder, report *arch.ContextReport) {
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

func renderPresetHealthCheck(b *strings.Builder, report *arch.ContextReport) {
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

func renderPresetOnboarding(b *strings.Builder, report *arch.ContextReport) {
	fmt.Fprintf(b, "# Onboarding: %s\n\n", report.ModulePath)
	fmt.Fprintf(b, "%d components, scanner=%s\n\n", len(report.Architecture.Services), report.Scanner)
	b.WriteString("## Top Components\n")
	n := min(len(report.Architecture.Services), 10)
	for i := range report.Architecture.Services[:n] {
		fmt.Fprintf(b, "- %s (%d LOC)\n", report.Architecture.Services[i].Name, report.Architecture.Services[i].LOC)
	}
}

func renderPresetPrePR(b *strings.Builder, report *arch.ContextReport) {
	fmt.Fprintf(b, "# Pre-PR Review: %s\n\n", report.ModulePath)
	fmt.Fprintf(b, "%d components, %d cycles, %d violations\n",
		len(report.Architecture.Services), len(report.Cycles), len(report.LayerViolations))
}

func (p *Protocol) renderPresetNormative(ctx context.Context, b *strings.Builder, path string, report *arch.ContextReport) {
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

	tbReport := constraint.ComputeTrustBoundaries(report.Architecture.Services, report.Architecture.Edges, desiredRolesForTrust(ctx, p, path))
	fmt.Fprintf(b, "\n## Trust Boundaries\n%s\n", tbReport.Summary)

	if desired, _ := p.db.GetDesiredState(ctx, path); desired != nil && len(desired.Constraints) > 0 {
		budgetReport := constraint.ComputeBudgetViolations(report.Architecture.Services, report.Architecture.Edges, desired.Constraints)
		fmt.Fprintf(b, "\n## Budgets\n%s\n", budgetReport.Summary)
	}
}

func renderPresetPreRefactor(b *strings.Builder, path string, report *arch.ContextReport) {
	fmt.Fprintf(b, "# Pre-Refactor Analysis: %s\n\n", report.ModulePath)

	spots := report.HotSpots
	if len(spots) > 10 {
		spots = spots[:10]
	}
	b.WriteString("## Hot Spots (churn × coupling)\n")
	for _, s := range spots {
		fmt.Fprintf(b, "- %s (churn:%d, fan-in:%d)\n", s.Component, s.Churn, s.FanIn)
	}

	da := analysis.NewFallback(path)
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

func (p *Protocol) renderPresetFullClinic(ctx context.Context, b *strings.Builder, path string, report *arch.ContextReport) {
	fmt.Fprintf(b, "# Full Clinic: %s\n\n", report.ModulePath)
	fmt.Fprintf(b, "%d components, %d edges\n\n", len(report.Architecture.Services), len(report.Architecture.Edges))

	fmt.Fprintf(b, "## Architecture\n")
	fmt.Fprintf(b, "- Cycles: %d\n", len(report.Cycles))
	fmt.Fprintf(b, "- Layer violations: %d\n", len(report.LayerViolations))

	idReport := constraint.ComputeImportDirection(report.Architecture.Edges, report.ImportDepth)
	fmt.Fprintf(b, "- Import direction: %s\n", idReport.Summary)

	tbReport := constraint.ComputeTrustBoundaries(report.Architecture.Services, report.Architecture.Edges, desiredRolesForTrust(ctx, p, path))
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

	da := analysis.NewFallback(path)
	if classes, err := da.Classes(path); err == nil {
		impls, _ := da.Implements(path)
		imReport := constraint.ComputeInterfaceMetrics(classes, impls)
		fmt.Fprintf(b, "\n## Interfaces\n%s\n", imReport.Summary)
	}

	if desired, _ := p.db.GetDesiredState(ctx, path); desired != nil && len(desired.Constraints) > 0 {
		budgetReport := constraint.ComputeBudgetViolations(report.Architecture.Services, report.Architecture.Edges, desired.Constraints)
		fmt.Fprintf(b, "\n## Budgets\n%s\n", budgetReport.Summary)
	}

	// Code Health Clinic pillars
	fa := analysis.NewFallback(path)
	if classes, err := fa.Classes(path); err == nil {
		impls, _ := fa.Implements(path)

		hexaClass := clinic.ComputeHexaClassification(report.Architecture.Services, report.Architecture.Edges, classes)
		desired, _ := p.db.GetDesiredState(ctx, path)
		roles, accepted := resolveRolesAndAccepted(hexaClass, desired)

		patternReport := clinic.ComputePatternScan(report.Architecture.Services, report.Architecture.Edges, report.Cycles, classes, impls, roles, accepted)
		fmt.Fprintf(b, "\n## Patterns & Smells\n%s\n", patternReport.Summary)

		hexaReport := clinic.ComputeHexaViolations(report.Architecture.Services, report.Architecture.Edges, classes)
		fmt.Fprintf(b, "\n## Hexagonal Architecture\n%s\n", hexaReport.Summary)

		solidReport := clinic.ComputeSOLIDScan(report.Architecture.Services, report.Architecture.Edges, classes, impls, hexaClass, path, roles, accepted)
		fmt.Fprintf(b, "\n## SOLID Principles\n%s\n", solidReport.Summary)
	}

	sqReport := clinic.ComputeSymbolQuality(report.Architecture.Services, report.Architecture.Edges)
	fmt.Fprintf(b, "\n## Symbol Quality\n%s\n", sqReport.Summary)
}

func (p *Protocol) renderPresetCodeHealth(b *strings.Builder, path string, report *arch.ContextReport) {
	fmt.Fprintf(b, "# Code Health Clinic: %s\n\n", report.ModulePath)
	fmt.Fprintf(b, "%d components, %d edges\n\n", len(report.Architecture.Services), len(report.Architecture.Edges))

	fa := analysis.NewFallback(path)
	classes, _ := fa.Classes(path)
	impls, _ := fa.Implements(path)

	hexaClass := clinic.ComputeHexaClassification(report.Architecture.Services, report.Architecture.Edges, classes)

	patternReport := clinic.ComputePatternScan(report.Architecture.Services, report.Architecture.Edges, report.Cycles, classes, impls, nil, nil)
	fmt.Fprintf(b, "## Patterns & Smells\n%s\n\n", patternReport.Summary)

	hexaReport := clinic.ComputeHexaViolations(report.Architecture.Services, report.Architecture.Edges, classes)
	fmt.Fprintf(b, "## Hexagonal Architecture\n%s\n\n", hexaReport.Summary)

	solidReport := clinic.ComputeSOLIDScan(report.Architecture.Services, report.Architecture.Edges, classes, impls, hexaClass, path, nil, nil)
	fmt.Fprintf(b, "## SOLID Principles\n%s\n\n", solidReport.Summary)

	sqReport := clinic.ComputeSymbolQuality(report.Architecture.Services, report.Architecture.Edges)
	fmt.Fprintf(b, "## Symbol Quality\n%s\n", sqReport.Summary)
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
		return nil, fmt.Errorf("%w: %q", ErrComponentNotFound, name)
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

	symNames := make([]string, 0, min(len(svc.Symbols), 20))
	for _, s := range svc.Symbols {
		if len(symNames) >= 20 {
			break
		}
		symNames = append(symNames, s.Name)
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
		Name: name, LOC: svc.LOC, Symbols: symNames,
		Imports: imports, Importers: importers,
		Churn: svc.Churn, Health: health,
	}, nil
}

// --- Natural language query ---

// QueryResult holds the answer to a natural language architecture question.
type QueryResult struct {
	Query  string `json:"query"`
	Action string `json:"resolved_action"`
	Answer any    `json:"answer"`
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

	before, hit, err := p.db.GetReport(ctx, path, beforeSHA)
	if err != nil || !hit {
		return nil, fmt.Errorf("%w: SHA %s", ErrNoCachedScan, beforeSHA)
	}
	after, hit, err := p.db.GetReport(ctx, path, afterSHA)
	if err != nil || !hit {
		return nil, fmt.Errorf("%w: SHA %s", ErrNoCachedScan, afterSHA)
	}

	return diffReports(beforeSHA, afterSHA, before, after), nil
}

func diffReports(beforeSHA, afterSHA string, before, after *arch.ContextReport) *ScanDiffReport {
	beforeSet := make(map[string]bool)
	for i := range before.Architecture.Services {
		beforeSet[before.Architecture.Services[i].Name] = true
	}
	afterSet := make(map[string]bool)
	for i := range after.Architecture.Services {
		afterSet[after.Architecture.Services[i].Name] = true
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

	addedEdges, removedEdges := diffEdges(before.Architecture.Edges, after.Architecture.Edges)

	locBefore := totalLOC(before)
	locAfter := totalLOC(after)

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
	}
}

func diffEdges(beforeEdges, afterEdges []arch.ArchEdge) (added, removed int) {
	beforeSet := make(map[[2]string]bool)
	for _, e := range beforeEdges {
		beforeSet[[2]string{e.From, e.To}] = true
	}
	afterSet := make(map[[2]string]bool)
	for _, e := range afterEdges {
		afterSet[[2]string{e.From, e.To}] = true
	}
	for e := range afterSet {
		if !beforeSet[e] {
			added++
		}
	}
	for e := range beforeSet {
		if !afterSet[e] {
			removed++
		}
	}
	return added, removed
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
	// Convert port.HistoryEntry to history.EntrySummary for backward compat.
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

func (p *Protocol) GetImpact(ctx context.Context, path, component string, cacheKey ...string) (*impact.ImpactResult, error) {
	path = p.resolvePath(path)
	if component == "" {
		return nil, ErrComponentRequired
	}
	report, err := p.getOrScan(path, cacheKey...)
	if err != nil {
		return nil, err
	}
	return impact.ComputeImpact(
		report.Architecture.Edges,
		report.Architecture.Services,
		component,
	)
}

func (p *Protocol) GetWhatIf(_ context.Context, path string, moves []impact.FileMove, cacheKey ...string) (*impact.GraphDelta, error) {
	path = p.resolvePath(path)
	report, err := p.getOrScan(path, cacheKey...)
	if err != nil {
		return nil, err
	}
	return impact.ComputeWhatIf(
		report.Architecture.Services,
		report.Architecture.Edges,
		report.Cycles,
		moves,
	)
}

func (p *Protocol) GetGaps(ctx context.Context, path string) (*constraint.GapReport, error) {
	path = p.resolvePath(path)
	report, err := p.getOrScan(path)
	if err != nil {
		return nil, err
	}
	return constraint.DetectGaps(report, path)
}

func (p *Protocol) GetBloaterScan(_ context.Context, path string, cacheKey ...string) (*clinic.BloaterReport, error) {
	path = p.resolvePath(path)
	report, err := p.getOrScan(path, cacheKey...)
	if err != nil {
		return nil, err
	}
	return clinic.ComputeBloaterScan(report), nil
}

func (p *Protocol) GetLeverage(_ context.Context, path, target string, cacheKey ...string) (*impact.LeverageReport, error) {
	path = p.resolvePath(path)
	if target == "" {
		return nil, ErrComponentRequired
	}
	report, err := p.getOrScan(path, cacheKey...)
	if err != nil {
		return nil, err
	}
	return impact.ComputeLeverage(
		report.Architecture.Edges,
		report.Architecture.Services,
		target,
	)
}

func (p *Protocol) GetRiskScores(_ context.Context, path string, cacheKey ...string) (*impact.RiskReport, error) {
	path = p.resolvePath(path)
	report, err := p.getOrScan(path, cacheKey...)
	if err != nil {
		return nil, err
	}
	return impact.ComputeRiskScores(
		report.Architecture.Services,
		report.Architecture.Edges,
		report.Coverage,
	), nil
}

func (p *Protocol) GetConsolidation(_ context.Context, path string, cacheKey ...string) (*impact.ConsolidationReport, error) {
	path = p.resolvePath(path)
	report, err := p.getOrScan(path, cacheKey...)
	if err != nil {
		return nil, err
	}
	return impact.ComputeConsolidation(
		report.Architecture.Services,
		report.Architecture.Edges,
	), nil
}

func (p *Protocol) GetBudgets(ctx context.Context, path string, cacheKey ...string) (*constraint.BudgetReport, error) {
	path = p.resolvePath(path)
	report, err := p.getOrScan(path, cacheKey...)
	if err != nil {
		return nil, err
	}
	desired, _ := p.db.GetDesiredState(ctx, path)
	if desired == nil || len(desired.Constraints) == 0 {
		return &constraint.BudgetReport{Summary: "no budgets defined"}, nil
	}
	return constraint.ComputeBudgetViolations(report.Architecture.Services, report.Architecture.Edges, desired.Constraints), nil
}

func (p *Protocol) GetBlastRadius(ctx context.Context, path string, files []string, since string, cacheKey ...string) (*impact.BlastRadiusReport, error) {
	path = p.resolvePath(path)
	report, err := p.getOrScan(path, cacheKey...)
	if err != nil {
		return nil, err
	}
	return impact.ComputeBlastRadius(
		report.Architecture.Edges,
		report.Architecture.Services,
		report.ModulePath,
		path,
		files,
		since,
	)
}

func (p *Protocol) GetImportDirection(ctx context.Context, path string, cacheKey ...string) (*constraint.ImportDirectionReport, error) {
	path = p.resolvePath(path)
	report, err := p.getOrScan(path, cacheKey...)
	if err != nil {
		return nil, err
	}
	return constraint.ComputeImportDirection(report.Architecture.Edges, report.ImportDepth), nil
}

func (p *Protocol) GetModuleDependencies(_ context.Context, path string, _ ...string) (*DependencyReport, error) {
	path = p.resolvePath(path)
	goModPath := filepath.Join(path, "go.mod")
	return ComputeDependencies(goModPath)
}

func (p *Protocol) GetTrustBoundaries(ctx context.Context, path string, cacheKey ...string) (*constraint.TrustBoundaryReport, error) {
	path = p.resolvePath(path)
	report, err := p.getOrScan(path, cacheKey...)
	if err != nil {
		return nil, err
	}
	return constraint.ComputeTrustBoundaries(report.Architecture.Services, report.Architecture.Edges, desiredRolesForTrust(ctx, p, path)), nil
}

// --- Code Health Clinic methods ---

func (p *Protocol) GetHexaValidation(ctx context.Context, path string, cacheKey ...string) (*clinic.HexaValidationReport, error) {
	path = p.resolvePath(path)
	report, err := p.getOrScan(path, cacheKey...)
	if err != nil {
		return nil, err
	}
	fa := analysis.NewFallback(path)
	classes, _ := fa.Classes(path)
	return clinic.ComputeHexaViolations(report.Architecture.Services, report.Architecture.Edges, classes), nil
}

func (p *Protocol) GetSOLIDScan(ctx context.Context, path string, cacheKey ...string) (*clinic.SOLIDReport, error) {
	path = p.resolvePath(path)
	report, err := p.getOrScan(path, cacheKey...)
	if err != nil {
		return nil, err
	}
	fa := analysis.NewFallback(path)
	classes, _ := fa.Classes(path)
	impls, _ := fa.Implements(path)
	hexaClass := clinic.ComputeHexaClassification(report.Architecture.Services, report.Architecture.Edges, classes)
	desired, _ := p.db.GetDesiredState(ctx, path)
	roles, accepted := resolveRolesAndAccepted(hexaClass, desired)
	return clinic.ComputeSOLIDScan(report.Architecture.Services, report.Architecture.Edges, classes, impls, hexaClass, path, roles, accepted), nil
}

func (p *Protocol) GetSymbolQuality(_ context.Context, path string, cacheKey ...string) (*clinic.SymbolQualityReport, error) {
	path = p.resolvePath(path)
	report, err := p.getOrScan(path, cacheKey...)
	if err != nil {
		return nil, err
	}
	return clinic.ComputeSymbolQuality(report.Architecture.Services, report.Architecture.Edges), nil
}

func (p *Protocol) GetVocabMap(_ context.Context, path string, cacheKey ...string) (*clinic.VocabMapReport, error) {
	path = p.resolvePath(path)
	report, err := p.getOrScan(path, cacheKey...)
	if err != nil {
		return nil, err
	}
	return clinic.ComputeVocabMap(report.Architecture.Services), nil
}

func (p *Protocol) GetPatternScan(ctx context.Context, path string, cacheKey ...string) (*clinic.PatternScanReport, error) {
	path = p.resolvePath(path)
	report, err := p.getOrScan(path, cacheKey...)
	if err != nil {
		return nil, err
	}
	fa := analysis.NewFallback(path)
	classes, _ := fa.Classes(path)
	impls, _ := fa.Implements(path)
	hexaClass := clinic.ComputeHexaClassification(report.Architecture.Services, report.Architecture.Edges, classes)
	desired, _ := p.db.GetDesiredState(ctx, path)
	roles, accepted := resolveRolesAndAccepted(hexaClass, desired)
	patternReport := clinic.ComputePatternScan(report.Architecture.Services, report.Architecture.Edges, report.Cycles, classes, impls, roles, accepted)

	// Enrich with call graph if available (Feature Envy move targets, God Component split suggestions).
	da := analysis.CachedDeepFallback(path)
	if cg, cgErr := da.CallGraph(path, analysis.CallGraphOpts{Depth: analysis.DefaultCallGraphDepth}); cgErr == nil && cg != nil {
		clinic.EnrichWithCallGraph(patternReport, cg.Edges)
	}

	return patternReport, nil
}

func (p *Protocol) GetPatternCatalog(filter string) *clinic.PatternCatalogReport {
	return clinic.GetPatternCatalog(filter)
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
	return nil, fmt.Errorf("%w: %q", ErrNoCachedReport, cacheKey)
}

// resolveRolesAndAccepted extracts roles and accepted violations from
// hexa classification and desired state. Either may be nil.
func resolveRolesAndAccepted(hexaClass *clinic.HexaClassificationReport, desired *port.DesiredState) (map[string]clinic.HexaRole, []port.AcceptedViolation) {
	var roles map[string]clinic.HexaRole
	var accepted []port.AcceptedViolation

	if hexaClass != nil {
		var overrides map[string]string
		if desired != nil {
			overrides = desired.Roles
		}
		roles = clinic.ResolveRoles(hexaClass, overrides)
	}
	if desired != nil {
		accepted = desired.Accepted
	}
	return roles, accepted
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
		return nil, fmt.Errorf("%w: %q", ErrNoCachedReport, ck)
	}

	sha := p.db.ResolveHEAD(path)
	if cached, hit, _ := p.db.GetReport(context.Background(), path, sha); hit {
		return cached, nil
	}
	r, err := arch.ScanAndBuild(path, arch.ScanOpts{ExcludeTests: true, ChurnDays: 30})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrScanFailed, err)
	}
	if sha != "" {
		_ = p.db.PutReport(context.Background(), path, sha, r)
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
			_ = checkoutRef(repoPath, currentBranch)
		}
	}()
	report, err := arch.ScanAndBuild(repoPath, arch.ScanOpts{ExcludeTests: true, ChurnDays: 30})
	if err != nil {
		return nil, err
	}
	_ = p.db.PutReport(context.Background(), repoPath, sha, report)
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

	// Health checks for stores that expose filesystem paths.
	if hc, ok := p.db.(HealthCheckable); ok {
		r.Checks = append(r.Checks,
			checkDir("cache_dir", hc.CacheRoot()),
			checkDir("history_dir", hc.HistoryDir()),
		)
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
	switch {
	case oldest != "":
		args = append(args, oldest+"^.."+newest)
	case limit > 0:
		args = append(args, "-n", strconv.Itoa(limit), newest)
	default:
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
	for i := range report.Architecture.Services {
		total += report.Architecture.Services[i].LOC
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
		return nil, ErrNoCommitsFound
	}

	commits = sampleCommits(commits, opts.Stride)

	currentBranch := getCurrentBranch(path)
	needsRestore := false
	defer func() {
		if needsRestore && currentBranch != "" {
			_ = checkoutRef(path, currentBranch)
		}
	}()

	steps := make([]EvolutionStep, 0, len(commits))
	var prevReport *arch.ContextReport

	for i, commit := range commits {
		report, cached, cacheErr := p.db.GetReport(ctx, path, commit.SHA)
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
			_ = p.db.PutReport(ctx, path, commit.SHA, report)
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

// changedFilesSince returns the list of changed files since a git ref.
func changedFilesSince(repoPath, since string) ([]string, error) {
	cmd := exec.Command("git", "diff", "--name-only", since)
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var files []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			files = append(files, line)
		}
	}
	return files, nil
}

// desiredRolesForTrust returns the desired-state roles map for trust boundary detection.
// Returns nil if no desired state is configured (falls back to heuristics).
func desiredRolesForTrust(ctx context.Context, p *Protocol, path string) map[string]string {
	desired, err := p.db.GetDesiredState(ctx, path)
	if err != nil || desired == nil {
		return nil
	}
	return desired.Roles
}
