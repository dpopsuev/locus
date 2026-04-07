package typediag

import (
	"fmt"
	"strings"

	"github.com/dpopsuev/locus/internal/diagram/core"
	"github.com/dpopsuev/oculus"
)

const kindInterface = "interface"

// Classes produces a Mermaid classDiagram from TypeAnalyzer data.
func Classes(in core.Input, opts core.Options) (string, error) {
	if in.Analyzer == nil {
		return "", core.ErrTypeAnalyzerRequired
	}
	classes, err := in.Analyzer.Classes(in.Root)
	if err != nil {
		return "", fmt.Errorf("classes: %w", err)
	}
	impls, _ := in.Analyzer.Implements(in.Root)

	if opts.Scope != "" {
		classes = filterClassesByPkg(classes, opts.Scope)
	}

	if len(classes) == 0 {
		return "", core.ErrNoTypesFound
	}

	var b strings.Builder
	if in.ResolvedTheme != nil {
		b.WriteString(in.ResolvedTheme.InitDirective() + "\n")
	}
	b.WriteString("classDiagram\n")

	declared := make(map[string]bool)
	for _, c := range classes {
		id := core.MermaidID(c.Name)
		declared[c.Name] = true

		b.WriteString(fmt.Sprintf("    class %s {\n", id))
		if c.Kind == kindInterface {
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

	for _, edge := range impls {
		if opts.Scope != "" {
			if !declared[edge.From] && !declared[edge.To] {
				continue
			}
		}
		fromID := core.MermaidID(edge.From)
		toID := core.MermaidID(edge.To)
		switch edge.Kind {
		case "implements":
			b.WriteString(fmt.Sprintf("    %s ..|> %s\n", fromID, toID))
		case "extends":
			b.WriteString(fmt.Sprintf("    %s --|> %s\n", fromID, toID))
		case "embeds":
			b.WriteString(fmt.Sprintf("    %s *-- %s\n", fromID, toID))
		}
	}

	return b.String(), nil
}

// FilterClassesByPkg filters classes by package scope. Exported for use by
// other typediag renderers.
func filterClassesByPkg(classes []oculus.ClassInfo, scope string) []oculus.ClassInfo {
	var filtered []oculus.ClassInfo
	for _, c := range classes {
		if c.Package == scope || strings.HasSuffix(c.Package, "/"+scope) {
			filtered = append(filtered, c)
		}
	}
	return filtered
}
