package clinic

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dpopsuev/locus/internal/arch"
	"github.com/dpopsuev/locus/internal/port"
)

// Bloater thresholds (language-agnostic).
const (
	LargeFileLOC      = 500
	LargeComponentLOC = 2000
)

// BloaterDetection records a single bloater smell.
type BloaterDetection struct {
	Component string        `json:"component"`
	File      string        `json:"file,omitempty"`
	Kind      string        `json:"kind"` // "large_file", "large_component"
	LOC       int           `json:"loc"`
	Threshold int           `json:"threshold"`
	Severity  port.Severity `json:"severity"`
}

// BloaterReport holds all bloater detections for a codebase.
type BloaterReport struct {
	Detections []BloaterDetection `json:"detections"`
	Summary    string             `json:"summary"`
}

// ComputeBloaterScan detects bloater code smells: large files (>500 LOC)
// and large components (>2000 LOC). Language-agnostic — uses LOC data
// populated by any scanner.
func ComputeBloaterScan(report *arch.ContextReport) *BloaterReport {
	var detections []BloaterDetection

	for i := range report.Architecture.Services {
		svc := &report.Architecture.Services[i]

		// Large component detection.
		if svc.LOC > LargeComponentLOC {
			severity := port.SeverityWarning
			if svc.LOC > LargeComponentLOC*2 {
				severity = port.SeverityError
			}
			detections = append(detections, BloaterDetection{
				Component: svc.Name,
				Kind:      "large_component",
				LOC:       svc.LOC,
				Threshold: LargeComponentLOC,
				Severity:  severity,
			})
		}
	}

	// Large file detection from project model.
	if report.Project != nil {
		for _, ns := range report.Project.Namespaces {
			component := shortComponentName(report.ModulePath, ns.ImportPath)
			for _, f := range ns.Files {
				if f.Lines > LargeFileLOC {
					severity := port.SeverityWarning
					if f.Lines > LargeFileLOC*2 {
						severity = port.SeverityError
					}
					detections = append(detections, BloaterDetection{
						Component: component,
						File:      f.Path,
						Kind:      "large_file",
						LOC:       f.Lines,
						Threshold: LargeFileLOC,
						Severity:  severity,
					})
				}
			}
		}
	}

	// Dead symbol detection.
	detections = append(detections, DetectDeadSymbols(report)...)

	sort.Slice(detections, func(i, j int) bool {
		return detections[i].LOC > detections[j].LOC
	})

	summary := fmt.Sprintf("%d bloater(s) detected", len(detections))
	if len(detections) > 0 {
		summary = fmt.Sprintf("%d bloater(s): %d large file(s), %d large component(s)",
			len(detections), countKind(detections, "large_file"), countKind(detections, "large_component"))
	}

	return &BloaterReport{Detections: detections, Summary: summary}
}

func shortComponentName(modPath, importPath string) string {
	if modPath != "" && len(importPath) > len(modPath)+1 {
		return importPath[len(modPath)+1:]
	}
	return importPath
}

// DetectDeadSymbols finds exported symbols with zero fan-in to their component.
// A symbol in a component with fan-in=0 is potentially dead code (unless it's
// an entrypoint). Language-agnostic.
func DetectDeadSymbols(report *arch.ContextReport) []BloaterDetection {
	if report.FanIn == nil {
		return nil
	}

	var detections []BloaterDetection
	for i := range report.Architecture.Services {
		svc := &report.Architecture.Services[i]
		// Skip entrypoints — they're expected to have 0 fan-in.
		if strings.HasPrefix(svc.Name, "cmd/") || svc.Name == "." {
			continue
		}
		fi := report.FanIn[svc.Name]
		if fi > 0 {
			continue
		}
		// Component has 0 fan-in — all its exported symbols are dead.
		for _, sym := range svc.Symbols {
			detections = append(detections, BloaterDetection{
				Component: svc.Name,
				File:      sym.File,
				Kind:      "dead_symbol",
				LOC:       0,
				Threshold: 0,
				Severity:  port.SeverityWarning,
			})
		}
	}
	return detections
}

func countKind(detections []BloaterDetection, kind string) int {
	n := 0
	for i := range detections {
		if detections[i].Kind == kind {
			n++
		}
	}
	return n
}
