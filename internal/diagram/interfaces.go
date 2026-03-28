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
		return "", ErrTypeAnalyzerRequired
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
	implEdges := filterImplEdges(impls)

	// Collect interfaces and their implementors.
	interfaces, classByName := collectInterfaces(classes)
	implementors := collectImplementors(implEdges, interfaces, classByName)

	// Nothing to render if no interfaces found.
	if len(interfaces) == 0 {
		return "", ErrNoInterfacesFound
	}

	var b strings.Builder
	if in.ResolvedTheme != nil {
		b.WriteString(in.ResolvedTheme.InitDirective() + "\n")
	}
	b.WriteString("classDiagram\n")

	renderInterfaceClasses(&b, classes, interfaces)
	renderImplementorClasses(&b, classes, implementors)
	renderImplEdges(&b, implEdges, interfaces, implementors, opts.Scope)

	return b.String(), nil
}

func filterImplEdges(impls []analysis.ImplEdge) []analysis.ImplEdge {
	var implEdges []analysis.ImplEdge
	for _, e := range impls {
		if e.Kind == "implements" {
			implEdges = append(implEdges, e)
		}
	}
	return implEdges
}

func collectInterfaces(classes []analysis.ClassInfo) (interfaces, classByName map[string]analysis.ClassInfo) {
	interfaces = make(map[string]analysis.ClassInfo)
	classByName = make(map[string]analysis.ClassInfo)

	for _, c := range classes {
		classByName[c.Name] = c
		if c.Kind == kindInterface {
			interfaces[c.Name] = c
		}
	}
	return interfaces, classByName
}

func collectImplementors(implEdges []analysis.ImplEdge, interfaces, classByName map[string]analysis.ClassInfo) map[string]analysis.ClassInfo {
	implementors := make(map[string]analysis.ClassInfo)
	for _, e := range implEdges {
		if _, isIface := interfaces[e.To]; !isIface {
			continue
		}
		if c, ok := classByName[e.From]; ok {
			implementors[c.Name] = c
		}
	}
	return implementors
}

func renderInterfaceClasses(b *strings.Builder, classes []analysis.ClassInfo, interfaces map[string]analysis.ClassInfo) {
	for _, c := range classes {
		if _, ok := interfaces[c.Name]; !ok {
			continue
		}
		renderClassBlock(b, c)
	}
}

func renderImplementorClasses(b *strings.Builder, classes []analysis.ClassInfo, implementors map[string]analysis.ClassInfo) {
	for _, c := range classes {
		if _, ok := implementors[c.Name]; !ok {
			continue
		}
		renderClassBlock(b, c)
	}
}

func renderImplEdges(b *strings.Builder, implEdges []analysis.ImplEdge, interfaces, implementors map[string]analysis.ClassInfo, scope string) {
	declared := make(map[string]bool, len(interfaces)+len(implementors))
	for name := range interfaces {
		declared[name] = true
	}
	for name := range implementors {
		declared[name] = true
	}

	for _, e := range implEdges {
		if scope != "" && !declared[e.From] && !declared[e.To] {
			continue
		}
		if !declared[e.From] || !declared[e.To] {
			continue
		}
		fmt.Fprintf(b, "    %s ..|> %s\n", mermaidID(e.From), mermaidID(e.To))
	}
}

// renderClassBlock writes a single class/interface block to the builder.
func renderClassBlock(b *strings.Builder, c analysis.ClassInfo) {
	id := mermaidID(c.Name)
	fmt.Fprintf(b, "    class %s {\n", id)
	if c.Kind == kindInterface {
		b.WriteString("        <<interface>>\n")
	}
	for _, f := range c.Fields {
		vis := "-"
		if f.Exported {
			vis = "+"
		}
		fmt.Fprintf(b, "        %s%s %s\n", vis, f.Type, f.Name)
	}
	for _, m := range c.Methods {
		vis := "-"
		if m.Exported {
			vis = "+"
		}
		fmt.Fprintf(b, "        %s%s\n", vis, m.Signature)
	}
	b.WriteString("    }\n")
}
