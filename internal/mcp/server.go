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
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"

	batterymcp "github.com/dpopsuev/battery/mcp"
	"github.com/dpopsuev/locus/internal/store"
	"github.com/dpopsuev/oculus/v3/analyzer"
	"github.com/dpopsuev/oculus/v3/arch"
	"github.com/dpopsuev/oculus/v3/clinic"
	"github.com/dpopsuev/oculus/v3/diagram"
	diagramcore "github.com/dpopsuev/oculus/v3/diagram/core"
	"github.com/dpopsuev/oculus/v3/engine"
	"github.com/dpopsuev/oculus/v3/lsp"
	"github.com/dpopsuev/oculus/v3/triage"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Log key constants for structured logging.
const (
	logKeyTool      = "tool"
	logKeyElapsed   = "elapsed"
	logKeyError     = "error"
	logKeyAction    = "action"
	logKeyPath      = "path"
	logKeySHA       = "sha"
	logKeyServices  = "services"
	logKeyEdges     = "edges"
	logKeyCacheKey  = "cache_key"
	logKeyCacheHit  = "cache_hit"
)

// Sentinel errors for input validation.
var (
	ErrUnknownCodographAction = errors.New("unknown codograph action")
	ErrUnknownAction          = errors.New("unknown action")
	ErrIntentRequired         = errors.New("intent is required")
	ErrMeshFQNRequired        = errors.New("fqn is required for mesh neighborhood")
	ErrMeshFromToRequired     = errors.New("from and to are required for mesh distance")
	ErrUnknownMeshView        = errors.New("unknown mesh view")
	ErrSymbolRequired         = errors.New("symbol is required")
	ErrConvergenceMinSymbols  = errors.New("convergence requires at least 2 symbols")
	ErrExplainEdgeParams      = errors.New("explain_edge requires symbol (source FQN) and query (target FQN)")
	ErrSymbolDiffParams       = errors.New("symbol_diff requires before_sha and after_sha")
	ErrUnknownContextAction   = errors.New("unknown context action")
	ErrQueryRequired          = errors.New("symbol_search requires a non-empty symbol or file; provide symbol= (name pattern) or file= (absolute path)")
	ErrCallersAtFileRequired  = errors.New("callers_at requires file=")
)

// defaultAnalysisTimeout caps every analysis dispatch.
// Operators can override via LOCUS_ANALYSIS_TIMEOUT (e.g. "10m").
const defaultAnalysisTimeout = 5 * time.Minute

// defaultScanTimeout caps scan_local. Scans are I/O-bound and can take much
// longer than analysis on large repos; progress heartbeats keep clients alive
// during the wait. Operators can override via LOCUS_SCAN_TIMEOUT (e.g. "60m").
const defaultScanTimeout = 30 * time.Minute

// scanProgressInterval is how often a progress heartbeat is sent to the client
// while a scan_local is running.
const scanProgressInterval = 10 * time.Second

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
	ActionWarm            = "warm"
)

// Analysis actions.
const (
	ActionDeps         = "deps"
	ActionImpact       = "impact"
	ActionCoupling     = "coupling"
	ActionCycles       = "cycles"
	ActionViolations   = "violations"
	ActionCallers      = "callers"
	ActionCallersAt    = "callers_at"
	ActionComponent    = "component"
	ActionSearch       = "search"
	ActionQuery        = "query"
	ActionPreset       = "preset"
	ActionScanDiff      = "scan_diff"
	ActionComponentDiff    = "component_diff"
	ActionMigrationOverlay = "migration_overlay"
	ActionRegisterMirror   = "register_mirror"
	ActionListMirrors      = "list_mirrors"
	ActionRiskScores   = "risk_scores"
	ActionSymbolSearch = "symbol_search"
	ActionCallees      = "callees"
	ActionCallPath     = "call_path"
	ActionSymbolGraph  = "symbol_graph"
	ActionPipelines    = "pipelines"
	ActionMesh         = "mesh"
	ActionProbe        = "probe"
	ActionScenario     = "scenario"
	ActionConvergence  = "convergence"
	ActionIsolate      = "isolate"
	ActionDiagnose     = "diagnose"
	ActionIslands      = "islands"
	ActionExplainEdge  = "explain_edge"
	ActionSymbolDiff   = "symbol_diff"
	// Intra-component queries (NED-14, NED-16, NED-17).
	ActionIntraDeps    = "intra_deps"
	ActionIntraCoupling = "intra_coupling"
	ActionTypeUsages   = "type_usages"
	// Merged from standalone tools.
	ActionBook         = "book"
	ActionContextRead  = "context_read"
	ActionContextWrite = "context_write"
	ActionTriage       = "triage"
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

// NewServer creates the Locus MCP server with tool handlers.
func NewServer(s store.Store, workspaceRoots []string, version string, pool ...lsp.Pool) (*batterymcp.Server, *triage.Registry, *engine.Engine) {
	proto := engine.New(s, workspaceRoots, pool...)
	bsrv := batterymcp.NewServer("locus", version).
		WithInstructions("Scan first (codograph scan_local), then walk (analysis probe/scenario/convergence/isolate). Results cached by SHA; pass cache_key to skip rescanning.")
	srv := bsrv.SDK()
	h := &handler{proto: proto, sproto: proto, ingestURL: os.Getenv("LOCUS_INGEST_URL")}
	// Record binary mtime at startup for stale binary detection.
	if exe, err := os.Executable(); err == nil {
		h.binPath = exe
		if info, err := os.Stat(exe); err == nil {
			h.binStart = info.ModTime()
		}
	}
	reg := triage.New()

	triage.AddTool(reg, srv, triage.ToolMeta{
		Name:        "codograph",
		Description: "Scan and compare repository architectures. Returns cache_key for downstream tools.",
		Keywords:   []string{"scan", "architecture", "overview", "remote", "github", "history", "diff", "branch", "compare", "cache", "flush", "rescan", "stale", "status", "desired", "rules", "layers"},
		Categories: []string{"architecture", "onboarding", "comparison"},
		Rationale:  map[string]string{"architecture": "Full codebase overview", "onboarding": "Best first step"},
		Priority:   1,
	}, noOut(h.handleCodograph))

	triage.AddTool(reg, srv, triage.ToolMeta{
		Name:        "analysis",
		Description: "Walk the symbol graph, query the knowledge book, and triage tool selection.",
		Keywords:   []string{"depend", "import", "impact", "coupling", "fan", "cycle", "circular", "caller", "callee", "call", "who", "symbol", "function", "find", "component", "pipeline", "data flow", "chain", "risk", "risky", "dangerous", "health", "review", "onboarding", "preset", "mesh", "zoom", "probe", "scenario", "trace", "convergence", "meet", "isolate", "disconnect", "break", "diagnose", "islands", "dead code", "unreachable", "explain", "snippet", "diff", "changed"},
		Categories: []string{"dependencies", "architecture"},
		Rationale:  map[string]string{"dependencies": "Component-level dependency analysis and cycle detection"},
		Priority:   2,
	}, noOut(h.handleAnalysis))

	triage.AddTool(reg, srv, triage.ToolMeta{
		Name:        "render_diagram",
		Description: "Render a Mermaid diagram.",
		Keywords:    []string{"diagram", "visual", "mermaid", "chart", "graph", "class", "sequence", "er", "draw", "render", "dataflow", "callgraph", "state", "layers", "c4", "tree", "hexa", "zones", "dsm"},
		Categories:  []string{"visualization"},
		Rationale:   map[string]string{"visualization": "Generate Mermaid charts from architecture data"},
		Priority:    1,
	}, noOut(h.handleRenderDiagram))

	h.reg = reg

	return bsrv, reg, proto
}

// scanProto is a narrow interface over the two engine methods used by
// handleScanProject. Keeping it separate from *engine.Engine lets tests
// inject a fake without wiring up a real store+cache.
type scanProto interface {
	ScanProject(ctx context.Context, path string, opts engine.ScanOpts) (*engine.ScanResult, error)
	CheckDriftOnScan(ctx context.Context, path string, report *arch.ContextReport) string
}

type handler struct {
	proto    *engine.Engine
	sproto   scanProto  // scan+drift; defaults to proto, replaceable in tests
	reg      *triage.Registry
	binStart time.Time // mtime of binary at startup, for stale detection
	binPath  string    // path to running binary

	// scanGroup deduplicates concurrent scan_local calls for the same workspace:
	// if N sessions call scan_local on the same path+intent simultaneously, only
	// one ctags process is spawned and all callers receive the same result when
	// it completes.
	scanGroup singleflight.Group

	// scanTotal counts every scan_local dispatch (including singleflight dedupes).
	// Monotonically increasing; non-zero value in logs signals repeated cold scans.
	scanTotal atomic.Int64

	// lastScannedPath is the resolved path of the most recent successful scan_local.
	// When analysis tools are called with no path= and no cache_key=, this is used
	// instead of falling back to the configured workspace root (LCS-BUG-97).
	lastScannedPath atomic.Value // stores string

	ingestURL string // if set, POST scan results to this Scribe ingest endpoint
}

// --- Input structs (per-tool, only relevant fields) ---

type codographActionInput struct {
	Action string `json:"action" jsonschema:"required,scan_local|scan_remote|history|diff|set_desired_state|get_desired_state|accept_violation|status|flush|warm"`

	Path            string   `json:"path,omitempty"`
	Depth           int      `json:"depth,omitempty"`
	ChurnDays       int      `json:"churn_days,omitempty" jsonschema:"git history window in days"`
	IncludeExternal bool     `json:"include_external,omitempty"`
	IncludeTests    bool     `json:"include_tests,omitempty"`
	IncludeCoverage bool     `json:"include_coverage,omitempty"`
	Budget          int      `json:"budget,omitempty" jsonschema:"max components in output"`
	Format          string   `json:"format,omitempty" jsonschema:"json or summary"`
	Intent          string   `json:"intent,omitempty" jsonschema:"architecture|coupling|health|full"`
	Scanner         string   `json:"scanner,omitempty" jsonschema:"auto|go|packages|rust|typescript|lsp|ctags|composite"`
	FileGranularity bool     `json:"file_granularity,omitempty" jsonschema:"TypeScript: file-level components"`
	Since           string   `json:"since,omitempty" jsonschema:"git ref for incremental scan"`
	URL             string   `json:"url,omitempty" jsonschema:"GitHub URL (scan_remote)"`
	Ref             string   `json:"ref,omitempty" jsonschema:"git ref (scan_remote)"`
	Keep            bool     `json:"keep,omitempty" jsonschema:"keep clone (scan_remote)"`
	Last            int      `json:"last,omitempty"`
	Diff            bool     `json:"diff,omitempty" jsonschema:"compare latest two scans"`
	Layers          []string `json:"layers,omitempty" jsonschema:"ordered layer names"`
	BranchA         string   `json:"branch_a,omitempty"`
	BranchB         string   `json:"branch_b,omitempty"`
	Component       string   `json:"component,omitempty"`
	Principle       string   `json:"principle,omitempty"`
	Reason          string   `json:"reason,omitempty"`
}

type analysisInput struct {
	Action    string   `json:"action" jsonschema:"required,deps|impact|coupling|cycles|violations|callers|callers_at|component|search|query|preset|scan_diff|risk_scores|symbol_search|callees|call_path|symbol_graph|pipelines|mesh|probe|scenario|convergence|isolate|diagnose|islands|explain_edge|symbol_diff|intra_deps|intra_coupling|type_usages|book|context_read|context_write|triage"`
	Symbols   []string `json:"symbols,omitempty" jsonschema:"FQNs for convergence"`
	Stress    bool     `json:"stress,omitempty" jsonschema:"enrich scenario nodes with fan-out"`
	Path      string   `json:"path,omitempty"`
	CacheKey  string   `json:"cache_key,omitempty" jsonschema:"cache key from scan_remote"`
	Component string   `json:"component,omitempty"`
	Symbol    string   `json:"symbol,omitempty" jsonschema:"name pattern (callers/symbol_search)"`
	SortBy    string   `json:"sort_by,omitempty"`
	TopN      int      `json:"top_n,omitempty"`
	View      string   `json:"view,omitempty" jsonschema:"hot_spots|edges"`
	ChurnDays int      `json:"churn_days,omitempty" jsonschema:"git history window in days"`
	Layers    []string `json:"layers,omitempty" jsonschema:"ordered layer names"`
	Format    string   `json:"format,omitempty" jsonschema:"json|summary"`
	Preset    string   `json:"preset,omitempty" jsonschema:"architecture_review|health_check|onboarding|pre_pr|full_clinic|code_health"`
	Query     string   `json:"query,omitempty" jsonschema:"component name (search) or natural language question (query)"`
	BeforeSHA string   `json:"before_sha,omitempty" jsonschema:"earlier SHA (scan_diff)"`
	AfterSHA  string   `json:"after_sha,omitempty" jsonschema:"later SHA (scan_diff)"`
	MinLength int      `json:"min_length,omitempty"`
	MeshView  string   `json:"mesh_view,omitempty" jsonschema:"full|neighborhood|distance|boundaries|aggregate"`
	Level     string   `json:"level,omitempty" jsonschema:"symbol|file|package|component"`
	FQN       string   `json:"fqn,omitempty" jsonschema:"fully qualified symbol name"`
	Hops      int      `json:"hops,omitempty" jsonschema:"neighborhood radius in hops"`
	From      string   `json:"from,omitempty" jsonschema:"source FQN (mesh distance)"`
	To        string   `json:"to,omitempty" jsonschema:"target FQN (mesh distance)"`
	MinWeight *float64 `json:"min_weight,omitempty" jsonschema:"edge weight filter, default 0.1"`
	Detail    string   `json:"detail,omitempty" jsonschema:"shallow (default) or full"`
	File      string   `json:"file,omitempty" jsonschema:"abs path: filters symbol_search; required by callers_at"`
	Line      int      `json:"line,omitempty" jsonschema:"1-based line (callers_at)"`
	Char      int      `json:"char,omitempty" jsonschema:"0-based char offset (callers_at)"`
	Keywords  []string `json:"keywords,omitempty" jsonschema:"keywords for book query"`
	Scope     string   `json:"scope,omitempty" jsonschema:"project|module|file|symbol"`
	Target    string   `json:"target,omitempty" jsonschema:"package, file path, or FQN"`
	Content   string   `json:"content,omitempty" jsonschema:"content to write (context_write)"`
	Intent    string   `json:"intent,omitempty" jsonschema:"natural language intent (triage)"`
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

// stalenessWarning checks whether the scan referenced by cacheKey is behind
// the current HEAD of path. Returns a non-empty warning string when stale.
// Fails open (returns "") when HEAD cannot be resolved or the handler has no
// proto (test mode).
func (h *handler) stalenessWarning(path, cacheKey string) string {
	if h.proto == nil || cacheKey == "" {
		return ""
	}
	// cache_key format: path@sha[-intent]  (e.g. /repo@abc123-full)
	atIdx := strings.LastIndex(cacheKey, "@")
	if atIdx < 0 {
		return ""
	}
	shaPart := cacheKey[atIdx+1:]
	if dashIdx := strings.Index(shaPart, "-"); dashIdx >= 0 {
		shaPart = shaPart[:dashIdx]
	}
	if shaPart == "" {
		return ""
	}
	resolvedPath := h.proto.ResolvePath(path)
	head := h.proto.ResolveHEAD(resolvedPath)
	if head == "" || head == shaPart {
		return ""
	}
	return fmt.Sprintf("Warning: cached scan is stale (scan@%s, HEAD=%s). Run scan_local to refresh.\n", shaPart, head)
}

// pathFromCacheKey extracts the repo path from a cache_key of the form
// path@sha[-intent][-file]. Uses LastIndex so intent suffixes like -full
// do not shift the split (unlike assuming a fixed 40-char SHA).
func pathFromCacheKey(cacheKey string) string {
	atIdx := strings.LastIndex(cacheKey, "@")
	if atIdx <= 0 {
		return ""
	}
	return cacheKey[:atIdx]
}

// prependWarning prepends a warning string to the first text content item of
// result. Returns a new CallToolResult; the original is not modified.
func prependWarning(warn string, r *sdkmcp.CallToolResult) *sdkmcp.CallToolResult {
	if warn == "" || r == nil || len(r.Content) == 0 {
		return r
	}
	out := &sdkmcp.CallToolResult{Content: make([]sdkmcp.Content, len(r.Content))}
	copy(out.Content, r.Content)
	if tc, ok := out.Content[0].(*sdkmcp.TextContent); ok {
		out.Content[0] = &sdkmcp.TextContent{Text: warn + tc.Text}
	}
	return out
}

// --- Codograph handler ---

func (h *handler) handleCodograph(ctx context.Context, req *sdkmcp.CallToolRequest, in codographActionInput) (*sdkmcp.CallToolResult, any, error) { //nolint:gocritic
	switch in.Action {
	case ActionScanLocal:
		return h.handleScanProject(ctx, req, &in)
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
	case ActionWarm:
		if err := h.proto.WarmLSP(ctx, in.Path); err != nil {
			return nil, nil, err
		}
		return text("LSP index warmed for " + in.Path), nil, nil
	default:
		return nil, nil, fmt.Errorf("%w %q", ErrUnknownCodographAction, in.Action)
	}
}

// --- Analysis handler ---

func (h *handler) handleAnalysis(ctx context.Context, _ *sdkmcp.CallToolRequest, in analysisInput) (result *sdkmcp.CallToolResult, raw any, retErr error) { //nolint:gocritic
	// Wrapping ctx preserves transport-level cancellations: if the transport
	// fires before our deadline, callers still see the shorter deadline.
	ctx, cancel := context.WithTimeout(ctx, analysisTimeout())
	defer cancel()

	// Prefer an explicit path; otherwise recover it from cache_key
	// (path@sha[-intent][-file]). Intent suffixes like -full break the old
	// "SHA is exactly 40 chars" parse and left Path empty — ProbeSymbol then
	// scanned workspaces[0] (dogfood: $HOME → 20GB+ RSS).
	if in.Path == "" && in.CacheKey != "" {
		if p := pathFromCacheKey(in.CacheKey); p != "" {
			in.Path = p
		}
	}

	// When no path and no cache_key are provided, resolve the default project.
	// Prefer a workspace root that is a registered (previously scanned) project
	// over lastScannedPath, which may point to a sibling project scanned later.
	// Fall back to lastScannedPath only when no workspace root matches (LCS-BUG-97).
	if in.Path == "" && in.CacheKey == "" {
		if resolved := h.resolveDefaultProject(ctx); resolved != "" {
			in.Path = resolved
		} else if v := h.lastScannedPath.Load(); v != nil {
			if p, ok := v.(string); ok && p != "" {
				in.Path = p
			}
		}
	}

	staleWarn := h.stalenessWarning(in.Path, in.CacheKey)
	defer func() {
		if staleWarn != "" && result != nil && retErr == nil {
			result = prependWarning(staleWarn, result)
		}
	}()

	h.logAnalysisEntry(ctx, &in)

	op := FindAnalysisOp(in.Action)
	if op == nil {
		return nil, nil, fmt.Errorf("%w %q", ErrUnknownAction, in.Action)
	}
	rawIn, _ := json.Marshal(in)
	opResult, err := op.Run(ctx, h, rawIn)
	if err != nil {
		return nil, nil, err
	}
	return text(opResult.Text), nil, nil
}

// --- Codograph sub-handlers ---

func (h *handler) handleScanProject(ctx context.Context, req *sdkmcp.CallToolRequest, in *codographActionInput) (*sdkmcp.CallToolResult, any, error) {
	ctx, cancel := context.WithTimeout(ctx, scanTimeout())
	defer cancel()

	effectiveScanner := in.Scanner
	scanN := h.scanTotal.Add(1)
	resolvedPath, sha, gitRepo, scanStart := h.probeScanCache(ctx, scanN, in.Path)



	sfKey := in.Path + "\x00" + in.Intent + "\x00" + effectiveScanner
	startScanHeartbeat(ctx, req, resolvedPath, in.Intent, scanStart)

	v, err, _ := h.scanGroup.Do(sfKey, func() (any, error) {
		r, scanErr := h.sproto.ScanProject(ctx, in.Path, engine.ScanOpts{
			Depth: in.Depth, ChurnDays: in.ChurnDays,
			IncludeExternal: in.IncludeExternal, IncludeTests: in.IncludeTests,
			IncludeCoverage: in.IncludeCoverage, Budget: in.Budget,
			Intent: in.Intent, Since: in.Since,
			Scanner:           effectiveScanner,
			TSFileGranularity: in.FileGranularity,
		})
		if scanErr != nil {
			return nil, scanErr
		}
		drift := h.sproto.CheckDriftOnScan(ctx, in.Path, r.Report)
		return &sfPayload{scanResult: r, driftText: engine.RenderScanSummary(r, drift)}, nil
	})

	slog.LogAttrs(ctx, slog.LevelInfo, "scan_local: completed",
		slog.Int64("scan_n", scanN), slog.String(logKeyPath, resolvedPath),
		slog.String(logKeySHA, sha), slog.Bool("git_repo", gitRepo),
		slog.Bool("will_cache", gitRepo), slog.Duration(logKeyElapsed, time.Since(scanStart)),
		slog.Any(logKeyError, err),
	)
	if err != nil {
		return nil, nil, err
	}
	h.lastScannedPath.Store(resolvedPath)

	if h.ingestURL != "" {
		go h.postScanToScribe(ctx, v.(*sfPayload).scanResult, resolvedPath)
	}

	res, renderErr := renderScanPayload(v.(*sfPayload), in.Format)
	return res, nil, renderErr
}

// sfPayload is the value shared between singleflight callers for a scan.
type sfPayload struct {
	scanResult *engine.ScanResult
	driftText  string
}

// probeScanCache resolves the path and SHA, logs cache disposition, and
// returns values needed by the scan dispatch and completion log.
func (h *handler) probeScanCache(ctx context.Context, scanN int64, path string) (resolvedPath, sha string, gitRepo bool, start time.Time) {
	start = time.Now()
	if h.proto == nil {
		return
	}
	resolvedPath = h.proto.ResolvePath(path)
	sha = h.proto.ResolveHEAD(resolvedPath)
	gitRepo = sha != ""
	if !gitRepo {
		slog.LogAttrs(ctx, slog.LevelWarn,
			"scan_local: workspace is not a git repo — SHA is empty; results will NOT be cached; every call re-scans",
			slog.Int64("scan_n", scanN), slog.String(logKeyPath, resolvedPath),
			slog.String("fix", "pass --workspace pointing at a git repo, or run locus serve inside one"),
		)
		return
	}
	slog.LogAttrs(ctx, slog.LevelDebug, "scan_local: cache probe",
		slog.Int64("scan_n", scanN), slog.String(logKeyPath, resolvedPath), slog.String(logKeySHA, sha),
	)
	return
}

// renderScanPayload formats a completed scan according to the requested format.
func renderScanPayload(payload *sfPayload, format string) (*sdkmcp.CallToolResult, error) {
	switch format {
	case FormatSummary:
		return text(arch.RenderMarkdown(payload.scanResult.Report)), nil
	case FormatJSON:
		data, err := arch.RenderJSON(payload.scanResult.Report)
		if err != nil {
			return nil, fmt.Errorf("render JSON: %w", err)
		}
		return text(string(data)), nil
	default:
		return text(payload.driftText), nil
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

// enrichedSymbolMatch is a SymbolMatch extended with call-graph metrics from ProbeSymbol.
// Fields sourced from ProbeResult are omitted when the symbol has no call-graph coverage.
type enrichedSymbolMatch struct {
	// Locator (always present)
	Symbol    string `json:"symbol"`
	Kind      string `json:"kind"`
	Component string `json:"component"`
	File      string `json:"file,omitempty"`
	Line      int    `json:"line,omitempty"`
	// Metrics (present when call-graph coverage exists)
	Exported        bool     `json:"exported,omitempty"`
	Params          []string `json:"params,omitempty"`
	Returns         []string `json:"returns,omitempty"`
	FanIn           int      `json:"fan_in,omitempty"`
	FanOut          int      `json:"fan_out,omitempty"`
	Instability     float64  `json:"instability,omitempty"`
	CrossPackage    int      `json:"cross_pkg_callees,omitempty"`
	Circuits        int      `json:"circuits,omitempty"`
	Boundaries      []string `json:"boundaries,omitempty"`
	CallGraphStatus string   `json:"call_graph_status,omitempty"`
}

type enrichedSymbolSearchReport struct {
	Query   string                `json:"query"`
	Matches []enrichedSymbolMatch `json:"matches"`
	Summary string                `json:"summary"`
}

const (
	symbolSearchFullLimit    = 10     // hard ceiling for detail:full — each match calls ProbeSymbol
	symbolSearchShallowLimit = 50
	detailFull               = "full" // detail param value for enriched symbol search
)

func (h *handler) symbolSearchFull(ctx context.Context, in *analysisInput, r *engine.SymbolSearchReport) (*sdkmcp.CallToolResult, any, error) {
	// Bug fix: cap is hard at symbolSearchFullLimit; if caller supplied a
	// smaller top_n honour it, but never silently exceed the ceiling.
	limit := in.TopN
	switch {
	case limit <= 0:
		limit = symbolSearchFullLimit
	case limit > symbolSearchFullLimit:
		// Caller asked for more than the hard cap — clamp and tell them.
		limit = symbolSearchFullLimit
	}

	total := len(r.Matches)
	if total > limit {
		r.Matches = r.Matches[:limit]
	}

	// Bug fix: ProbeSymbol must use the same path as the arch scan so that
	// remote scans (cache_key points to a cloned repo) stay consistent.
	// For local scans in.Path == probe path, so this is a no-op.
	probePath := in.Path
	if in.CacheKey != "" {
		if p := pathFromCacheKey(in.CacheKey); p != "" {
			probePath = p
		}
	}

	enriched := make([]enrichedSymbolMatch, 0, len(r.Matches))
	for _, m := range r.Matches {
		em := enrichedSymbolMatch{
			Symbol:    m.Symbol,
			Kind:      m.Kind,
			Component: m.Component,
			File:      m.File,
			Line:      m.Line,
		}
		if pr, err := h.proto.ProbeSymbol(ctx, probePath, m.Symbol); err == nil && pr != nil {
			em.Exported = pr.Exported
			em.Params = pr.Params
			em.Returns = pr.Returns
			em.FanIn = pr.FanIn
			em.FanOut = pr.FanOut
			em.Instability = pr.Instability
			em.CrossPackage = pr.CrossPkg
			em.Circuits = pr.Circuits
			em.Boundaries = pr.Boundaries
			em.CallGraphStatus = string(pr.CallGraphStatus)
		}
		enriched = append(enriched, em)
	}

	summary := fmt.Sprintf("%d symbol(s) matching %q (full depth, showing %d of %d)", total, in.Symbol, len(enriched), total)
	if total > limit {
		summary += fmt.Sprintf("; capped at %d — narrow your symbol or pass top_n ≤ %d", symbolSearchFullLimit, symbolSearchFullLimit)
	}
	return jsonResult(&enrichedSymbolSearchReport{Query: in.Symbol, Matches: enriched, Summary: summary})
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
		return h.proto.GetCachedReport(ctx, in.CacheKey)
	}
	intent := DiagramMinIntent[in.Type]
	result, err := h.proto.ScanProject(ctx, path, engine.ScanOpts{
		Depth:             in.Depth,
		Intent:            intent,
		TSFileGranularity: in.FileGranularity,
	})
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
		hexaClass := clinic.ComputeHexaClassification(report.Architecture.Services, report.Architecture.Edges, classes)
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
	Path            string `json:"path" jsonschema:"required"`
	Type            string `json:"type" jsonschema:"required,dependency|c4|coupling|churn|layers|tree|classes|sequence|er|interfaces|hexa|zones|symbol_dsm"`
	Scope           string `json:"scope,omitempty" jsonschema:"limit to sub-package"`
	Depth           int    `json:"depth,omitempty"`
	TopN            int    `json:"top_n,omitempty"`
	Entry           string `json:"entry,omitempty" jsonschema:"entry point for sequence/callgraph"`
	ExportedOnly    bool   `json:"exported_only,omitempty" jsonschema:"class diagrams only"`
	Enrich          string `json:"enrich,omitempty" jsonschema:"node labels: loc, fan_in, churn"`
	Theme           string `json:"theme,omitempty" jsonschema:"light|dark|natural"`
	Format          string `json:"format,omitempty" jsonschema:"mermaid|facts|both"`
	CacheKey        string `json:"cache_key,omitempty" jsonschema:"cache key from scan_remote"`
	FileGranularity bool   `json:"file_granularity,omitempty" jsonschema:"TypeScript: file-level nodes"`
}

// --- Helpers ---

func text(s string) *sdkmcp.CallToolResult {
	return &sdkmcp.CallToolResult{
		Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: s}},
	}
}

func jsonResult(data any) (*sdkmcp.CallToolResult, any, error) {
	b, _ := json.Marshal(data)
	return text(string(b)), nil, nil
}

// startScanHeartbeat fires a goroutine that emits notifications/progress every
// scanProgressInterval while the scan context is live. It is a no-op when req
// or its Session are nil (test mode, stateless transport).
func startScanHeartbeat(ctx context.Context, req *sdkmcp.CallToolRequest, path, intent string, start time.Time) {
	if req == nil || req.Session == nil {
		return
	}
	token := req.Params.GetProgressToken()
	go func() {
		ticker := time.NewTicker(scanProgressInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				elapsed := time.Since(start).Round(time.Second)
				_ = req.Session.NotifyProgress(ctx, &sdkmcp.ProgressNotificationParams{
					ProgressToken: token,
					Message:       fmt.Sprintf("scan_local running: %s (elapsed %s, intent=%s)", path, elapsed, intent),
					Progress:      elapsed.Seconds(),
				})
			}
		}
	}()
}

// analysisTimeout returns the effective per-call deadline for analysis
// dispatches.  It honours LOCUS_ANALYSIS_TIMEOUT so operators can tune it
// without recompiling (e.g. "10m" for a very large monorepo).
func analysisTimeout() time.Duration {
	if s := os.Getenv("LOCUS_ANALYSIS_TIMEOUT"); s != "" {
		if d, err := time.ParseDuration(s); err == nil && d > 0 {
			return d
		}
	}
	return defaultAnalysisTimeout
}

// scanTimeout returns the effective per-call deadline for scan_local.
// Scans are I/O-bound and warrant a much longer budget than analysis;
// progress heartbeats keep MCP clients alive during the wait.
func scanTimeout() time.Duration {
	if s := os.Getenv("LOCUS_SCAN_TIMEOUT"); s != "" {
		if d, err := time.ParseDuration(s); err == nil && d > 0 {
			return d
		}
	}
	return defaultScanTimeout
}

// resolveDefaultProject picks the right scanned project when no explicit path
// is provided. It checks workspace roots against registered (scanned) projects
// and returns the first match. If a workspace root is inside a project
// (e.g. CWD is a subdirectory), the enclosing project wins.
func (h *handler) resolveDefaultProject(ctx context.Context) string {
	status, err := h.proto.Status(ctx)
	if err != nil || len(status.Projects) == 0 {
		return ""
	}
	for _, ws := range status.Workspaces {
		var best string
		for _, p := range status.Projects {
			if ws == p.Path || strings.HasPrefix(ws, p.Path+string(filepath.Separator)) {
				if len(p.Path) > len(best) {
					best = p.Path
				}
			}
		}
		if best != "" {
			return best
		}
	}
	return ""
}

// logAnalysisEntry records the path, SHA, and cache key for every dispatch.
// sha_resolved=false means HEAD could not be resolved — the workspace is
// likely not a git repo and every call will trigger a cold scan.
func (h *handler) logAnalysisEntry(ctx context.Context, in *analysisInput) {
	resolvedPath := h.proto.ResolvePath(in.Path)
	sha := h.proto.ResolveHEAD(resolvedPath)
	slog.LogAttrs(ctx, slog.LevelInfo, "analysis dispatch",
		slog.String(logKeyAction, in.Action),
		slog.String(logKeyPath, resolvedPath),
		slog.String(logKeySHA, sha),
		slog.String(logKeyCacheKey, in.CacheKey),
		slog.Bool("sha_resolved", sha != ""),
	)
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
