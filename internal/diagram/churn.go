package diagram

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dpopsuev/locus/internal/arch"
	"github.com/dpopsuev/locus/internal/history"
)

// renderChurn produces a Mermaid xychart-beta showing component/edge counts
// over time from codograph history. When history is unavailable, falls back
// to a bar chart of per-component churn from the current scan.
func renderChurn(report *arch.ContextReport, hist []history.EntrySummary, opts Options) string {
	if len(hist) >= 2 {
		return renderChurnTimeline(hist, opts)
	}
	return renderChurnBar(report, opts)
}

func renderChurnTimeline(hist []history.EntrySummary, opts Options) string {
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
	b.WriteString("xychart-beta\n")
	b.WriteString("    title \"Codograph history\"\n")

	var dates, components, edges []string
	for _, e := range recent {
		dates = append(dates, fmt.Sprintf("\"%s\"", e.Timestamp.Format("Jan 02")))
		components = append(components, fmt.Sprintf("%d", e.Components))
		edges = append(edges, fmt.Sprintf("%d", e.Edges))
	}

	fmt.Fprintf(&b, "    x-axis [%s]\n", strings.Join(dates, ", "))
	b.WriteString("    y-axis \"Count\"\n")
	fmt.Fprintf(&b, "    line [%s]\n", strings.Join(components, ", "))
	fmt.Fprintf(&b, "    line [%s]\n", strings.Join(edges, ", "))

	return b.String()
}

func renderChurnBar(report *arch.ContextReport, opts Options) string {
	type entry struct {
		name  string
		churn int
	}

	var entries []entry
	for _, svc := range report.Architecture.Services {
		if svc.Churn > 0 {
			entries = append(entries, entry{name: svc.Name, churn: svc.Churn})
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

	var b strings.Builder
	b.WriteString("xychart-beta\n")
	b.WriteString("    title \"Component churn\"\n")

	var names, values []string
	for _, e := range entries {
		names = append(names, fmt.Sprintf("\"%s\"", e.name))
		values = append(values, fmt.Sprintf("%d", e.churn))
	}

	fmt.Fprintf(&b, "    x-axis [%s]\n", strings.Join(names, ", "))
	b.WriteString("    y-axis \"Commits\"\n")
	fmt.Fprintf(&b, "    bar [%s]\n", strings.Join(values, ", "))

	return b.String()
}
