package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/dpopsuev/locus/internal/analysis"
	"github.com/dpopsuev/locus/internal/arch"
	"github.com/dpopsuev/locus/internal/diagram"
	"github.com/dpopsuev/locus/internal/protocol"
	"github.com/dpopsuev/locus/internal/store"
	"github.com/dpopsuev/locus/internal/triage"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Codograph action names.
const (
	ActionScanLocal  = "scan_local"
	ActionScanRemote = "scan_remote"
	ActionHistory    = "history"
	ActionDiff            = "diff"
	ActionSetDesiredState = "set_desired_state"
	ActionGetDesiredState = "get_desired_state"
	ActionStatus          = "status"
)

// Analysis action names.
const (
	ActionDeps        = "deps"
	ActionImpact      = "impact"
	ActionCoupling    = "coupling"
	ActionCycles      = "cycles"
	ActionViolations  = "violations"
	ActionCoverage    = "coverage"
	ActionAPISurface  = "api_surface"
	ActionConventions = "conventions"
	ActionGaps        = "gaps"
	ActionScanDiff    = "scan_diff"
	ActionCallers     = "callers"
	ActionCrossRepo   = "cross_repo"
	ActionPreset      = "preset"
	ActionComponent   = "component"
	ActionQuery       = "query"
	ActionSearch      = "search"
	ActionDrift            = "drift"
	ActionSuggestArch      = "suggest_architecture"
	ActionBlastRadius      = "blast_radius"
	ActionImportDirection  = "import_direction"
	ActionTrustBoundaries  = "trust_boundaries"
	ActionBudgets          = "budgets"
	ActionModDependencies  = "mod_dependencies"
	ActionSymbolBlast      = "symbol_blast"
	ActionDiffIntelligence = "diff_intelligence"
)

// Coupling view names.
const (
	ViewHotSpots = "hot_spots"
	ViewEdges    = "edges"
)

// Output format names.
const (
	FormatJSON    = "json"
	FormatSummary = "summary"
	FormatFacts   = "facts"
	FormatBoth    = "both"
)

// Diagram type names.
const (
	DiagramDependency = "dependency"
	DiagramC4         = "c4"
	DiagramCoupling   = "coupling"
	DiagramChurn      = "churn"
	DiagramLayers     = "layers"
	DiagramTree       = "tree"
	DiagramClasses    = "classes"
	DiagramSequence   = "sequence"
	DiagramER         = "er"
	DiagramDataflow   = "dataflow"
	DiagramCallgraph  = "callgraph"
	DiagramState      = "state"
	DiagramZones      = "zones"
	DiagramInterfaces = "interfaces"
)

// DiagramMinIntent maps diagram types to the minimum scan intent needed.
var DiagramMinIntent = map[string]string{
	DiagramDependency: string(arch.IntentArchitecture),
	DiagramC4:         string(arch.IntentArchitecture),
	DiagramTree:       string(arch.IntentArchitecture),
	DiagramLayers:     string(arch.IntentCoupling),
	DiagramCoupling:   string(arch.IntentCoupling),
	DiagramChurn:      string(arch.IntentHealth),
	DiagramClasses:    string(arch.IntentHealth),
	DiagramSequence:   string(arch.IntentHealth),
	DiagramER:         string(arch.IntentHealth),
	DiagramDataflow:   string(arch.IntentHealth),
	DiagramCallgraph:  string(arch.IntentHealth),
	DiagramState:      string(arch.IntentHealth),
	DiagramZones:      string(arch.IntentCoupling),
	DiagramInterfaces: string(arch.IntentHealth),
}

func NewServer(s store.Store, workspaceRoots []string, version string) (*sdkmcp.Server, *triage.Registry) {
	proto := protocol.New(s, workspaceRoots)
	srv := sdkmcp.NewServer(
		&sdkmcp.Implementation{Name: "locus", Version: version},
		&sdkmcp.ServerOptions{
			Instructions: "Locus is a spatial context bus for AI agents. Point it at any repository to get architecture, " +
				"dependency graph, churn, hot spots, and symbols. Results are cached by git HEAD SHA. " +
				"Workflow: codograph status to check cache, then scan_local (or scan_remote) which returns a cache_key. " +
				"Pass cache_key to analysis and render_diagram to avoid re-scanning. " +
				"Use intent param for scan depth: architecture (fast), coupling, health (default), full. " +
				"Output: default ~50 token summary; format=json for full; format=summary for <500 tokens; format=facts for assertions. " +
				"Key actions: analysis coupling view=hot_spots (risk), analysis violations (layer checks), " +
				"analysis drift (desired-state validation), analysis search (find by keyword), " +
				"analysis preset=architecture_review (one-call summary). " +
				"codograph set_desired_state to persist architecture rules. render_diagram type=zones for zone overview.",
		},
	)
	h := &handler{proto: proto}
	reg := triage.New()

	triage.AddTool(reg, srv, triage.ToolMeta{
		Name: "codograph",
		Description: "Scan and compare repository architectures. " +
			"Actions: scan_local (codebase scan, use intent for depth), scan_remote (GitHub via shallow clone), " +
			"history (past scans, diff=true to compare), diff (compare branches), " +
			"status (check cache + workspaces), set_desired_state (persist layer rules), get_desired_state (read rules). " +
			"Scan returns cache_key for downstream tools. Use intent=architecture for fast structure-only scans.",
		Keywords:   []string{"scan", "architecture", "overview", "remote", "github", "history", "diff", "branch", "compare"},
		Categories: []string{"architecture", "onboarding", "comparison"},
		Rationale: map[string]string{
			"architecture": "Full codebase overview with dependency graph and metrics",
			"onboarding":   "Best first step for understanding an unfamiliar codebase",
			"comparison":   "Compare branches or track architectural evolution",
		},
		Priority: 1,
	}, noOut(h.handleCodograph))

	triage.AddTool(reg, srv, triage.ToolMeta{
		Name: "analysis",
		Description: "Analyze component dependencies, impact, and coupling. " +
			"Actions: deps (fan-in/fan-out), impact (blast radius), coupling (table, view=hot_spots/edges), " +
			"cycles (dependency cycles), violations (layer purity), scan_diff (compare two SHAs), " +
			"callers (reverse symbol lookup), cross_repo (compare two repos), " +
			"drift (check against desired state), suggest_architecture (infer rules from code), " +
			"search (find components by keyword), component (single-package drill-down), " +
			"preset (architecture_review/health_check/onboarding/pre_pr), query (natural language). " +
			"Also: coverage, api_surface, conventions, gaps, budgets (check health constraints). " +
			"Pass cache_key from scan_remote to avoid re-scanning. format=summary for <500 tokens.",
		Keywords:   []string{"depend", "import", "impact", "blast", "coupling", "fan", "upstream", "downstream", "cycle", "circular", "loop", "coverage", "convention", "gap"},
		Categories: []string{"dependencies", "refactoring", "performance", "architecture"},
		Rationale: map[string]string{
			"dependencies": "Component-level dependency analysis, coupling metrics, and cycle detection",
			"refactoring":  "Assess risk and blast radius before changes",
			"performance":  "Identify highly coupled components and circular dependencies",
			"architecture": "Cycles violate clean layering, conventions show patterns",
		},
		Priority: 2,
	}, noOut(h.handleAnalysis))

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

// --- consolidated input types ---

type codographActionInput struct {
	Action string `json:"action" jsonschema:"required,scan_local | scan_remote | history | diff"`

	Path            string `json:"path,omitempty" jsonschema:"absolute path to local repository"`
	Depth           int    `json:"depth,omitempty" jsonschema:"directory grouping depth for components"`
	ChurnDays       int    `json:"churn_days,omitempty" jsonschema:"git history window in days for churn metrics"`
	IncludeExternal bool   `json:"include_external,omitempty" jsonschema:"include external/vendor dependencies in scan"`
	IncludeTests    bool   `json:"include_tests,omitempty" jsonschema:"include test files in scan"`
	IncludeCoverage bool   `json:"include_coverage,omitempty" jsonschema:"compute test coverage metrics"`
	Budget          int    `json:"budget,omitempty" jsonschema:"max components to include in output"`
	Format          string `json:"format,omitempty" jsonschema:"output format: json (default) or summary"`
	Intent          string `json:"intent,omitempty" jsonschema:"scan depth: architecture (fast, structure only), coupling (+ cycles/deps), health (default, + churn/nesting), full (+ coverage/authors)"`
	Since           string `json:"since,omitempty" jsonschema:"git ref to diff against for incremental scan (e.g. HEAD~1)"`

	URL string `json:"url,omitempty" jsonschema:"GitHub repository URL (scan_remote)"`
	Ref string `json:"ref,omitempty" jsonschema:"git ref to scan (scan_remote)"`

	Keep bool `json:"keep,omitempty" jsonschema:"keep cloned repo after scan_remote"`

	Last int  `json:"last,omitempty" jsonschema:"number of history entries to return"`
	Diff bool `json:"diff,omitempty" jsonschema:"if true, compare latest two scans (history)"`

	Layers  []string `json:"layers,omitempty" jsonschema:"ordered layer names for desired state"`
	BranchA string   `json:"branch_a,omitempty" jsonschema:"first branch to compare (diff)"`
	BranchB string `json:"branch_b,omitempty" jsonschema:"second branch to compare (diff)"`
}

type analysisActionInput struct {
	Action   string `json:"action" jsonschema:"required,deps | impact | coupling | cycles | violations | scan_diff | coverage | api_surface | conventions | gaps | blast_radius | import_direction | trust_boundaries | budgets | mod_dependencies | symbol_blast | diff_intelligence"`
	Path     string `json:"path,omitempty" jsonschema:"absolute path to local repository (defaults to workspace root)"`
	CacheKey string `json:"cache_key,omitempty" jsonschema:"cache key from scan_remote (use instead of path for remote repos)"`

	Component string   `json:"component,omitempty" jsonschema:"component path for deps/impact/coupling edges"`
	Symbol    string   `json:"symbol,omitempty" jsonschema:"symbol name for callers reverse lookup"`
	BeforeSHA string   `json:"before_sha,omitempty" jsonschema:"git SHA of earlier scan for scan_diff"`
	AfterSHA  string   `json:"after_sha,omitempty" jsonschema:"git SHA of later scan for scan_diff (default: HEAD)"`
	SortBy    string   `json:"sort_by,omitempty" jsonschema:"sort field for coupling table"`
	TopN      int      `json:"top_n,omitempty" jsonschema:"limit results to top N entries"`
	View      string   `json:"view,omitempty" jsonschema:"coupling view: hot_spots for risk areas, edges for edge list"`
	ChurnDays int      `json:"churn_days,omitempty" jsonschema:"git history window in days (coupling hot_spots)"`
	Layers    []string `json:"layers,omitempty" jsonschema:"ordered layer names for purity checking (cycles)"`
	Threshold float64  `json:"threshold,omitempty" jsonschema:"minimum coverage threshold to flag (coverage)"`
	Trusted   []string `json:"trusted,omitempty" jsonschema:"trusted import prefixes to exclude (api_surface)"`
	Format    string   `json:"format,omitempty" jsonschema:"output format: json (default) or summary (concise <500 tokens)"`
	PathB     string   `json:"path_b,omitempty" jsonschema:"second repo path for cross_repo comparison"`
	CacheKeyB string   `json:"cache_key_b,omitempty" jsonschema:"second cache key for cross_repo comparison"`
	Preset    string   `json:"preset,omitempty" jsonschema:"preset name: architecture_review, health_check, onboarding, pre_pr"`
	Query     string   `json:"query,omitempty" jsonschema:"natural language architecture question"`
	Files     []string `json:"files,omitempty" jsonschema:"changed files for blast_radius analysis"`
	Since     string   `json:"since,omitempty" jsonschema:"git ref for blast_radius (e.g. HEAD~1, main)"`
}

// --- dispatchers ---

func (h *handler) handleCodograph(ctx context.Context, req *sdkmcp.CallToolRequest, in codographActionInput) (*sdkmcp.CallToolResult, any, error) {
	switch in.Action {
	case ActionScanLocal:
		return h.handleScanProject(ctx, req, scanProjectInput{
			Path: in.Path, Depth: in.Depth, ChurnDays: in.ChurnDays,
			IncludeExternal: in.IncludeExternal, IncludeTests: in.IncludeTests,
			IncludeCoverage: in.IncludeCoverage, Budget: in.Budget, Format: in.Format,
			Intent: in.Intent, Since: in.Since,
		})
	case ActionScanRemote:
		return h.handleCodographRemote(ctx, req, remoteInput{
			URL: in.URL, Ref: in.Ref, Depth: in.Depth,
			ChurnDays: in.ChurnDays, Budget: in.Budget, Keep: in.Keep,
			Intent: in.Intent,
		})
	case ActionHistory:
		return h.handleGetCodographHistory(ctx, req, historyInput{
			Path: in.Path, Last: in.Last, Diff: in.Diff,
		})
	case ActionDiff:
		return h.handleDiffBranches(ctx, req, diffBranchesInput{
			Path: in.Path, BranchA: in.BranchA, BranchB: in.BranchB,
		})
	case ActionSetDesiredState:
		ds := &store.DesiredState{Layers: in.Layers}
		if err := h.proto.SetDesiredState(ctx, in.Path, ds); err != nil {
			return nil, nil, err
		}
		return text("desired state saved"), nil, nil
	case ActionGetDesiredState:
		ds, err := h.proto.GetDesiredState(ctx, in.Path)
		if err != nil {
			return nil, nil, err
		}
		if ds == nil {
			return text("no desired state configured"), nil, nil
		}
		return jsonResult(ds)
	case ActionStatus:
		r, err := h.proto.Status(ctx)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(r)
	default:
		return nil, nil, fmt.Errorf("unknown codograph action %q (valid: %s, %s, %s, %s)",
			in.Action, ActionScanLocal, ActionScanRemote, ActionHistory, ActionDiff)
	}
}

func (h *handler) handleAnalysis(ctx context.Context, req *sdkmcp.CallToolRequest, in analysisActionInput) (*sdkmcp.CallToolResult, any, error) {
	switch in.Action {
	case ActionDeps:
		return h.handleGetDependencies(ctx, req, depsInput{
			Path: in.Path, Component: in.Component, CacheKey: in.CacheKey,
		})
	case ActionImpact:
		return h.handleGetImpact(ctx, req, impactInput{
			Path: in.Path, Component: in.Component, CacheKey: in.CacheKey,
		})
	case ActionCoupling:
		topN := in.TopN
		if in.Format == FormatSummary && topN == 0 {
			topN = 5
		}
		return h.handleGetCouplingTable(ctx, req, couplingInput{
			Path: in.Path, SortBy: in.SortBy, TopN: topN,
			View: in.View, ChurnDays: in.ChurnDays, Component: in.Component,
			CacheKey: in.CacheKey,
		})
	case ActionCycles:
		return h.handleGetCycles(ctx, req, cyclesInput{
			Path: in.Path, Layers: in.Layers, CacheKey: in.CacheKey,
			Format: in.Format,
		})
	case ActionViolations:
		return h.handleGetViolations(ctx, req, violationsInput{
			Path: in.Path, Layers: in.Layers, CacheKey: in.CacheKey,
			Format: in.Format,
		})
	case ActionCallers:
		r, err := h.proto.GetCallers(ctx, in.Path, in.Symbol, in.CacheKey)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(r)
	case ActionScanDiff:
		return h.handleScanDiff(ctx, req, scanDiffInput{
			Path: in.Path, BeforeSHA: in.BeforeSHA, AfterSHA: in.AfterSHA,
		})
	case ActionCoverage:
		return h.handleGetCycles(ctx, req, cyclesInput{
			Path: in.Path, Analysis: ActionCoverage, Threshold: in.Threshold,
			CacheKey: in.CacheKey,
		})
	case ActionAPISurface:
		return h.handleGetCycles(ctx, req, cyclesInput{
			Path: in.Path, Analysis: ActionAPISurface, Trusted: in.Trusted,
			CacheKey: in.CacheKey,
		})
	case ActionConventions:
		return h.handleGetCycles(ctx, req, cyclesInput{
			Path: in.Path, Analysis: ActionConventions, CacheKey: in.CacheKey,
		})
	case ActionGaps:
		return h.handleGetCycles(ctx, req, cyclesInput{
			Path: in.Path, Analysis: ActionGaps, CacheKey: in.CacheKey,
		})
	case ActionCrossRepo:
		r, err := h.proto.GetCrossRepo(ctx, in.Path, in.PathB, in.CacheKey, in.CacheKeyB)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(r)
	case ActionPreset:
		r, err := h.proto.RunPreset(ctx, in.Path, in.Preset, in.CacheKey)
		if err != nil {
			return nil, nil, err
		}
		return text(r), nil, nil
	case ActionComponent:
		r, err := h.proto.GetComponentDetail(ctx, in.Path, in.Component, in.CacheKey)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(r)
	case ActionDrift:
		r, err := h.proto.GetDrift(ctx, in.Path, in.CacheKey)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(r)
	case ActionSuggestArch:
		r, err := h.proto.SuggestArchitecture(ctx, in.Path, in.CacheKey)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(r)
	case ActionSearch:
		r, err := h.proto.SearchComponents(ctx, in.Path, in.Query, in.CacheKey)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(r)
	case ActionQuery:
		r, err := h.proto.AnswerQuery(ctx, in.Path, in.Query, in.CacheKey)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(r)
	case ActionBlastRadius:
		r, err := h.proto.GetBlastRadius(ctx, in.Path, in.Files, in.Since, in.CacheKey)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(r)
	case ActionImportDirection:
		r, err := h.proto.GetImportDirection(ctx, in.Path, in.CacheKey)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(r)
	case ActionTrustBoundaries:
		r, err := h.proto.GetTrustBoundaries(ctx, in.Path, in.CacheKey)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(r)
	case ActionModDependencies:
		r, err := h.proto.GetModuleDependencies(ctx, in.Path, in.CacheKey)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(r)
	case ActionBudgets:
		r, err := h.proto.GetBudgets(ctx, in.Path, in.CacheKey)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(r)
	case ActionSymbolBlast:
		r, err := h.proto.GetSymbolBlastRadius(ctx, in.Path, in.Symbol, in.CacheKey)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(r)
	case ActionDiffIntelligence:
		r, err := h.proto.GetDiffIntelligence(ctx, in.Path, in.Since, in.CacheKey)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(r)
	default:
		return nil, nil, fmt.Errorf("unknown analysis action %q (valid: %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s)",
			in.Action, ActionDeps, ActionImpact, ActionCoupling, ActionCycles,
			ActionViolations, ActionScanDiff, ActionCoverage, ActionAPISurface, ActionConventions, ActionGaps,
			ActionBlastRadius, ActionImportDirection, ActionTrustBoundaries, ActionBudgets, ActionModDependencies,
			ActionSymbolBlast, ActionDiffIntelligence)
	}
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
	Intent          string `json:"intent,omitempty"`
	Since           string `json:"since,omitempty"`
}

func (h *handler) handleScanProject(ctx context.Context, _ *sdkmcp.CallToolRequest, in scanProjectInput) (*sdkmcp.CallToolResult, any, error) {
	result, err := h.proto.ScanProject(ctx, in.Path, protocol.ScanOpts{
		Depth: in.Depth, ChurnDays: in.ChurnDays,
		IncludeExternal: in.IncludeExternal, IncludeTests: in.IncludeTests,
		IncludeCoverage: in.IncludeCoverage, Budget: in.Budget,
		Intent: in.Intent, Since: in.Since,
	})
	if err != nil {
		return nil, nil, err
	}
	switch in.Format {
	case FormatSummary:
		return text(arch.RenderMarkdown(result.Report)), nil, nil
	case FormatJSON:
		data, err := arch.RenderJSON(result.Report)
		if err != nil {
			return nil, nil, fmt.Errorf("render JSON: %w", err)
		}
		return text(string(data)), nil, nil
	default:
		driftInfo := h.proto.CheckDriftOnScan(ctx, in.Path, result.Report)
		return text(protocol.RenderScanSummary(result, driftInfo)), nil, nil
	}
}


type depsInput struct {
	Path      string `json:"path"`
	Component string `json:"component"`
	CacheKey  string `json:"cache_key,omitempty"`
}

func (h *handler) handleGetDependencies(ctx context.Context, _ *sdkmcp.CallToolRequest, in depsInput) (*sdkmcp.CallToolResult, any, error) {
	r, err := h.proto.GetDependencies(ctx, in.Path, in.Component, in.CacheKey)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(r)
}


type impactInput struct {
	Path      string `json:"path"`
	Component string `json:"component"`
	CacheKey  string `json:"cache_key,omitempty"`
}

func (h *handler) handleGetImpact(ctx context.Context, _ *sdkmcp.CallToolRequest, in impactInput) (*sdkmcp.CallToolResult, any, error) {
	r, err := h.proto.GetImpact(ctx, in.Path, in.Component, in.CacheKey)
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
	CacheKey  string `json:"cache_key,omitempty"`
}

func (h *handler) handleGetCouplingTable(ctx context.Context, _ *sdkmcp.CallToolRequest, in couplingInput) (*sdkmcp.CallToolResult, any, error) {
	path := in.Path
	switch in.View {
	case ViewHotSpots:
		spots, err := h.proto.GetHotSpots(ctx, path, in.ChurnDays, in.TopN, in.CacheKey)
		if err != nil {
			return nil, nil, err
		}
		data, _ := json.MarshalIndent(spots, "", "  ")
		return text(string(data)), nil, nil
	case ViewEdges:
		result, err := h.proto.GetEdgeList(ctx, path, in.Component, in.CacheKey)
		if err != nil {
			return nil, nil, err
		}
		return text(result), nil, nil
	default:
		result, err := h.proto.GetCouplingTable(ctx, path, in.SortBy, in.TopN, in.CacheKey)
		if err != nil {
			return nil, nil, err
		}
		return text(result), nil, nil
	}
}


type remoteInput struct {
	URL       string `json:"url"`
	Ref       string `json:"ref,omitempty"`
	Depth     int    `json:"depth,omitempty"`
	ChurnDays int    `json:"churn_days,omitempty"`
	Budget    int    `json:"budget,omitempty"`
	Keep      bool   `json:"keep,omitempty"`
	Intent    string `json:"intent,omitempty"`
}

func (h *handler) handleCodographRemote(ctx context.Context, _ *sdkmcp.CallToolRequest, in remoteInput) (*sdkmcp.CallToolResult, any, error) {
	result, err := h.proto.CodographRemote(ctx, in.URL, protocol.RemoteOpts{
		Ref: in.Ref, Keep: in.Keep, Depth: in.Depth,
		ChurnDays: in.ChurnDays, Budget: in.Budget, Intent: in.Intent,
	})
	if err != nil {
		return nil, nil, err
	}
	data, err := arch.RenderJSON(result.Report)
	if err != nil {
		return nil, nil, fmt.Errorf("render JSON: %w", err)
	}
	// Append cache_key so downstream tools can reference this scan.
	out := fmt.Sprintf("%s\n\ncache_key: %s", string(data), result.CacheKey)
	return text(out), nil, nil
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
	Trusted   []string `json:"trusted,omitempty"`   // for api_surface
	CacheKey  string   `json:"cache_key,omitempty"`
	Format    string   `json:"format,omitempty"`
}

func (h *handler) handleGetCycles(ctx context.Context, _ *sdkmcp.CallToolRequest, in cyclesInput) (*sdkmcp.CallToolResult, any, error) {
	path := in.Path
	switch in.Analysis {
	case ActionCoverage:
		report, err := h.proto.GetCoverage(ctx, path, in.Threshold)
		if err != nil {
			return nil, nil, err
		}
		data, _ := json.MarshalIndent(report, "", "  ")
		return text(string(data)), nil, nil
	case ActionAPISurface:
		report, err := h.proto.GetAPISurface(ctx, path, in.Trusted, in.CacheKey)
		if err != nil {
			return nil, nil, err
		}
		data, _ := json.MarshalIndent(report, "", "  ")
		return text(string(data)), nil, nil
	case ActionConventions:
		report, err := h.proto.GetConventions(ctx, path)
		if err != nil {
			return nil, nil, err
		}
		data, _ := json.MarshalIndent(report, "", "  ")
		return text(string(data)), nil, nil
	case ActionGaps:
		report, err := h.proto.GetGaps(ctx, path)
		if err != nil {
			return nil, nil, err
		}
		data, _ := json.MarshalIndent(report, "", "  ")
		return text(string(data)), nil, nil
	default:
		report, err := h.proto.GetCycles(ctx, path, in.Layers, in.CacheKey)
		if err != nil {
			return nil, nil, err
		}
		if in.Format == FormatSummary {
			return text(renderCyclesSummary(report)), nil, nil
		}
		data, _ := json.MarshalIndent(report, "", "  ")
		return text(string(data)), nil, nil
	}
}

const maxSummaryCycles = 3

func renderCyclesSummary(r *protocol.CycleReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d cycle(s), %d violation(s)\n", len(r.Cycles), len(r.LayerViolations))
	n := min(len(r.Cycles), maxSummaryCycles)
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "  cycle: %s\n", strings.Join(r.Cycles[i], " → "))
	}
	if len(r.Cycles) > maxSummaryCycles {
		fmt.Fprintf(&b, "  ... and %d more\n", len(r.Cycles)-maxSummaryCycles)
	}
	return b.String()
}

// --- violations ---

type violationsInput struct {
	Path     string   `json:"path"`
	Layers   []string `json:"layers,omitempty"`
	CacheKey string   `json:"cache_key,omitempty"`
	Format   string   `json:"format,omitempty"`
}

const maxSummaryViolations = 3

func (h *handler) handleGetViolations(ctx context.Context, _ *sdkmcp.CallToolRequest, in violationsInput) (*sdkmcp.CallToolResult, any, error) {
	report, err := h.proto.GetViolations(ctx, in.Path, in.Layers, in.CacheKey)
	if err != nil {
		return nil, nil, err
	}
	if in.Format == FormatSummary {
		var b strings.Builder
		fmt.Fprintf(&b, "%s\n", report.Summary)
		n := min(len(report.Violations), maxSummaryViolations)
		for i := 0; i < n; i++ {
			v := report.Violations[i]
			fmt.Fprintf(&b, "  %s → %s\n", v.From, v.To)
		}
		if len(report.Violations) > maxSummaryViolations {
			fmt.Fprintf(&b, "  ... and %d more\n", len(report.Violations)-maxSummaryViolations)
		}
		return text(b.String()), nil, nil
	}
	return jsonResult(report)
}

// --- scan diff ---

type scanDiffInput struct {
	Path      string `json:"path"`
	BeforeSHA string `json:"before_sha"`
	AfterSHA  string `json:"after_sha,omitempty"`
}

func (h *handler) handleScanDiff(ctx context.Context, _ *sdkmcp.CallToolRequest, in scanDiffInput) (*sdkmcp.CallToolResult, any, error) {
	report, err := h.proto.GetScanDiff(ctx, in.Path, in.BeforeSHA, in.AfterSHA)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(report)
}

// --- diagrams ---

type diagramInput struct {
	Path         string `json:"path" jsonschema:"required,absolute path to local repository"`
	Type         string `json:"type" jsonschema:"required,diagram type: dependency, c4, coupling, churn, layers, tree, classes, sequence, er, interfaces"`
	Scope        string `json:"scope,omitempty" jsonschema:"limit diagram to a sub-package or directory"`
	Depth        int    `json:"depth,omitempty" jsonschema:"directory grouping depth for components"`
	TopN         int    `json:"top_n,omitempty" jsonschema:"limit to top N components in diagram"`
	Entry        string `json:"entry,omitempty" jsonschema:"entry point component for sequence/callgraph diagrams"`
	ExportedOnly bool   `json:"exported_only,omitempty" jsonschema:"only include exported symbols in class diagrams"`
	Enrich       string `json:"enrich,omitempty" jsonschema:"comma-separated metrics on dependency node labels: loc, fan_in, churn"`
	Theme        string `json:"theme,omitempty" jsonschema:"Mermaid theme: light, dark, or natural"`
	Format       string `json:"format,omitempty" jsonschema:"output format: mermaid (default), facts (plain-text assertions), both"`
	CacheKey     string `json:"cache_key,omitempty" jsonschema:"cache key from scan_remote (use instead of path for remote repos)"`
}

func (h *handler) handleRenderDiagram(ctx context.Context, _ *sdkmcp.CallToolRequest, in diagramInput) (*sdkmcp.CallToolResult, any, error) {
	path := in.Path
	if path == "" && len(h.proto.Workspaces()) > 0 {
		path = h.proto.Workspaces()[0]
	}

	var report *arch.ContextReport
	var err error
	if in.CacheKey != "" {
		report, err = h.proto.GetCachedReport(in.CacheKey)
	} else {
		intent := DiagramMinIntent[in.Type]
		result, scanErr := h.proto.ScanProject(ctx, path, protocol.ScanOpts{
			Depth:  in.Depth,
			Intent: intent,
		})
		if scanErr != nil {
			return nil, nil, scanErr
		}
		report = result.Report
		err = nil
	}
	if err != nil {
		return nil, nil, err
	}

	input := diagram.Input{Report: report, Root: path}

	if in.Type == DiagramChurn && path != "" {
		hist, _ := h.proto.GetHistory(ctx, path, 20)
		input.History = hist
	}

	// Tier 2/3 diagrams need analyzers — only available for local repos.
	if path != "" {
		switch in.Type {
		case DiagramClasses, DiagramSequence, DiagramER, DiagramInterfaces:
			input.Analyzer = analysis.NewFallback(path)
		}
		switch in.Type {
		case DiagramDataflow, DiagramCallgraph, DiagramState:
			input.DeepAnalyzer = analysis.NewDeepFallback(path)
		}
	}

	switch in.Format {
	case FormatFacts:
		return text(diagram.RenderFacts(report)), nil, nil
	case FormatBoth:
		facts := diagram.RenderFacts(report)
		mermaid, err := diagram.Render(input, diagram.Options{
			Type: in.Type, Scope: in.Scope, Depth: in.Depth,
			TopN: in.TopN, Entry: in.Entry, ExportedOnly: in.ExportedOnly,
			Theme: in.Theme, Enrich: in.Enrich,
		})
		if err != nil {
			return nil, nil, err
		}
		return text(mermaid + "\n\n" + facts), nil, nil
	default:
		out, err := diagram.Render(input, diagram.Options{
			Type: in.Type, Scope: in.Scope, Depth: in.Depth,
			TopN: in.TopN, Entry: in.Entry, ExportedOnly: in.ExportedOnly,
			Theme: in.Theme, Enrich: in.Enrich,
		})
		if err != nil {
			return nil, nil, err
		}
		return text(out), nil, nil
	}
}

// --- triage ---

type triageInput struct {
	Intent string `json:"intent" jsonschema:"required,natural language description of what you want to do"`
	Path   string `json:"path,omitempty" jsonschema:"optional repository path for context"`
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
	return func(ctx context.Context, req *sdkmcp.CallToolRequest, in In) (*sdkmcp.CallToolResult, any, error) {
		tool := ""
		if req != nil {
			tool = req.Params.Name
		}
		start := time.Now()
		result, out, err := h(ctx, req, in)
		elapsed := time.Since(start)
		if err != nil {
			slog.Error("tool call failed", "tool", tool, "elapsed", elapsed, "error", err)
		} else {
			slog.Debug("tool call", "tool", tool, "elapsed", elapsed)
		}
		return result, out, err
	}
}
