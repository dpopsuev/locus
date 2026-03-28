package diagram

import (
	"fmt"
	"strings"

	"github.com/dpopsuev/locus/internal/analysis"
)

// renderER produces a Mermaid erDiagram showing type relationships
// through field references and composition.
func renderER(in Input, opts Options) (string, error) {
	if in.Analyzer == nil {
		return "", ErrTypeAnalyzerRequired
	}
	classes, err := in.Analyzer.Classes(in.Root)
	if err != nil {
		return "", fmt.Errorf("er: %w", err)
	}
	refs, _ := in.Analyzer.FieldRefs(in.Root)

	if opts.Scope != "" {
		classes = filterClassesByPkg(classes, opts.Scope)
	}

	var entities []analysis.ClassInfo
	entitySet := make(map[string]bool)
	for _, c := range classes {
		if c.Kind == "struct" || c.Kind == "class" {
			entities = append(entities, c)
			entitySet[c.Name] = true
		}
	}

	if len(entities) == 0 {
		return "", ErrNoEntitiesFound
	}

	var b strings.Builder
	if in.ResolvedTheme != nil {
		b.WriteString(in.ResolvedTheme.InitDirective() + "\n")
	}
	b.WriteString("erDiagram\n")

	for _, e := range entities {
		id := erID(e.Name)
		b.WriteString(fmt.Sprintf("    %s {\n", id))
		for _, f := range e.Fields {
			typStr := sanitizeERType(f.Type)
			b.WriteString(fmt.Sprintf("        %s %s\n", typStr, f.Name))
		}
		b.WriteString("    }\n")
	}

	// Relationships from field references
	emitted := make(map[string]bool)
	for _, ref := range refs {
		if !entitySet[ref.Owner] || !entitySet[ref.RefType] {
			continue
		}
		key := ref.Owner + "->" + ref.RefType
		if emitted[key] {
			continue
		}
		emitted[key] = true
		fmt.Fprintf(&b, "    %s ||--o{ %s : %q\n",
			erID(ref.Owner), erID(ref.RefType), ref.Field)
	}

	return b.String(), nil
}

func erID(name string) string {
	return mermaidID(name)
}

func sanitizeERType(t string) string {
	t = strings.ReplaceAll(t, "*", "ptr_")
	t = strings.ReplaceAll(t, "[]", "slice_")
	t = strings.ReplaceAll(t, "[", "_")
	t = strings.ReplaceAll(t, "]", "_")
	t = strings.ReplaceAll(t, " ", "_")
	t = strings.ReplaceAll(t, ".", "_")
	return t
}
