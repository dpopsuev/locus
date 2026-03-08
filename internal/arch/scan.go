package arch

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/mod/modfile"

	"github.com/dpopsuev/locus/internal/model"
	"github.com/dpopsuev/locus/internal/survey"
)

// ScanOpts controls the behavior of ScanAndBuild.
type ScanOpts struct {
	ScannerOverride string
	ExcludeTests    bool
	IncludeExternal bool
	IncludeCoverage bool
	Grouped         bool
	Depth           int
	ChurnDays       int
	GitDays         int
	Authors         bool
	Budget          int
	Format          string // "json", "md", "mermaid"
}

// HotSpot identifies a component with high fan-in and high churn.
type HotSpot struct {
	Component string `json:"component"`
	FanIn     int    `json:"fan_in"`
	Churn     int    `json:"churn"`
}

// ContextReport is the full output of a ScanAndBuild invocation.
type ContextReport struct {
	Project        *model.Project        `json:"project"`
	Architecture   ArchModel             `json:"architecture"`
	ModulePath     string                `json:"module_path"`
	Scanner        string                `json:"scanner"`
	SuggestedDepth int                   `json:"suggested_depth,omitempty"`
	HotSpots          []HotSpot             `json:"hot_spots,omitempty"`
	Cycles            []Cycle               `json:"cycles,omitempty"`
	ImportDepth       DepthMap              `json:"import_depth,omitempty"`
	LayerViolations   []LayerViolation      `json:"layer_violations,omitempty"`
	Coverage          []CoverageResult      `json:"coverage,omitempty"`
	APISurfaces       []APISurface          `json:"api_surfaces,omitempty"`
	BoundaryCrossings []BoundaryCrossing    `json:"boundary_crossings,omitempty"`
	RecentCommits     []PackageCommit       `json:"recent_commits,omitempty"`
	Authors           map[string][]Author   `json:"authors,omitempty"`
	FileHotSpots      []HotFile             `json:"file_hot_spots,omitempty"`
	Anchors           []SemanticAnchor      `json:"anchors,omitempty"`
}

// ScanAndBuild scans any repository and produces a ContextReport.
// It requires no .mos directory -- all inputs come from the source tree and git.
func ScanAndBuild(root string, opts ScanOpts) (*ContextReport, error) {
	sc := &survey.AutoScanner{Override: opts.ScannerOverride}
	proj, err := sc.Scan(root)
	if err != nil {
		return nil, fmt.Errorf("survey scan: %w", err)
	}

	modPath := DetectProjectPath(root)
	if modPath == "" {
		modPath = proj.Path
	}

	syncOpts := SyncOptions{
		ModulePath:      modPath,
		ExcludeTests:    opts.ExcludeTests,
		IncludeExternal: opts.IncludeExternal,
	}

	grouped := opts.Grouped
	depth := opts.Depth
	if depth > 0 {
		grouped = true
	}

	if grouped {
		groups, _ := LoadComponentGroups(root)
		if len(groups) == 0 {
			d := depth
			if d == 0 {
				d = 2
			}
			groups = InferDefaultGroups(proj, modPath, d)
		}
		syncOpts.Groups = groups
	}

	if opts.ChurnDays > 0 {
		syncOpts.ChurnData = ComputeChurn(root, opts.ChurnDays, modPath)
	}

	archModel := ProjectToArchModel(proj, syncOpts)

	report := &ContextReport{
		Project:      proj,
		Architecture: archModel,
		ModulePath:   modPath,
		Scanner:      resolvedScannerName(opts.ScannerOverride, root),
	}

	report.SuggestedDepth = computeSuggestedDepth(proj, modPath, len(archModel.Services))
	report.HotSpots = computeHotSpots(archModel)
	report.Cycles = DetectCycles(archModel.Edges)
	report.ImportDepth = ComputeImportDepth(archModel.Edges)
	report.APISurfaces = ComputeAPISurface(archModel)
	report.BoundaryCrossings = DetectBoundaryCrossings(archModel, nil)

	if opts.IncludeCoverage {
		report.Coverage, _ = RunGoCoverage(root, modPath)
	}

	gitDays := opts.GitDays
	if gitDays <= 0 {
		gitDays = opts.ChurnDays
	}
	if gitDays > 0 {
		report.RecentCommits = RecentCommits(root, gitDays, modPath)
		report.FileHotSpots = FileHotSpots(root, gitDays)
	}
	if opts.Authors {
		report.Authors = AuthorOwnership(root, modPath)
	}

	if proj.Language == model.LangGo {
		report.Anchors = extractProjectAnchors(root, proj, modPath)
	}

	return report, nil
}

func extractProjectAnchors(root string, proj *model.Project, modPath string) []SemanticAnchor {
	absRoot, _ := filepath.Abs(root)
	var all []SemanticAnchor
	for _, ns := range proj.Namespaces {
		rel := shortImportPath(modPath, ns.ImportPath)
		pkgDir := filepath.Join(absRoot, rel)
		if rel == "." {
			pkgDir = absRoot
		}
		anchors := ExtractAnchors(pkgDir, rel)
		all = append(all, anchors...)
	}
	return all
}

func resolvedScannerName(override, root string) string {
	if override != "" && override != "auto" {
		return override
	}
	lang := survey.DetectLanguage(root)
	switch lang {
	case model.LangGo:
		return "packages"
	case model.LangRust:
		return "rust"
	case model.LangTypeScript:
		return "typescript"
	case model.LangPython:
		return "python"
	default:
		return "auto"
	}
}

func computeHotSpots(m ArchModel) []HotSpot {
	fanIn := make(map[string]int)
	for _, e := range m.Edges {
		fanIn[e.To]++
	}
	var spots []HotSpot
	for _, s := range m.Services {
		fi := fanIn[s.Name]
		if fi >= 3 && s.Churn >= 5 {
			spots = append(spots, HotSpot{Component: s.Name, FanIn: fi, Churn: s.Churn})
		}
	}
	return spots
}

func computeSuggestedDepth(proj *model.Project, modPath string, flatCount int) int {
	if flatCount <= 3 {
		return 0
	}
	bestDepth := 0
	bestCount := flatCount
	for d := 1; d <= 5; d++ {
		groups := InferDefaultGroups(proj, modPath, d)
		grouped := make(map[string]bool)
		ungrouped := 0
		for _, g := range groups {
			grouped[g.Name] = true
		}
		for _, ns := range proj.Namespaces {
			rel := ns.ImportPath
			if strings.HasPrefix(rel, modPath+"/") {
				rel = strings.TrimPrefix(rel, modPath+"/")
			}
			parts := strings.SplitN(rel, "/", d+1)
			var prefix string
			if len(parts) > d {
				prefix = strings.Join(parts[:d], "/")
			} else {
				prefix = strings.Join(parts, "/")
			}
			if !grouped[prefix] {
				ungrouped++
			}
		}
		count := len(groups) + ungrouped
		if count >= flatCount {
			break
		}
		if count < bestCount {
			bestCount = count
			bestDepth = d
		}
	}
	if bestDepth > 0 && bestCount < flatCount {
		return bestDepth
	}
	return 0
}

// DetectProjectPath reads project metadata files to determine the module/project path.
func DetectProjectPath(root string) string {
	absRoot, _ := filepath.Abs(root)
	fallback := filepath.Base(absRoot)

	if data, err := os.ReadFile(filepath.Join(root, "go.mod")); err == nil {
		if f, err := modfile.Parse("go.mod", data, nil); err == nil {
			return f.Module.Mod.Path
		}
	}

	if data, err := os.ReadFile(filepath.Join(root, "Cargo.toml")); err == nil {
		if name := parseCargoProjectName(data); name != "" {
			return name
		}
	}

	if data, err := os.ReadFile(filepath.Join(root, "package.json")); err == nil {
		if name := parsePackageJSONName(data); name != "" {
			return name
		}
	}

	return fallback
}

func parseCargoProjectName(data []byte) string {
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "name") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				return strings.Trim(strings.TrimSpace(parts[1]), `"'`)
			}
		}
	}
	return ""
}

func parsePackageJSONName(data []byte) string {
	var pkg struct {
		Name string `json:"name"`
	}
	if json.Unmarshal(data, &pkg) == nil {
		return pkg.Name
	}
	return ""
}

// InferDefaultGroups builds component groups from namespace prefix patterns.
func InferDefaultGroups(proj *model.Project, modPath string, depth int) []ComponentGroup {
	prefixMap := make(map[string][]string)
	for _, ns := range proj.Namespaces {
		rel := ns.ImportPath
		if strings.HasPrefix(rel, modPath+"/") {
			rel = strings.TrimPrefix(rel, modPath+"/")
		}
		parts := strings.SplitN(rel, "/", depth+1)
		var prefix string
		if len(parts) > depth {
			prefix = strings.Join(parts[:depth], "/")
		} else {
			prefix = strings.Join(parts, "/")
		}
		prefixMap[prefix] = append(prefixMap[prefix], rel)
	}

	var groups []ComponentGroup
	for prefix, pkgs := range prefixMap {
		if len(pkgs) > 1 {
			groups = append(groups, ComponentGroup{Name: prefix, Packages: pkgs})
		}
	}
	return groups
}
