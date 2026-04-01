package impact

import (
	"fmt"
	"os/exec"
	"sort"
	"strings"

	"github.com/dpopsuev/locus/internal/arch"
)

// Risk level constants used across protocol analysis.
const (
	RiskLow      = "low"
	RiskMedium   = "medium"
	RiskHigh     = "high"
	RiskCritical = "critical"
)

// BlastRadiusReport holds the aggregate blast radius for a set of changed files.
type BlastRadiusReport struct {
	ChangedFiles       []string          `json:"changed_files"`
	ChangedComponents  []string          `json:"changed_components"`
	AffectedComponents []string          `json:"affected_components"`
	BlastRadius        int               `json:"blast_radius"`
	RiskLevel          string            `json:"risk_level"`
	PerComponent       []ComponentImpact `json:"per_component"`
	Summary            string            `json:"summary"`
}

// ComponentImpact holds the direct and transitive dependent counts for one component.
type ComponentImpact struct {
	Component      string `json:"component"`
	DirectDeps     int    `json:"direct_dependents"`
	TransitiveDeps int    `json:"transitive_dependents"`
}

// ComputeBlastRadius calculates the aggregate blast radius from changed files or a git ref.
// If since is provided, it runs git diff --name-only to discover changed files.
// Otherwise it uses the provided files list directly.
func ComputeBlastRadius(
	edges []arch.ArchEdge,
	services []arch.ArchService,
	modulePath string,
	repoPath string,
	files []string,
	since string,
) (*BlastRadiusReport, error) {
	if since != "" {
		discovered, err := changedFilesSince(repoPath, since)
		if err != nil {
			return nil, fmt.Errorf("git diff: %w", err)
		}
		files = discovered
	}
	if len(files) == 0 {
		return &BlastRadiusReport{
			ChangedFiles: files,
			RiskLevel:    RiskLow,
			Summary:      "no changed files",
		}, nil
	}

	// Map files to component names by taking the directory path.
	compSet := make(map[string]bool)
	for _, f := range files {
		dir := strings.TrimPrefix(f, modulePath+"/")
		// Take the directory portion.
		idx := strings.LastIndex(dir, "/")
		if idx >= 0 {
			dir = dir[:idx]
		} else {
			dir = "."
		}
		compSet[dir] = true
	}

	// Build set of known component names for validation.
	knownComps := make(map[string]bool, len(services))
	for i := range services {
		knownComps[services[i].Name] = true
	}

	// Only keep components that exist in the architecture.
	var changedComponents []string
	for c := range compSet {
		if knownComps[c] {
			changedComponents = append(changedComponents, c)
		}
	}
	sort.Strings(changedComponents)

	// For each changed component, compute impact and union affected sets.
	allAffected := make(map[string]bool)
	perComponent := make([]ComponentImpact, 0, len(changedComponents))
	for _, comp := range changedComponents {
		impact, err := ComputeImpact(edges, services, comp)
		if err != nil {
			continue
		}
		perComponent = append(perComponent, ComponentImpact{
			Component:      comp,
			DirectDeps:     len(impact.DirectDeps),
			TransitiveDeps: len(impact.TransDeps),
		})
		for _, d := range impact.TransDeps {
			allAffected[d] = true
		}
	}

	// Add the changed components themselves to the affected set.
	for _, c := range changedComponents {
		allAffected[c] = true
	}

	affected := make([]string, 0, len(allAffected))
	for a := range allAffected {
		affected = append(affected, a)
	}
	sort.Strings(affected)

	total := len(knownComps)
	blastRadius := 0
	if total > 0 {
		blastRadius = len(allAffected) * 100 / total
	}

	riskLevel := RiskLow
	switch {
	case blastRadius >= 50:
		riskLevel = RiskCritical
	case blastRadius >= 25:
		riskLevel = RiskHigh
	case blastRadius >= 10:
		riskLevel = RiskMedium
	}

	summary := fmt.Sprintf("%d changed file(s) in %d component(s), %d/%d affected (%d%%), risk=%s",
		len(files), len(changedComponents), len(allAffected), total, blastRadius, riskLevel)

	return &BlastRadiusReport{
		ChangedFiles:       files,
		ChangedComponents:  changedComponents,
		AffectedComponents: affected,
		BlastRadius:        blastRadius,
		RiskLevel:          riskLevel,
		PerComponent:       perComponent,
		Summary:            summary,
	}, nil
}

// changedFilesSince returns the list of changed files since a git ref.
func changedFilesSince(repoPath, since string) ([]string, error) {
	cmd := exec.Command("git", "diff", "--name-only", since)
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var files []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			files = append(files, line)
		}
	}
	return files, nil
}
