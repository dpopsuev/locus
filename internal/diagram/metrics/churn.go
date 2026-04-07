package metrics

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dpopsuev/locus/internal/diagram/core"
	"github.com/dpopsuev/locus/internal/graph"
)

// Churn renders a churn diagram, either timeline or bar chart.
func Churn(in core.Input, opts core.Options) string {
	if len(in.History) >= 2 {
		return renderChurnTimeline(in, opts)
	}
	return renderChurnBar(in, opts)
}

func renderChurnTimeline(in core.Input, opts core.Options) string {
	hist := in.History
	topN := opts.TopN
	if topN <= 0 {
		topN = len(hist)
	}
	if topN > len(hist) {
		topN = len(hist)
	}
	recent := hist
	if len(recent) > topN {
		recent = recent[len(recent)-topN:]
	}

	var b strings.Builder
	b.WriteString(in.ResolvedTheme.InitDirective() + "\n")
	b.WriteString("xychart-beta\n")
	b.WriteString("    title \"Codograph history\"\n")

	dates := make([]string, 0, len(recent))
	components := make([]string, 0, len(recent))
	edges := make([]string, 0, len(recent))
	for _, e := range recent {
		dates = append(dates, fmt.Sprintf("%q", e.Timestamp.Format("Jan 02")))
		components = append(components, fmt.Sprintf("%d", e.Components))
		edges = append(edges, fmt.Sprintf("%d", e.Edges))
	}

	fmt.Fprintf(&b, "    x-axis [%s]\n", strings.Join(dates, ", "))
	b.WriteString("    y-axis \"Count\"\n")
	fmt.Fprintf(&b, "    line [%s]\n", strings.Join(components, ", "))
	fmt.Fprintf(&b, "    line [%s]\n", strings.Join(edges, ", "))

	return b.String()
}

func renderChurnBar(in core.Input, opts core.Options) string {
	report := in.Report

	fi := graph.FanIn(report.Architecture.Edges)

	type entry struct {
		name   string
		churn  int
		health core.Health
	}

	var entries []entry
	for i := range report.Architecture.Services {
		svc := &report.Architecture.Services[i]
		if svc.Churn > 0 {
			h := core.ClassifyHealth(fi[svc.Name], svc.Churn)
			entries = append(entries, entry{name: svc.Name, churn: svc.Churn, health: h})
		}
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].churn > entries[j].churn })

	topN := opts.TopN
	if topN <= 0 {
		topN = 10
	}
	if topN > len(entries) {
		topN = len(entries)
	}
	entries = entries[:topN]

	if len(entries) == 0 {
		return "xychart-beta\n    title \"No churn data available\"\n    x-axis [\"N/A\"]\n    y-axis \"Churn\" 0 --> 1\n    bar [0]\n"
	}

	healthMarker := func(h core.Health) string {
		switch h {
		case core.Fatal:
			return " \u2718"
		case core.Sick:
			return " \u26A0"
		default:
			return " \u2714"
		}
	}

	var b strings.Builder
	b.WriteString(in.ResolvedTheme.InitDirective() + "\n")
	b.WriteString("xychart-beta\n")
	b.WriteString("    title \"Component churn\"\n")

	names := make([]string, 0, len(entries))
	values := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, fmt.Sprintf("%q", e.name+healthMarker(e.health)))
		values = append(values, fmt.Sprintf("%d", e.churn))
	}

	fmt.Fprintf(&b, "    x-axis [%s]\n", strings.Join(names, ", "))
	b.WriteString("    y-axis \"Commits\"\n")
	fmt.Fprintf(&b, "    bar [%s]\n", strings.Join(values, ", "))

	return b.String()
}
