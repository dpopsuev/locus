package arch

import (
	"fmt"
	"sort"
	"strings"
)

// RenderMarkdown produces a human-readable markdown summary of a ContextReport.
// Designed for direct agent consumption without any post-processing.
func RenderMarkdown(report *ContextReport) string {
	fanIn := make(map[string]int)
	fanOut := make(map[string]int)
	for _, e := range report.Architecture.Edges {
		fanIn[e.To]++
		fanOut[e.From]++
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", report.ModulePath)
	fmt.Fprintf(&b, "Scanner: %s | Components: %d | Edges: %d\n\n",
		report.Scanner, len(report.Architecture.Services), len(report.Architecture.Edges))

	b.WriteString(RenderCouplingTable(report, "fan_in", 0))
	b.WriteByte('\n')
	b.WriteString(RenderEdgeList(report, ""))
	b.WriteByte('\n')

	if len(report.HotSpots) > 0 {
		b.WriteString("## Hot Spots\n\n")
		spots := make([]HotSpot, len(report.HotSpots))
		copy(spots, report.HotSpots)
		sort.Slice(spots, func(i, j int) bool { return spots[i].Churn > spots[j].Churn })
		n := len(spots)
		if n > 10 {
			n = 10
		}
		for _, s := range spots[:n] {
			nest := ""
			if s.Nesting > 0 {
				nest = fmt.Sprintf("  nesting=%d", s.Nesting)
			}
			fmt.Fprintf(&b, "- %s  churn=%d  fan_in=%d%s\n", s.Component, s.Churn, fanIn[s.Component], nest)
		}
		b.WriteByte('\n')
	}

	return b.String()
}

// RenderCouplingTable produces a markdown table of components with fan-in, fan-out,
// churn, nesting, and symbol count. sortBy is "fan_in", "fan_out", "churn", or "nesting".
// topN=0 means all.
func RenderCouplingTable(report *ContextReport, sortBy string, topN int) string {
	fanIn := make(map[string]int)
	fanOut := make(map[string]int)
	for _, e := range report.Architecture.Edges {
		fanIn[e.To]++
		fanOut[e.From]++
	}

	type row struct {
		Name       string
		FanIn      int
		FanOut     int
		LOC        int
		Churn      int
		Symbols    int
		MaxNesting int
	}

	rows := make([]row, 0, len(report.Architecture.Services))
	for _, svc := range report.Architecture.Services {
		fi := fanIn[svc.Name]
		fo := fanOut[svc.Name]
		if fi > 0 || fo > 0 {
			rows = append(rows, row{
				Name:       svc.Name,
				FanIn:      fi,
				FanOut:     fo,
				LOC:        svc.LOC,
				Churn:      svc.Churn,
				Symbols:    len(svc.Symbols),
				MaxNesting: svc.MaxNesting,
			})
		}
	}

	switch sortBy {
	case "fan_out":
		sort.Slice(rows, func(i, j int) bool { return rows[i].FanOut > rows[j].FanOut })
	case "churn":
		sort.Slice(rows, func(i, j int) bool { return rows[i].Churn > rows[j].Churn })
	case "nesting":
		sort.Slice(rows, func(i, j int) bool { return rows[i].MaxNesting > rows[j].MaxNesting })
	case "loc":
		sort.Slice(rows, func(i, j int) bool { return rows[i].LOC > rows[j].LOC })
	default:
		sort.Slice(rows, func(i, j int) bool { return rows[i].FanIn > rows[j].FanIn })
	}

	if topN > 0 && topN < len(rows) {
		rows = rows[:topN]
	}

	var b strings.Builder
	b.WriteString("## Package Coupling\n\n")

	nameW := len("Package")
	for _, r := range rows {
		if len(r.Name) > nameW {
			nameW = len(r.Name)
		}
	}

	fmt.Fprintf(&b, "%-*s  %6s  %7s  %5s  %5s  %7s  %7s\n", nameW, "Package", "Fan-In", "Fan-Out", "LOC", "Churn", "MaxNest", "Symbols")
	fmt.Fprintf(&b, "%s  %s  %s  %s  %s  %s  %s\n",
		strings.Repeat("-", nameW),
		strings.Repeat("-", 6),
		strings.Repeat("-", 7),
		strings.Repeat("-", 5),
		strings.Repeat("-", 5),
		strings.Repeat("-", 7),
		strings.Repeat("-", 7))

	for _, r := range rows {
		fmt.Fprintf(&b, "%-*s  %6d  %7d  %5d  %5d  %7d  %7d\n", nameW, r.Name, r.FanIn, r.FanOut, r.LOC, r.Churn, r.MaxNesting, r.Symbols)
	}

	return b.String()
}

// RenderEdgeList produces a readable list of dependency edges.
// If component is non-empty, only edges involving that component are shown.
func RenderEdgeList(report *ContextReport, component string) string {
	var b strings.Builder
	b.WriteString("## Dependencies\n\n")

	edges := report.Architecture.Edges
	sorted := make([]ArchEdge, len(edges))
	copy(sorted, edges)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].From < sorted[j].From })

	for _, e := range sorted {
		if component != "" && e.From != component && e.To != component {
			continue
		}
		detail := ""
		if e.Weight > 0 || e.CallSites > 0 || e.LOCSurface > 0 {
			parts := make([]string, 0, 3)
			if e.Weight > 0 {
				parts = append(parts, fmt.Sprintf("weight=%d", e.Weight))
			}
			if e.CallSites > 0 {
				parts = append(parts, fmt.Sprintf("calls=%d", e.CallSites))
			}
			if e.LOCSurface > 0 {
				parts = append(parts, fmt.Sprintf("loc=%d", e.LOCSurface))
			}
			detail = "  (" + strings.Join(parts, ", ") + ")"
		}
		fmt.Fprintf(&b, "  %s -> %s%s\n", e.From, e.To, detail)
	}

	return b.String()
}
