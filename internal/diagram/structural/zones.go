package structural

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dpopsuev/locus/internal/diagram/core"
	"github.com/dpopsuev/locus/internal/graph"
)

// Zones renders a zone overview graph diagram.
func Zones(in core.Input, _ core.Options) string {
	m := in.Report.Architecture
	rt := in.ResolvedTheme

	fi := graph.FanIn(m.Edges)
	churnMap := make(map[string]int)
	symbolCount := make(map[string]int)
	for i := range m.Services {
		churnMap[m.Services[i].Name] = m.Services[i].Churn
		symbolCount[m.Services[i].Name] = len(m.Services[i].Symbols)
	}

	// Group components by top-level directory (zone).
	zones := make(map[string][]string)
	for i := range m.Services {
		s := &m.Services[i]
		zone := topLevelDir(s.Name)
		zones[zone] = append(zones[zone], s.Name)
	}

	// Sort zone names for determinism.
	zoneNames := make([]string, 0, len(zones))
	for z := range zones {
		zoneNames = append(zoneNames, z)
	}
	sort.Strings(zoneNames)

	var b strings.Builder
	b.WriteString(rt.InitDirective() + "\n")
	b.WriteString("graph TD\n")
	b.WriteString(rt.ClassDefs() + "\n")

	for _, zone := range zoneNames {
		components := zones[zone]
		sort.Strings(components)

		zoneID := core.MermaidID(zone)
		fmt.Fprintf(&b, "    subgraph %s[\"%s\"]\n", zoneID, zone)

		for _, name := range components {
			id := core.MermaidID(name)
			syms := symbolCount[name]
			churn := churnMap[name]
			label := name
			if syms > 0 || churn > 0 {
				label += fmt.Sprintf("\\n%d symbols", syms)
				if churn > 0 {
					label += fmt.Sprintf(" | churn:%d", churn)
				}
			}
			h := core.ClassifyHealth(fi[name], churnMap[name])
			suffix := rt.NodeSuffix(h)
			fmt.Fprintf(&b, "        %s[\"%s\"]%s\n", id, label, suffix)
		}
		b.WriteString("    end\n")
	}

	// Render cross-zone edges.
	for _, e := range m.Edges {
		fromZone := topLevelDir(e.From)
		toZone := topLevelDir(e.To)
		if fromZone != toZone {
			fmt.Fprintf(&b, "    %s --> %s\n", core.MermaidID(e.From), core.MermaidID(e.To))
		}
	}

	return b.String()
}

func topLevelDir(name string) string {
	if idx := strings.Index(name, "/"); idx >= 0 {
		return name[:idx]
	}
	return "(root)"
}
