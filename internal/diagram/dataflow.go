package diagram

import (
	"fmt"
	"strings"
)

// renderDataflow generates a Mermaid flowchart LR with DFD conventions:
//   - Stadium shapes for external entities
//   - Rectangles for processes
//   - Cylinders for data stores
//   - Subgraph trust boundaries
func renderDataflow(in Input, opts Options) (string, error) {
	if in.DeepAnalyzer == nil {
		return "", fmt.Errorf("dataflow diagram requires a DeepAnalyzer")
	}

	entry := opts.Entry
	if entry == "" {
		entry = "main"
	}
	depth := opts.Depth
	if depth <= 0 {
		depth = 8
	}

	flow, err := in.DeepAnalyzer.DataFlowTrace(in.Root, entry, depth)
	if err != nil {
		return "", fmt.Errorf("dataflow trace from %q: %w", entry, err)
	}

	var b strings.Builder
	b.WriteString("flowchart LR\n")

	nodeIDs := make(map[string]string)
	nextID := 0
	getID := func(name string) string {
		if id, ok := nodeIDs[name]; ok {
			return id
		}
		nextID++
		id := fmt.Sprintf("n%d", nextID)
		nodeIDs[name] = id
		return id
	}

	// Render nodes with shape conventions
	for _, n := range flow.Nodes {
		id := getID(n.Name)
		safe := sanitizeMermaid(n.Name)
		switch n.Kind {
		case "external":
			b.WriteString(fmt.Sprintf("    %s([%s])\n", id, safe))
		case "data_store":
			b.WriteString(fmt.Sprintf("    %s[(%s)]\n", id, safe))
		case "entry":
			b.WriteString(fmt.Sprintf("    %s[[\"%s\"]]\n", id, safe))
		default:
			b.WriteString(fmt.Sprintf("    %s[\"%s\"]\n", id, safe))
		}
	}

	// Render trust boundaries as subgraphs
	for _, boundary := range flow.Boundaries {
		safeName := sanitizeMermaid(boundary.Name)
		subID := strings.ReplaceAll(strings.ToLower(safeName), " ", "_")
		b.WriteString(fmt.Sprintf("    subgraph %s [\"%s\"]\n", subID, safeName))
		for _, nodeName := range boundary.Nodes {
			if id, ok := nodeIDs[nodeName]; ok {
				b.WriteString(fmt.Sprintf("        %s\n", id))
			}
		}
		b.WriteString("    end\n")
	}

	// Render edges
	for _, e := range flow.Edges {
		fromID := getID(e.From)
		toID := getID(e.To)
		if e.Label != "" {
			b.WriteString(fmt.Sprintf("    %s -->|\"%s\"| %s\n", fromID, sanitizeMermaid(e.Label), toID))
		} else {
			b.WriteString(fmt.Sprintf("    %s --> %s\n", fromID, toID))
		}
	}

	if flow.Layer != "" {
		b.WriteString(fmt.Sprintf("    %%%% layer: %s\n", flow.Layer))
	}

	return b.String(), nil
}

func sanitizeMermaid(s string) string {
	r := strings.NewReplacer(
		`"`, "'",
		`(`, "[",
		`)`, "]",
		`{`, "[",
		`}`, "]",
	)
	return r.Replace(s)
}
