package arch

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// NewMCPServer creates an MCP server that exposes codebase context tools.
func NewMCPServer() *sdkmcp.Server {
	srv := sdkmcp.NewServer(
		&sdkmcp.Implementation{Name: "mcontext", Version: "0.1.0"},
		nil,
	)

	sdkmcp.AddTool(srv, &sdkmcp.Tool{
		Name:        "scan_project",
		Description: "Scan a repository and return its full codebase context: architecture, dependency graph, churn, hot spots, and symbols.",
	}, noOutputSchema(handleScanProject))

	sdkmcp.AddTool(srv, &sdkmcp.Tool{
		Name:        "suggest_depth",
		Description: "Analyze a repository and suggest the optimal --depth grouping level.",
	}, noOutputSchema(handleSuggestDepth))

	sdkmcp.AddTool(srv, &sdkmcp.Tool{
		Name:        "get_hot_spots",
		Description: "Return the hottest components in a repository (high fan-in + high churn).",
	}, noOutputSchema(handleGetHotSpots))

	sdkmcp.AddTool(srv, &sdkmcp.Tool{
		Name:        "get_dependencies",
		Description: "Return fan-in and fan-out edges for a specific component in a repository.",
	}, noOutputSchema(handleGetDependencies))

	return srv
}

func noOutputSchema[In, Out any](h func(context.Context, *sdkmcp.CallToolRequest, In) (*sdkmcp.CallToolResult, Out, error)) sdkmcp.ToolHandlerFor[In, any] {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest, input In) (*sdkmcp.CallToolResult, any, error) {
		res, out, err := h(ctx, req, input)
		return res, out, err
	}
}

// --- Tool input types ---

type scanProjectInput struct {
	Path            string `json:"path"`
	Depth           int    `json:"depth,omitempty"`
	ChurnDays       int    `json:"churn_days,omitempty"`
	IncludeExternal bool   `json:"include_external,omitempty"`
	IncludeTests    bool   `json:"include_tests,omitempty"`
	Budget          int    `json:"budget,omitempty"`
}

type suggestDepthInput struct {
	Path string `json:"path"`
}

type getHotSpotsInput struct {
	Path      string `json:"path"`
	ChurnDays int    `json:"churn_days,omitempty"`
	TopN      int    `json:"top_n,omitempty"`
}

type getDependenciesInput struct {
	Path      string `json:"path"`
	Component string `json:"component"`
}

// --- Tool handlers ---

func handleScanProject(_ context.Context, _ *sdkmcp.CallToolRequest, input scanProjectInput) (*sdkmcp.CallToolResult, any, error) {
	path := input.Path
	if path == "" {
		path = "."
	}
	churnDays := input.ChurnDays
	if churnDays == 0 {
		churnDays = 30
	}

	report, err := ScanAndBuild(path, ScanOpts{
		ExcludeTests:    !input.IncludeTests,
		IncludeExternal: input.IncludeExternal,
		Depth:           input.Depth,
		ChurnDays:       churnDays,
		Budget:          input.Budget,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("scan failed: %w", err)
	}

	data, err := RenderJSON(report)
	if err != nil {
		return nil, nil, fmt.Errorf("render JSON: %w", err)
	}

	return &sdkmcp.CallToolResult{
		Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: string(data)}},
	}, nil, nil
}

func handleSuggestDepth(_ context.Context, _ *sdkmcp.CallToolRequest, input suggestDepthInput) (*sdkmcp.CallToolResult, any, error) {
	path := input.Path
	if path == "" {
		path = "."
	}

	report, err := ScanAndBuild(path, ScanOpts{ExcludeTests: true})
	if err != nil {
		return nil, nil, fmt.Errorf("scan failed: %w", err)
	}

	result := struct {
		SuggestedDepth int    `json:"suggested_depth"`
		Components     int    `json:"flat_components"`
		Reasoning      string `json:"reasoning"`
	}{
		SuggestedDepth: report.SuggestedDepth,
		Components:     len(report.Architecture.Services),
	}

	if report.SuggestedDepth > 0 {
		result.Reasoning = fmt.Sprintf("Flat scan produces %d components. --depth %d reduces this while preserving meaningful grouping.",
			len(report.Architecture.Services), report.SuggestedDepth)
	} else {
		result.Reasoning = fmt.Sprintf("Flat scan produces %d components, which is already manageable. No grouping needed.",
			len(report.Architecture.Services))
	}

	data, _ := json.MarshalIndent(result, "", "  ")
	return &sdkmcp.CallToolResult{
		Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: string(data)}},
	}, nil, nil
}

func handleGetHotSpots(_ context.Context, _ *sdkmcp.CallToolRequest, input getHotSpotsInput) (*sdkmcp.CallToolResult, any, error) {
	path := input.Path
	if path == "" {
		path = "."
	}
	churnDays := input.ChurnDays
	if churnDays == 0 {
		churnDays = 30
	}
	topN := input.TopN
	if topN == 0 {
		topN = 10
	}

	report, err := ScanAndBuild(path, ScanOpts{
		ExcludeTests: true,
		ChurnDays:    churnDays,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("scan failed: %w", err)
	}

	spots := report.HotSpots
	sort.Slice(spots, func(i, j int) bool { return spots[i].Churn > spots[j].Churn })
	if len(spots) > topN {
		spots = spots[:topN]
	}

	data, _ := json.MarshalIndent(spots, "", "  ")
	return &sdkmcp.CallToolResult{
		Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: string(data)}},
	}, nil, nil
}

func handleGetDependencies(_ context.Context, _ *sdkmcp.CallToolRequest, input getDependenciesInput) (*sdkmcp.CallToolResult, any, error) {
	path := input.Path
	if path == "" {
		path = "."
	}
	if input.Component == "" {
		return nil, nil, fmt.Errorf("component is required")
	}

	report, err := ScanAndBuild(path, ScanOpts{ExcludeTests: true})
	if err != nil {
		return nil, nil, fmt.Errorf("scan failed: %w", err)
	}

	type depResult struct {
		Component string     `json:"component"`
		FanIn     []jsonEdge `json:"fan_in"`
		FanOut    []jsonEdge `json:"fan_out"`
	}

	result := depResult{Component: input.Component}
	for _, e := range report.Architecture.Edges {
		je := jsonEdge{From: e.From, To: e.To, Weight: e.Weight, CallSites: e.CallSites, LOCSurface: e.LOCSurface, Protocol: e.Protocol}
		if e.To == input.Component {
			result.FanIn = append(result.FanIn, je)
		}
		if e.From == input.Component {
			result.FanOut = append(result.FanOut, je)
		}
	}

	data, _ := json.MarshalIndent(result, "", "  ")
	return &sdkmcp.CallToolResult{
		Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: string(data)}},
	}, nil, nil
}
