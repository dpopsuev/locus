package diagram

import (
	"fmt"
	"strings"
)

// renderSequence produces a Mermaid sequenceDiagram tracing a call chain
// from an entry point through function calls.
func renderSequence(in Input, opts Options) (string, error) {
	if in.Analyzer == nil {
		return "", fmt.Errorf("sequence diagram requires a TypeAnalyzer")
	}

	entry := opts.Entry
	if entry == "" {
		eps, _ := in.Analyzer.EntryPoints(in.Root)
		if len(eps) == 0 {
			return "", fmt.Errorf("sequence diagram: no --entry provided and no entry points detected")
		}
		entry = eps[0].Name
	}

	depth := opts.Depth
	if depth <= 0 {
		depth = 5
	}

	calls, err := in.Analyzer.CallChain(in.Root, entry, depth)
	if err != nil {
		return "", fmt.Errorf("sequence: %w", err)
	}
	if len(calls) == 0 {
		return "", fmt.Errorf("sequence diagram: no calls found from entry %q", entry)
	}

	var b strings.Builder
	if in.ResolvedTheme != nil {
		b.WriteString(in.ResolvedTheme.InitDirective() + "\n")
	}
	b.WriteString("sequenceDiagram\n")

	// Collect unique participants in order of appearance
	seen := make(map[string]bool)
	var participants []string
	addParticipant := func(name string) {
		if !seen[name] {
			seen[name] = true
			participants = append(participants, name)
		}
	}

	for _, c := range calls {
		addParticipant(c.Caller)
		addParticipant(c.Callee)
	}

	for _, p := range participants {
		b.WriteString(fmt.Sprintf("    participant %s\n", seqID(p)))
	}

	for _, c := range calls {
		b.WriteString(fmt.Sprintf("    %s->>%s: %s()\n", seqID(c.Caller), seqID(c.Callee), c.Callee))
	}

	return b.String(), nil
}

func seqID(name string) string {
	return mermaidID(name)
}
