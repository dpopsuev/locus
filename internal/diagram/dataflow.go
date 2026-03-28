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
		return "", ErrDeepAnalyzerRequired
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
	if in.ResolvedTheme != nil {
		b.WriteString(in.ResolvedTheme.InitDirective() + "\n")
	}
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
			fmt.Fprintf(&b, "    %s([%s])\n", id, safe)
		case "data_store":
			fmt.Fprintf(&b, "    %s[(%s)]\n", id, safe)
		case "entry":
			fmt.Fprintf(&b, "    %s[[%q]]\n", id, safe)
		default:
			fmt.Fprintf(&b, "    %s[%q]\n", id, safe)
		}
	}

	// Render trust boundaries as subgraphs
	for _, boundary := range flow.Boundaries {
		safeName := sanitizeMermaid(boundary.Name)
		subID := strings.ReplaceAll(strings.ToLower(safeName), " ", "_")
		fmt.Fprintf(&b, "    subgraph %s [%q]\n", subID, safeName)
		for _, nodeName := range boundary.Nodes {
			if id, ok := nodeIDs[nodeName]; ok {
				fmt.Fprintf(&b, "        %s\n", id)
			}
		}
		b.WriteString("    end\n")
	}

	// Render edges
	for _, e := range flow.Edges {
		fromID := getID(e.From)
		toID := getID(e.To)
		if e.Label != "" {
			fmt.Fprintf(&b, "    %s -->|%q| %s\n", fromID, sanitizeMermaid(e.Label), toID)
		} else {
			fmt.Fprintf(&b, "    %s --> %s\n", fromID, toID)
		}
	}

	if flow.Layer != "" {
		fmt.Fprintf(&b, "    %%%% layer: %s\n", flow.Layer)
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
