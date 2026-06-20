package mcp

import (
	"context"
	"encoding/json"
)

// AnalysisOp is a single named analysis action. Each op owns its input
// validation and delegates to the engine for execution. The handler
// provides common setup (timeout, staleness, path fallback) before dispatch.
type AnalysisOp struct {
	Name string
	Run  func(ctx context.Context, h *handler, in json.RawMessage) (*result, error)
}

type result struct {
	Text string
	Raw  any
}

func textRes(s string) *result                { return &result{Text: s} }
func jsonOp(data any) (*result, error)        { b, _ := json.Marshal(data); return &result{Text: string(b)}, nil }

var analysisOps []AnalysisOp

// FindAnalysisOp returns the op with the given name, or nil.
func FindAnalysisOp(name string) *AnalysisOp {
	for i := range analysisOps {
		if analysisOps[i].Name == name {
			return &analysisOps[i]
		}
	}
	return nil
}
