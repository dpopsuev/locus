package diagram

import (
	"fmt"
	"strings"

	"github.com/dpopsuev/locus/internal/analysis"
)

// renderClasses produces a Mermaid classDiagram from TypeAnalyzer data.
func renderClasses(in Input, opts Options) (string, error) {
	if in.Analyzer == nil {
		return "", fmt.Errorf("classes diagram requires a TypeAnalyzer")
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
		return "", fmt.Errorf("no types found for classes diagram")
	}

	var b strings.Builder
	b.WriteString("classDiagram\n")

	declared := make(map[string]bool)
	for _, c := range classes {
		id := classID(c.Name)
		declared[c.Name] = true

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

	for _, edge := range impls {
		if opts.Scope != "" {
			if !declared[edge.From] && !declared[edge.To] {
				continue
			}
		}
		fromID := classID(edge.From)
		toID := classID(edge.To)
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

func filterClassesByPkg(classes []analysis.ClassInfo, scope string) []analysis.ClassInfo {
	var filtered []analysis.ClassInfo
	for _, c := range classes {
		if c.Package == scope || strings.HasSuffix(c.Package, "/"+scope) {
			filtered = append(filtered, c)
		}
	}
	return filtered
}

func classID(name string) string {
	return mermaidID(name)
}
