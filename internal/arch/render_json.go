package arch

import (
	"encoding/json"
	"sort"
)

// jsonReport is the top-level JSON output structure for mcontext.
type jsonReport struct {
	Project        string              `json:"project"`
	Scanner        string              `json:"scanner"`
	Components     []jsonComponent     `json:"components"`
	Edges          []jsonEdge          `json:"edges"`
	SuggestedDepth int                 `json:"suggested_depth,omitempty"`
	HotSpots       []HotSpot           `json:"hot_spots,omitempty"`
	RecentCommits  []PackageCommit     `json:"recent_commits,omitempty"`
	Authors        map[string][]Author `json:"authors,omitempty"`
	FileHotSpots   []HotFile           `json:"file_hot_spots,omitempty"`
	Anchors        []SemanticAnchor    `json:"anchors,omitempty"`
}

type jsonComponent struct {
	Name    string       `json:"name"`
	Package string       `json:"package,omitempty"`
	FanIn   int          `json:"fan_in"`
	FanOut  int          `json:"fan_out"`
	Churn   int          `json:"churn,omitempty"`
	Symbols []jsonSymbol `json:"symbols,omitempty"`
}

type jsonSymbol struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}

type jsonEdge struct {
	From       string `json:"from"`
	To         string `json:"to"`
	Weight     int    `json:"weight,omitempty"`
	CallSites  int    `json:"call_sites,omitempty"`
	LOCSurface int    `json:"loc_surface,omitempty"`
	Protocol   string `json:"protocol,omitempty"`
}

// RenderJSON serializes a ContextReport into the mcontext JSON schema.
func RenderJSON(report *ContextReport) ([]byte, error) {
	fanIn := make(map[string]int)
	fanOut := make(map[string]int)
	for _, e := range report.Architecture.Edges {
		fanIn[e.To]++
		fanOut[e.From]++
	}

	components := make([]jsonComponent, 0, len(report.Architecture.Services))
	for _, svc := range report.Architecture.Services {
		c := jsonComponent{
			Name:    svc.Name,
			Package: svc.Package,
			FanIn:   fanIn[svc.Name],
			FanOut:  fanOut[svc.Name],
			Churn:   svc.Churn,
		}
		for _, sym := range svc.Symbols {
			c.Symbols = append(c.Symbols, jsonSymbol{Name: sym, Kind: "symbol"})
		}
		components = append(components, c)
	}

	// Enrich symbols from the project model when available.
	if report.Project != nil {
		svcSymbols := buildSymbolIndex(report)
		for i := range components {
			if syms, ok := svcSymbols[components[i].Name]; ok {
				components[i].Symbols = syms
			}
		}
	}

	edges := make([]jsonEdge, 0, len(report.Architecture.Edges))
	for _, e := range report.Architecture.Edges {
		edges = append(edges, jsonEdge{
			From:       e.From,
			To:         e.To,
			Weight:     e.Weight,
			CallSites:  e.CallSites,
			LOCSurface: e.LOCSurface,
			Protocol:   e.Protocol,
		})
	}

	jr := jsonReport{
		Project:        report.ModulePath,
		Scanner:        report.Scanner,
		Components:     components,
		Edges:          edges,
		SuggestedDepth: report.SuggestedDepth,
		HotSpots:       report.HotSpots,
		RecentCommits:  report.RecentCommits,
		Authors:        report.Authors,
		FileHotSpots:   report.FileHotSpots,
		Anchors:        report.Anchors,
	}

	return json.MarshalIndent(jr, "", "  ")
}

func buildSymbolIndex(report *ContextReport) map[string][]jsonSymbol {
	if report.Project == nil {
		return nil
	}
	modPath := report.ModulePath
	result := make(map[string][]jsonSymbol)
	for _, ns := range report.Project.Namespaces {
		rel := shortImportPath(modPath, ns.ImportPath)
		var syms []jsonSymbol
		for _, s := range ns.Symbols {
			if s.Exported {
				syms = append(syms, jsonSymbol{Name: s.Name, Kind: s.Kind.String()})
			}
		}
		if len(syms) > 0 {
			sort.Slice(syms, func(i, j int) bool { return syms[i].Name < syms[j].Name })
			result[rel] = syms
		}
	}
	return result
}
