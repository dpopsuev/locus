package diagram

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

func renderTree(in Input, opts Options) string {
	report := in.Report
	rt := in.ResolvedTheme

	fi := fanIn(report.Architecture.Edges)
	churnMap := make(map[string]int)
	for i := range report.Architecture.Services {
		churnMap[report.Architecture.Services[i].Name] = report.Architecture.Services[i].Churn
	}

	root := filepath.Base(report.ModulePath)
	if root == "" || root == "." {
		root = "project"
	}

	depth := opts.Depth
	if depth <= 0 {
		depth = 1
	}

	type node struct {
		name     string
		children []string
		symbols  int
	}

	groups := make(map[string]*node)
	var order []string

	for i := range report.Architecture.Services {
		svc := &report.Architecture.Services[i]
		g := groupName(svc.Name, depth)
		if _, ok := groups[g]; !ok {
			groups[g] = &node{name: g}
			order = append(order, g)
		}
		if svc.Name != g {
			groups[g].children = append(groups[g].children, svc.Name)
		}
		groups[g].symbols += len(svc.Symbols)
	}

	sort.Strings(order)

	healthIcon := func(name string) string {
		h := ClassifyHealth(fi[name], churnMap[name])
		switch h {
		case Fatal:
			return "\u2718 "
		case Sick:
			return "\u26A0 "
		default:
			return ""
		}
	}

	_ = rt

	var b strings.Builder
	b.WriteString("mindmap\n")
	fmt.Fprintf(&b, "    root((\"%s\"))\n", root)

	for _, gName := range order {
		g := groups[gName]
		if len(g.children) == 0 {
			label := healthIcon(g.name) + g.name
			if g.symbols > 0 {
				label += fmt.Sprintf(" (%d sym)", g.symbols)
			}
			fmt.Fprintf(&b, "        %s\n", label)
			continue
		}

		label := healthIcon(g.name) + g.name
		if g.symbols > 0 {
			label += fmt.Sprintf(" (%d sym)", g.symbols)
		}
		fmt.Fprintf(&b, "        %s\n", label)
		sort.Strings(g.children)
		for _, child := range g.children {
			var symCount int
			for i := range report.Architecture.Services {
				if report.Architecture.Services[i].Name == child {
					symCount = len(report.Architecture.Services[i].Symbols)
					break
				}
			}
			childLabel := healthIcon(child) + child
			if symCount > 0 {
				childLabel += fmt.Sprintf(" (%d sym)", symCount)
			}
			fmt.Fprintf(&b, "            %s\n", childLabel)
		}
	}

	return b.String()
}
