package behavioral

import (
	"fmt"
	"strings"

	"github.com/dpopsuev/locus/internal/diagram/core"
	"github.com/dpopsuev/oculus"
)

// State generates a Mermaid stateDiagram-v2 from detected state
// machine patterns (const/iota groups + switch transitions).
func State(in core.Input, opts core.Options) (string, error) {
	if in.DeepAnalyzer == nil {
		return "", core.ErrDeepAnalyzerRequired
	}

	machines, err := in.DeepAnalyzer.DetectStateMachines(in.Root)
	if err != nil {
		return "", fmt.Errorf("detect state machines: %w", err)
	}

	if len(machines) == 0 {
		return "stateDiagram-v2\n    note right of [*]: No state machines detected\n", nil
	}

	// If scope is set, filter to matching machines
	if opts.Scope != "" {
		var filtered []oculus.StateMachine
		for _, m := range machines {
			if strings.Contains(m.Name, opts.Scope) || strings.Contains(m.Package, opts.Scope) {
				filtered = append(filtered, m)
			}
		}
		if len(filtered) > 0 {
			machines = filtered
		}
	}

	// Limit if TopN is set
	if opts.TopN > 0 && len(machines) > opts.TopN {
		machines = machines[:opts.TopN]
	}

	var b strings.Builder
	if in.ResolvedTheme != nil {
		b.WriteString(in.ResolvedTheme.InitDirective() + "\n")
	}
	b.WriteString("stateDiagram-v2\n")

	for idx, m := range machines {
		if len(machines) > 1 {
			stateID := strings.ReplaceAll(m.Name, " ", "_")
			b.WriteString(fmt.Sprintf("    state \"%s [%s]\" as %s_%d {\n", sanitizeMermaid(m.Name), sanitizeMermaid(m.Package), stateID, idx))
		}

		indent := "    "
		if len(machines) > 1 {
			indent = "        "
		}

		// Initial state transition
		if m.Initial != "" {
			b.WriteString(fmt.Sprintf("%s[*] --> %s\n", indent, m.Initial))
		}

		// State declarations
		for _, s := range m.States {
			b.WriteString(fmt.Sprintf("%s%s\n", indent, s))
		}

		// Transitions
		for _, t := range m.Transitions {
			if t.Trigger != "" {
				b.WriteString(fmt.Sprintf("%s%s --> %s : %s\n", indent, t.From, t.To, sanitizeMermaid(t.Trigger)))
			} else {
				b.WriteString(fmt.Sprintf("%s%s --> %s\n", indent, t.From, t.To))
			}
		}

		if len(machines) > 1 {
			b.WriteString("    }\n")
		}
	}

	return b.String(), nil
}
