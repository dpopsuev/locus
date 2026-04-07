package metrics

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dpopsuev/locus/internal/diagram/core"
)

// Coupling renders a Sankey diagram of coupling flows.
func Coupling(in core.Input, opts core.Options) string {
	report := in.Report
	rt := in.ResolvedTheme

	type flow struct {
		from  string
		to    string
		value int
	}

	flows := make([]flow, 0, len(report.Architecture.Edges))
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
	fmt.Fprintf(&b, "%%%% Health legend: Healthy=%s  Sick=%s  Fatal=%s\n",
		rt.ColorHex("green"), rt.ColorHex("yellow"), rt.ColorHex("red"))
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
