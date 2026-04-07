package core

import "strings"

// MermaidID converts a component name into a valid Mermaid node identifier.
func MermaidID(name string) string {
	r := strings.NewReplacer(" ", "_", "-", "_", ".", "_", "/", "_")
	return r.Replace(name)
}
