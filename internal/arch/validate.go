package arch

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var (
	errUnsupportedFormat = errors.New("unsupported format")
	errNoMermaidContent  = errors.New("no components or edges found in mermaid input")
)

// ArchDrift reports the delta between a desired and actual architecture.
type ArchDrift struct {
	MissingComponents []string   `json:"missing_components,omitempty"`
	ExtraComponents   []string   `json:"extra_components,omitempty"`
	MissingEdges      []ArchEdge `json:"missing_edges,omitempty"`
	ExtraEdges        []ArchEdge `json:"extra_edges,omitempty"`
	Summary           string     `json:"summary"`
}

// ValidateArchitecture computes the drift between a desired and actual ArchModel.
func ValidateArchitecture(desired, actual ArchModel) *ArchDrift {
	desiredComps := make(map[string]bool, len(desired.Services))
	for i := range desired.Services {
		desiredComps[desired.Services[i].Name] = true
	}
	actualComps := make(map[string]bool, len(actual.Services))
	for i := range actual.Services {
		actualComps[actual.Services[i].Name] = true
	}

	var missing, extra []string
	for c := range desiredComps {
		if !actualComps[c] {
			missing = append(missing, c)
		}
	}
	for c := range actualComps {
		if !desiredComps[c] {
			extra = append(extra, c)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)

	type edgeKey struct{ from, to string }
	desiredEdges := make(map[edgeKey]ArchEdge, len(desired.Edges))
	for _, e := range desired.Edges {
		desiredEdges[edgeKey{e.From, e.To}] = e
	}
	actualEdges := make(map[edgeKey]ArchEdge, len(actual.Edges))
	for _, e := range actual.Edges {
		actualEdges[edgeKey{e.From, e.To}] = e
	}

	var missingEdges, extraEdges []ArchEdge
	for k, e := range desiredEdges {
		if _, ok := actualEdges[k]; !ok {
			missingEdges = append(missingEdges, e)
		}
	}
	for k, e := range actualEdges {
		if _, ok := desiredEdges[k]; !ok {
			extraEdges = append(extraEdges, e)
		}
	}
	sortEdges(missingEdges)
	sortEdges(extraEdges)

	summary := fmt.Sprintf("components: %d missing, %d extra; edges: %d missing, %d extra",
		len(missing), len(extra), len(missingEdges), len(extraEdges))

	return &ArchDrift{
		MissingComponents: missing,
		ExtraComponents:   extra,
		MissingEdges:      missingEdges,
		ExtraEdges:        extraEdges,
		Summary:           summary,
	}
}

// ParseDesiredState parses a desired architecture from mermaid or JSON input.
func ParseDesiredState(input, format string) (*ArchModel, error) {
	switch strings.ToLower(format) {
	case "json":
		return parseDesiredJSON(input)
	case "mermaid", "":
		return parseDesiredMermaid(input)
	default:
		return nil, fmt.Errorf("%w: %s (use json or mermaid)", errUnsupportedFormat, format)
	}
}

func parseDesiredJSON(input string) (*ArchModel, error) {
	var m ArchModel
	if err := json.Unmarshal([]byte(input), &m); err != nil {
		return nil, fmt.Errorf("parse JSON architecture: %w", err)
	}
	return &m, nil
}

var (
	mermaidNodeRe = regexp.MustCompile(`^\s+(\w+)\[["']?([^"'\]]+)["']?\]`)
	mermaidEdgeRe = regexp.MustCompile(`^\s+(\w+)\s+--[->]+(?:\|[^|]*\|)?\s*(\w+)`)
)

func parseDesiredMermaid(input string) (*ArchModel, error) {
	m := &ArchModel{}
	nodeLabels := make(map[string]string)

	for _, line := range strings.Split(input, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "graph") || strings.HasPrefix(trimmed, "%%") {
			continue
		}

		if matches := mermaidNodeRe.FindStringSubmatch(line); matches != nil {
			id, label := matches[1], matches[2]
			nodeLabels[id] = label
			continue
		}

		if matches := mermaidEdgeRe.FindStringSubmatch(line); matches != nil {
			from, to := matches[1], matches[2]
			fromName := nodeLabels[from]
			if fromName == "" {
				fromName = from
			}
			toName := nodeLabels[to]
			if toName == "" {
				toName = to
			}
			m.Edges = append(m.Edges, ArchEdge{From: fromName, To: toName})

			if !hasService(m.Services, fromName) {
				m.Services = append(m.Services, ArchService{Name: fromName})
			}
			if !hasService(m.Services, toName) {
				m.Services = append(m.Services, ArchService{Name: toName})
			}
			continue
		}

		// Bare node reference (just an ID on a line, possibly as part of an edge)
	}

	if len(m.Services) == 0 && len(m.Edges) == 0 {
		return nil, errNoMermaidContent
	}
	return m, nil
}

func hasService(services []ArchService, name string) bool {
	for i := range services {
		if services[i].Name == name {
			return true
		}
	}
	return false
}

func sortEdges(edges []ArchEdge) {
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From != edges[j].From {
			return edges[i].From < edges[j].From
		}
		return edges[i].To < edges[j].To
	})
}
