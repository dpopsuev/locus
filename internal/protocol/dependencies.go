package protocol

import (
	"fmt"
	"os"

	"golang.org/x/mod/modfile"
)

// ModuleDep represents a single Go module dependency.
type ModuleDep struct {
	Path     string `json:"path"`
	Version  string `json:"version"`
	Indirect bool   `json:"indirect"`
	Replace  string `json:"replace,omitempty"`
}

// DependencyReport holds parsed go.mod dependency information.
type DependencyReport struct {
	Module    string      `json:"module"`
	GoVersion string      `json:"go_version"`
	Direct    []ModuleDep `json:"direct"`
	Indirect  []ModuleDep `json:"indirect"`
	Replaces  []ModuleDep `json:"replaces"`
	Summary   string      `json:"summary"`
}

// ComputeDependencies parses a go.mod file and returns a structured dependency report.
func ComputeDependencies(goModPath string) (*DependencyReport, error) {
	data, err := os.ReadFile(goModPath)
	if err != nil {
		return nil, fmt.Errorf("read go.mod: %w", err)
	}

	f, err := modfile.Parse(goModPath, data, nil)
	if err != nil {
		return nil, fmt.Errorf("parse go.mod: %w", err)
	}

	report := &DependencyReport{
		Module: f.Module.Mod.Path,
	}
	if f.Go != nil {
		report.GoVersion = f.Go.Version
	}

	for _, req := range f.Require {
		dep := ModuleDep{
			Path:     req.Mod.Path,
			Version:  req.Mod.Version,
			Indirect: req.Indirect,
		}
		if req.Indirect {
			report.Indirect = append(report.Indirect, dep)
		} else {
			report.Direct = append(report.Direct, dep)
		}
	}

	for _, rep := range f.Replace {
		dep := ModuleDep{
			Path:    rep.Old.Path,
			Version: rep.Old.Version,
		}
		if rep.New.Version != "" {
			dep.Replace = rep.New.Path + "@" + rep.New.Version
		} else {
			dep.Replace = rep.New.Path
		}
		report.Replaces = append(report.Replaces, dep)
	}

	report.Summary = fmt.Sprintf("%d direct, %d indirect, %d replaces",
		len(report.Direct), len(report.Indirect), len(report.Replaces))

	return report, nil
}
