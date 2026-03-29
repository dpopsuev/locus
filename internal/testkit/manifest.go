package testkit

import (
	"encoding/json"
	"fmt"
	"os"
)

// Manifest defines the ground-truth expectations for a testkit fixture.
type Manifest struct {
	Language              string            `json:"language"`
	Marker                string            `json:"marker"`
	ExpectedComponentsMin int               `json:"expected_components_min"`
	ExpectedEdgesMin      int               `json:"expected_edges_min"`
	ScanIntent            string            `json:"scan_intent,omitempty"`
	ExpectedPatterns      []ExpectedPattern `json:"expected_patterns,omitempty"`
	ExpectedSmells        []ExpectedSmell   `json:"expected_smells,omitempty"`
	ExpectedHexa          *ExpectedHexa     `json:"expected_hexa,omitempty"`
	ExpectedSOLID         *ExpectedSOLID    `json:"expected_solid,omitempty"`
	ExpectedSymbols       *ExpectedSymbols  `json:"expected_symbols,omitempty"`
	ExpectedDiagrams      []string          `json:"expected_diagrams,omitempty"`
	ExpectedPresets       []string          `json:"expected_presets,omitempty"`
}

// ExpectedPattern declares a design pattern that should be detected.
type ExpectedPattern struct {
	ID            string  `json:"id"`
	Component     string  `json:"component,omitempty"`
	MinConfidence float64 `json:"min_confidence,omitempty"`
}

// ExpectedSmell declares a code smell that should be detected.
type ExpectedSmell struct {
	ID        string `json:"id"`
	Component string `json:"component,omitempty"`
	Severity  string `json:"severity,omitempty"`
}

// ExpectedHexa declares hexagonal architecture expectations.
type ExpectedHexa struct {
	Domain        []string `json:"domain,omitempty"`
	Adapter       []string `json:"adapter,omitempty"`
	Infra         []string `json:"infra,omitempty"`
	Port          []string `json:"port,omitempty"`
	Entrypoint    []string `json:"entrypoint,omitempty"`
	MaxViolations int      `json:"max_violations"`
}

// ExpectedSOLID declares SOLID principle expectations.
type ExpectedSOLID struct {
	MaxViolations int      `json:"max_violations"`
	Principles    []string `json:"principles,omitempty"`
}

// ExpectedSymbols declares naming quality expectations.
type ExpectedSymbols struct {
	Abbreviations []string `json:"abbreviations,omitempty"`
	GenericNames  []string `json:"generic_names,omitempty"`
}

// LoadManifest reads and parses a manifest.json file.
func LoadManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	return &m, nil
}
