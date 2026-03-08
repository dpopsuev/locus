package protocol

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dpopsuev/locus/internal/cache"
	"github.com/dpopsuev/locus/internal/history"
	"github.com/dpopsuev/locus/internal/remote"
	"github.com/dpopsuev/locus/internal/arch"
	"github.com/dpopsuev/locus/internal/cursor"
)

// Protocol encapsulates all Locus business logic.
// Both CLI and MCP are thin wrappers around this.
type Protocol struct {
	cache      *cache.ScanCache
	historyDir string
	workspaces []string
}

func New(sc *cache.ScanCache, historyDir string, workspaces []string) *Protocol {
	return &Protocol{cache: sc, historyDir: historyDir, workspaces: workspaces}
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
}

// RemoteOpts controls a remote codograph.
type RemoteOpts struct {
	Ref       string
	Keep      bool
	Depth     int
	ChurnDays int
	Budget    int
}

// BranchDiffResult wraps branch metadata with the diff.
type BranchDiffResult struct {
	BranchA string               `json:"branch_a"`
	BranchB string               `json:"branch_b"`
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

// --- Operations ---

func (p *Protocol) ScanProject(_ context.Context, path string, opts ScanOpts) (*arch.ContextReport, error) {
	path = p.resolvePath(path)
	churnDays := opts.ChurnDays
	if churnDays == 0 {
		churnDays = 30
	}

	sha := cache.ResolveHEAD(path)
	if cached, hit, err := p.cache.Get(path, sha); err == nil && hit {
		return cached, nil
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
	})
	if err != nil {
		return nil, fmt.Errorf("scan failed: %w", err)
	}
	if sha != "" {
		p.cache.Put(path, sha, report)
		abs, _ := filepath.Abs(path)
		_ = history.Record(p.cache, p.historyDir, history.Local, abs, sha, report)
	}
	return report, nil
}

func (p *Protocol) SuggestDepth(_ context.Context, path string) (*SuggestDepthResult, error) {
	path = p.resolvePath(path)
	report, err := arch.ScanAndBuild(path, arch.ScanOpts{ExcludeTests: true})
	if err != nil {
		return nil, fmt.Errorf("scan failed: %w", err)
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

func (p *Protocol) GetHotSpots(_ context.Context, path string, churnDays, topN int) ([]arch.HotSpot, error) {
	path = p.resolvePath(path)
	report, err := p.getOrScan(path)
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

func (p *Protocol) GetDependencies(_ context.Context, path, component string) (*DepResult, error) {
	path = p.resolvePath(path)
	if component == "" {
		return nil, fmt.Errorf("component is required")
	}
	report, err := p.getOrScan(path)
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

func (p *Protocol) GetCouplingTable(_ context.Context, path, sortBy string, topN int) (string, error) {
	path = p.resolvePath(path)
	report, err := p.getOrScan(path)
	if err != nil {
		return "", err
	}
	if sortBy == "" {
		sortBy = "fan_in"
	}
	return arch.RenderCouplingTable(report, sortBy, topN), nil
}

func (p *Protocol) GetEdgeList(_ context.Context, path, component string) (string, error) {
	path = p.resolvePath(path)
	report, err := p.getOrScan(path)
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

func (p *Protocol) GetCycles(_ context.Context, path string, layers []string) (*CycleReport, error) {
	path = p.resolvePath(path)
	report, err := p.getOrScan(path)
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

// CoverageReport holds per-component coverage data.
type CoverageReport struct {
	Coverage      []arch.CoverageResult `json:"coverage"`
	BelowThreshold []arch.CoverageResult `json:"below_threshold,omitempty"`
}

func (p *Protocol) GetCoverage(_ context.Context, path string, threshold float64) (*CoverageReport, error) {
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

func (p *Protocol) GetAPISurface(_ context.Context, path string, trusted []string) (*APISurfaceReport, error) {
	path = p.resolvePath(path)
	report, err := p.getOrScan(path)
	if err != nil {
		return nil, err
	}
	return &APISurfaceReport{
		Surfaces:  report.APISurfaces,
		Crossings: report.BoundaryCrossings,
	}, nil
}

func (p *Protocol) ValidateArchitecture(_ context.Context, path, desiredState, format string) (*arch.ArchDrift, error) {
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

func (p *Protocol) CodographRemote(ctx context.Context, url string, opts RemoteOpts) (*arch.ContextReport, error) {
	if url == "" {
		return nil, fmt.Errorf("url is required")
	}
	result, err := remote.Codograph(ctx, url, remote.Opts{
		Ref:       opts.Ref,
		Keep:      opts.Keep,
		Depth:     opts.Depth,
		ChurnDays: opts.ChurnDays,
		Budget:    opts.Budget,
	})
	if err != nil {
		return nil, fmt.Errorf("remote codography: %w", err)
	}
	cacheKey := remote.CacheKey(url, result.RefSHA)
	_ = p.cache.Put(cacheKey, result.RefSHA, result.Report)
	_ = history.Record(p.cache, p.historyDir, history.Remote, remote.NormalizeURL(url), result.RefSHA, result.Report)
	return result.Report, nil
}

func (p *Protocol) GetHistory(_ context.Context, path string, last int) ([]history.EntrySummary, error) {
	path = p.resolvePath(path)
	abs, _ := filepath.Abs(path)
	if last <= 0 {
		last = 10
	}
	return history.List(p.historyDir, abs, last)
}

func (p *Protocol) DiffCodographs(_ context.Context, path string) (*history.CodographDiff, error) {
	path = p.resolvePath(path)
	abs, _ := filepath.Abs(path)
	prev, err := history.GetReport(p.cache, p.historyDir, abs, -2)
	if err != nil {
		return nil, fmt.Errorf("get previous codograph: %w", err)
	}
	latest, err := history.GetReport(p.cache, p.historyDir, abs, -1)
	if err != nil {
		return nil, fmt.Errorf("get latest codograph: %w", err)
	}
	return history.DiffReports(prev, latest), nil
}

func (p *Protocol) DiffBranches(_ context.Context, path, branchA, branchB string) (*BranchDiffResult, error) {
	path = p.resolvePath(path)
	if branchA == "" || branchB == "" {
		return nil, fmt.Errorf("both branch_a and branch_b are required")
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

func (p *Protocol) GetRules(_ context.Context, path string) ([]cursor.Rule, error) {
	path = p.resolvePath(path)
	return cursor.ReadRules(path)
}

func (p *Protocol) GetSkills(_ context.Context, path string) ([]cursor.Skill, error) {
	path = p.resolvePath(path)
	return cursor.ReadSkills(path)
}

// --- helpers ---

func (p *Protocol) getOrScan(path string) (*arch.ContextReport, error) {
	sha := cache.ResolveHEAD(path)
	if cached, hit, _ := p.cache.Get(path, sha); hit {
		return cached, nil
	}
	r, err := arch.ScanAndBuild(path, arch.ScanOpts{ExcludeTests: true, ChurnDays: 30})
	if err != nil {
		return nil, fmt.Errorf("scan failed: %w", err)
	}
	if sha != "" {
		p.cache.Put(path, sha, r)
	}
	return r, nil
}

func (p *Protocol) scanBranch(repoPath, ref string) (*arch.ContextReport, error) {
	sha, err := cache.ResolveBranch(repoPath, ref)
	if err != nil {
		return nil, err
	}
	if cached, hit, _ := p.cache.Get(repoPath, sha); hit {
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
	p.cache.Put(repoPath, sha, report)
	return report, nil
}

func (p *Protocol) resolvePath(path string) string {
	if path != "" {
		return path
	}
	if len(p.workspaces) > 0 {
		return p.workspaces[0]
	}
	return "."
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

	r.Checks = append(r.Checks, checkDir("cache_dir", p.cache.Root()))
	r.Checks = append(r.Checks, checkDir("history_dir", p.historyDir))
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
