package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dpopsuev/locus/internal/config"
	"github.com/dpopsuev/locus/internal/store"
	oculus "github.com/dpopsuev/oculus"
	"github.com/dpopsuev/oculus/analyzer"
	"github.com/dpopsuev/oculus/arch"
	clinichexa "github.com/dpopsuev/oculus/clinic/hexa"
	"github.com/dpopsuev/oculus/diagram"
	diagramcore "github.com/dpopsuev/oculus/diagram/core"
	"github.com/dpopsuev/oculus/engine"
	gitpkg "github.com/dpopsuev/oculus/git"
	"github.com/dpopsuev/oculus/impact"
	"github.com/dpopsuev/oculus/lint"
	"github.com/dpopsuev/oculus/lsp"
	"github.com/dpopsuev/oculus/triage"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Log key constants for structured logging.
const (
	logKeyTool    = "tool"
	logKeyElapsed = "elapsed"
	logKeyError   = "error"
)

// Sentinel errors for input validation.
var (
	ErrUnknownCodographAction = errors.New("unknown codograph action")
	ErrUnknownAction          = errors.New("unknown action")
	ErrIntentRequired         = errors.New("intent is required")
	ErrMeshFQNRequired        = errors.New("fqn is required for mesh neighborhood")
	ErrMeshFromToRequired     = errors.New("from and to are required for mesh distance")
	ErrUnknownMeshView        = errors.New("unknown mesh view")
)

// --- Action constants ---

// Codograph actions.
const (
	ActionScanLocal       = "scan_local"
	ActionScanRemote      = "scan_remote"
	ActionHistory         = "history"
	ActionDiff            = "diff"
	ActionSetDesiredState = "set_desired_state"
	ActionGetDesiredState = "get_desired_state"
	ActionAcceptViolation = "accept_violation"
	ActionStatus          = "status"
	ActionFlush           = "flush"
)

// Analysis actions.
const (
	ActionDeps         = "deps"
	ActionImpact       = "impact"
	ActionCoupling     = "coupling"
	ActionCycles       = "cycles"
	ActionViolations   = "violations"
	ActionCallers      = "callers"
	ActionComponent    = "component"
	ActionSearch       = "search"
	ActionQuery        = "query"
	ActionPreset       = "preset"
	ActionScanDiff     = "scan_diff"
	ActionRiskScores   = "risk_scores"
	ActionSymbolSearch = "symbol_search"
	ActionCallees      = "callees"
	ActionCallPath     = "call_path"
	ActionSymbolGraph  = "symbol_graph"
	ActionPipelines    = "pipelines"
	ActionMesh         = "mesh"
)

// Clinic actions.
const (
	ActionPatternScan    = "pattern_scan"
	ActionPatternCatalog = "pattern_catalog"
	ActionHexaValidate   = "hexa_validate"
	ActionSOLIDScan      = "solid_scan"
	ActionSymbolQuality  = "symbol_quality"
	ActionVocabMap       = "vocab_map"
	ActionBloaterScan    = "bloater_scan"
)

// Constraint actions.
const (
	ActionBlastRadius      = "blast_radius"
	ActionImportDirection  = "import_direction"
	ActionTrustBoundaries  = "trust_boundaries"
	ActionBudgets          = "budgets"
	ActionModDependencies  = "mod_dependencies"
	ActionSymbolBlast      = "symbol_blast"
	ActionInterfaceMetrics = "interface_metrics"
	ActionLeverage         = "leverage"
	ActionCoverage         = "coverage"
	ActionAPISurface       = "api_surface"
	ActionConventions      = "conventions"
	ActionGaps             = "gaps"
	ActionConsolidate      = "consolidate"
)

// Refactor actions.
const (
	ActionCrossRepo        = "cross_repo"
	ActionDrift            = "drift"
	ActionSuggestArch      = "suggest_architecture"
	ActionWhatIf           = "what_if"
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

// Diagram type constants.
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
	DiagramHexa       = "hexa"
	DiagramSymbolDSM  = "symbol_dsm"
)

// DiagramMinIntent maps diagram types to minimum scan intent needed.
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
	DiagramHexa:       string(arch.IntentCoupling),
	DiagramSymbolDSM:  string(arch.IntentHealth),
}

// --- Server ---

func NewServer(s store.Store, workspaceRoots []string, version string, pool ...lsp.Pool) (*sdkmcp.Server, *triage.Registry) {
	proto := engine.New(s, workspaceRoots, pool...)
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
	// Record binary mtime at startup for stale binary detection (BUG-33).
	if exe, err := os.Executable(); err == nil {
		h.binPath = exe
		if info, err := os.Stat(exe); err == nil {
			h.binStart = info.ModTime()
		}
	}
	reg := triage.New()

	triage.AddTool(reg, srv, triage.ToolMeta{
		Name: "codograph",
		Description: "Scan and compare repository architectures. " +
			"Actions: scan_local, scan_remote, history, diff, status, set_desired_state, get_desired_state, accept_violation. " +
			"Scan returns cache_key for downstream tools.",
		Keywords:   []string{"scan", "architecture", "overview", "remote", "github", "history", "diff", "branch", "compare"},
		Categories: []string{"architecture", "onboarding", "comparison"},
		Rationale:  map[string]string{"architecture": "Full codebase overview", "onboarding": "Best first step"},
		Priority:   1,
	}, noOut(h.handleCodograph))

	triage.AddTool(reg, srv, triage.ToolMeta{
		Name: "analysis",
		Description: "Core dependency analysis. " +
			"Actions: deps, impact, coupling, cycles, violations, callers, component, search, query, preset, scan_diff, symbol_graph, pipelines.",
		Keywords:   []string{"depend", "import", "impact", "blast", "coupling", "fan", "cycle", "circular"},
		Categories: []string{"dependencies", "architecture"},
		Rationale:  map[string]string{"dependencies": "Component-level dependency analysis and cycle detection"},
		Priority:   2,
	}, noOut(h.handleAnalysis))

	triage.AddTool(reg, srv, triage.ToolMeta{
		Name: "clinic",
		Description: "Code quality and design pattern analysis. " +
			"Actions: pattern_scan, pattern_catalog, hexa_validate, solid_scan, symbol_quality, vocab_map.",
		Keywords:   []string{"pattern", "smell", "solid", "hexagonal", "quality", "symbol", "naming"},
		Categories: []string{"quality", "refactoring"},
		Rationale:  map[string]string{"quality": "SOLID, hexagonal, pattern/smell detection"},
		Priority:   2,
	}, noOut(h.handleClinic))

	triage.AddTool(reg, srv, triage.ToolMeta{
		Name: "constraint",
		Description: "Architecture enforcement and health metrics. " +
			"Actions: blast_radius, import_direction, trust_boundaries, budgets, mod_dependencies, " +
			"symbol_blast, interface_metrics, leverage, coverage, api_surface, conventions, gaps.",
		Keywords:   []string{"constraint", "boundary", "budget", "coverage", "leverage", "trust", "direction"},
		Categories: []string{"enforcement", "health"},
		Rationale:  map[string]string{"enforcement": "Architecture rules, budgets, and boundary checks"},
		Priority:   2,
	}, noOut(h.handleConstraint))

	triage.AddTool(reg, srv, triage.ToolMeta{
		Name: "refactor",
		Description: "Refactoring intelligence and what-if simulation. " +
			"Actions: cross_repo, drift, suggest_architecture, what_if, diff_intelligence.",
		Keywords:   []string{"refactor", "what-if", "drift", "suggest", "diff", "cross-repo"},
		Categories: []string{"refactoring", "comparison"},
		Rationale:  map[string]string{"refactoring": "Simulate refactors, detect drift, compare repos"},
		Priority:   2,
	}, noOut(h.handleRefactor))

	triage.AddTool(reg, srv, triage.ToolMeta{
		Name:        "render_diagram",
		Description: "Render a Mermaid diagram. Types: dependency, c4, coupling, churn, layers, tree, classes, sequence, er, interfaces, hexa, zones.",
		Keywords:    []string{"diagram", "visual", "mermaid", "chart", "graph", "class", "sequence", "er"},
		Categories:  []string{"visualization"},
		Rationale:   map[string]string{"visualization": "Generate Mermaid charts from architecture data"},
		Priority:    1,
	}, noOut(h.handleRenderDiagram))

	triage.AddTool(reg, srv, triage.ToolMeta{
		Name: "lint",
		Description: "Run architectural linters. " +
			"Checks hexagonal architecture, SOLID, pattern smells, symbol quality, layer purity, and budget constraints.",
		Keywords:   []string{"lint", "quality", "violations", "check", "architecture", "solid", "hexa"},
		Categories: []string{"quality", "enforcement"},
		Rationale:  map[string]string{"quality": "Unified architectural lint pass", "enforcement": "Check all rules in one call"},
		Priority:   1,
	}, noOut(h.handleLint))

	h.reg = reg
	triage.AddTool(reg, srv, triage.ToolMeta{
		Name:        "triage",
		Description: "Map a natural language intent to a ranked list of Locus tools.",
		Keywords:    []string{"help", "what", "which", "recommend", "suggest", "guide"},
		Categories:  []string{"meta"},
		Rationale:   map[string]string{"meta": "Discover relevant tools"},
		Priority:    1,
	}, noOut(h.handleTriage))

	return srv, reg
}

type handler struct {
	proto    *engine.Engine
	reg      *triage.Registry
	binStart time.Time // mtime of binary at startup, for stale detection
	binPath  string    // path to running binary
}

// --- Input structs (per-tool, only relevant fields) ---

type codographActionInput struct {
	Action string `json:"action" jsonschema:"required,scan_local | scan_remote | history | diff | set_desired_state | get_desired_state | accept_violation | status"`

	Path            string   `json:"path,omitempty" jsonschema:"absolute path to local repository"`
	Depth           int      `json:"depth,omitempty" jsonschema:"directory grouping depth"`
	ChurnDays       int      `json:"churn_days,omitempty" jsonschema:"git history window in days"`
	IncludeExternal bool     `json:"include_external,omitempty" jsonschema:"include external dependencies"`
	IncludeTests    bool     `json:"include_tests,omitempty" jsonschema:"include test files"`
	IncludeCoverage bool     `json:"include_coverage,omitempty" jsonschema:"compute test coverage"`
	Budget          int      `json:"budget,omitempty" jsonschema:"max components in output"`
	Format          string   `json:"format,omitempty" jsonschema:"output format: json or summary"`
	Intent          string   `json:"intent,omitempty" jsonschema:"scan depth: architecture, coupling, health, full"`
	Since           string   `json:"since,omitempty" jsonschema:"git ref for incremental scan"`
	URL             string   `json:"url,omitempty" jsonschema:"GitHub URL (scan_remote)"`
	Ref             string   `json:"ref,omitempty" jsonschema:"git ref (scan_remote)"`
	Keep            bool     `json:"keep,omitempty" jsonschema:"keep clone (scan_remote)"`
	Last            int      `json:"last,omitempty" jsonschema:"history entries to return"`
	Diff            bool     `json:"diff,omitempty" jsonschema:"compare latest two scans"`
	Layers          []string `json:"layers,omitempty" jsonschema:"ordered layer names"`
	BranchA         string   `json:"branch_a,omitempty" jsonschema:"first branch (diff)"`
	BranchB         string   `json:"branch_b,omitempty" jsonschema:"second branch (diff)"`
	Component       string   `json:"component,omitempty" jsonschema:"component for accept_violation"`
	Principle       string   `json:"principle,omitempty" jsonschema:"principle for accept_violation"`
	Reason          string   `json:"reason,omitempty" jsonschema:"reason for accept_violation"`
}

type analysisInput struct {
	Action    string   `json:"action" jsonschema:"required,deps | impact | coupling | cycles | violations | callers | component | search | query | preset | scan_diff | risk_scores | symbol_search | callees | call_path | symbol_graph | pipelines | mesh"`
	Path      string   `json:"path,omitempty" jsonschema:"absolute path to local repository"`
	CacheKey  string   `json:"cache_key,omitempty" jsonschema:"cache key from scan_remote"`
	Component string   `json:"component,omitempty" jsonschema:"component path for deps/impact/coupling"`
	Symbol    string   `json:"symbol,omitempty" jsonschema:"symbol name for callers"`
	SortBy    string   `json:"sort_by,omitempty" jsonschema:"sort field for coupling table"`
	TopN      int      `json:"top_n,omitempty" jsonschema:"limit results to top N"`
	View      string   `json:"view,omitempty" jsonschema:"coupling view: hot_spots, edges"`
	ChurnDays int      `json:"churn_days,omitempty" jsonschema:"git history window (coupling hot_spots)"`
	Layers    []string `json:"layers,omitempty" jsonschema:"ordered layer names (cycles)"`
	Format    string   `json:"format,omitempty" jsonschema:"output format: json, summary"`
	Preset    string   `json:"preset,omitempty" jsonschema:"preset: architecture_review, health_check, onboarding, pre_pr, full_clinic, code_health"`
	Query     string   `json:"query,omitempty" jsonschema:"natural language question"`
	BeforeSHA string   `json:"before_sha,omitempty" jsonschema:"earlier SHA (scan_diff)"`
	AfterSHA  string   `json:"after_sha,omitempty" jsonschema:"later SHA (scan_diff)"`
	MinLength int      `json:"min_length,omitempty" jsonschema:"minimum pipeline length (pipelines)"`
	MeshView  string   `json:"mesh_view,omitempty" jsonschema:"mesh view: full, neighborhood, distance, boundaries, aggregate (mesh)"`
	Level     string   `json:"level,omitempty" jsonschema:"aggregation level: symbol, file, package, component (mesh aggregate)"`
	FQN       string   `json:"fqn,omitempty" jsonschema:"fully qualified symbol name (mesh neighborhood)"`
	Hops      int      `json:"hops,omitempty" jsonschema:"neighborhood radius in hops (mesh neighborhood)"`
	From      string   `json:"from,omitempty" jsonschema:"source symbol FQN (mesh distance)"`
	To        string   `json:"to,omitempty" jsonschema:"target symbol FQN (mesh distance)"`
	MinWeight *float64 `json:"min_weight,omitempty" jsonschema:"minimum edge weight filter (mesh boundaries/neighborhood, default 0.1)"`
}

type clinicInput struct {
	Action   string `json:"action" jsonschema:"required,pattern_scan | pattern_catalog | hexa_validate | solid_scan | symbol_quality | vocab_map | bloater_scan"`
	Path     string `json:"path,omitempty" jsonschema:"absolute path to local repository"`
	CacheKey string `json:"cache_key,omitempty" jsonschema:"cache key from scan_remote"`
	Filter   string `json:"filter,omitempty" jsonschema:"filter for pattern_catalog: pattern, smell, or name"`
}

type constraintInput struct {
	Action    string   `json:"action" jsonschema:"required,blast_radius | import_direction | trust_boundaries | budgets | mod_dependencies | symbol_blast | interface_metrics | leverage | coverage | api_surface | conventions | gaps | consolidate"`
	Path      string   `json:"path,omitempty" jsonschema:"absolute path to local repository"`
	CacheKey  string   `json:"cache_key,omitempty" jsonschema:"cache key from scan_remote"`
	Component string   `json:"component,omitempty" jsonschema:"component for leverage"`
	Symbol    string   `json:"symbol,omitempty" jsonschema:"symbol for symbol_blast"`
	Files     []string `json:"files,omitempty" jsonschema:"changed files for blast_radius"`
	Since     string   `json:"since,omitempty" jsonschema:"git ref for blast_radius"`
	Threshold float64  `json:"threshold,omitempty" jsonschema:"coverage threshold"`
	Trusted   []string `json:"trusted,omitempty" jsonschema:"trusted prefixes (api_surface)"`
}

type refactorInput struct {
	Action    string            `json:"action" jsonschema:"required,cross_repo | drift | suggest_architecture | what_if | diff_intelligence"`
	Path      string            `json:"path,omitempty" jsonschema:"absolute path to local repository"`
	CacheKey  string            `json:"cache_key,omitempty" jsonschema:"cache key from scan_remote"`
	PathB     string            `json:"path_b,omitempty" jsonschema:"second repo path (cross_repo)"`
	CacheKeyB string            `json:"cache_key_b,omitempty" jsonschema:"second cache key (cross_repo)"`
	Moves     []impact.FileMove `json:"moves,omitempty" jsonschema:"component moves for what_if"`
	Since     string            `json:"since,omitempty" jsonschema:"git ref for diff_intelligence"`
}

// staleBinaryWarning returns a warning string if the on-disk binary has been
// updated since the MCP server started. Empty string if current.
func (h *handler) staleBinaryWarning() string {
	if h.binPath == "" || h.binStart.IsZero() {
		return ""
	}
	info, err := os.Stat(h.binPath)
	if err != nil {
		return ""
	}
	if info.ModTime().After(h.binStart) {
		return "⚠ Locus binary upgraded on disk — restart MCP server for latest analysis"
	}
	return ""
}

// --- Codograph handler ---

func (h *handler) handleCodograph(ctx context.Context, req *sdkmcp.CallToolRequest, in codographActionInput) (*sdkmcp.CallToolResult, any, error) { //nolint:gocritic
	switch in.Action {
	case ActionScanLocal:
		return h.handleScanProject(ctx, &in)
	case ActionScanRemote:
		return h.handleCodographRemote(ctx, &in)
	case ActionHistory:
		return h.handleGetCodographHistory(ctx, &in)
	case ActionDiff:
		return h.handleDiffBranches(ctx, &in)
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
	case ActionAcceptViolation:
		if err := h.proto.AcceptViolation(ctx, in.Path, store.AcceptedViolation{
			Component: in.Component, Principle: in.Principle, Reason: in.Reason,
		}); err != nil {
			return nil, nil, err
		}
		return text(fmt.Sprintf("accepted violation: %s/%s", in.Component, in.Principle)), nil, nil
	case ActionStatus:
		r, err := h.proto.Status(ctx)
		if err != nil {
			return nil, nil, err
		}
		if warn := h.staleBinaryWarning(); warn != "" {
			r.Version = warn
		}
		return jsonResult(r)
	case ActionFlush:
		n, err := h.proto.FlushCache(ctx, in.Path)
		if err != nil {
			return nil, nil, err
		}
		return text(fmt.Sprintf("flushed %d project cache(s)", n)), nil, nil
	default:
		return nil, nil, fmt.Errorf("%w %q", ErrUnknownCodographAction, in.Action)
	}
}

// --- Analysis handler ---

func (h *handler) handleAnalysis(ctx context.Context, _ *sdkmcp.CallToolRequest, in analysisInput) (*sdkmcp.CallToolResult, any, error) { //nolint:gocritic
	switch in.Action {
	case ActionDeps:
		r, err := h.proto.GetDependencies(ctx, in.Path, in.Component, in.CacheKey)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(r)
	case ActionImpact:
		r, err := h.proto.GetImpact(ctx, in.Path, in.Component, in.CacheKey)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(r)
	case ActionCoupling:
		topN := in.TopN
		if in.Format == FormatSummary && topN == 0 {
			topN = 5
		}
		return h.handleCoupling(ctx, in.Path, in.SortBy, topN, in.View, in.ChurnDays, in.Component, in.CacheKey)
	case ActionCycles:
		return h.handleCycles(ctx, in.Path, in.Layers, in.CacheKey, in.Format)
	case ActionViolations:
		return h.handleViolations(ctx, in.Path, in.Layers, in.CacheKey, in.Format)
	case ActionCallers:
		r, err := h.proto.GetCallers(ctx, in.Path, in.Symbol, in.CacheKey)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(r)
	case ActionComponent, ActionSearch, ActionQuery:
		return h.dispatchAnalysisLookup(ctx, &in)
	case ActionPreset:
		r, err := h.proto.RunPreset(ctx, in.Path, in.Preset, in.CacheKey)
		if err != nil {
			return nil, nil, err
		}
		return text(r), nil, nil
	case ActionScanDiff:
		r, err := h.proto.GetScanDiff(ctx, in.Path, in.BeforeSHA, in.AfterSHA)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(r)
	case ActionRiskScores, ActionSymbolSearch, ActionCallees, ActionCallPath, ActionSymbolGraph, ActionPipelines, ActionMesh:
		return h.dispatchAnalysisExtended(ctx, &in)
	default:
		return nil, nil, fmt.Errorf("%w %q", ErrUnknownAction, in.Action)
	}
}

// --- Clinic handler (table-driven) ---

func (h *handler) dispatchAnalysisExtended(ctx context.Context, in *analysisInput) (*sdkmcp.CallToolResult, any, error) {
	switch in.Action {
	case ActionRiskScores:
		r, err := h.proto.GetRiskScores(ctx, in.Path, in.CacheKey)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(r)
	case ActionSymbolSearch:
		r, err := h.proto.SearchSymbols(ctx, in.Path, in.Query, in.CacheKey)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(r)
	case ActionCallees:
		r, err := h.proto.GetCallees(ctx, in.Path, in.Symbol, in.CacheKey)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(r)
	case ActionCallPath:
		r, err := h.proto.GetCallPath(ctx, in.Path, in.Symbol, in.Query, in.CacheKey)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(r)
	case ActionSymbolGraph:
		r, err := h.proto.GetSymbolGraph(ctx, in.Path)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(r)
	case ActionPipelines:
		minLen := in.MinLength
		if minLen <= 0 {
			minLen = 3
		}
		r, err := h.proto.DetectPipelines(ctx, in.Path, minLen, in.CacheKey)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(r)
	case ActionMesh:
		return h.handleMesh(ctx, in)
	default:
		return nil, nil, fmt.Errorf("%w %q", ErrUnknownAction, in.Action)
	}
}

func (h *handler) handleMesh(ctx context.Context, in *analysisInput) (*sdkmcp.CallToolResult, any, error) {
	mesh, err := h.proto.GetMesh(ctx, in.Path, in.CacheKey)
	if err != nil {
		return nil, nil, err
	}

	view := in.MeshView
	if view == "" {
		view = "full"
	}

	switch view {
	case "full":
		return jsonResult(mesh)
	case "neighborhood":
		if in.FQN == "" {
			return nil, nil, ErrMeshFQNRequired
		}
		hops := in.Hops
		if hops <= 0 {
			hops = 1
		}
		return jsonResult(mesh.NeighborhoodWeighted(in.FQN, hops))
	case "distance":
		if in.From == "" || in.To == "" {
			return nil, nil, ErrMeshFromToRequired
		}
		return jsonResult(mesh.Distance(in.From, in.To))
	case "boundaries":
		minW := 0.5 // default: internal edges only (cross-component + same-component)
		if in.MinWeight != nil {
			minW = *in.MinWeight
		}
		return jsonResult(mesh.BoundariesMinWeight(minW))
	case "aggregate":
		level := oculus.MeshPackage // default
		switch in.Level {
		case "symbol":
			level = oculus.MeshSymbol
		case "file":
			level = oculus.MeshFile
		case "package":
			level = oculus.MeshPackage
		case "component":
			level = oculus.MeshComponent
		}
		return jsonResult(mesh.Aggregate(level))
	default:
		return nil, nil, fmt.Errorf("%w %q (use: full, neighborhood, distance, boundaries, aggregate)", ErrUnknownMeshView, view)
	}
}

func (h *handler) handleClinic(ctx context.Context, _ *sdkmcp.CallToolRequest, in clinicInput) (*sdkmcp.CallToolResult, any, error) { //nolint:gocritic
	switch in.Action {
	case ActionPatternScan:
		r, err := h.proto.GetPatternScan(ctx, in.Path, in.CacheKey)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(r)
	case ActionPatternCatalog:
		return jsonResult(h.proto.GetPatternCatalog(in.Filter))
	case ActionHexaValidate:
		r, err := h.proto.GetHexaValidation(ctx, in.Path, in.CacheKey)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(r)
	case ActionSOLIDScan:
		r, err := h.proto.GetSOLIDScan(ctx, in.Path, in.CacheKey)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(r)
	case ActionSymbolQuality:
		r, err := h.proto.GetSymbolQuality(ctx, in.Path, in.CacheKey)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(r)
	case ActionVocabMap:
		r, err := h.proto.GetVocabMap(ctx, in.Path, in.CacheKey)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(r)
	case ActionBloaterScan:
		r, err := h.proto.GetBloaterScan(ctx, in.Path, in.CacheKey)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(r)
	default:
		return nil, nil, fmt.Errorf("%w %q", ErrUnknownAction, in.Action)
	}
}

// --- Constraint handler (table-driven) ---

func (h *handler) dispatchAnalysisLookup(ctx context.Context, in *analysisInput) (*sdkmcp.CallToolResult, any, error) {
	switch in.Action {
	case ActionComponent:
		r, err := h.proto.GetComponentDetail(ctx, in.Path, in.Component, in.CacheKey)
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
	default: // ActionQuery
		r, err := h.proto.AnswerQuery(ctx, in.Path, in.Query, in.CacheKey)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(r)
	}
}

func (h *handler) handleConstraint(ctx context.Context, _ *sdkmcp.CallToolRequest, in constraintInput) (*sdkmcp.CallToolResult, any, error) { //nolint:gocritic
	switch in.Action {
	case ActionBlastRadius:
		r, err := h.proto.GetBlastRadius(ctx, in.Path, in.Files, in.Since, in.CacheKey)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(r)
	case ActionImportDirection, ActionTrustBoundaries, ActionBudgets,
		ActionModDependencies, ActionInterfaceMetrics:
		return h.dispatchConstraintSimple(ctx, &in)
	case ActionSymbolBlast:
		r, err := h.proto.GetSymbolBlastRadius(ctx, in.Path, in.Symbol, in.CacheKey)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(r)
	case ActionLeverage:
		r, err := h.proto.GetLeverage(ctx, in.Path, in.Component, in.CacheKey)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(r)
	case ActionCoverage:
		r, err := h.proto.GetCoverage(ctx, in.Path, in.Threshold)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(r)
	case ActionAPISurface:
		r, err := h.proto.GetAPISurface(ctx, in.Path, in.Trusted, in.CacheKey)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(r)
	case ActionConventions:
		r, err := h.proto.GetConventions(ctx, in.Path)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(r)
	case ActionGaps:
		r, err := h.proto.GetGaps(ctx, in.Path)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(r)
	case ActionConsolidate:
		r, err := h.proto.GetConsolidation(ctx, in.Path, in.CacheKey)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(r)
	default:
		return nil, nil, fmt.Errorf("%w %q", ErrUnknownAction, in.Action)
	}
}

// --- Refactor handler ---

func (h *handler) dispatchConstraintSimple(ctx context.Context, in *constraintInput) (*sdkmcp.CallToolResult, any, error) {
	switch in.Action {
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
	case ActionBudgets:
		r, err := h.proto.GetBudgets(ctx, in.Path, in.CacheKey)
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
	default: // ActionInterfaceMetrics
		r, err := h.proto.GetInterfaceMetrics(ctx, in.Path, in.CacheKey)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(r)
	}
}

func (h *handler) handleRefactor(ctx context.Context, _ *sdkmcp.CallToolRequest, in refactorInput) (*sdkmcp.CallToolResult, any, error) { //nolint:gocritic
	switch in.Action {
	case ActionCrossRepo:
		r, err := h.proto.GetCrossRepo(ctx, in.Path, in.PathB, in.CacheKey, in.CacheKeyB)
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
	case ActionWhatIf:
		r, err := h.proto.GetWhatIf(ctx, in.Path, in.Moves, in.CacheKey)
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
		return nil, nil, fmt.Errorf("%w %q", ErrUnknownAction, in.Action)
	}
}

// --- Codograph sub-handlers ---

func (h *handler) handleScanProject(ctx context.Context, in *codographActionInput) (*sdkmcp.CallToolResult, any, error) {
	result, err := h.proto.ScanProject(ctx, in.Path, engine.ScanOpts{
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
		data, jsonErr := arch.RenderJSON(result.Report)
		if jsonErr != nil {
			return nil, nil, fmt.Errorf("render JSON: %w", jsonErr)
		}
		return text(string(data)), nil, nil
	default:
		driftInfo := h.proto.CheckDriftOnScan(ctx, in.Path, result.Report)
		return text(engine.RenderScanSummary(result, driftInfo)), nil, nil
	}
}

func (h *handler) handleCodographRemote(ctx context.Context, in *codographActionInput) (*sdkmcp.CallToolResult, any, error) {
	result, err := h.proto.CodographRemote(ctx, in.URL, engine.RemoteOpts{
		Ref: in.Ref, Keep: in.Keep, Depth: in.Depth,
		ChurnDays: in.ChurnDays, Budget: in.Budget, Intent: in.Intent,
	})
	if err != nil {
		return nil, nil, err
	}
	data, jsonErr := arch.RenderJSON(result.Report)
	if jsonErr != nil {
		return nil, nil, fmt.Errorf("render JSON: %w", jsonErr)
	}
	out := fmt.Sprintf("%s\n\ncache_key: %s", string(data), result.CacheKey)
	return text(out), nil, nil
}

func (h *handler) handleGetCodographHistory(ctx context.Context, in *codographActionInput) (*sdkmcp.CallToolResult, any, error) {
	if in.Diff {
		result, err := h.proto.DiffCodographs(ctx, in.Path)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(result)
	}
	entries, err := h.proto.GetHistory(ctx, in.Path, in.Last)
	if err != nil {
		return nil, nil, fmt.Errorf("list history: %w", err)
	}
	return jsonResult(entries)
}

func (h *handler) handleDiffBranches(ctx context.Context, in *codographActionInput) (*sdkmcp.CallToolResult, any, error) {
	r, err := h.proto.DiffBranches(ctx, in.Path, in.BranchA, in.BranchB)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(r)
}

// --- Analysis sub-handlers ---

func (h *handler) handleCoupling(ctx context.Context, path, sortBy string, topN int, view string, churnDays int, component, cacheKey string) (*sdkmcp.CallToolResult, any, error) {
	switch view {
	case ViewHotSpots:
		spots, err := h.proto.GetHotSpots(ctx, path, churnDays, topN, cacheKey)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(spots)
	case ViewEdges:
		result, err := h.proto.GetEdgeList(ctx, path, component, cacheKey)
		if err != nil {
			return nil, nil, err
		}
		return text(result), nil, nil
	default:
		result, err := h.proto.GetCouplingTable(ctx, path, sortBy, topN, cacheKey)
		if err != nil {
			return nil, nil, err
		}
		return text(result), nil, nil
	}
}

func (h *handler) handleCycles(ctx context.Context, path string, layers []string, cacheKey, format string) (*sdkmcp.CallToolResult, any, error) {
	report, err := h.proto.GetCycles(ctx, path, layers, cacheKey)
	if err != nil {
		return nil, nil, err
	}
	if format == FormatSummary {
		return text(renderCyclesSummary(report)), nil, nil
	}
	return jsonResult(report)
}

func (h *handler) handleViolations(ctx context.Context, path string, layers []string, cacheKey, format string) (*sdkmcp.CallToolResult, any, error) {
	report, err := h.proto.GetViolations(ctx, path, layers, cacheKey)
	if err != nil {
		return nil, nil, err
	}
	if format == FormatSummary {
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

const maxSummaryCycles = 3
const maxSummaryViolations = 3

func renderCyclesSummary(r *engine.CycleReport) string {
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

// --- Diagram handler ---

func (h *handler) handleRenderDiagram(ctx context.Context, _ *sdkmcp.CallToolRequest, in diagramInput) (*sdkmcp.CallToolResult, any, error) { //nolint:gocritic
	path := in.Path
	if path == "" && len(h.proto.Workspaces()) > 0 {
		path = h.proto.Workspaces()[0]
	}

	report, err := h.resolveDiagramReport(ctx, path, in)
	if err != nil {
		return nil, nil, err
	}

	input := diagramcore.Input{Report: report, Root: path}
	h.enrichDiagramInput(ctx, path, in.Type, report, &input)

	opts := diagramcore.Options{
		Type: in.Type, Scope: in.Scope, Depth: in.Depth,
		TopN: in.TopN, Entry: in.Entry, ExportedOnly: in.ExportedOnly,
		Theme: in.Theme, Enrich: in.Enrich,
	}

	switch in.Format {
	case FormatFacts:
		return text(diagram.RenderFacts(report)), nil, nil
	case FormatBoth:
		facts := diagram.RenderFacts(report)
		mermaid, renderErr := diagram.Render(input, opts)
		if renderErr != nil {
			return nil, nil, renderErr
		}
		return text(mermaid + "\n\n" + facts), nil, nil
	default:
		out, renderErr := diagram.Render(input, opts)
		if renderErr != nil {
			return nil, nil, renderErr
		}
		return text(out), nil, nil
	}
}

func (h *handler) resolveDiagramReport(ctx context.Context, path string, in diagramInput) (*arch.ContextReport, error) {
	if in.CacheKey != "" {
		return h.proto.GetCachedReport(in.CacheKey)
	}
	intent := DiagramMinIntent[in.Type]
	result, err := h.proto.ScanProject(ctx, path, engine.ScanOpts{Depth: in.Depth, Intent: intent})
	if err != nil {
		return nil, err
	}
	return result.Report, nil
}

func (h *handler) enrichDiagramInput(ctx context.Context, path, diagramType string, report *arch.ContextReport, input *diagramcore.Input) {
	if path == "" {
		return
	}
	pool := h.proto.Pool()
	switch diagramType {
	case DiagramClasses, DiagramSequence, DiagramER, DiagramInterfaces, DiagramHexa:
		input.Analyzer = analyzer.NewFallback(path, pool)
	}
	if diagramType == DiagramHexa {
		fa := analyzer.NewFallback(path, pool)
		classes, _ := fa.Classes(ctx, path)
		hexaClass := clinichexa.ComputeHexaClassification(report.Architecture.Services, report.Architecture.Edges, classes)
		input.HexaRoles = make(map[string]string, len(hexaClass.Components))
		for _, c := range hexaClass.Components {
			input.HexaRoles[c.Name] = string(c.Role)
		}
	}
	if diagramType == DiagramChurn {
		input.History, _ = h.proto.GetHistory(ctx, path, 20)
	}
	switch diagramType {
	case DiagramDataflow, DiagramCallgraph, DiagramState:
		input.DeepAnalyzer = analyzer.CachedDeepFallback(path, pool)
	case DiagramSymbolDSM:
		input.SymbolGraph, _ = h.proto.GetSymbolGraph(ctx, path)
	}
}

type diagramInput struct {
	Path         string `json:"path" jsonschema:"required,absolute path to local repository"`
	Type         string `json:"type" jsonschema:"required,diagram type: dependency, c4, coupling, churn, layers, tree, classes, sequence, er, interfaces, hexa, zones, symbol_dsm"`
	Scope        string `json:"scope,omitempty" jsonschema:"limit to sub-package"`
	Depth        int    `json:"depth,omitempty" jsonschema:"grouping depth"`
	TopN         int    `json:"top_n,omitempty" jsonschema:"limit to top N components"`
	Entry        string `json:"entry,omitempty" jsonschema:"entry point for sequence/callgraph"`
	ExportedOnly bool   `json:"exported_only,omitempty" jsonschema:"exported symbols only (class diagrams)"`
	Enrich       string `json:"enrich,omitempty" jsonschema:"node label metrics: loc, fan_in, churn"`
	Theme        string `json:"theme,omitempty" jsonschema:"theme: light, dark, natural"`
	Format       string `json:"format,omitempty" jsonschema:"output: mermaid, facts, both"`
	CacheKey     string `json:"cache_key,omitempty" jsonschema:"cache key from scan_remote"`
}

// --- Lint handler ---

type lintInput struct {
	Path     string   `json:"path" jsonschema:"required,absolute path to local repository"`
	CacheKey string   `json:"cache_key,omitempty" jsonschema:"cache key from scan_remote"`
	Linters  []string `json:"linters,omitempty" jsonschema:"linter categories to enable: hexa, solid, pattern, symbol, layer, budget"`
	Since    string   `json:"since,omitempty" jsonschema:"git ref for incremental mode"`
	Format   string   `json:"format,omitempty" jsonschema:"output format: json, summary"`
}

func (h *handler) handleLint(ctx context.Context, _ *sdkmcp.CallToolRequest, in lintInput) (*sdkmcp.CallToolResult, any, error) { //nolint:gocritic
	path := in.Path
	if path == "" && len(h.proto.Workspaces()) > 0 {
		path = h.proto.Workspaces()[0]
	}

	// Resolve the scan report.
	var report *arch.ContextReport
	if in.CacheKey != "" {
		r, err := h.proto.GetCachedReport(in.CacheKey)
		if err != nil {
			return nil, nil, err
		}
		report = r
	} else {
		result, err := h.proto.ScanProject(ctx, path, engine.ScanOpts{
			Intent: string(arch.IntentHealth),
		})
		if err != nil {
			return nil, nil, err
		}
		report = result.Report
	}

	// Load optional .locus.yaml config.
	ds, _ := config.LoadLocusConfig(path)

	// Parse linter categories.
	var categories []lint.Category
	for _, s := range in.Linters {
		s = strings.TrimSpace(s)
		if s != "" {
			categories = append(categories, lint.Category(s))
		}
	}

	// Resolve changed components for incremental mode.
	var changed []string
	if in.Since != "" {
		files, gitErr := gitpkg.ChangedFilesSince(path, in.Since)
		if gitErr == nil {
			changed = lintFilesToComponents(files)
		}
	}

	lintReport := lint.Run(report, lint.RunOpts{
		EnabledLinters:    categories,
		DesiredState:      ds,
		Root:              path,
		ChangedComponents: changed,
	})

	if in.Format == FormatSummary {
		return text(lintReport.Summary), nil, nil
	}
	return jsonResult(lintReport)
}

// lintFilesToComponents deduplicates file paths into component directories.
func lintFilesToComponents(files []string) []string {
	seen := make(map[string]bool, len(files))
	var result []string
	for _, f := range files {
		dir := filepath.Dir(f)
		if dir == "." {
			continue
		}
		if !seen[dir] {
			seen[dir] = true
			result = append(result, dir)
		}
	}
	return result
}

// --- Triage handler ---

type triageInput struct {
	Intent string `json:"intent" jsonschema:"required,what you want to do"`
	Path   string `json:"path,omitempty" jsonschema:"optional repository path"`
}

func (h *handler) handleTriage(_ context.Context, _ *sdkmcp.CallToolRequest, in triageInput) (*sdkmcp.CallToolResult, any, error) { //nolint:gocritic
	if in.Intent == "" {
		return nil, nil, ErrIntentRequired
	}
	return jsonResult(h.reg.Triage(in.Intent, in.Path))
}

// --- Helpers ---

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
		slog.LogAttrs(ctx, slog.LevelInfo, "tool call started", slog.String(logKeyTool, tool))
		start := time.Now()
		result, out, err := h(ctx, req, in)
		elapsed := time.Since(start)
		if err != nil {
			slog.LogAttrs(ctx, slog.LevelError, "tool call failed", slog.String(logKeyTool, tool), slog.Duration(logKeyElapsed, elapsed), slog.Any(logKeyError, err))
		} else {
			slog.LogAttrs(ctx, slog.LevelInfo, "tool call done", slog.String(logKeyTool, tool), slog.Duration(logKeyElapsed, elapsed))
		}
		return result, out, err
	}
}
