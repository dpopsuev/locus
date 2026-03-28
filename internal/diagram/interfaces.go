package diagram

import (
	"fmt"
	"strings"

	"github.com/dpopsuev/locus/internal/analysis"
)

// renderInterfaces produces a Mermaid classDiagram showing only interfaces
// and the structs that implement them.
func renderInterfaces(in Input, opts Options) (string, error) {
	if in.Analyzer == nil {
		return "", fmt.Errorf("interfaces diagram requires a TypeAnalyzer")
	}
	classes, err := in.Analyzer.Classes(in.Root)
	if err != nil {
		return "", fmt.Errorf("interfaces: %w", err)
	}
	impls, _ := in.Analyzer.Implements(in.Root)

	if opts.Scope != "" {
		classes = filterClassesByPkg(classes, opts.Scope)
	}

	// Filter impl edges to only "implements" kind.
	var implEdges []analysis.ImplEdge
	for _, e := range impls {
		if e.Kind == "implements" {
			implEdges = append(implEdges, e)
		}
	}

	// Collect interfaces and their implementors.
	interfaces := make(map[string]analysis.ClassInfo)
	implementors := make(map[string]analysis.ClassInfo)

	for _, c := range classes {
		if c.Kind == "interface" {
			interfaces[c.Name] = c
		}
	}

	// Build a lookup for all classes by name.
	classByName := make(map[string]analysis.ClassInfo)
	for _, c := range classes {
		classByName[c.Name] = c
	}

	// Find implementor structs referenced by impl edges.
	for _, e := range implEdges {
		if _, isIface := interfaces[e.To]; !isIface {
			continue
		}
		if c, ok := classByName[e.From]; ok {
			implementors[c.Name] = c
		}
	}

	// Nothing to render if no interfaces found.
	if len(interfaces) == 0 {
		return "", fmt.Errorf("no interfaces found for interfaces diagram")
	}

	var b strings.Builder
	if in.ResolvedTheme != nil {
		b.WriteString(in.ResolvedTheme.InitDirective() + "\n")
	}
	b.WriteString("classDiagram\n")

	// Render interfaces.
	for _, c := range classes {
		if _, ok := interfaces[c.Name]; !ok {
			continue
		}
		renderClassBlock(&b, c)
	}

	// Render implementor structs.
	for _, c := range classes {
		if _, ok := implementors[c.Name]; !ok {
			continue
		}
		renderClassBlock(&b, c)
	}

	// Render implements edges.
	declared := make(map[string]bool)
	for name := range interfaces {
		declared[name] = true
	}
	for name := range implementors {
		declared[name] = true
	}

	for _, e := range implEdges {
		if opts.Scope != "" {
			if !declared[e.From] && !declared[e.To] {
				continue
			}
		}
		if !declared[e.From] || !declared[e.To] {
			continue
		}
		b.WriteString(fmt.Sprintf("    %s ..|> %s\n", mermaidID(e.From), mermaidID(e.To)))
	}

	return b.String(), nil
}

// renderClassBlock writes a single class/interface block to the builder.
func renderClassBlock(b *strings.Builder, c analysis.ClassInfo) {
	id := mermaidID(c.Name)
	b.WriteString(fmt.Sprintf("    class %s {\n", id))
	if c.Kind == "interface" {
		b.WriteString("        <<interface>>\n")
	}
	for _, f := range c.Fields {
		vis := "-"
		if f.Exported {
			vis = "+"
		}
		b.WriteString(fmt.Sprintf("        %s%s %s\n", vis, f.Type, f.Name))
	}
	for _, m := range c.Methods {
		vis := "-"
		if m.Exported {
			vis = "+"
		}
		b.WriteString(fmt.Sprintf("        %s%s\n", vis, m.Signature))
	}
	b.WriteString("    }\n")
}
