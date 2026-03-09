package diagram

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dpopsuev/locus/internal/arch"
)

// renderTree produces a Mermaid mindmap of the module hierarchy.
// Root = project name, branches = first-level groups, leaves = packages.
func renderTree(report *arch.ContextReport, opts Options) string {
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

	for _, svc := range report.Architecture.Services {
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

	var b strings.Builder
	b.WriteString("mindmap\n")
	fmt.Fprintf(&b, "    root((\"%s\"))\n", root)

	for _, gName := range order {
		g := groups[gName]
		if len(g.children) == 0 {
			label := g.name
			if g.symbols > 0 {
				label += fmt.Sprintf(" (%d sym)", g.symbols)
			}
			fmt.Fprintf(&b, "        %s\n", label)
			continue
		}

		label := g.name
		if g.symbols > 0 {
			label += fmt.Sprintf(" (%d sym)", g.symbols)
		}
		fmt.Fprintf(&b, "        %s\n", label)
		sort.Strings(g.children)
		for _, child := range g.children {
			var symCount int
			for _, svc := range report.Architecture.Services {
				if svc.Name == child {
					symCount = len(svc.Symbols)
					break
				}
			}
			childLabel := child
			if symCount > 0 {
				childLabel += fmt.Sprintf(" (%d sym)", symCount)
			}
			fmt.Fprintf(&b, "            %s\n", childLabel)
		}
	}

	return b.String()
}
