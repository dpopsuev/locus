package diagram

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dpopsuev/locus/internal/arch"
)

// renderCoupling produces a Mermaid sankey-beta diagram showing fan-in/fan-out
// coupling flows between components.
func renderCoupling(report *arch.ContextReport, opts Options) string {
	type flow struct {
		from  string
		to    string
		value int
	}

	var flows []flow
	for _, e := range report.Architecture.Edges {
		if opts.Scope != "" && e.From != opts.Scope && e.To != opts.Scope {
			continue
		}
		v := e.CallSites
		if v <= 0 {
			v = e.LOCSurface
		}
		if v <= 0 {
			v = e.Weight
		}
		if v <= 0 {
			v = 1
		}
		flows = append(flows, flow{from: e.From, to: e.To, value: v})
	}

	sort.Slice(flows, func(i, j int) bool { return flows[i].value > flows[j].value })

	if opts.TopN > 0 && len(flows) > opts.TopN {
		flows = flows[:opts.TopN]
	}

	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("config:\n")
	b.WriteString("  sankey:\n")
	b.WriteString("    showValues: true\n")
	b.WriteString("---\n")
	b.WriteString("sankey-beta\n\n")

	for _, f := range flows {
		fmt.Fprintf(&b, "%s,%s,%d\n", sanitizeSankey(f.from), sanitizeSankey(f.to), f.value)
	}

	return b.String()
}

func sanitizeSankey(s string) string {
	if strings.ContainsAny(s, ",\"") {
		return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
	}
	return s
}

// fanIn computes fan-in counts from edges.
func fanIn(edges []arch.ArchEdge) map[string]int {
	m := make(map[string]int)
	for _, e := range edges {
		m[e.To]++
	}
	return m
}

// fanOut computes fan-out counts from edges.
func fanOut(edges []arch.ArchEdge) map[string]int {
	m := make(map[string]int)
	for _, e := range edges {
		m[e.From]++
	}
	return m
}
