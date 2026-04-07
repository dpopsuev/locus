package behavioral

import (
	"fmt"
	"strings"

	"github.com/dpopsuev/locus/internal/diagram/core"
)

// Sequence produces a Mermaid sequenceDiagram tracing a call chain
// from an entry point through function calls.
func Sequence(in core.Input, opts core.Options) (string, error) {
	if in.Analyzer == nil {
		return "", core.ErrTypeAnalyzerRequired
	}

	entry := opts.Entry
	if entry == "" {
		eps, _ := in.Analyzer.EntryPoints(in.Root)
		if len(eps) == 0 {
			return "", core.ErrNoEntryProvided
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
		return "", fmt.Errorf("%w %q", core.ErrNoCallsFound, entry)
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
		b.WriteString(fmt.Sprintf("    participant %s\n", core.MermaidID(p)))
	}

	for _, c := range calls {
		b.WriteString(fmt.Sprintf("    %s->>%s: %s()\n", core.MermaidID(c.Caller), core.MermaidID(c.Callee), c.Callee))
	}

	return b.String(), nil
}
