package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dpopsuev/locus/internal/analysis"
	"github.com/dpopsuev/locus/internal/arch"
	"github.com/dpopsuev/locus/internal/cache"
	"github.com/dpopsuev/locus/internal/diagram"
	"github.com/dpopsuev/locus/internal/protocol"
	"github.com/dpopsuev/locus/internal/triage"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func NewServer(sc *cache.ScanCache, historyDir string, workspaceRoots []string) (*sdkmcp.Server, *triage.Registry) {
	srv := sdkmcp.NewServer(
		&sdkmcp.Implementation{Name: "locus", Version: "0.1.0"},
		&sdkmcp.ServerOptions{
			Instructions: "Locus is a spatial context bus for AI agents. " +
				"Point it at any repository to get architecture, dependency graph, churn, hot spots, and symbols. " +
				"Results are cached by git HEAD SHA. Use scan_project to analyze a repo, get_hot_spots for risk areas, " +
				"and codograph_remote for GitHub repos you don't have locally.",
		},
	)
	h := &handler{proto: protocol.New(sc, historyDir, workspaceRoots)}
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
		Name:        "suggest_depth",
		Description: "Analyze a repository and suggest the optimal --depth grouping level.",
		Keywords:    []string{"depth", "group", "granularity"},
		Categories:  []string{"architecture"},
		Rationale: map[string]string{
			"architecture": "Optimal grouping level for readable diagrams",
		},
		Priority: 5,
	}, noOut(h.handleSuggestDepth))

	triage.AddTool(reg, srv, triage.ToolMeta{
		Name:        "get_hot_spots",
		Description: "Return the hottest components in a repository (high fan-in + high churn).",
		Keywords:    []string{"perf", "bottleneck", "hot", "slow", "latency", "churn", "risk"},
		Categories:  []string{"performance", "refactoring", "architecture"},
		DefaultArgs: map[string]any{"top_n": 10},
		Rationale: map[string]string{
			"performance":  "High fan-in + high churn = likely bottleneck",
			"refactoring":  "Most-changed components with most dependents = highest risk",
			"architecture": "Structural hot spots reveal design pressure points",
		},
		Priority: 1,
	}, noOut(h.handleGetHotSpots))

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
		Name:        "get_rules",
		Description: "Return all .cursor/rules/*.mdc rules for a workspace root, with frontmatter metadata and body content.",
		Keywords:    []string{"rule", "convention", "standard", "lint"},
		Categories:  []string{"onboarding", "governance"},
		Rationale: map[string]string{
			"onboarding":  "Discover coding standards and conventions for this workspace",
			"governance":  "Review enforced rules and lint configurations",
		},
		Priority: 3,
	}, noOut(h.handleGetRules))

	triage.AddTool(reg, srv, triage.ToolMeta{
		Name:        "get_skills",
		Description: "Return all .cursor/skills/*/SKILL.md skills for a workspace root, with frontmatter metadata and body content.",
		Keywords:    []string{"skill", "capability", "workflow"},
		Categories:  []string{"onboarding"},
		Rationale: map[string]string{
			"onboarding": "Discover available agent skills and workflows",
		},
		Priority: 4,
	}, noOut(h.handleGetSkills))

	triage.AddTool(reg, srv, triage.ToolMeta{
		Name:        "get_coupling_table",
		Description: "Return a pre-formatted coupling table: package name, fan-in, fan-out, churn, symbol count. Sorted and filtered so agents never need Python/jq glue.",
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
		Name:        "get_edge_list",
		Description: "Return a pre-formatted list of dependency edges. Optionally filter to a single component.",
		Keywords:    []string{"edge", "dependency", "import", "link"},
		Categories:  []string{"dependencies", "architecture"},
		Rationale: map[string]string{
			"dependencies": "Raw edge list for dependency analysis",
			"architecture": "Complete import graph as flat list",
		},
		Priority: 3,
	}, noOut(h.handleGetEdgeList))

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
		Description: "List past codographs for a repository path. Returns timestamps, HEAD SHAs, source (local/remote), and component/edge counts. Use the 'last' parameter to limit results.",
		Keywords:    []string{"history", "past", "trend", "evolution"},
		Categories:  []string{"churn", "history"},
		Rationale: map[string]string{
			"churn":   "Track architectural evolution over time",
			"history": "See past scans and compare growth",
		},
		Priority: 1,
	}, noOut(h.handleGetCodographHistory))

	triage.AddTool(reg, srv, triage.ToolMeta{
		Name:        "diff_codographs",
		Description: "Compare two codographs and return a diff: added/removed components, added/removed edges, churn deltas, and a human-readable summary. Defaults to comparing the latest two codographs for a given path.",
		Keywords:    []string{"diff", "compare", "change", "delta"},
		Categories:  []string{"churn", "comparison"},
		Rationale: map[string]string{
			"churn":      "What changed between scans",
			"comparison": "Structural diff of two points in time",
		},
		Priority: 1,
	}, noOut(h.handleDiffCodographs))

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
		Description: "Detect circular dependencies, compute import depth per component, and check layer purity. Pure graph analysis on cached edges.",
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
		Name:        "get_coverage",
		Description: "Run test coverage analysis and return per-component coverage percentages. Optionally filter to components below a threshold.",
		Keywords:    []string{"test", "coverage", "untested", "gap", "quality"},
		Categories:  []string{"testing", "quality"},
		Rationale: map[string]string{
			"testing": "Identify untested or under-tested components",
			"quality": "Coverage gaps correlate with defect density",
		},
		Priority: 1,
	}, noOut(h.handleGetCoverage))

	triage.AddTool(reg, srv, triage.ToolMeta{
		Name:        "get_api_surface",
		Description: "Return exported symbol counts per component and trust boundary crossings.",
		Keywords:    []string{"api", "surface", "export", "boundary", "trust", "attack"},
		Categories:  []string{"security", "architecture"},
		Rationale: map[string]string{
			"security":     "Large API surface = larger attack surface",
			"architecture": "Exported symbols define component contracts",
		},
		Priority: 1,
	}, noOut(h.handleGetAPISurface))

	triage.AddTool(reg, srv, triage.ToolMeta{
		Name:        "validate_architecture",
		Description: "Diff a desired-state architecture (mermaid or JSON) against a live scan, reporting missing/extra components and edges.",
		Keywords:    []string{"drift", "desired", "validate", "compliance", "enforce"},
		Categories:  []string{"compliance", "architecture"},
		Rationale: map[string]string{
			"compliance":   "Detect drift from intended architecture",
			"architecture": "Enforce desired-state constraints on live code",
		},
		Priority: 1,
	}, noOut(h.handleValidateArchitecture))

	triage.AddTool(reg, srv, triage.ToolMeta{
		Name:        "render_diagram",
		Description: "Render a Mermaid diagram from repository structure. Types: dependency (flowchart), c4 (C4 Component), coupling (Sankey), churn (XY chart), layers (block), tree (mindmap), classes (classDiagram), sequence (sequenceDiagram), er (erDiagram). Returns valid Mermaid text.",
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

func (h *handler) handleGetRules(ctx context.Context, _ *sdkmcp.CallToolRequest, in pathInput) (*sdkmcp.CallToolResult, any, error) {
	rules, err := h.proto.GetRules(ctx, in.Path)
	if err != nil {
		return nil, nil, fmt.Errorf("read rules: %w", err)
	}
	return jsonResult(rules)
}

func (h *handler) handleGetSkills(ctx context.Context, _ *sdkmcp.CallToolRequest, in pathInput) (*sdkmcp.CallToolResult, any, error) {
	skills, err := h.proto.GetSkills(ctx, in.Path)
	if err != nil {
		return nil, nil, fmt.Errorf("read skills: %w", err)
	}
	return jsonResult(skills)
}

type couplingInput struct {
	Path   string `json:"path"`
	SortBy string `json:"sort_by,omitempty"`
	TopN   int    `json:"top_n,omitempty"`
}

func (h *handler) handleGetCouplingTable(ctx context.Context, _ *sdkmcp.CallToolRequest, in couplingInput) (*sdkmcp.CallToolResult, any, error) {
	md, err := h.proto.GetCouplingTable(ctx, in.Path, in.SortBy, in.TopN)
	if err != nil {
		return nil, nil, err
	}
	return text(md), nil, nil
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
}

func (h *handler) handleGetCodographHistory(ctx context.Context, _ *sdkmcp.CallToolRequest, in historyInput) (*sdkmcp.CallToolResult, any, error) {
	entries, err := h.proto.GetHistory(ctx, in.Path, in.Last)
	if err != nil {
		return nil, nil, fmt.Errorf("list history: %w", err)
	}
	return jsonResult(entries)
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
	Path   string   `json:"path"`
	Layers []string `json:"layers,omitempty"`
}

func (h *handler) handleGetCycles(ctx context.Context, _ *sdkmcp.CallToolRequest, in cyclesInput) (*sdkmcp.CallToolResult, any, error) {
	r, err := h.proto.GetCycles(ctx, in.Path, in.Layers)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(r)
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
	})
	if err != nil {
		return nil, nil, err
	}
	return text(out), nil, nil
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
