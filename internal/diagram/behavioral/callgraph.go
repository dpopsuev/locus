package behavioral

import (
	"fmt"
	"strings"

	"github.com/dpopsuev/locus/internal/diagram/core"
	"github.com/dpopsuev/locus/internal/oculus"
)

// CallGraph generates a Mermaid flowchart TB with function nodes
// clustered in package subgraphs. Cross-package calls use dotted edges.
func CallGraph(in core.Input, opts core.Options) (string, error) {
	if in.DeepAnalyzer == nil {
		return "", core.ErrDeepAnalyzerRequired
	}

	cgOpts := oculus.CallGraphOpts{
		Entry:        opts.Entry,
		Depth:        opts.Depth,
		ExportedOnly: opts.ExportedOnly,
		Scope:        opts.Scope,
	}
	if cgOpts.Depth <= 0 {
		cgOpts.Depth = 10
	}

	cg, err := in.DeepAnalyzer.CallGraph(in.Root, cgOpts)
	if err != nil {
		return "", fmt.Errorf("call graph: %w", err)
	}

	var b strings.Builder
	if in.ResolvedTheme != nil {
		b.WriteString(in.ResolvedTheme.InitDirective() + "\n")
	}
	b.WriteString("flowchart TB\n")

	// Group nodes by package
	pkgNodes := make(map[string][]oculus.FuncNode)
	for _, n := range cg.Nodes {
		pkgNodes[n.Package] = append(pkgNodes[n.Package], n)
	}

	nodeIDs := make(map[string]string)
	nextID := 0
	getID := func(pkg, name string) string {
		key := pkg + "." + name
		if id, ok := nodeIDs[key]; ok {
			return id
		}
		nextID++
		id := fmt.Sprintf("f%d", nextID)
		nodeIDs[key] = id
		return id
	}

	// Render package subgraphs with their function nodes
	for pkg, nodes := range pkgNodes {
		safePkg := strings.ReplaceAll(pkg, "/", "_")
		safePkg = strings.ReplaceAll(safePkg, "(", "")
		safePkg = strings.ReplaceAll(safePkg, ")", "")
		fmt.Fprintf(&b, "    subgraph %s [%q]\n", safePkg, sanitizeMermaid(pkg))
		for _, n := range nodes {
			id := getID(n.Package, n.Name)
			fmt.Fprintf(&b, "        %s[%q]\n", id, sanitizeMermaid(n.Name))
		}
		b.WriteString("    end\n")
	}

	// Render edges
	for _, e := range cg.Edges {
		fromID := getID(e.CallerPkg, e.Caller)
		toID := getID(e.CalleePkg, e.Callee)
		if fromID == toID {
			continue
		}
		if e.CrossPkg {
			fmt.Fprintf(&b, "    %s -.-> %s\n", fromID, toID)
		} else {
			fmt.Fprintf(&b, "    %s --> %s\n", fromID, toID)
		}
	}

	if cg.Layer != "" {
		fmt.Fprintf(&b, "    %%%% layer: %s\n", cg.Layer)
	}

	return b.String(), nil
}
