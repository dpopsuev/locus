package protocol

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
)

// Sentinel errors for batch dispatch.
var (
	ErrBatchPathRequired = errors.New("batch: path is required")
	ErrUnknownAction     = errors.New("unknown action")
	ErrUnsupportedBatch  = errors.New("action not supported in batch mode")
)

// BatchRequest describes a batch of queries to execute against a repository.
type BatchRequest struct {
	Path     string        `json:"path"`
	CacheKey string        `json:"cache_key,omitempty"`
	Intent   string        `json:"intent,omitempty"`
	Actions  []BatchAction `json:"actions"`
}

// BatchAction describes a single query within a batch.
type BatchAction struct {
	Name   string         `json:"name"`
	Params map[string]any `json:"params,omitempty"`
}

// BatchResponse holds results for a batch query.
type BatchResponse struct {
	CacheKey string        `json:"cache_key"`
	Results  []BatchResult `json:"results"`
}

// BatchResult is a single action's outcome within a batch.
type BatchResult struct {
	Action string          `json:"action"`
	OK     bool            `json:"ok"`
	Err    string          `json:"error,omitempty"`
	Data   json.RawMessage `json:"data,omitempty"`
}

// BatchQuery executes a batch of actions, sharing a single scan/cache.
func (p *Protocol) BatchQuery(ctx context.Context, req BatchRequest) (*BatchResponse, error) {
	if req.Path == "" {
		return nil, ErrBatchPathRequired
	}

	cacheKey := req.CacheKey
	if cacheKey == "" {
		intent := req.Intent
		if intent == "" {
			intent = "health"
		}
		sr, err := p.ScanProject(ctx, req.Path, ScanOpts{Intent: intent})
		if err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		cacheKey = sr.CacheKey
	}

	results := make([]BatchResult, len(req.Actions))
	for i, a := range req.Actions {
		results[i] = p.batchDispatch(ctx, req.Path, cacheKey, a)
	}

	return &BatchResponse{CacheKey: cacheKey, Results: results}, nil
}

func (p *Protocol) batchDispatch(ctx context.Context, path, cacheKey string, a BatchAction) BatchResult {
	data, err := p.batchRun(ctx, path, cacheKey, a)
	if err != nil {
		return BatchResult{Action: a.Name, OK: false, Err: err.Error()}
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return BatchResult{Action: a.Name, OK: false, Err: fmt.Sprintf("marshal: %v", err)}
	}
	return BatchResult{Action: a.Name, OK: true, Data: raw}
}

func (p *Protocol) batchRun(ctx context.Context, path, cacheKey string, a BatchAction) (any, error) {
	type dispatcher func(context.Context, string, string, BatchAction) (any, error, bool)

	for _, d := range []dispatcher{p.batchAnalysis, p.batchClinic, p.batchConstraint, p.batchRefactor} {
		if r, err, ok := d(ctx, path, cacheKey, a); ok {
			return r, err
		}
	}
	return nil, fmt.Errorf("%w: %q", ErrUnknownAction, a.Name)
}

//nolint:revive // (any, error, bool) is the intentional 3-return dispatch pattern
func (p *Protocol) batchAnalysis(ctx context.Context, path, cacheKey string, a BatchAction) (any, error, bool) {
	switch a.Name {
	case "hot_spots":
		r, err := p.GetHotSpots(ctx, path, batchInt(a, "churn_days"), batchInt(a, "top_n"), cacheKey)
		return r, err, true
	case "deps":
		r, err := p.GetDependencies(ctx, path, batchStr(a, "component"), cacheKey)
		return r, err, true
	case "coupling":
		r, err := p.GetCouplingTable(ctx, path, batchStr(a, "sort_by"), batchInt(a, "top_n"), cacheKey)
		return r, err, true
	case "cycles":
		r, err := p.GetCycles(ctx, path, batchStrSlice(a, "layers"), cacheKey)
		return r, err, true
	case "violations":
		r, err := p.GetViolations(ctx, path, batchStrSlice(a, "layers"), cacheKey)
		return r, err, true
	case "callers":
		r, err := p.GetCallers(ctx, path, batchStr(a, "symbol"), cacheKey)
		return r, err, true
	case "callees":
		r, err := p.GetCallees(ctx, path, batchStr(a, "symbol"), cacheKey)
		return r, err, true
	case "call_path":
		r, err := p.GetCallPath(ctx, path, batchStr(a, "from"), batchStr(a, "to"), cacheKey)
		return r, err, true
	case "component":
		r, err := p.GetComponentDetail(ctx, path, batchStr(a, "name"), cacheKey)
		return r, err, true
	case "search":
		r, err := p.SearchComponents(ctx, path, batchStr(a, "query"), cacheKey)
		return r, err, true
	case "symbol_search":
		r, err := p.SearchSymbols(ctx, path, batchStr(a, "pattern"), cacheKey)
		return r, err, true
	case "risk_scores":
		r, err := p.GetRiskScores(ctx, path, cacheKey)
		return r, err, true
	case "preset":
		r, err := p.RunPreset(ctx, path, batchStr(a, "name"), cacheKey)
		return r, err, true
	case "impact":
		r, err := p.GetImpact(ctx, path, batchStr(a, "component"), cacheKey)
		return r, err, true
	default:
		return nil, nil, false
	}
}

//nolint:revive // (any, error, bool) is the intentional 3-return dispatch pattern
func (p *Protocol) batchClinic(ctx context.Context, path, cacheKey string, a BatchAction) (any, error, bool) {
	switch a.Name {
	case "pattern_scan":
		r, err := p.GetPatternScan(ctx, path, cacheKey)
		return r, err, true
	case "pattern_catalog":
		return p.GetPatternCatalog(batchStr(a, "filter")), nil, true
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
func (p *Protocol) batchConstraint(ctx context.Context, path, cacheKey string, a BatchAction) (any, error, bool) {
	switch a.Name {
	case "blast_radius":
		r, err := p.GetBlastRadius(ctx, path, batchStrSlice(a, "files"), batchStr(a, "since"), cacheKey)
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
		r, err := p.GetSymbolBlastRadius(ctx, path, batchStr(a, "symbol"), cacheKey)
		return r, err, true
	case "interface_metrics":
		r, err := p.GetInterfaceMetrics(ctx, path, cacheKey)
		return r, err, true
	case "leverage":
		r, err := p.GetLeverage(ctx, path, batchStr(a, "target"), cacheKey)
		return r, err, true
	case "api_surface":
		r, err := p.GetAPISurface(ctx, path, batchStrSlice(a, "trusted"), cacheKey)
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
func (p *Protocol) batchRefactor(ctx context.Context, path, cacheKey string, a BatchAction) (any, error, bool) {
	switch a.Name {
	case "drift":
		r, err := p.GetDrift(ctx, path, cacheKey)
		return r, err, true
	case "what_if":
		return nil, fmt.Errorf("%w: what_if requires structured FileMove params", ErrUnsupportedBatch), true
	case "diff_intelligence":
		r, err := p.GetDiffIntelligence(ctx, path, batchStr(a, "since"), cacheKey)
		return r, err, true
	default:
		return nil, nil, false
	}
}

// --- Param helpers ---

func batchStr(a BatchAction, key string) string {
	v, ok := a.Params[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

func batchInt(a BatchAction, key string) int {
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

func batchStrSlice(a BatchAction, key string) []string {
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
