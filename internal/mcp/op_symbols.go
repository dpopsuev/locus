package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func init() {
	analysisOps = append(analysisOps,
		opSymbolSearch, opCallees, opCallPath, opSymbolGraph,
		opCallers, opCallersAt, opPipelines, opSymbolDiff,
	)
}

var opSymbolSearch = AnalysisOp{
	Name: ActionSymbolSearch,
	Run: func(ctx context.Context, h *handler, raw json.RawMessage) (*result, error) {
		var in analysisInput
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, err
		}
		if in.Symbol == "" && in.File == "" {
			return nil, ErrQueryRequired
		}
		r, err := h.proto.SearchSymbolsFiltered(ctx, in.Path, in.Symbol, in.File, in.CacheKey)
		if err != nil {
			return nil, err
		}
		if in.Detail == detailFull {
			res, _, err := h.symbolSearchFull(ctx, &in, r)
			if err != nil {
				return nil, err
			}
			if res != nil && len(res.Content) > 0 {
				if tc, ok := res.Content[0].(*sdkmcp.TextContent); ok {
					return &result{Text: tc.Text}, nil
				}
			}
			return textRes(""), nil
		}
		const defaultShallowLimit = 50
		limit := in.TopN
		if limit <= 0 {
			limit = defaultShallowLimit
		}
		if len(r.Matches) > limit {
			r.Matches = r.Matches[:limit]
			r.Summary = fmt.Sprintf("%s (showing first %d; use detail:\"full\" on a narrower query for metrics)", r.Summary, limit)
		}
		return jsonOp(r)
	},
}

var opCallees = AnalysisOp{
	Name: ActionCallees,
	Run: func(ctx context.Context, h *handler, raw json.RawMessage) (*result, error) {
		var in analysisInput
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, err
		}
		r, err := h.proto.GetCallees(ctx, in.Path, in.Symbol, in.CacheKey)
		if err != nil {
			return nil, err
		}
		return jsonOp(r)
	},
}

var opCallPath = AnalysisOp{
	Name: ActionCallPath,
	Run: func(ctx context.Context, h *handler, raw json.RawMessage) (*result, error) {
		var in analysisInput
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, err
		}
		r, err := h.proto.GetCallPath(ctx, in.Path, in.Symbol, in.Query, in.CacheKey)
		if err != nil {
			return nil, err
		}
		return jsonOp(r)
	},
}

var opSymbolGraph = AnalysisOp{
	Name: ActionSymbolGraph,
	Run: func(ctx context.Context, h *handler, raw json.RawMessage) (*result, error) {
		var in analysisInput
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, err
		}
		r, err := h.proto.GetSymbolGraph(ctx, in.Path)
		if err != nil {
			return nil, err
		}
		return jsonOp(r)
	},
}

var opCallers = AnalysisOp{
	Name: ActionCallers,
	Run: func(ctx context.Context, h *handler, raw json.RawMessage) (*result, error) {
		var in analysisInput
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, err
		}
		r, err := h.proto.GetCallers(ctx, in.Path, in.Symbol, in.CacheKey)
		if err != nil {
			return nil, err
		}
		return jsonOp(r)
	},
}

var opCallersAt = AnalysisOp{
	Name: ActionCallersAt,
	Run: func(ctx context.Context, h *handler, raw json.RawMessage) (*result, error) {
		var in analysisInput
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, err
		}
		if in.File == "" {
			return nil, ErrCallersAtFileRequired
		}
		r, err := h.proto.GetCallersAt(ctx, in.Path, in.File, in.Line, in.Char, in.CacheKey)
		if err != nil {
			return nil, err
		}
		return jsonOp(r)
	},
}

var opPipelines = AnalysisOp{
	Name: ActionPipelines,
	Run: func(ctx context.Context, h *handler, raw json.RawMessage) (*result, error) {
		var in analysisInput
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, err
		}
		minLen := in.MinLength
		if minLen <= 0 {
			minLen = 3
		}
		r, err := h.proto.DetectPipelines(ctx, in.Path, minLen, in.CacheKey)
		if err != nil {
			return nil, err
		}
		return jsonOp(r)
	},
}

var opSymbolDiff = AnalysisOp{
	Name: ActionSymbolDiff,
	Run: func(ctx context.Context, h *handler, raw json.RawMessage) (*result, error) {
		var in analysisInput
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, err
		}
		if in.BeforeSHA == "" || in.AfterSHA == "" {
			return nil, ErrSymbolDiffParams
		}
		r, err := h.proto.DiffSymbolGraphs(ctx, in.BeforeSHA, in.AfterSHA)
		if err != nil {
			return nil, err
		}
		return jsonOp(r)
	},
}
