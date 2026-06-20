package mcp

import (
	"context"
	"encoding/json"
	"fmt"
)

func init() {
	analysisOps = append(analysisOps,
		opComponent, opSearch, opQuery, opPreset,
		opBook, opContextRead, opContextWrite, opTriage,
		opIntraDeps, opIntraCoupling, opTypeUsages,
		opScanDiff, opComponentDiff, opMigrationOverlay,
		opRegisterMirror, opListMirrors,
	)
}

var opComponent = AnalysisOp{
	Name: ActionComponent,
	Run: func(ctx context.Context, h *handler, raw json.RawMessage) (*result, error) {
		var in analysisInput
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, err
		}
		r, err := h.proto.GetComponentDetail(ctx, in.Path, in.Component, in.CacheKey)
		if err != nil {
			return nil, err
		}
		return jsonOp(r)
	},
}

var opSearch = AnalysisOp{
	Name: ActionSearch,
	Run: func(ctx context.Context, h *handler, raw json.RawMessage) (*result, error) {
		var in analysisInput
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, err
		}
		r, err := h.proto.SearchComponents(ctx, in.Path, in.Query, in.CacheKey)
		if err != nil {
			return nil, err
		}
		return jsonOp(r)
	},
}

var opQuery = AnalysisOp{
	Name: ActionQuery,
	Run: func(ctx context.Context, h *handler, raw json.RawMessage) (*result, error) {
		var in analysisInput
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, err
		}
		r, err := h.proto.AnswerQuery(ctx, in.Path, in.Query, in.CacheKey)
		if err != nil {
			return nil, err
		}
		return jsonOp(r)
	},
}

var opPreset = AnalysisOp{
	Name: ActionPreset,
	Run: func(ctx context.Context, h *handler, raw json.RawMessage) (*result, error) {
		var in analysisInput
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, err
		}
		r, err := h.proto.RunPreset(ctx, in.Path, in.Preset, in.CacheKey)
		if err != nil {
			return nil, err
		}
		return textRes(r), nil
	},
}

var opBook = AnalysisOp{
	Name: ActionBook,
	Run: func(_ context.Context, h *handler, raw json.RawMessage) (*result, error) {
		var in analysisInput
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, err
		}
		hops := in.Hops
		if hops <= 0 {
			hops = 2
		}
		r, err := h.proto.QueryBook(in.Keywords, hops)
		if err != nil {
			return nil, err
		}
		return jsonOp(r)
	},
}

var opContextRead = AnalysisOp{
	Name: ActionContextRead,
	Run: func(context.Context, *handler, json.RawMessage) (*result, error) {
		return textRes("context read not yet wired to engine"), nil
	},
}

var opContextWrite = AnalysisOp{
	Name: ActionContextWrite,
	Run: func(context.Context, *handler, json.RawMessage) (*result, error) {
		return textRes("context write not yet wired to engine"), nil
	},
}

var opTriage = AnalysisOp{
	Name: ActionTriage,
	Run: func(_ context.Context, h *handler, raw json.RawMessage) (*result, error) {
		var in analysisInput
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, err
		}
		if in.Intent == "" {
			return nil, ErrIntentRequired
		}
		return jsonOp(h.reg.Triage(in.Intent, in.Path))
	},
}

var opIntraDeps = AnalysisOp{
	Name: ActionIntraDeps,
	Run: func(ctx context.Context, h *handler, raw json.RawMessage) (*result, error) {
		var in analysisInput
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, err
		}
		r, err := h.proto.GetIntraPackageDeps(ctx, in.Path, in.Component, in.CacheKey)
		if err != nil {
			return nil, err
		}
		return jsonOp(r)
	},
}

var opIntraCoupling = AnalysisOp{
	Name: ActionIntraCoupling,
	Run: func(ctx context.Context, h *handler, raw json.RawMessage) (*result, error) {
		var in analysisInput
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, err
		}
		r, err := h.proto.GetIntraCoupling(ctx, in.Path, in.Component, in.CacheKey)
		if err != nil {
			return nil, err
		}
		return jsonOp(r)
	},
}

var opTypeUsages = AnalysisOp{
	Name: ActionTypeUsages,
	Run: func(ctx context.Context, h *handler, raw json.RawMessage) (*result, error) {
		var in analysisInput
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, err
		}
		r, err := h.proto.GetTypeUsages(ctx, in.Path, in.Query, in.CacheKey)
		if err != nil {
			return nil, err
		}
		return jsonOp(r)
	},
}

var opScanDiff = AnalysisOp{
	Name: ActionScanDiff,
	Run: func(ctx context.Context, h *handler, raw json.RawMessage) (*result, error) {
		var in analysisInput
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, err
		}
		r, err := h.proto.GetScanDiff(ctx, in.Path, in.BeforeSHA, in.AfterSHA)
		if err != nil {
			return nil, err
		}
		return jsonOp(r)
	},
}

var opComponentDiff = AnalysisOp{
	Name: ActionComponentDiff,
	Run: func(ctx context.Context, h *handler, raw json.RawMessage) (*result, error) {
		var in analysisInput
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, err
		}
		r, err := h.proto.GetComponentRangeDiff(ctx, in.Path, in.BeforeSHA, in.AfterSHA, in.CacheKey)
		if err != nil {
			return nil, err
		}
		return jsonOp(r)
	},
}

var opMigrationOverlay = AnalysisOp{
	Name: ActionMigrationOverlay,
	Run: func(ctx context.Context, h *handler, raw json.RawMessage) (*result, error) {
		var in analysisInput
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, err
		}
		r, err := h.proto.ComputeMigrationOverlay(ctx, in.Path, in.Component, in.Query, in.CacheKey)
		if err != nil {
			return nil, err
		}
		return jsonOp(r)
	},
}

var opRegisterMirror = AnalysisOp{
	Name: ActionRegisterMirror,
	Run: func(ctx context.Context, h *handler, raw json.RawMessage) (*result, error) {
		var in analysisInput
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, err
		}
		if err := h.proto.RegisterMirror(ctx, in.Path, in.From, in.To); err != nil {
			return nil, err
		}
		return textRes(fmt.Sprintf("registered mirror: %s → %s", in.From, in.To)), nil
	},
}

var opListMirrors = AnalysisOp{
	Name: ActionListMirrors,
	Run: func(ctx context.Context, h *handler, raw json.RawMessage) (*result, error) {
		var in analysisInput
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, err
		}
		r, err := h.proto.ListMirrors(ctx, in.Path)
		if err != nil {
			return nil, err
		}
		return jsonOp(r)
	},
}
