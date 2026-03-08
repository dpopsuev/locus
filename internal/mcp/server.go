package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dpopsuev/locus/internal/cache"
	"github.com/dpopsuev/locus/internal/protocol"
	"github.com/dpopsuev/mos/moslib/arch"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func NewServer(sc *cache.ScanCache, historyDir string, workspaceRoots []string) *sdkmcp.Server {
	srv := sdkmcp.NewServer(
		&sdkmcp.Implementation{Name: "locus", Version: "0.3.0"},
		nil,
	)
	h := &handler{proto: protocol.New(sc, historyDir, workspaceRoots)}

	sdkmcp.AddTool(srv, &sdkmcp.Tool{
		Name:        "scan_project",
		Description: "Scan a repository and return its full codebase context: architecture, dependency graph, churn, hot spots, and symbols. Results are cached by git HEAD SHA.",
	}, noOut(h.handleScanProject))

	sdkmcp.AddTool(srv, &sdkmcp.Tool{
		Name:        "suggest_depth",
		Description: "Analyze a repository and suggest the optimal --depth grouping level.",
	}, noOut(h.handleSuggestDepth))

	sdkmcp.AddTool(srv, &sdkmcp.Tool{
		Name:        "get_hot_spots",
		Description: "Return the hottest components in a repository (high fan-in + high churn).",
	}, noOut(h.handleGetHotSpots))

	sdkmcp.AddTool(srv, &sdkmcp.Tool{
		Name:        "get_dependencies",
		Description: "Return fan-in and fan-out edges for a specific component in a repository.",
	}, noOut(h.handleGetDependencies))

	sdkmcp.AddTool(srv, &sdkmcp.Tool{
		Name:        "get_rules",
		Description: "Return all .cursor/rules/*.mdc rules for a workspace root, with frontmatter metadata and body content.",
	}, noOut(h.handleGetRules))

	sdkmcp.AddTool(srv, &sdkmcp.Tool{
		Name:        "get_skills",
		Description: "Return all .cursor/skills/*/SKILL.md skills for a workspace root, with frontmatter metadata and body content.",
	}, noOut(h.handleGetSkills))

	sdkmcp.AddTool(srv, &sdkmcp.Tool{
		Name:        "get_coupling_table",
		Description: "Return a pre-formatted coupling table: package name, fan-in, fan-out, churn, symbol count. Sorted and filtered so agents never need Python/jq glue.",
	}, noOut(h.handleGetCouplingTable))

	sdkmcp.AddTool(srv, &sdkmcp.Tool{
		Name:        "get_edge_list",
		Description: "Return a pre-formatted list of dependency edges. Optionally filter to a single component.",
	}, noOut(h.handleGetEdgeList))

	sdkmcp.AddTool(srv, &sdkmcp.Tool{
		Name:        "codograph_remote",
		Description: "Produce a codograph from a remote GitHub repository via shallow clone. Accepts a git URL (HTTPS, SSH, or shorthand like github.com/org/repo) and optional ref (branch/tag). Returns the same ContextReport as scan_project. Results are cached by URL+SHA.",
	}, noOut(h.handleCodographRemote))

	sdkmcp.AddTool(srv, &sdkmcp.Tool{
		Name:        "get_codograph_history",
		Description: "List past codographs for a repository path. Returns timestamps, HEAD SHAs, source (local/remote), and component/edge counts. Use the 'last' parameter to limit results.",
	}, noOut(h.handleGetCodographHistory))

	sdkmcp.AddTool(srv, &sdkmcp.Tool{
		Name:        "diff_codographs",
		Description: "Compare two codographs and return a diff: added/removed components, added/removed edges, churn deltas, and a human-readable summary. Defaults to comparing the latest two codographs for a given path.",
	}, noOut(h.handleDiffCodographs))

	sdkmcp.AddTool(srv, &sdkmcp.Tool{
		Name:        "diff_branches",
		Description: "Compare architecture between two git branches. Scans each branch (cache-aware: previously scanned branches are instant hits) and returns the diff.",
	}, noOut(h.handleDiffBranches))

	return srv
}

type handler struct {
	proto *protocol.Protocol
}

// --- handlers ---

type scanProjectInput struct {
	Path            string `json:"path"`
	Depth           int    `json:"depth,omitempty"`
	ChurnDays       int    `json:"churn_days,omitempty"`
	IncludeExternal bool   `json:"include_external,omitempty"`
	IncludeTests    bool   `json:"include_tests,omitempty"`
	Budget          int    `json:"budget,omitempty"`
	Format          string `json:"format,omitempty"`
}

func (h *handler) handleScanProject(ctx context.Context, _ *sdkmcp.CallToolRequest, in scanProjectInput) (*sdkmcp.CallToolResult, any, error) {
	report, err := h.proto.ScanProject(ctx, in.Path, protocol.ScanOpts{
		Depth: in.Depth, ChurnDays: in.ChurnDays,
		IncludeExternal: in.IncludeExternal, IncludeTests: in.IncludeTests,
		Budget: in.Budget,
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
