package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	oculus "github.com/dpopsuev/oculus/v3"
)

func init() {
	analysisOps = append(analysisOps,
		opProbe, opScenario, opConvergence, opIsolate,
		opDiagnose, opIslands, opExplainEdge,
		opMesh,
	)
}

var opProbe = AnalysisOp{
	Name: ActionProbe,
	Run: func(ctx context.Context, h *handler, raw json.RawMessage) (*result, error) {
		var in analysisInput
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, err
		}
		if in.Symbol == "" {
			return nil, ErrSymbolRequired
		}
		r, err := h.proto.ProbeSymbol(ctx, in.Path, in.Symbol)
		if err != nil {
			return nil, err
		}
		return jsonOp(r)
	},
}

var opScenario = AnalysisOp{
	Name: ActionScenario,
	Run: func(ctx context.Context, h *handler, raw json.RawMessage) (*result, error) {
		var in analysisInput
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, err
		}
		if in.Symbol == "" {
			return nil, ErrSymbolRequired
		}
		depth := in.Hops
		if depth <= 0 {
			depth = 10
		}
		r, err := h.proto.GetScenario(ctx, in.Path, in.Symbol, depth, in.Stress)
		if err != nil {
			return nil, err
		}
		return jsonOp(r)
	},
}

var opConvergence = AnalysisOp{
	Name: ActionConvergence,
	Run: func(ctx context.Context, h *handler, raw json.RawMessage) (*result, error) {
		var in analysisInput
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, err
		}
		if len(in.Symbols) < 2 {
			return nil, ErrConvergenceMinSymbols
		}
		r, err := h.proto.GetConvergence(ctx, in.Path, in.Symbols)
		if err != nil {
			return nil, err
		}
		return jsonOp(r)
	},
}

var opIsolate = AnalysisOp{
	Name: ActionIsolate,
	Run: func(ctx context.Context, h *handler, raw json.RawMessage) (*result, error) {
		var in analysisInput
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, err
		}
		if in.Symbol == "" {
			return nil, ErrSymbolRequired
		}
		r, err := h.proto.IsolateSymbol(ctx, in.Path, in.Symbol)
		if err != nil {
			return nil, err
		}
		return jsonOp(r)
	},
}

var opDiagnose = AnalysisOp{
	Name: ActionDiagnose,
	Run: func(ctx context.Context, h *handler, raw json.RawMessage) (*result, error) {
		var in analysisInput
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, err
		}
		if in.Symbol == "" {
			return nil, ErrSymbolRequired
		}
		r, err := h.proto.Diagnose(ctx, in.Path, in.Symbol)
		if err != nil {
			return nil, err
		}
		return jsonOp(r)
	},
}

var opIslands = AnalysisOp{
	Name: ActionIslands,
	Run: func(ctx context.Context, h *handler, raw json.RawMessage) (*result, error) {
		var in analysisInput
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, err
		}
		r, err := h.proto.FindIslands(ctx, in.Path, in.Symbols)
		if err != nil {
			return nil, err
		}
		return jsonOp(r)
	},
}

var opExplainEdge = AnalysisOp{
	Name: ActionExplainEdge,
	Run: func(ctx context.Context, h *handler, raw json.RawMessage) (*result, error) {
		var in analysisInput
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, err
		}
		if in.Symbol == "" || in.Query == "" {
			return nil, ErrExplainEdgeParams
		}
		sg, err := h.proto.GetSymbolGraph(ctx, in.Path)
		if err != nil {
			return nil, err
		}
		for i := range sg.Edges {
			if sg.Edges[i].SourceFQN == in.Symbol && sg.Edges[i].TargetFQN == in.Query {
				snippet := oculus.ExplainEdge(in.Path, sg.Edges[i], 3)
				return textRes(snippet), nil
			}
		}
		return textRes("edge not found"), nil
	},
}

var opMesh = AnalysisOp{
	Name: ActionMesh,
	Run: func(ctx context.Context, h *handler, raw json.RawMessage) (*result, error) {
		var in analysisInput
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, err
		}
		mesh, err := h.proto.GetMesh(ctx, in.Path, in.CacheKey)
		if err != nil {
			return nil, err
		}
		view := in.MeshView
		if view == "" {
			view = detailFull
		}
		switch view {
		case detailFull:
			return jsonOp(mesh)
		case "neighborhood":
			if in.FQN == "" {
				return nil, ErrMeshFQNRequired
			}
			hops := in.Hops
			if hops <= 0 {
				hops = 1
			}
			return jsonOp(mesh.NeighborhoodWeighted(in.FQN, hops))
		case "distance":
			if in.From == "" || in.To == "" {
				return nil, ErrMeshFromToRequired
			}
			return jsonOp(mesh.Distance(in.From, in.To))
		case "boundaries":
			minW := 0.5
			if in.MinWeight != nil {
				minW = *in.MinWeight
			}
			return jsonOp(mesh.BoundariesMinWeight(minW))
		case "aggregate":
			level := oculus.MeshPackage
			switch in.Level {
			case "symbol":
				level = oculus.MeshSymbol
			case "file":
				level = oculus.MeshFile
			case "component":
				level = oculus.MeshComponent
			}
			return jsonOp(mesh.Aggregate(level))
		default:
			return nil, fmt.Errorf("%w %q (use: full, neighborhood, distance, boundaries, aggregate)", ErrUnknownMeshView, view)
		}
	},
}
