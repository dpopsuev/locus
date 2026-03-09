package diagram

import (
	"fmt"
	"strings"

	"github.com/dpopsuev/locus/internal/arch"
)

// renderC4 produces a Mermaid C4Component diagram.
// Top-level depth groups become containers; leaf packages become components.
func renderC4(report *arch.ContextReport, opts Options) string {
	depth := opts.Depth
	if depth <= 0 {
		depth = report.SuggestedDepth
	}
	if depth <= 0 {
		depth = 1
	}

	type container struct {
		name       string
		components []arch.ArchService
	}

	groups := make(map[string]*container)
	var order []string

	for _, svc := range report.Architecture.Services {
		g := groupName(svc.Name, depth)
		if _, ok := groups[g]; !ok {
			groups[g] = &container{name: g}
			order = append(order, g)
		}
		groups[g].components = append(groups[g].components, svc)
	}

	var b strings.Builder
	b.WriteString("C4Component\n")
	fmt.Fprintf(&b, "    title %s\n\n", report.ModulePath)

	for _, gName := range order {
		g := groups[gName]
		id := mermaidID(g.name)

		if len(g.components) == 1 && g.components[0].Name == g.name {
			comp := g.components[0]
			tech := "package"
			desc := fmt.Sprintf("%d symbols", len(comp.Symbols))
			if comp.Churn > 0 {
				desc += fmt.Sprintf(", churn %d", comp.Churn)
			}
			fmt.Fprintf(&b, "    Component(%s, \"%s\", \"%s\", \"%s\")\n", id, comp.Name, tech, desc)
			continue
		}

		fmt.Fprintf(&b, "    Container_Boundary(%s_boundary, \"%s\") {\n", id, g.name)
		for _, comp := range g.components {
			cid := mermaidID(comp.Name)
			tech := "package"
			desc := fmt.Sprintf("%d symbols", len(comp.Symbols))
			if comp.Churn > 0 {
				desc += fmt.Sprintf(", churn %d", comp.Churn)
			}
			fmt.Fprintf(&b, "        Component(%s, \"%s\", \"%s\", \"%s\")\n", cid, comp.Name, tech, desc)
		}
		b.WriteString("    }\n")
	}

	b.WriteByte('\n')
	seen := make(map[[2]string]bool)
	for _, e := range report.Architecture.Edges {
		fromG := groupName(e.From, depth)
		toG := groupName(e.To, depth)
		fromID := mermaidID(e.From)
		toID := mermaidID(e.To)
		key := [2]string{fromID, toID}
		if seen[key] {
			continue
		}
		seen[key] = true
		if fromG == toG {
			continue
		}
		label := "uses"
		if e.Protocol != "" && e.Protocol != "import" {
			label = e.Protocol
		}
		fmt.Fprintf(&b, "    Rel(%s, %s, \"%s\")\n", fromID, toID, label)
	}

	return b.String()
}

func groupName(name string, depth int) string {
	parts := strings.SplitN(name, "/", depth+1)
	if len(parts) > depth {
		parts = parts[:depth]
	}
	return strings.Join(parts, "/")
}
