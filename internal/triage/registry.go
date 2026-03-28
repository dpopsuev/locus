package triage

import (
	"sort"
	"strings"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// ToolMeta is the single source of truth for a Locus tool. It drives both
// MCP registration (Name, Description) and triage matching (Keywords,
// Categories, Rationale).
type ToolMeta struct {
	Name        string
	Description string
	Keywords    []string
	Categories  []string
	DefaultArgs map[string]any
	Rationale   map[string]string // category -> why this tool matters
	Priority    int               // ordering within a category (lower = run first)
}

// TriageResult is the output of matching an intent against the registry.
type TriageResult struct {
	Category   string      `json:"category"`
	Confidence float64     `json:"confidence"`
	Tools      []ToolMatch `json:"tools"`
}

// ToolMatch is a single tool recommendation within a TriageResult.
type ToolMatch struct {
	Name   string         `json:"name"`
	Params map[string]any `json:"params,omitempty"`
	Reason string         `json:"reason"`
}

// Registry holds all registered tools and provides intent-based triage.
type Registry struct {
	tools []ToolMeta
}

func New() *Registry {
	return &Registry{}
}

// Register adds a tool to the registry for triage matching.
func (r *Registry) Register(meta ToolMeta) {
	r.tools = append(r.tools, meta)
}

// AddTool registers a tool for both MCP serving and triage matching.
// This is the converged registration point — one call replaces separate
// sdkmcp.AddTool + triage.Register calls.
func AddTool[In any](r *Registry, srv *sdkmcp.Server, meta ToolMeta, handler sdkmcp.ToolHandlerFor[In, any]) {
	sdkmcp.AddTool(srv, &sdkmcp.Tool{
		Name:        meta.Name,
		Description: meta.Description,
	}, handler)
	r.Register(meta)
}

// Triage matches a natural language intent against registered tools and
// returns a ranked list of recommendations grouped by the best-matching
// category. If path is non-empty, it is injected into each tool's params.
func (r *Registry) Triage(intent, path string) TriageResult {
	tokens := tokenize(intent)
	if len(tokens) == 0 {
		return r.fallback(path)
	}

	type scored struct {
		meta  ToolMeta
		score float64
	}

	// Score each tool by keyword overlap with intent tokens.
	var hits []scored
	for _, t := range r.tools {
		s := jaccardScore(tokens, t.Keywords)
		if s > 0 {
			hits = append(hits, scored{meta: t, score: s})
		}
	}
	if len(hits) == 0 {
		return r.fallback(path)
	}

	// Find the best category: for each category, sum scores of tools in it.
	catScore := make(map[string]float64)
	for _, h := range hits {
		for _, cat := range h.meta.Categories {
			catScore[cat] += h.score
		}
	}
	bestCat := ""
	bestScore := 0.0
	for cat, s := range catScore {
		if s > bestScore || (s == bestScore && (bestCat == "" || cat < bestCat)) {
			bestScore = s
			bestCat = cat
		}
	}

	// Filter to tools in the best category, sorted by priority.
	var matches []scored
	for _, h := range hits {
		for _, cat := range h.meta.Categories {
			if cat == bestCat {
				matches = append(matches, h)
				break
			}
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].meta.Priority != matches[j].meta.Priority {
			return matches[i].meta.Priority < matches[j].meta.Priority
		}
		return matches[i].score > matches[j].score
	})

	confidence := bestScore / float64(len(catScore))
	if confidence > 1 {
		confidence = 1
	}

	tools := make([]ToolMatch, 0, len(matches))
	for _, m := range matches {
		tm := ToolMatch{
			Name:   m.meta.Name,
			Params: mergeParams(m.meta.DefaultArgs, path),
			Reason: m.meta.Rationale[bestCat],
		}
		if tm.Reason == "" {
			tm.Reason = m.meta.Description
		}
		tools = append(tools, tm)
	}

	return TriageResult{
		Category:   bestCat,
		Confidence: confidence,
		Tools:      tools,
	}
}

// List returns all registered tools.
func (r *Registry) List() []ToolMeta {
	out := make([]ToolMeta, len(r.tools))
	copy(out, r.tools)
	return out
}

// ByCategory returns tools that belong to the given category.
func (r *Registry) ByCategory(cat string) []ToolMeta {
	var out []ToolMeta
	for _, t := range r.tools {
		for _, c := range t.Categories {
			if strings.EqualFold(c, cat) {
				out = append(out, t)
				break
			}
		}
	}
	return out
}

func (r *Registry) fallback(path string) TriageResult {
	return TriageResult{
		Category:   "general",
		Confidence: 0,
		Tools: []ToolMatch{{
			Name:   "scan_project",
			Params: mergeParams(nil, path),
			Reason: "No specific intent matched — start with a full architecture scan",
		}},
	}
}

func mergeParams(defaults map[string]any, path string) map[string]any {
	out := make(map[string]any, len(defaults)+1)
	for k, v := range defaults {
		out[k] = v
	}
	if path != "" {
		out["path"] = path
	}
	return out
}
