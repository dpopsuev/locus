package diagram

import (
	"fmt"
	"strings"

	"github.com/dpopsuev/locus/internal/arch"
	"github.com/dpopsuev/locus/internal/graph"
)

func renderDependency(in Input, opts Options) string {
	m := in.Report.Architecture
	rt := in.ResolvedTheme

	fi := graph.FanIn(m.Edges)
	churnMap := make(map[string]int)
	for i := range m.Services {
		churnMap[m.Services[i].Name] = m.Services[i].Churn
	}

	var b strings.Builder
	b.WriteString(rt.InitDirective() + "\n")
	b.WriteString("graph TD\n")
	b.WriteString(rt.ClassDefs() + "\n")

	enrichSet := parseEnrich(opts.Enrich)

	for i := range m.Services {
		s := &m.Services[i]
		if opts.Scope != "" && s.Name != opts.Scope && !isEdgeNeighbor(m.Edges, opts.Scope, s.Name) {
			continue
		}
		id := mermaidID(s.Name)
		label := s.Name
		if len(enrichSet) > 0 {
			label += enrichLabel(*s, fi[s.Name], enrichSet)
		} else if s.Churn > 0 {
			label += fmt.Sprintf(" [churn:%d]", s.Churn)
		}
		h := ClassifyHealth(fi[s.Name], churnMap[s.Name])
		suffix := rt.NodeSuffix(h)
		if strings.HasPrefix(s.Name, "cmd/") {
			suffix = ":::entry"
		}
		fmt.Fprintf(&b, "    %s[\"%s\"]%s\n", id, label, suffix)
	}

	for _, e := range m.Edges {
		if opts.Scope != "" && e.From != opts.Scope && e.To != opts.Scope {
			continue
		}
		fromID := mermaidID(e.From)
		toID := mermaidID(e.To)
		if e.Weight > 0 {
			fmt.Fprintf(&b, "    %s -->|\"%d\"| %s\n", fromID, e.Weight, toID)
		} else {
			fmt.Fprintf(&b, "    %s --> %s\n", fromID, toID)
		}
	}

	return b.String()
}

func parseEnrich(enrich string) map[string]bool {
	if enrich == "" {
		return nil
	}
	set := make(map[string]bool)
	for _, s := range strings.Split(enrich, ",") {
		s = strings.TrimSpace(s)
		if s != "" {
			set[s] = true
		}
	}
	return set
}

func enrichLabel(s arch.ArchService, fi int, enrichSet map[string]bool) string {
	var parts []string
	if enrichSet["loc"] && s.LOC > 0 {
		parts = append(parts, fmt.Sprintf("%d LOC", s.LOC))
	}
	if enrichSet["fan_in"] {
		parts = append(parts, fmt.Sprintf("fan-in:%d", fi))
	}
	if enrichSet["churn"] && s.Churn > 0 {
		parts = append(parts, fmt.Sprintf("churn:%d", s.Churn))
	}
	if len(parts) == 0 {
		return ""
	}
	return "\\n" + strings.Join(parts, " | ")
}

func isEdgeNeighbor(edges []arch.ArchEdge, scope, name string) bool {
	for _, e := range edges {
		if (e.From == scope && e.To == name) || (e.To == scope && e.From == name) {
			return true
		}
	}
	return false
}
