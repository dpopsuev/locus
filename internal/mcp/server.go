package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/dpopsuev/locus/internal/analysis"
	"github.com/dpopsuev/locus/internal/arch"
	"github.com/dpopsuev/locus/internal/cache"
	"github.com/dpopsuev/locus/internal/diagram"
	"github.com/dpopsuev/locus/internal/protocol"
	"github.com/dpopsuev/locus/internal/triage"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func NewServer(sc *cache.ScanCache, historyDir string, workspaceRoots []string) (*sdkmcp.Server, *triage.Registry) {
	pathMap := os.Getenv("LOCUS_PATH_MAP")
	proto := protocol.NewWithPathMapper(sc, historyDir, workspaceRoots, pathMap)
	srv := sdkmcp.NewServer(
		&sdkmcp.Implementation{Name: "locus", Version: "0.2.1"},
		&sdkmcp.ServerOptions{
			Instructions: "Locus is a spatial context bus for AI agents. " +
				"Point it at any repository to get architecture, dependency graph, churn, hot spots, and symbols. " +
				"Results are cached by git HEAD SHA. Use scan_project to analyze a repo, get_coupling_table with view=hot_spots for risk areas, " +
				"and codograph_remote for GitHub repos you don't have locally.",
		},
	)
	h := &handler{proto: proto}
	reg := triage.New()

	triage.AddTool(reg, srv, triage.ToolMeta{
		Name:        "scan_project",
		Description: "Scan a repository and return its full codebase context: architecture, dependency graph, churn, hot spots, and symbols. Results are cached by git HEAD SHA.",
		Keywords:    []string{"scan", "architecture", "overview", "structure", "codebase"},
		Categories:  []string{"architecture", "onboarding"},
		DefaultArgs: map[string]any{"format": "summary"},
		Rationale: map[string]string{
			"architecture": "Full codebase overview with dependency graph and metrics",
			"onboarding":   "Best first step for understanding an unfamiliar codebase",
		},
		Priority: 1,
	}, noOut(h.handleScanProject))

	triage.AddTool(reg, srv, triage.ToolMeta{
		Name:        "get_dependencies",
		Description: "Return fan-in and fan-out edges for a specific component in a repository.",
		Keywords:    []string{"depend", "import", "upstream", "downstream"},
		Categories:  []string{"dependencies", "refactoring"},
		Rationale: map[string]string{
			"dependencies": "Direct upstream/downstream view of a single component",
			"refactoring":  "Impact analysis for changes to a specific package",
		},
		Priority: 2,
	}, noOut(h.handleGetDependencies))

	triage.AddTool(reg, srv, triage.ToolMeta{
		Name:        "get_impact",
		Description: "Compute transitive blast radius for a component change. Returns direct/transitive dependents, blast radius %, and risk level.",
		Keywords:    []string{"impact", "blast", "radius", "change", "refactor", "dependent"},
		Categories:  []string{"refactoring", "dependencies"},
		Rationale: map[string]string{
			"refactoring":   "Assess risk before changing a component",
			"dependencies":  "See who is affected by a component change",
		},
		Priority: 2,
	}, noOut(h.handleGetImpact))

	triage.AddTool(reg, srv, triage.ToolMeta{
		Name:        "get_coupling_table",
		Description: "Return a pre-formatted coupling table: package name, fan-in, fan-out, churn, symbol count. Sorted and filtered so agents never need Python/jq glue. Use view=hot_spots for risk areas (high fan-in + churn), view=edges for dependency edge list.",
		Keywords:    []string{"coupling", "fan", "blast", "depend"},
		Categories:  []string{"performance", "architecture", "dependencies"},
		DefaultArgs: map[string]any{"sort_by": "fan_in"},
		Rationale: map[string]string{
			"performance":  "Most depended-on = highest blast radius",
			"architecture": "Quantified coupling metrics for design review",
			"dependencies": "Fan-in/fan-out table for dependency analysis",
		},
		Priority: 2,
	}, noOut(h.handleGetCouplingTable))

	triage.AddTool(reg, srv, triage.ToolMeta{
		Name:        "codograph_remote",
		Description: "Produce a codograph from a remote GitHub repository via shallow clone. Accepts a git URL (HTTPS, SSH, or shorthand like github.com/org/repo) and optional ref (branch/tag). Returns the same ContextReport as scan_project. Results are cached by URL+SHA.",
		Keywords:    []string{"remote", "github", "clone", "external"},
		Categories:  []string{"architecture", "onboarding"},
		Rationale: map[string]string{
			"architecture": "Analyze any GitHub repo without cloning it locally",
			"onboarding":   "Quick architecture view of an external dependency",
		},
		Priority: 2,
	}, noOut(h.handleCodographRemote))

	triage.AddTool(reg, srv, triage.ToolMeta{
		Name:        "get_codograph_history",
		Description: "List past codographs for a repository path. Returns timestamps, HEAD SHAs, source (local/remote), and component/edge counts. Use the 'last' parameter to limit results. Set diff=true to compare the latest two codographs.",
		Keywords:    []string{"history", "past", "trend", "evolution", "diff", "compare"},
		Categories:  []string{"churn", "history"},
		Rationale: map[string]string{
			"churn":   "Track architectural evolution over time",
			"history": "See past scans and compare growth",
		},
		Priority: 1,
	}, noOut(h.handleGetCodographHistory))

	triage.AddTool(reg, srv, triage.ToolMeta{
		Name:        "diff_branches",
		Description: "Compare architecture between two git branches. Scans each branch (cache-aware: previously scanned branches are instant hits) and returns the diff.",
		Keywords:    []string{"branch", "compare", "merge", "pr", "review"},
		Categories:  []string{"comparison", "review"},
		Rationale: map[string]string{
			"comparison": "Structural diff between git branches",
			"review":     "Assess architectural impact of a PR or merge",
		},
		Priority: 1,
	}, noOut(h.handleDiffBranches))

	triage.AddTool(reg, srv, triage.ToolMeta{
		Name:        "get_cycles",
		Description: "Detect circular dependencies, compute import depth per component, and check layer purity. Use analysis=coverage for test coverage, analysis=api_surface for exported symbols, analysis=conventions for coding patterns, analysis=gaps for undocumented/under-tested components.",
		Keywords:    []string{"cycle", "circular", "loop", "deadlock"},
		Categories:  []string{"performance", "architecture", "dependencies"},
		Rationale: map[string]string{
			"performance":  "Circular deps can cause initialization deadlocks",
			"architecture": "Cycles violate clean layering principles",
			"dependencies": "Circular deps cause build and deploy issues",
		},
		Priority: 1,
	}, noOut(h.handleGetCycles))

	triage.AddTool(reg, srv, triage.ToolMeta{
		Name:        "render_diagram",
		Description: "Render a Mermaid diagram from repository structure. Types: dependency (flowchart), c4 (C4 Component), coupling (Sankey), churn (XY chart), layers (block), tree (mindmap), classes (classDiagram), sequence (sequenceDiagram), er (erDiagram). Returns valid Mermaid text. Use theme param (light/dark/natural) for themed output.",
		Keywords:    []string{"diagram", "visual", "mermaid", "chart", "graph", "class", "sequence", "er", "entity", "hierarchy", "call"},
		Categories:  []string{"architecture", "visualization"},
		Rationale: map[string]string{
			"architecture":  "Visual dependency, type hierarchy, call chain, and entity diagrams",
			"visualization": "Generate Mermaid charts from live architecture data",
		},
		Priority: 1,
	}, noOut(h.handleRenderDiagram))

	// --- triage tool (self-registering) ---

	h.reg = reg
	triage.AddTool(reg, srv, triage.ToolMeta{
		Name:        "triage",
		Description: "Map a natural language intent to a ranked list of Locus tools. No LLM — pure keyword matching.",
		Keywords:    []string{"help", "what", "which", "recommend", "suggest", "guide"},
		Categories:  []string{"meta"},
		Rationale: map[string]string{
			"meta": "Discover relevant tools for a given intent",
		},
		Priority: 1,
	}, noOut(h.handleTriage))

	return srv, reg
}

type handler struct {
	proto *protocol.Protocol
	reg   *triage.Registry
}

// --- handlers ---

type scanProjectInput struct {
	Path            string `json:"path"`
	Depth           int    `json:"depth,omitempty"`
	ChurnDays       int    `json:"churn_days,omitempty"`
	IncludeExternal bool   `json:"include_external,omitempty"`
	IncludeTests    bool   `json:"include_tests,omitempty"`
	IncludeCoverage bool   `json:"include_coverage,omitempty"`
	Budget          int    `json:"budget,omitempty"`
	Format          string `json:"format,omitempty"`
}

func (h *handler) handleScanProject(ctx context.Context, _ *sdkmcp.CallToolRequest, in scanProjectInput) (*sdkmcp.CallToolResult, any, error) {
	report, err := h.proto.ScanProject(ctx, in.Path, protocol.ScanOpts{
		Depth: in.Depth, ChurnDays: in.ChurnDays,
		IncludeExternal: in.IncludeExternal, IncludeTests: in.IncludeTests,
		IncludeCoverage: in.IncludeCoverage, Budget: in.Budget,
	})
	if err != nil {
		return nil, nil, err
	}
	if in.Format == "summary" {
		return text(arch.RenderMarkdown(report)), nil, nil
	}
	data, err := arch.RenderJSON(report)
	if err != nil {
		return nil, nil, fmt.Errorf("render JSON: %w", err)
	}
	return text(string(data)), nil, nil
}

type pathInput struct {
	Path string `json:"path"`
}

func (h *handler) handleSuggestDepth(ctx context.Context, _ *sdkmcp.CallToolRequest, in pathInput) (*sdkmcp.CallToolResult, any, error) {
	r, err := h.proto.SuggestDepth(ctx, in.Path)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(r)
}

type hotSpotsInput struct {
	Path      string `json:"path"`
	ChurnDays int    `json:"churn_days,omitempty"`
	TopN      int    `json:"top_n,omitempty"`
}

func (h *handler) handleGetHotSpots(ctx context.Context, _ *sdkmcp.CallToolRequest, in hotSpotsInput) (*sdkmcp.CallToolResult, any, error) {
	spots, err := h.proto.GetHotSpots(ctx, in.Path, in.ChurnDays, in.TopN)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(spots)
}

type depsInput struct {
	Path      string `json:"path"`
	Component string `json:"component"`
}

func (h *handler) handleGetDependencies(ctx context.Context, _ *sdkmcp.CallToolRequest, in depsInput) (*sdkmcp.CallToolResult, any, error) {
	r, err := h.proto.GetDependencies(ctx, in.Path, in.Component)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(r)
}

func (h *handler) handleGetConventions(ctx context.Context, _ *sdkmcp.CallToolRequest, in pathInput) (*sdkmcp.CallToolResult, any, error) {
	r, err := h.proto.GetConventions(ctx, in.Path)
	if err != nil {
		return nil, nil, fmt.Errorf("detect conventions: %w", err)
	}
	return jsonResult(r)
}

type impactInput struct {
	Path      string `json:"path"`
	Component string `json:"component"`
}

func (h *handler) handleGetImpact(ctx context.Context, _ *sdkmcp.CallToolRequest, in impactInput) (*sdkmcp.CallToolResult, any, error) {
	r, err := h.proto.GetImpact(ctx, in.Path, in.Component)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(r)
}

func (h *handler) handleGetKnowledgeGaps(ctx context.Context, _ *sdkmcp.CallToolRequest, in pathInput) (*sdkmcp.CallToolResult, any, error) {
	r, err := h.proto.GetGaps(ctx, in.Path)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(r)
}

type couplingInput struct {
	Path      string `json:"path"`
	SortBy    string `json:"sort_by,omitempty"`
	TopN      int    `json:"top_n,omitempty"`
	View      string `json:"view,omitempty"`       // coupling|hot_spots|edges (default: coupling)
	ChurnDays int    `json:"churn_days,omitempty"` // for hot_spots view
	Component string `json:"component,omitempty"`  // for edges view
}

func (h *handler) handleGetCouplingTable(ctx context.Context, _ *sdkmcp.CallToolRequest, in couplingInput) (*sdkmcp.CallToolResult, any, error) {
	path := in.Path
	switch in.View {
	case "hot_spots":
		spots, err := h.proto.GetHotSpots(ctx, path, in.ChurnDays, in.TopN)
		if err != nil {
			return nil, nil, err
		}
		data, _ := json.MarshalIndent(spots, "", "  ")
		return text(string(data)), nil, nil
	case "edges":
		result, err := h.proto.GetEdgeList(ctx, path, in.Component)
		if err != nil {
			return nil, nil, err
		}
		return text(result), nil, nil
	default:
		result, err := h.proto.GetCouplingTable(ctx, path, in.SortBy, in.TopN)
		if err != nil {
			return nil, nil, err
		}
		return text(result), nil, nil
	}
}

type edgeListInput struct {
	Path      string `json:"path"`
	Component string `json:"component,omitempty"`
}

func (h *handler) handleGetEdgeList(ctx context.Context, _ *sdkmcp.CallToolRequest, in edgeListInput) (*sdkmcp.CallToolResult, any, error) {
	md, err := h.proto.GetEdgeList(ctx, in.Path, in.Component)
	if err != nil {
		return nil, nil, err
	}
	return text(md), nil, nil
}

type remoteInput struct {
	URL       string `json:"url"`
	Ref       string `json:"ref,omitempty"`
	Depth     int    `json:"depth,omitempty"`
	ChurnDays int    `json:"churn_days,omitempty"`
	Budget    int    `json:"budget,omitempty"`
	Keep      bool   `json:"keep,omitempty"`
}

func (h *handler) handleCodographRemote(ctx context.Context, _ *sdkmcp.CallToolRequest, in remoteInput) (*sdkmcp.CallToolResult, any, error) {
	report, err := h.proto.CodographRemote(ctx, in.URL, protocol.RemoteOpts{
		Ref: in.Ref, Keep: in.Keep, Depth: in.Depth,
		ChurnDays: in.ChurnDays, Budget: in.Budget,
	})
	if err != nil {
		return nil, nil, err
	}
	data, err := arch.RenderJSON(report)
	if err != nil {
		return nil, nil, fmt.Errorf("render JSON: %w", err)
	}
	return text(string(data)), nil, nil
}

type historyInput struct {
	Path string `json:"path"`
	Last int    `json:"last,omitempty"`
	Diff bool   `json:"diff,omitempty"`
}

func (h *handler) handleGetCodographHistory(ctx context.Context, _ *sdkmcp.CallToolRequest, in historyInput) (*sdkmcp.CallToolResult, any, error) {
	path := in.Path
	if in.Diff {
		result, err := h.proto.DiffCodographs(ctx, path)
		if err != nil {
			return nil, nil, err
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return text(string(data)), nil, nil
	}
	entries, err := h.proto.GetHistory(ctx, path, in.Last)
	if err != nil {
		return nil, nil, fmt.Errorf("list history: %w", err)
	}
	data, _ := json.MarshalIndent(entries, "", "  ")
	return text(string(data)), nil, nil
}

func (h *handler) handleDiffCodographs(ctx context.Context, _ *sdkmcp.CallToolRequest, in pathInput) (*sdkmcp.CallToolResult, any, error) {
	d, err := h.proto.DiffCodographs(ctx, in.Path)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(d)
}

type diffBranchesInput struct {
	Path    string `json:"path"`
	BranchA string `json:"branch_a"`
	BranchB string `json:"branch_b"`
}

func (h *handler) handleDiffBranches(ctx context.Context, _ *sdkmcp.CallToolRequest, in diffBranchesInput) (*sdkmcp.CallToolResult, any, error) {
	r, err := h.proto.DiffBranches(ctx, in.Path, in.BranchA, in.BranchB)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(r)
}

// --- CON-303: cycles ---

type cyclesInput struct {
	Path      string   `json:"path"`
	Layers    []string `json:"layers,omitempty"`
	Analysis  string   `json:"analysis,omitempty"`  // cycles|coverage|api_surface|conventions|gaps|all (default: cycles)
	Threshold float64  `json:"threshold,omitempty"` // for coverage
	Trusted   []string `json:"trusted,omitempty"`    // for api_surface
}

func (h *handler) handleGetCycles(ctx context.Context, _ *sdkmcp.CallToolRequest, in cyclesInput) (*sdkmcp.CallToolResult, any, error) {
	path := in.Path
	switch in.Analysis {
	case "coverage":
		report, err := h.proto.GetCoverage(ctx, path, in.Threshold)
		if err != nil {
			return nil, nil, err
		}
		data, _ := json.MarshalIndent(report, "", "  ")
		return text(string(data)), nil, nil
	case "api_surface":
		report, err := h.proto.GetAPISurface(ctx, path, in.Trusted)
		if err != nil {
			return nil, nil, err
		}
		data, _ := json.MarshalIndent(report, "", "  ")
		return text(string(data)), nil, nil
	case "conventions":
		report, err := h.proto.GetConventions(ctx, path)
		if err != nil {
			return nil, nil, err
		}
		data, _ := json.MarshalIndent(report, "", "  ")
		return text(string(data)), nil, nil
	case "gaps":
		report, err := h.proto.GetGaps(ctx, path)
		if err != nil {
			return nil, nil, err
		}
		data, _ := json.MarshalIndent(report, "", "  ")
		return text(string(data)), nil, nil
	default:
		report, err := h.proto.GetCycles(ctx, path, in.Layers)
		if err != nil {
			return nil, nil, err
		}
		data, _ := json.MarshalIndent(report, "", "  ")
		return text(string(data)), nil, nil
	}
}

// --- CON-304: coverage ---

type coverageInput struct {
	Path      string  `json:"path"`
	Threshold float64 `json:"threshold,omitempty"`
}

func (h *handler) handleGetCoverage(ctx context.Context, _ *sdkmcp.CallToolRequest, in coverageInput) (*sdkmcp.CallToolResult, any, error) {
	r, err := h.proto.GetCoverage(ctx, in.Path, in.Threshold)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(r)
}

// --- CON-305: API surface ---

type apiSurfaceInput struct {
	Path    string   `json:"path"`
	Trusted []string `json:"trusted,omitempty"`
}

func (h *handler) handleGetAPISurface(ctx context.Context, _ *sdkmcp.CallToolRequest, in apiSurfaceInput) (*sdkmcp.CallToolResult, any, error) {
	r, err := h.proto.GetAPISurface(ctx, in.Path, in.Trusted)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(r)
}

// --- CON-325: validation ---

type validateInput struct {
	Path         string `json:"path"`
	DesiredState string `json:"desired_state"`
	Format       string `json:"format,omitempty"`
}

func (h *handler) handleValidateArchitecture(ctx context.Context, _ *sdkmcp.CallToolRequest, in validateInput) (*sdkmcp.CallToolResult, any, error) {
	r, err := h.proto.ValidateArchitecture(ctx, in.Path, in.DesiredState, in.Format)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(r)
}

// --- CON-354: diagrams ---

type diagramInput struct {
	Path         string `json:"path"`
	Type         string `json:"type"`
	Scope        string `json:"scope,omitempty"`
	Depth        int    `json:"depth,omitempty"`
	TopN         int    `json:"top_n,omitempty"`
	Entry        string `json:"entry,omitempty"`
	ExportedOnly bool   `json:"exported_only,omitempty"`
	Theme        string `json:"theme,omitempty"`
}

func (h *handler) handleRenderDiagram(ctx context.Context, _ *sdkmcp.CallToolRequest, in diagramInput) (*sdkmcp.CallToolResult, any, error) {
	path := in.Path
	if path == "" && len(h.proto.Workspaces()) > 0 {
		path = h.proto.Workspaces()[0]
	}

	report, err := h.proto.ScanProject(ctx, path, protocol.ScanOpts{
		Depth: in.Depth,
	})
	if err != nil {
		return nil, nil, err
	}

	input := diagram.Input{Report: report, Root: path}

	if in.Type == "churn" {
		hist, _ := h.proto.GetHistory(ctx, path, 20)
		input.History = hist
	}

	// Tier 2 diagrams need a TypeAnalyzer
	switch in.Type {
	case "classes", "sequence", "er":
		input.Analyzer = analysis.NewFallback(path)
	}

	// Tier 3 diagrams need a DeepAnalyzer
	switch in.Type {
	case "dataflow", "callgraph", "state":
		input.DeepAnalyzer = analysis.NewDeepFallback(path)
	}

	out, err := diagram.Render(input, diagram.Options{
		Type:         in.Type,
		Scope:        in.Scope,
		Depth:        in.Depth,
		TopN:         in.TopN,
		Entry:        in.Entry,
		ExportedOnly: in.ExportedOnly,
		Theme:        in.Theme,
	})
	if err != nil {
		return nil, nil, err
	}
	return text(out), nil, nil
}

// --- health ---

type healthCheckInput struct{}

func (h *handler) handleHealthCheck(ctx context.Context, _ *sdkmcp.CallToolRequest, _ healthCheckInput) (*sdkmcp.CallToolResult, any, error) {
	result := h.proto.Health(ctx)
	var b fmt.Stringer = buildHealthText(result)
	return text(b.String()), nil, nil
}

func buildHealthText(r *protocol.HealthResult) *strings.Builder {
	b := &strings.Builder{}
	status := "HEALTHY"
	if !r.OK {
		status = "UNHEALTHY"
	}
	fmt.Fprintf(b, "Locus Health: %s\n\n", status)
	for _, c := range r.Checks {
		mark := "OK"
		if !c.OK {
			mark = "FAIL"
		}
		fmt.Fprintf(b, "  [%s] %s", mark, c.Name)
		if c.Detail != "" {
			fmt.Fprintf(b, " — %s", c.Detail)
		}
		b.WriteString("\n")
	}
	return b
}

// --- evolution ---

type evolutionInput struct {
	Path      string `json:"path"`
	OldestRef string `json:"oldest_ref,omitempty"`
	NewestRef string `json:"newest_ref,omitempty"`
	Steps     int    `json:"steps,omitempty"`
	Stride    int    `json:"stride,omitempty"`
	Depth     int    `json:"depth,omitempty"`
}

func (h *handler) handleEvolution(ctx context.Context, _ *sdkmcp.CallToolRequest, in evolutionInput) (*sdkmcp.CallToolResult, any, error) {
	result, err := h.proto.Evolution(ctx, protocol.EvolutionOpts{
		Path:      in.Path,
		OldestRef: in.OldestRef,
		NewestRef: in.NewestRef,
		Steps:     in.Steps,
		Stride:    in.Stride,
		Depth:     in.Depth,
	})
	if err != nil {
		return nil, nil, err
	}
	return text(protocol.RenderEvolutionTable(result)), nil, nil
}

// --- triage ---

type triageInput struct {
	Intent string `json:"intent"`
	Path   string `json:"path,omitempty"`
}

func (h *handler) handleTriage(_ context.Context, _ *sdkmcp.CallToolRequest, in triageInput) (*sdkmcp.CallToolResult, any, error) {
	if in.Intent == "" {
		return nil, nil, fmt.Errorf("intent is required")
	}
	result := h.reg.Triage(in.Intent, in.Path)
	return jsonResult(result)
}

// --- helpers ---

func text(s string) *sdkmcp.CallToolResult {
	return &sdkmcp.CallToolResult{
		Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: s}},
	}
}

func jsonResult(data any) (*sdkmcp.CallToolResult, any, error) {
	b, _ := json.MarshalIndent(data, "", "  ")
	return text(string(b)), nil, nil
}

func noOut[In any](h func(context.Context, *sdkmcp.CallToolRequest, In) (*sdkmcp.CallToolResult, any, error)) sdkmcp.ToolHandlerFor[In, any] {
	return h
}
