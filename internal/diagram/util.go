package diagram

import "strings"

// mermaidID converts a component name into a valid Mermaid node identifier.
func mermaidID(name string) string {
	r := strings.NewReplacer(" ", "_", "-", "_", ".", "_", "/", "_")
	return r.Replace(name)
}
