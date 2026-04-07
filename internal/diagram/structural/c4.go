package structural

import (
	"fmt"
	"strings"

	"github.com/dpopsuev/locus/internal/diagram/core"
	"github.com/dpopsuev/locus/internal/graph"
)

// C4 renders a C4 Component diagram.
func C4(in core.Input, opts core.Options) string {
	report := in.Report
	rt := in.ResolvedTheme

	depth := opts.Depth
	if depth <= 0 {
		depth = report.SuggestedDepth
	}
	if depth <= 0 {
		depth = 1
	}

	fi := graph.FanIn(report.Architecture.Edges)
	churnMap := make(map[string]int)
	for i := range report.Architecture.Services {
		churnMap[report.Architecture.Services[i].Name] = report.Architecture.Services[i].Churn
	}

	type container struct {
		name       string
		components []struct {
			name   string
			churn  int
			syms   int
			health core.Health
		}
	}

	groups := make(map[string]*container)
	var order []string

	for i := range report.Architecture.Services {
		svc := &report.Architecture.Services[i]
		g := groupName(svc.Name, depth)
		if _, ok := groups[g]; !ok {
			groups[g] = &container{name: g}
			order = append(order, g)
		}
		h := core.ClassifyHealth(fi[svc.Name], churnMap[svc.Name])
		groups[g].components = append(groups[g].components, struct {
			name   string
			churn  int
			syms   int
			health core.Health
		}{name: svc.Name, churn: svc.Churn, syms: len(svc.Symbols), health: h})
	}

	var b strings.Builder
	b.WriteString("C4Component\n")
	fmt.Fprintf(&b, "    title %s\n\n", report.ModulePath)

	for _, gName := range order {
		g := groups[gName]
		id := core.MermaidID(g.name)

		if len(g.components) == 1 && g.components[0].name == g.name {
			comp := g.components[0]
			tech := "package"
			desc := fmt.Sprintf("%d symbols", comp.syms)
			if comp.churn > 0 {
				desc += fmt.Sprintf(", churn %d", comp.churn)
			}
			tag := rt.HealthClass(comp.health)
			fmt.Fprintf(&b, "    Component(%s, \"%s\", \"%s\", \"%s\", $tags=\"%s\")\n", id, comp.name, tech, desc, tag)
			continue
		}

		fmt.Fprintf(&b, "    Container_Boundary(%s_boundary, \"%s\") {\n", id, g.name)
		for _, comp := range g.components {
			cid := core.MermaidID(comp.name)
			tech := "package"
			desc := fmt.Sprintf("%d symbols", comp.syms)
			if comp.churn > 0 {
				desc += fmt.Sprintf(", churn %d", comp.churn)
			}
			tag := rt.HealthClass(comp.health)
			fmt.Fprintf(&b, "        Component(%s, \"%s\", \"%s\", \"%s\", $tags=\"%s\")\n", cid, comp.name, tech, desc, tag)
		}
		b.WriteString("    }\n")
	}

	b.WriteByte('\n')
	seen := make(map[[2]string]bool)
	for _, e := range report.Architecture.Edges {
		fromG := groupName(e.From, depth)
		toG := groupName(e.To, depth)
		fromID := core.MermaidID(e.From)
		toID := core.MermaidID(e.To)
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

	b.WriteByte('\n')
	fmt.Fprintf(&b, "    UpdateElementStyle(*, $fontColor=\"%s\", $borderColor=\"%s\")\n",
		rt.ColorHex("text"), rt.ColorHex("blue"))
	fmt.Fprintf(&b, "    UpdateElementStyle(healthy, $bgColor=\"%s\", $borderColor=\"%s\")\n",
		rt.ColorHex("green"), rt.ColorHex("green"))
	fmt.Fprintf(&b, "    UpdateElementStyle(sick, $bgColor=\"%s\", $borderColor=\"%s\")\n",
		rt.ColorHex("yellow"), rt.ColorHex("yellow"))
	fmt.Fprintf(&b, "    UpdateElementStyle(fatal, $bgColor=\"%s\", $borderColor=\"%s\")\n",
		rt.ColorHex("red"), rt.ColorHex("red"))

	return b.String()
}

func groupName(name string, depth int) string {
	parts := strings.SplitN(name, "/", depth+1)
	if len(parts) > depth {
		parts = parts[:depth]
	}
	return strings.Join(parts, "/")
}
