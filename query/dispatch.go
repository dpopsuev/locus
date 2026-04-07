package query

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/dpopsuev/locus/internal/protocol"
)

// Sentinel errors for dispatch.
var (
	ErrUnknownAction    = errors.New("unknown action")
	ErrUnsupportedBatch = errors.New("action not supported in batch mode")
)

// dispatch routes a single action to the corresponding Protocol method,
// marshals the result to JSON, and wraps it in a Result.
func dispatch(ctx context.Context, p *protocol.Protocol, path, cacheKey string, a Action) Result {
	data, err := run(ctx, p, path, cacheKey, a)
	if err != nil {
		return Result{Action: a.Name, OK: false, Err: err.Error()}
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return Result{Action: a.Name, OK: false, Err: fmt.Sprintf("marshal: %v", err)}
	}
	return Result{Action: a.Name, OK: true, Data: raw}
}

// run dispatches to category-specific sub-routers. Each returns (result, error, matched).
func run(ctx context.Context, p *protocol.Protocol, path, cacheKey string, a Action) (any, error) {
	type dispatcher func(context.Context, *protocol.Protocol, string, string, Action) (any, error, bool)

	for _, d := range []dispatcher{runAnalysis, runClinic, runConstraint, runRefactor} {
		if r, err, ok := d(ctx, p, path, cacheKey, a); ok {
			return r, err
		}
	}
	return nil, fmt.Errorf("%w: %q", ErrUnknownAction, a.Name)
}

//nolint:revive // (any, error, bool) is the intentional 3-return dispatch pattern
func runAnalysis(ctx context.Context, p *protocol.Protocol, path, cacheKey string, a Action) (any, error, bool) {
	switch a.Name {
	case "hot_spots":
		r, err := p.GetHotSpots(ctx, path, paramInt(a, "churn_days"), paramInt(a, "top_n"), cacheKey)
		return r, err, true
	case "deps":
		r, err := p.GetDependencies(ctx, path, paramStr(a, "component"), cacheKey)
		return r, err, true
	case "coupling":
		r, err := p.GetCouplingTable(ctx, path, paramStr(a, "sort_by"), paramInt(a, "top_n"), cacheKey)
		return r, err, true
	case "cycles":
		r, err := p.GetCycles(ctx, path, paramStrSlice(a, "layers"), cacheKey)
		return r, err, true
	case "violations":
		r, err := p.GetViolations(ctx, path, paramStrSlice(a, "layers"), cacheKey)
		return r, err, true
	case "callers":
		r, err := p.GetCallers(ctx, path, paramStr(a, "symbol"), cacheKey)
		return r, err, true
	case "callees":
		r, err := p.GetCallees(ctx, path, paramStr(a, "symbol"), cacheKey)
		return r, err, true
	case "call_path":
		r, err := p.GetCallPath(ctx, path, paramStr(a, "from"), paramStr(a, "to"), cacheKey)
		return r, err, true
	case "component":
		r, err := p.GetComponentDetail(ctx, path, paramStr(a, "name"), cacheKey)
		return r, err, true
	case "search":
		r, err := p.SearchComponents(ctx, path, paramStr(a, "query"), cacheKey)
		return r, err, true
	case "symbol_search":
		r, err := p.SearchSymbols(ctx, path, paramStr(a, "pattern"), cacheKey)
		return r, err, true
	case "risk_scores":
		r, err := p.GetRiskScores(ctx, path, cacheKey)
		return r, err, true
	case "preset":
		r, err := p.RunPreset(ctx, path, paramStr(a, "name"), cacheKey)
		return r, err, true
	case "impact":
		r, err := p.GetImpact(ctx, path, paramStr(a, "component"), cacheKey)
		return r, err, true
	default:
		return nil, nil, false
	}
}

//nolint:revive // (any, error, bool) is the intentional 3-return dispatch pattern
func runClinic(ctx context.Context, p *protocol.Protocol, path, cacheKey string, a Action) (any, error, bool) {
	switch a.Name {
	case "pattern_scan":
		r, err := p.GetPatternScan(ctx, path, cacheKey)
		return r, err, true
	case "pattern_catalog":
		return p.GetPatternCatalog(paramStr(a, "filter")), nil, true
	case "hexa_validate":
		r, err := p.GetHexaValidation(ctx, path, cacheKey)
		return r, err, true
	case "solid_scan":
		r, err := p.GetSOLIDScan(ctx, path, cacheKey)
		return r, err, true
	case "symbol_quality":
		r, err := p.GetSymbolQuality(ctx, path, cacheKey)
		return r, err, true
	case "vocab_map":
		r, err := p.GetVocabMap(ctx, path, cacheKey)
		return r, err, true
	case "bloater_scan":
		r, err := p.GetBloaterScan(ctx, path, cacheKey)
		return r, err, true
	default:
		return nil, nil, false
	}
}

//nolint:revive // (any, error, bool) is the intentional 3-return dispatch pattern
func runConstraint(ctx context.Context, p *protocol.Protocol, path, cacheKey string, a Action) (any, error, bool) {
	switch a.Name {
	case "blast_radius":
		r, err := p.GetBlastRadius(ctx, path, paramStrSlice(a, "files"), paramStr(a, "since"), cacheKey)
		return r, err, true
	case "import_direction":
		r, err := p.GetImportDirection(ctx, path, cacheKey)
		return r, err, true
	case "trust_boundaries":
		r, err := p.GetTrustBoundaries(ctx, path, cacheKey)
		return r, err, true
	case "budgets":
		r, err := p.GetBudgets(ctx, path, cacheKey)
		return r, err, true
	case "mod_dependencies":
		r, err := p.GetModuleDependencies(ctx, path, cacheKey)
		return r, err, true
	case "symbol_blast":
		r, err := p.GetSymbolBlastRadius(ctx, path, paramStr(a, "symbol"), cacheKey)
		return r, err, true
	case "interface_metrics":
		r, err := p.GetInterfaceMetrics(ctx, path, cacheKey)
		return r, err, true
	case "leverage":
		r, err := p.GetLeverage(ctx, path, paramStr(a, "target"), cacheKey)
		return r, err, true
	case "api_surface":
		r, err := p.GetAPISurface(ctx, path, paramStrSlice(a, "trusted"), cacheKey)
		return r, err, true
	case "conventions":
		r, err := p.GetConventions(ctx, path)
		return r, err, true
	case "gaps":
		r, err := p.GetGaps(ctx, path)
		return r, err, true
	case "consolidate":
		r, err := p.GetConsolidation(ctx, path, cacheKey)
		return r, err, true
	default:
		return nil, nil, false
	}
}

//nolint:revive // (any, error, bool) is the intentional 3-return dispatch pattern
func runRefactor(ctx context.Context, p *protocol.Protocol, path, cacheKey string, a Action) (any, error, bool) {
	switch a.Name {
	case "drift":
		r, err := p.GetDrift(ctx, path, cacheKey)
		return r, err, true
	case "what_if":
		return nil, fmt.Errorf("%w: what_if requires structured FileMove params", ErrUnsupportedBatch), true
	case "diff_intelligence":
		r, err := p.GetDiffIntelligence(ctx, path, paramStr(a, "since"), cacheKey)
		return r, err, true
	default:
		return nil, nil, false
	}
}

// --- Param helpers ---

func paramStr(a Action, key string) string {
	v, ok := a.Params[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

func paramInt(a Action, key string) int {
	v, ok := a.Params[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case string:
		i, _ := strconv.Atoi(n)
		return i
	default:
		return 0
	}
}

func paramStrSlice(a Action, key string) []string {
	v, ok := a.Params[key]
	if !ok {
		return nil
	}
	switch s := v.(type) {
	case []string:
		return s
	case []any:
		out := make([]string, 0, len(s))
		for _, item := range s {
			if str, ok := item.(string); ok {
				out = append(out, str)
			}
		}
		return out
	default:
		return nil
	}
}
