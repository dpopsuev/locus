package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

func init() {
	analysisOps = append(analysisOps,
		opDeps, opImpact, opCoupling, opCycles, opViolations, opRiskScores,
		opComplexityHints, opTaint,
	)
}

var opDeps = AnalysisOp{
	Name: ActionDeps,
	Run: func(ctx context.Context, h *handler, raw json.RawMessage) (*result, error) {
		var in analysisInput
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, err
		}
		r, err := h.proto.GetDependencies(ctx, in.Path, in.Component, in.CacheKey)
		if err != nil {
			return nil, err
		}
		return jsonOp(r)
	},
}

var opImpact = AnalysisOp{
	Name: ActionImpact,
	Run: func(ctx context.Context, h *handler, raw json.RawMessage) (*result, error) {
		var in analysisInput
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, err
		}
		r, err := h.proto.GetImpact(ctx, in.Path, in.Component, in.CacheKey)
		if err != nil {
			return nil, err
		}
		return jsonOp(r)
	},
}

var opCoupling = AnalysisOp{
	Name: ActionCoupling,
	Run: func(ctx context.Context, h *handler, raw json.RawMessage) (*result, error) {
		var in analysisInput
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, err
		}
		topN := in.TopN
		if in.Format == FormatSummary && topN == 0 {
			topN = 5
		}
		switch in.View {
		case ViewHotSpots:
			spots, err := h.proto.GetHotSpots(ctx, in.Path, in.ChurnDays, topN, in.CacheKey)
			if err != nil {
				return nil, err
			}
			return jsonOp(spots)
		case ViewEdges:
			r, err := h.proto.GetEdgeList(ctx, in.Path, in.Component, in.CacheKey)
			if err != nil {
				return nil, err
			}
			return textRes(r), nil
		default:
			r, err := h.proto.GetCouplingTable(ctx, in.Path, in.SortBy, topN, in.CacheKey)
			if err != nil {
				return nil, err
			}
			return textRes(r), nil
		}
	},
}

var opCycles = AnalysisOp{
	Name: ActionCycles,
	Run: func(ctx context.Context, h *handler, raw json.RawMessage) (*result, error) {
		var in analysisInput
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, err
		}
		report, err := h.proto.GetCycles(ctx, in.Path, in.Layers, in.CacheKey)
		if err != nil {
			return nil, err
		}
		if in.Format == FormatSummary {
			return textRes(renderCyclesSummary(report)), nil
		}
		return jsonOp(report)
	},
}

var opViolations = AnalysisOp{
	Name: ActionViolations,
	Run: func(ctx context.Context, h *handler, raw json.RawMessage) (*result, error) {
		var in analysisInput
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, err
		}
		report, err := h.proto.GetViolations(ctx, in.Path, in.Layers, in.CacheKey)
		if err != nil {
			return nil, err
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
			return textRes(b.String()), nil
		}
		return jsonOp(report)
	},
}

var opRiskScores = AnalysisOp{
	Name: ActionRiskScores,
	Run: func(ctx context.Context, h *handler, raw json.RawMessage) (*result, error) {
		var in analysisInput
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, err
		}
		r, err := h.proto.GetRiskScores(ctx, in.Path, in.CacheKey)
		if err != nil {
			return nil, err
		}
		return jsonOp(r)
	},
}

var opComplexityHints = AnalysisOp{
	Name: ActionComplexityHints,
	Run: func(ctx context.Context, h *handler, raw json.RawMessage) (*result, error) {
		var in analysisInput
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, err
		}
		r, err := h.proto.GetComplexityHints(ctx, in.Path, in.TopN, in.CacheKey)
		if err != nil {
			return nil, err
		}
		return jsonOp(r)
	},
}

var opTaint = AnalysisOp{
	Name: ActionTaint,
	Run: func(ctx context.Context, h *handler, raw json.RawMessage) (*result, error) {
		var in analysisInput
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, err
		}
		source, sink := in.From, in.To
		if source == "" {
			source = in.Symbol
		}
		if sink == "" {
			sink = in.Query
		}
		if source == "" || sink == "" {
			return nil, ErrTaintFromToRequired
		}
		r, err := h.proto.TaintQuery(ctx, in.Path, source, sink, in.CacheKey)
		if err != nil {
			return nil, err
		}
		return jsonOp(r)
	},
}
