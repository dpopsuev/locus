package organs

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dpopsuev/locus/internal/store"
	"github.com/dpopsuev/oculus/v3/engine"
	"github.com/dpopsuev/tako/agent/organ"
)

type Locus struct {
	engine *engine.Engine
}

func New(s store.Store, workspaceRoots []string) *Locus {
	return &Locus{engine: engine.New(s, workspaceRoots)}
}

func (l *Locus) Capabilities() []organ.Func {
	return []organ.Func{
		l.scanOrgan(),
		l.probeOrgan(),
		l.callersOrgan(),
		l.depsOrgan(),
		l.violationsOrgan(),
		l.searchOrgan(),
	}
}

func (l *Locus) scanOrgan() organ.Func {
	return organ.Func{
		Name:        "locus_scan",
		Description: "Scan a repository's architecture. Returns component count, edges, cycles, violations.",
		Schema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"path": {"type": "string", "description": "absolute path to repository"},
				"intent": {"type": "string", "description": "scan depth: architecture, coupling, health, full", "default": "health"}
			},
			"required": ["path"]
		}`),
		Mode:   organ.ReadAction,
		Source: organ.Environment,
		Reads:  []string{"code-graph"},
		Execute: func(ctx context.Context, input json.RawMessage) (organ.Result, error) {
			var in struct {
				Path   string `json:"path"`
				Intent string `json:"intent"`
			}
			json.Unmarshal(input, &in)
			if in.Path == "" {
				return organ.ErrorResult("path is required"), nil
			}
			if in.Intent == "" {
				in.Intent = "health"
			}
			result, err := l.engine.ScanProject(ctx, in.Path, engine.ScanOpts{Intent: in.Intent})
			if err != nil {
				return organ.ErrorResult(err.Error()), nil
			}
			return jsonText(result)
		},
	}
}

func (l *Locus) probeOrgan() organ.Func {
	return organ.Func{
		Name:        "locus_probe",
		Description: "All vitals for one symbol: fan-in, fan-out, instability, callers, callees, package.",
		Schema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"path": {"type": "string", "description": "absolute path to repository"},
				"symbol": {"type": "string", "description": "symbol name (function or type)"}
			},
			"required": ["path", "symbol"]
		}`),
		Mode:   organ.ReadAction,
		Source: organ.Environment,
		Reads:  []string{"code-graph"},
		Execute: func(ctx context.Context, input json.RawMessage) (organ.Result, error) {
			var in struct {
				Path   string `json:"path"`
				Symbol string `json:"symbol"`
			}
			json.Unmarshal(input, &in)
			if in.Symbol == "" {
				return organ.ErrorResult("symbol is required"), nil
			}
			r, err := l.engine.ProbeSymbol(ctx, in.Path, in.Symbol)
			if err != nil {
				return organ.ErrorResult(err.Error()), nil
			}
			return jsonText(r)
		},
	}
}

func (l *Locus) callersOrgan() organ.Func {
	return organ.Func{
		Name:        "locus_callers",
		Description: "Find all callers of a symbol. Returns caller name, package, file, line.",
		Schema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"path": {"type": "string", "description": "absolute path to repository"},
				"symbol": {"type": "string", "description": "symbol name (function or type)"}
			},
			"required": ["path", "symbol"]
		}`),
		Mode:   organ.ReadAction,
		Source: organ.Environment,
		Reads:  []string{"code-graph"},
		Execute: func(ctx context.Context, input json.RawMessage) (organ.Result, error) {
			var in struct {
				Path   string `json:"path"`
				Symbol string `json:"symbol"`
			}
			json.Unmarshal(input, &in)
			if in.Symbol == "" {
				return organ.ErrorResult("symbol is required"), nil
			}
			r, err := l.engine.GetCallers(ctx, in.Path, in.Symbol, "")
			if err != nil {
				return organ.ErrorResult(err.Error()), nil
			}
			return jsonText(r)
		},
	}
}

func (l *Locus) depsOrgan() organ.Func {
	return organ.Func{
		Name:        "locus_deps",
		Description: "Get dependencies of a component. Returns imports and dependents.",
		Schema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"path": {"type": "string", "description": "absolute path to repository"},
				"component": {"type": "string", "description": "component path (e.g. agent/cerebrum)"}
			},
			"required": ["path", "component"]
		}`),
		Mode:   organ.ReadAction,
		Source: organ.Environment,
		Reads:  []string{"code-graph"},
		Execute: func(ctx context.Context, input json.RawMessage) (organ.Result, error) {
			var in struct {
				Path      string `json:"path"`
				Component string `json:"component"`
			}
			json.Unmarshal(input, &in)
			r, err := l.engine.GetDependencies(ctx, in.Path, in.Component, "")
			if err != nil {
				return organ.ErrorResult(err.Error()), nil
			}
			return jsonText(r)
		},
	}
}

func (l *Locus) violationsOrgan() organ.Func {
	return organ.Func{
		Name:        "locus_violations",
		Description: "Check architecture layer violations. Returns violations with from/to components.",
		Schema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"path": {"type": "string", "description": "absolute path to repository"}
			},
			"required": ["path"]
		}`),
		Mode:   organ.ReadAction,
		Source: organ.Environment,
		Reads:  []string{"code-graph"},
		Execute: func(ctx context.Context, input json.RawMessage) (organ.Result, error) {
			var in struct {
				Path string `json:"path"`
			}
			json.Unmarshal(input, &in)
			r, err := l.engine.GetViolations(ctx, in.Path, nil, "")
			if err != nil {
				return organ.ErrorResult(err.Error()), nil
			}
			return jsonText(r)
		},
	}
}

func (l *Locus) searchOrgan() organ.Func {
	return organ.Func{
		Name:        "locus_search",
		Description: "Search for a symbol by name pattern. Returns matching symbols with file, line, kind.",
		Schema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"path": {"type": "string", "description": "absolute path to repository"},
				"symbol": {"type": "string", "description": "symbol name pattern to search"}
			},
			"required": ["path", "symbol"]
		}`),
		Mode:   organ.ReadAction,
		Source: organ.Environment,
		Reads:  []string{"code-graph"},
		Execute: func(ctx context.Context, input json.RawMessage) (organ.Result, error) {
			var in struct {
				Path   string `json:"path"`
				Symbol string `json:"symbol"`
			}
			json.Unmarshal(input, &in)
			if in.Symbol == "" {
				return organ.ErrorResult("symbol is required"), nil
			}
			r, err := l.engine.SearchSymbols(ctx, in.Path, in.Symbol, "")
			if err != nil {
				return organ.ErrorResult(err.Error()), nil
			}
			return jsonText(r)
		},
	}
}

func jsonText(v any) (organ.Result, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return organ.ErrorResult(fmt.Sprintf("json marshal: %v", err)), nil
	}
	return organ.TextResult(string(b)), nil
}
