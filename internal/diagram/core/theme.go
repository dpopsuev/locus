package core

import (
	"fmt"
	"sort"
	"strings"
)

// ThemeNatural is the default theme mode name.
const ThemeNatural = "natural"

// Health classifies a component's risk level.
type Health int

const (
	Healthy Health = iota
	Sick
	Fatal
)

const (
	// FatalFanIn is the minimum fan-in for Fatal health classification.
	FatalFanIn = 5
	// FatalChurn is the minimum churn for Fatal health classification.
	FatalChurn = 15
	// SickFanIn is the minimum fan-in for Sick health classification.
	SickFanIn = 3
	// SickChurn is the minimum churn for Sick health classification.
	SickChurn = 8
)

// ClassifyHealth returns a health level based on fan-in and churn.
func ClassifyHealth(fanIn, churn int) Health {
	if fanIn >= FatalFanIn && churn >= FatalChurn {
		return Fatal
	}
	if fanIn >= SickFanIn && churn >= SickChurn {
		return Sick
	}
	return Healthy
}

// ThemeColor holds per-mode hex values for a single semantic color.
type ThemeColor struct {
	Dark    string `yaml:"dark"`
	Light   string `yaml:"light"`
	Natural string `yaml:"natural"`
}

func (tc ThemeColor) Resolve(mode string) string {
	switch mode {
	case "dark":
		return tc.Dark
	case "light":
		return tc.Light
	case ThemeNatural:
		return tc.Natural
	default:
		return tc.Natural
	}
}

// Shape references semantic color names and a Mermaid shape type.
type Shape struct {
	Mermaid string `yaml:"mermaid,omitempty"`
	Fill    string `yaml:"fill"`
	Stroke  string `yaml:"stroke"`
	Color   string `yaml:"color,omitempty"`
}

// Theme is the three-layer design token registry.
type Theme struct {
	Colors   map[string]ThemeColor        `yaml:"colors"`
	Shapes   map[string]Shape             `yaml:"shapes"`
	Diagrams map[string]map[string]string `yaml:"diagrams"`
}

// ResolvedTheme holds compiled hex values ready for Mermaid output.
type ResolvedTheme struct {
	Mode           string
	ResolvedColors map[string]string
	Shapes         map[string]Shape
	ShapeHex       map[string]ResolvedShape
	Diagrams       map[string]map[string]string
}

// ResolvedShape holds compiled hex values for a single shape.
type ResolvedShape struct {
	Fill   string
	Stroke string
	Color  string
}

// DefaultTheme returns the built-in design token registry.
func DefaultTheme() *Theme {
	return &Theme{
		Colors: map[string]ThemeColor{
			"green":   {Dark: "#68D391", Light: "#276749", Natural: "#38A169"},
			"yellow":  {Dark: "#F6E05E", Light: "#975A16", Natural: "#D69E2E"},
			"red":     {Dark: "#FC8181", Light: "#C53030", Natural: "#E53E3E"},
			"blue":    {Dark: "#63B3ED", Light: "#2B6CB0", Natural: "#4A90D9"},
			"surface": {Dark: "#2D3748", Light: "#EDF2F7", Natural: "#F7FAFC"},
			"text":    {Dark: "#E2E8F0", Light: "#1A202C", Natural: "#2D3748"},
			"muted":   {Dark: "#4A5568", Light: "#A0AEC0", Natural: "#718096"},
			"canvas":  {Dark: "#1A202C", Light: "#FFFFFF", Natural: "#FFFFFF"},
		},
		Shapes: map[string]Shape{
			"healthy":        {Mermaid: "rect", Fill: "green", Stroke: "green", Color: "canvas"},
			"sick":           {Mermaid: "rect", Fill: "yellow", Stroke: "yellow", Color: "canvas"},
			"fatal":          {Mermaid: "rect", Fill: "red", Stroke: "red", Color: "canvas"},
			"component":      {Mermaid: "rect", Fill: "surface", Stroke: "blue", Color: "text"},
			"entry":          {Mermaid: "rounded", Fill: "blue", Stroke: "blue", Color: "canvas"},
			"boundary":       {Fill: "canvas", Stroke: "muted", Color: "text"},
			"edge":           {Stroke: "blue"},
			"violation_edge": {Stroke: "red"},
		},
		Diagrams: map[string]map[string]string{
			"dependency": {
				"default_node": "component",
				"healthy_node": "healthy",
				"sick_node":    "sick",
				"fatal_node":   "fatal",
				"entry_node":   "entry",
				"edge":         "edge",
			},
			"c4": {
				"component":          "component",
				"healthy_component":  "healthy",
				"sick_component":     "sick",
				"fatal_component":    "fatal",
				"container_boundary": "boundary",
			},
			"layers": {
				"block":          "component",
				"violation_edge": "violation_edge",
			},
			"coupling": {
				"flow_color": "blue",
			},
			"churn": {
				"bar_healthy": "green",
				"bar_sick":    "yellow",
				"bar_fatal":   "red",
				"line_color":  "blue",
				"axis_color":  "text",
			},
			"tree": {
				"node_color": "blue",
			},
			"classes": {
				"class_node":     "component",
				"interface_node": "entry",
			},
			"er": {
				"entity": "component",
			},
			"sequence": {
				"actor":       "entry",
				"participant": "component",
			},
		},
	}
}

// Resolve compiles the theme for a specific mode, producing hex-ready values.
func (t *Theme) Resolve(mode string) *ResolvedTheme {
	if mode == "" {
		mode = ThemeNatural
	}

	resolved := make(map[string]string, len(t.Colors))
	for name, tc := range t.Colors {
		resolved[name] = tc.Resolve(mode)
	}

	shapeHex := make(map[string]ResolvedShape, len(t.Shapes))
	for name, s := range t.Shapes {
		rs := ResolvedShape{}
		if s.Fill != "" {
			rs.Fill = resolved[s.Fill]
		}
		if s.Stroke != "" {
			rs.Stroke = resolved[s.Stroke]
		}
		if s.Color != "" {
			rs.Color = resolved[s.Color]
		}
		shapeHex[name] = rs
	}

	return &ResolvedTheme{
		Mode:           mode,
		ResolvedColors: resolved,
		Shapes:         t.Shapes,
		ShapeHex:       shapeHex,
		Diagrams:       t.Diagrams,
	}
}

// ClassDefs emits Mermaid classDef lines for all shapes that have fill/stroke.
func (rt *ResolvedTheme) ClassDefs() string {
	lines := make([]string, 0, len(rt.ShapeHex))
	names := make([]string, 0, len(rt.ShapeHex))
	for n := range rt.ShapeHex {
		names = append(names, n)
	}
	sort.Strings(names)

	for _, name := range names {
		rs := rt.ShapeHex[name]
		if rs.Fill == "" && rs.Stroke == "" {
			continue
		}
		var parts []string
		if rs.Fill != "" {
			parts = append(parts, "fill:"+rs.Fill)
		}
		if rs.Stroke != "" {
			parts = append(parts, "stroke:"+rs.Stroke)
		}
		if rs.Color != "" {
			parts = append(parts, "color:"+rs.Color)
		}
		lines = append(lines, fmt.Sprintf("    classDef %s %s", name, strings.Join(parts, ",")))
	}
	return strings.Join(lines, "\n")
}

// InitDirective emits the Mermaid init directive for base theme overrides.
func (rt *ResolvedTheme) InitDirective() string {
	surface := rt.ResolvedColors["surface"]
	text := rt.ResolvedColors["text"]
	blue := rt.ResolvedColors["blue"]
	canvas := rt.ResolvedColors["canvas"]

	return fmt.Sprintf(
		"%%%%{init: {'theme': 'base', 'themeVariables': {'primaryColor': '%s', 'primaryTextColor': '%s', 'primaryBorderColor': '%s', 'lineColor': '%s', 'background': '%s', 'fontSize': '14px'}}}%%%%",
		surface, text, blue, blue, canvas,
	)
}

// HealthClass returns the classDef name for a health level.
func (rt *ResolvedTheme) HealthClass(h Health) string {
	switch h {
	case Sick:
		return "sick"
	case Fatal:
		return "fatal"
	default:
		return "healthy"
	}
}

// NodeSuffix returns ":::className" for appending to Mermaid node definitions.
func (rt *ResolvedTheme) NodeSuffix(h Health) string {
	return ":::" + rt.HealthClass(h)
}

// ColorHex returns the resolved hex for a semantic color name.
func (rt *ResolvedTheme) ColorHex(name string) string {
	return rt.ResolvedColors[name]
}
