package typediag

import (
	"fmt"
	"strings"

	"github.com/dpopsuev/locus/internal/diagram/core"
	"github.com/dpopsuev/locus/internal/oculus"
)

// Interfaces produces a Mermaid classDiagram showing only interfaces
// and the structs that implement them.
func Interfaces(in core.Input, opts core.Options) (string, error) {
	if in.Analyzer == nil {
		return "", core.ErrTypeAnalyzerRequired
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
		return "", core.ErrNoInterfacesFound
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

func filterImplEdges(impls []oculus.ImplEdge) []oculus.ImplEdge {
	var implEdges []oculus.ImplEdge
	for _, e := range impls {
		if e.Kind == "implements" {
			implEdges = append(implEdges, e)
		}
	}
	return implEdges
}

func collectInterfaces(classes []oculus.ClassInfo) (interfaces, classByName map[string]oculus.ClassInfo) {
	interfaces = make(map[string]oculus.ClassInfo)
	classByName = make(map[string]oculus.ClassInfo)

	for _, c := range classes {
		classByName[c.Name] = c
		if c.Kind == kindInterface {
			interfaces[c.Name] = c
		}
	}
	return interfaces, classByName
}

func collectImplementors(implEdges []oculus.ImplEdge, interfaces, classByName map[string]oculus.ClassInfo) map[string]oculus.ClassInfo {
	implementors := make(map[string]oculus.ClassInfo)
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

func renderInterfaceClasses(b *strings.Builder, classes []oculus.ClassInfo, interfaces map[string]oculus.ClassInfo) {
	for _, c := range classes {
		if _, ok := interfaces[c.Name]; !ok {
			continue
		}
		renderClassBlock(b, c)
	}
}

func renderImplementorClasses(b *strings.Builder, classes []oculus.ClassInfo, implementors map[string]oculus.ClassInfo) {
	for _, c := range classes {
		if _, ok := implementors[c.Name]; !ok {
			continue
		}
		renderClassBlock(b, c)
	}
}

func renderImplEdges(b *strings.Builder, implEdges []oculus.ImplEdge, interfaces, implementors map[string]oculus.ClassInfo, scope string) {
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
		fmt.Fprintf(b, "    %s ..|> %s\n", core.MermaidID(e.From), core.MermaidID(e.To))
	}
}

// renderClassBlock writes a single class/interface block to the builder.
func renderClassBlock(b *strings.Builder, c oculus.ClassInfo) {
	id := core.MermaidID(c.Name)
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
