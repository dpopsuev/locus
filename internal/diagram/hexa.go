package diagram

import (
	"fmt"
	"sort"
	"strings"
)

// hexaRoleMeta holds rendering metadata for a hexagonal architecture role.
type hexaRoleMeta struct {
	label string
	emoji string
}

var hexaRoles = map[string]hexaRoleMeta{
	"domain":     {label: "Domain Core", emoji: "\U0001f3db"},    // 🏛
	"port":       {label: "Ports", emoji: "\U0001f50c"},          // 🔌
	"adapter":    {label: "Adapters", emoji: "\u26a1"},           // ⚡
	"infra":      {label: "Infrastructure", emoji: "\U0001f527"}, // 🔧
	"app":        {label: "Application", emoji: "\U0001f4e6"},    // 📦
	"entrypoint": {label: "Entry Points", emoji: "\U0001f680"},   // 🚀
}

// hexaRoleOrder defines the rendering order for subgraphs (inside-out).
var hexaRoleOrder = []string{"domain", "port", "app", "adapter", "infra", "entrypoint"}

func renderHexa(in Input, opts Options) (string, error) {
	if len(in.HexaRoles) == 0 {
		return "", ErrHexaRolesRequired
	}

	rt := in.ResolvedTheme
	m := in.Report.Architecture

	// Build set of in-scope components for scope filtering.
	inScope := make(map[string]bool, len(in.HexaRoles))
	for name := range in.HexaRoles {
		if opts.Scope == "" || matchScope(name, opts.Scope) {
			inScope[name] = true
		}
	}

	// Group components by role.
	groups := make(map[string][]string)
	for name, role := range in.HexaRoles {
		if !inScope[name] {
			continue
		}
		groups[role] = append(groups[role], name)
	}

	// Sort component lists within each group for determinism.
	for role := range groups {
		sort.Strings(groups[role])
	}

	var b strings.Builder
	b.WriteString(rt.InitDirective() + "\n")
	b.WriteString("graph TD\n")
	b.WriteString(rt.ClassDefs() + "\n")

	// Render subgraphs in defined order.
	for _, role := range hexaRoleOrder {
		components := groups[role]
		if len(components) == 0 {
			continue
		}
		meta := hexaRoles[role]
		groupID := mermaidID(role + "_group")
		fmt.Fprintf(&b, "    subgraph %s[\"%s %s\"]\n", groupID, meta.emoji, meta.label)
		for _, name := range components {
			id := mermaidID(name)
			fmt.Fprintf(&b, "        %s[\"%s\"]\n", id, name)
		}
		b.WriteString("    end\n")
	}

	// Render edges.
	for _, e := range m.Edges {
		if !inScope[e.From] || !inScope[e.To] {
			continue
		}
		fromID := mermaidID(e.From)
		toID := mermaidID(e.To)
		fromRole := in.HexaRoles[e.From]
		toRole := in.HexaRoles[e.To]

		if isHexaViolation(fromRole, toRole) {
			fmt.Fprintf(&b, "    %s -.->|\"violation\"| %s\n", fromID, toID)
		} else {
			fmt.Fprintf(&b, "    %s --> %s\n", fromID, toID)
		}
	}

	// Add violation classDef for styling.
	b.WriteString("    classDef violation stroke:#ff0000,stroke-width:2px,stroke-dasharray: 5 5\n")

	return b.String(), nil
}

// isHexaViolation returns true when the dependency direction violates
// hexagonal architecture rules (inner layers must not depend on outer layers).
func isHexaViolation(fromRole, toRole string) bool {
	switch fromRole {
	case "domain":
		return toRole == "adapter" || toRole == "infra" || toRole == "app"
	case "port":
		return toRole == "adapter" || toRole == "infra"
	default:
		return false
	}
}

// matchScope checks whether a component name matches the scope filter.
// A component matches if it equals the scope or starts with scope + "/".
func matchScope(name, scope string) bool {
	return name == scope || strings.HasPrefix(name, scope+"/")
}
