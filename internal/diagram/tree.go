package diagram

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dpopsuev/locus/internal/arch"
)

// DefaultTopSymbols is the number of exported symbols shown per component in tree diagrams.
const DefaultTopSymbols = 5

// treeCtx holds pre-computed lookup tables used during tree rendering.
type treeCtx struct {
	fi         map[string]int
	churnMap   map[string]int
	svcSymbols map[string][]string
	symCount   map[string]int
}

func newTreeCtx(report *arch.ContextReport, topN int) *treeCtx {
	tc := &treeCtx{
		fi:         fanIn(report.Architecture.Edges),
		churnMap:   make(map[string]int),
		svcSymbols: make(map[string][]string),
		symCount:   make(map[string]int),
	}
	for i := range report.Architecture.Services {
		svc := &report.Architecture.Services[i]
		tc.churnMap[svc.Name] = svc.Churn
		tc.svcSymbols[svc.Name] = topSorted(svc.Symbols, topN)
		tc.symCount[svc.Name] = len(svc.Symbols)
	}
	return tc
}

func (tc *treeCtx) healthIcon(name string) string {
	h := ClassifyHealth(tc.fi[name], tc.churnMap[name])
	switch h {
	case Fatal:
		return "\u2718 "
	case Sick:
		return "\u26A0 "
	default:
		return ""
	}
}

func renderTree(in Input, opts Options) string {
	report := in.Report
	rt := in.ResolvedTheme

	root := filepath.Base(report.ModulePath)
	if root == "" || root == "." {
		root = "project"
	}

	depth := opts.Depth
	if depth <= 0 {
		depth = 1
	}
	topN := opts.TopN
	if topN <= 0 {
		topN = DefaultTopSymbols
	}

	tc := newTreeCtx(report, topN)

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

	var b strings.Builder
	if rt != nil {
		b.WriteString(rt.InitDirective() + "\n")
	}
	b.WriteString("mindmap\n")
	fmt.Fprintf(&b, "    root((\"%s\"))\n", root)

	for _, gName := range order {
		g := groups[gName]
		if len(g.children) == 0 {
			writeTreeNode(&b, tc, g.name, g.symbols, "        ")
			writeSymbols(&b, tc.svcSymbols[g.name], "            ")
			continue
		}

		writeTreeNode(&b, tc, g.name, g.symbols, "        ")
		sort.Strings(g.children)
		for _, child := range g.children {
			writeTreeNode(&b, tc, child, tc.symCount[child], "            ")
			writeSymbols(&b, tc.svcSymbols[child], "                ")
		}
	}

	return b.String()
}

func writeTreeNode(b *strings.Builder, tc *treeCtx, name string, symbols int, indent string) {
	label := tc.healthIcon(name) + name
	if symbols > 0 {
		label += fmt.Sprintf(" (%d sym)", symbols)
	}
	fmt.Fprintf(b, "%s%s\n", indent, label)
}

func writeSymbols(b *strings.Builder, syms []string, indent string) {
	for _, sym := range syms {
		fmt.Fprintf(b, "%s%s\n", indent, sym)
	}
}

// topSorted returns the first N symbols sorted alphabetically.
func topSorted(symbols []string, n int) []string {
	if len(symbols) == 0 {
		return nil
	}
	sorted := make([]string, len(symbols))
	copy(sorted, symbols)
	sort.Strings(sorted)
	if len(sorted) > n {
		sorted = sorted[:n]
	}
	return sorted
}
