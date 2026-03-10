package diagram

import (
	"fmt"
	"strings"

	"github.com/dpopsuev/locus/internal/arch"
)

func renderDependency(in Input, opts Options) string {
	m := in.Report.Architecture
	rt := in.ResolvedTheme

	fi := fanIn(m.Edges)
	churnMap := make(map[string]int)
	for _, s := range m.Services {
		churnMap[s.Name] = s.Churn
	}

	var b strings.Builder
	b.WriteString(rt.InitDirective() + "\n")
	b.WriteString("graph TD\n")
	b.WriteString(rt.ClassDefs() + "\n")

	for _, s := range m.Services {
		if opts.Scope != "" && s.Name != opts.Scope && !isEdgeNeighbor(m.Edges, opts.Scope, s.Name) {
			continue
		}
		id := mermaidID(s.Name)
		label := s.Name
		if s.Churn > 0 {
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

func isEdgeNeighbor(edges []arch.ArchEdge, scope, name string) bool {
	for _, e := range edges {
		if (e.From == scope && e.To == name) || (e.To == scope && e.From == name) {
			return true
		}
	}
	return false
}
