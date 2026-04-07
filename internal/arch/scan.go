package arch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/mod/modfile"
	"golang.org/x/sync/errgroup"

	archanchors "github.com/dpopsuev/locus/internal/arch/anchors"
	archgit "github.com/dpopsuev/locus/internal/arch/git"
	"github.com/dpopsuev/locus/internal/graph"
	"github.com/dpopsuev/locus/internal/model"
	"github.com/dpopsuev/locus/internal/survey"
	"github.com/dpopsuev/oculus"
	olang "github.com/dpopsuev/oculus/lang"
)

// ScanIntent controls what level of analysis to perform.
type ScanIntent string

const (
	// IntentArchitecture performs structure-only analysis (L0): survey, arch model, LOC.
	IntentArchitecture ScanIntent = "architecture"
	// IntentCoupling adds coupling analysis (L1): cycles, import depth, hot spots, API surfaces.
	IntentCoupling ScanIntent = "coupling"
	// IntentHealth adds churn and nesting (L2): git history, tree-sitter depth, file hotspots.
	IntentHealth ScanIntent = "health"
	// IntentFull adds coverage, authors, and anchors (L3).
	IntentFull ScanIntent = "full"
)

// ScanLevel returns the numeric level for an intent (0-3).
func (i ScanIntent) ScanLevel() int {
	switch i {
	case IntentArchitecture:
		return 0
	case IntentCoupling:
		return 1
	case IntentHealth:
		return 2
	case IntentFull:
		return 3
	default:
		return 2 // default to health for backward compat
	}
}

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
	Intent          ScanIntent
	Since           string // git ref to diff against (e.g. HEAD~1) for incremental scan
}

// NewScanOpts returns ScanOpts with sensible defaults.
func NewScanOpts() ScanOpts {
	return ScanOpts{
		Depth:     DefaultGroupingDepth,
		ChurnDays: 90,
		Intent:    IntentHealth,
	}
}

const (
	// MinFanInHotSpot is the minimum fan-in count for a hot spot.
	MinFanInHotSpot = 3
	// MinChurnHotSpot is the minimum churn count for a hot spot.
	MinChurnHotSpot = 5
	// MinNestingHotSpot is the minimum nesting depth for a hot spot.
	MinNestingHotSpot = 4
	// DefaultGroupingDepth is the default component grouping depth.
	DefaultGroupingDepth = 2
	// MaxDepthSearch is the max depth evaluated for suggested depth.
	MaxDepthSearch = 5
	// MaxHotSpotsMarkdown is the max hotspots displayed in markdown output.
	MaxHotSpotsMarkdown = 10
)

// Slog attribute keys.
const (
	logKeyPath       = "path"
	logKeyLanguage   = "language"
	logKeyNamespaces = "namespaces"
)

// HotSpot identifies a component with high fan-in, high churn, and/or deep nesting.
type HotSpot struct {
	Component string `json:"component"`
	FanIn     int    `json:"fan_in"`
	Churn     int    `json:"churn"`
	Nesting   int    `json:"nesting,omitempty"`
}

// ScanCore holds the primary scan output — project model, architecture, and metadata.
type ScanCore struct {
	Project        *model.Project `json:"project"`
	Architecture   ArchModel      `json:"architecture"`
	ModulePath     string         `json:"module_path"`
	Scanner        string         `json:"scanner"`
	SuggestedDepth int            `json:"suggested_depth,omitempty"`
}

// GraphMetrics holds graph-derived analysis: hot spots, cycles, depths, and violations.
type GraphMetrics struct {
	HotSpots          []HotSpot              `json:"hot_spots,omitempty"`
	Cycles            []graph.Cycle          `json:"cycles,omitempty"`
	ImportDepth       graph.DepthMap         `json:"import_depth,omitempty"`
	LayerViolations   []graph.LayerViolation `json:"layer_violations,omitempty"`
	APISurfaces       []APISurface           `json:"api_surfaces,omitempty"`
	BoundaryCrossings []BoundaryCrossing     `json:"boundary_crossings,omitempty"`
	FanIn             graph.CountMap         `json:"fan_in,omitempty"`
	FanOut            graph.CountMap         `json:"fan_out,omitempty"`
}

// GitContext holds git-derived data: coverage, commits, authors, file hotspots.
type GitContext struct {
	Coverage      []archgit.CoverageResult    `json:"coverage,omitempty"`
	RecentCommits []archgit.PackageCommit     `json:"recent_commits,omitempty"`
	Authors       map[string][]archgit.Author `json:"authors,omitempty"`
	FileHotSpots  []archgit.HotFile           `json:"file_hot_spots,omitempty"`
}

// DeepContext holds deep analysis data requiring AST/LSP inspection.
type DeepContext struct {
	Anchors []archanchors.SemanticAnchor `json:"anchors,omitempty"`
}

// ContextReport is the full output of a ScanAndBuild invocation.
// Embeds sub-structs for SRP; field access is unchanged via Go promotion.
type ContextReport struct {
	ScanCore
	GraphMetrics
	GitContext
	DeepContext
}

// ScanAndBuild scans any repository and produces a ContextReport.
// It requires no config directory -- all inputs come from the source tree and git.
// The opts.Intent field controls how deep the analysis goes:
//
//	L0 (architecture): structure + LOC
//	L1 (coupling):     L0 + cycles, import depth, hot spots, API surfaces
//	L2 (health):       L1 + churn, nesting, git history (default)
//	L3 (full):         L2 + coverage, authors, anchors
func ScanAndBuild(root string, opts ScanOpts) (*ContextReport, error) {
	level := opts.Intent.ScanLevel()

	// --- L0: structure ---
	sc := &survey.AutoScanner{Override: opts.ScannerOverride}

	// Incremental scan: if Since is set, identify changed packages and merge.
	if opts.Since != "" {
		return incrementalScan(root, opts, sc)
	}

	proj, err := sc.Scan(root)
	if err != nil {
		return nil, fmt.Errorf("survey scan: %w", err)
	}

	slog.LogAttrs(context.Background(), slog.LevelDebug, "scan: project scanned",
		slog.String(logKeyPath, proj.Path),
		slog.Any(logKeyLanguage, proj.Language),
		slog.Int(logKeyNamespaces, len(proj.Namespaces)),
	)

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
				d = DefaultGroupingDepth
			}
			groups = InferDefaultGroups(proj, modPath, d)
		}
		syncOpts.Groups = groups
	}

	// Churn is only computed at L2+.
	if level >= 2 && opts.ChurnDays > 0 {
		syncOpts.ChurnData = archgit.ComputeChurn(root, opts.ChurnDays, modPath)
	}

	archModel := ProjectToArchModel(proj, syncOpts)
	populateLOC(root, proj, modPath, &archModel)

	report := &ContextReport{
		ScanCore: ScanCore{
			Project:      proj,
			Architecture: archModel,
			ModulePath:   modPath,
			Scanner:      resolvedScannerName(opts.ScannerOverride, root),
		},
	}

	report.SuggestedDepth = computeSuggestedDepth(proj, modPath, len(archModel.Services))

	if level < 1 {
		return report, nil
	}

	// --- L1: coupling ---
	spots := computeHotSpots(archModel)
	if spots == nil {
		spots = []HotSpot{}
	}
	report.HotSpots = spots

	cycles := graph.DetectCycles(archModel.Edges)
	if cycles == nil {
		cycles = []graph.Cycle{}
	}
	report.Cycles = cycles
	report.ImportDepth = graph.ImportDepth(archModel.Edges)
	report.APISurfaces = ComputeAPISurface(archModel)
	report.BoundaryCrossings = DetectBoundaryCrossings(archModel, nil)
	report.FanIn = graph.FanIn(archModel.Edges)
	report.FanOut = graph.FanOut(archModel.Edges)

	if level < 2 {
		return report, nil
	}

	// --- L2: health (churn, nesting, git history) ---
	runL2Health(root, modPath, opts, &archModel, report)

	if level < 3 {
		return report, nil
	}

	// --- L3: full (coverage, authors, anchors) ---
	runL3Full(root, modPath, opts, proj, report)

	return report, nil
}

// runL2Health runs nesting depth, recent commits, and file hotspots in parallel.
func runL2Health(root, modPath string, opts ScanOpts, archModel *ArchModel, report *ContextReport) {
	var (
		commits  []archgit.PackageCommit
		hotFiles []archgit.HotFile
	)
	g, _ := errgroup.WithContext(context.Background())
	g.Go(func() error {
		applyNestingDepth(root, modPath, archModel)
		return nil
	})
	gitDays := opts.GitDays
	if gitDays <= 0 {
		gitDays = opts.ChurnDays
	}
	if gitDays > 0 {
		g.Go(func() error { commits = archgit.RecentCommits(root, gitDays, modPath); return nil })
		g.Go(func() error { hotFiles = archgit.FileHotSpots(root, gitDays); return nil })
	}
	_ = g.Wait()
	report.Architecture = *archModel
	report.RecentCommits = commits
	report.FileHotSpots = hotFiles
}

// runL3Full runs coverage, author ownership, and anchor extraction in parallel.
func runL3Full(root, modPath string, opts ScanOpts, proj *model.Project, report *ContextReport) {
	var (
		coverage []archgit.CoverageResult
		authors  map[string][]archgit.Author
		anchors  []archanchors.SemanticAnchor
	)
	g, _ := errgroup.WithContext(context.Background())
	if opts.IncludeCoverage && proj.Language == model.LangGo {
		g.Go(func() error { coverage, _ = archgit.RunGoCoverage(root, modPath); return nil })
	}
	if opts.Authors {
		g.Go(func() error { authors = archgit.AuthorOwnership(root, modPath); return nil })
	}
	if proj.Language == model.LangGo {
		g.Go(func() error { anchors = extractProjectAnchors(root, proj, modPath); return nil })
	}
	_ = g.Wait()
	report.Coverage = coverage
	report.Authors = authors
	report.Anchors = anchors
}

// incrementalScan performs a full scan but is aware of what changed since a ref.
// Currently does a full scan but marks the report with the since ref for downstream use.
// Future: partial package re-scan + merge with cached baseline.
func incrementalScan(root string, opts ScanOpts, _ *survey.AutoScanner) (*ContextReport, error) {
	changedPkgs := changedPackages(root, opts.Since)

	// Full scan for now — incremental merge requires cached baseline.
	opts.Since = "" // prevent recursion
	report, err := ScanAndBuild(root, opts)
	if err != nil {
		return nil, err
	}

	// Mark changed packages in the report for downstream consumers.
	changedSet := make(map[string]bool, len(changedPkgs))
	for _, p := range changedPkgs {
		changedSet[p] = true
	}
	for i := range report.Architecture.Services {
		if changedSet[report.Architecture.Services[i].Name] {
			report.Architecture.Services[i].Changed = true
		}
	}

	return report, nil
}

// changedPackages returns package directories with changes since the given git ref.
func changedPackages(root, since string) []string {
	cmd := exec.Command("git", "diff", "--name-only", since+"..HEAD") //nolint:gosec // since is a git ref from CLI input
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	pkgSet := make(map[string]bool)
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		dir := filepath.Dir(line)
		if dir == "." {
			dir = "(root)"
		}
		pkgSet[filepath.ToSlash(dir)] = true
	}

	pkgs := make([]string, 0, len(pkgSet))
	for p := range pkgSet {
		pkgs = append(pkgs, p)
	}
	sort.Strings(pkgs)
	return pkgs
}

func extractProjectAnchors(root string, proj *model.Project, modPath string) []archanchors.SemanticAnchor {
	absRoot, _ := filepath.Abs(root)
	var all []archanchors.SemanticAnchor
	for _, ns := range proj.Namespaces {
		rel := shortImportPath(modPath, ns.ImportPath)
		pkgDir := filepath.Join(absRoot, rel)
		if rel == "." {
			pkgDir = absRoot
		}
		anchors := archanchors.ExtractAnchors(pkgDir, rel)
		all = append(all, anchors...)
	}
	return all
}

func resolvedScannerName(override, root string) string {
	if override != "" && override != "auto" {
		return override
	}
	detected := olang.DetectLanguage(root)
	switch detected {
	case olang.Go:
		return "packages"
	case olang.Rust:
		return "rust"
	case olang.TypeScript:
		return "typescript"
	case olang.Python:
		return "python"
	default:
		return "auto"
	}
}

func computeHotSpots(m ArchModel) []HotSpot {
	fanIn := graph.FanIn(m.Edges)
	var spots []HotSpot
	for i := range m.Services {
		s := &m.Services[i]
		fi := fanIn[s.Name]
		if fi >= MinFanInHotSpot && (s.Churn >= MinChurnHotSpot || s.MaxNesting >= MinNestingHotSpot) {
			spots = append(spots, HotSpot{
				Component: s.Name,
				FanIn:     fi,
				Churn:     s.Churn,
				Nesting:   s.MaxNesting,
			})
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
	for d := 1; d <= MaxDepthSearch; d++ {
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

	if data, err := os.ReadFile(filepath.Join(root, "pyproject.toml")); err == nil {
		if name := parsePyprojectName(data); name != "" {
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

func parsePyprojectName(data []byte) string {
	inProject := false
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "[project]" {
			inProject = true
			continue
		}
		if strings.HasPrefix(trimmed, "[") {
			inProject = false
			continue
		}
		if inProject && strings.HasPrefix(trimmed, "name") {
			parts := strings.SplitN(trimmed, "=", 2)
			if len(parts) == 2 {
				return strings.Trim(strings.TrimSpace(parts[1]), `"'`)
			}
		}
	}
	return ""
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

// populateLOC sets LOC on each ArchService. If the scanner already counted lines
// during its file walk, it aggregates from the project model (zero I/O). Otherwise
// it falls back to reading all source files from disk.
func populateLOC(root string, proj *model.Project, modPath string, m *ArchModel) {
	if linesPopulated(proj) {
		applyLOCFromProject(proj, modPath, m)
	} else {
		applyLOC(root, proj, modPath, m)
	}
}

// linesPopulated returns true if the scanner already counted lines during its file walk.
func linesPopulated(proj *model.Project) bool {
	if len(proj.Namespaces) == 0 {
		return false
	}
	ns := proj.Namespaces[0]
	for _, f := range ns.Files {
		if f.Lines > 0 {
			return true
		}
	}
	return false
}

// applyLOCFromProject aggregates pre-computed file lines into ArchService.LOC
// without re-reading files from disk.
func applyLOCFromProject(proj *model.Project, modPath string, m *ArchModel) {
	locByComponent := make(map[string]int)
	for _, ns := range proj.Namespaces {
		component := shortImportPath(modPath, ns.ImportPath)
		for _, f := range ns.Files {
			locByComponent[component] += f.Lines
		}
	}
	for i := range m.Services {
		if loc, ok := locByComponent[m.Services[i].Name]; ok {
			m.Services[i].LOC = loc
		}
	}
}

// applyLOC reads source files referenced by the project model and
// populates LOC (line count) on each ArchService.
func applyLOC(root string, proj *model.Project, modPath string, m *ArchModel) {
	absRoot, _ := filepath.Abs(root)
	locByComponent := make(map[string]int)
	for _, ns := range proj.Namespaces {
		component := shortImportPath(modPath, ns.ImportPath)
		for _, f := range ns.Files {
			path := filepath.Join(absRoot, f.Path)
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			lines := bytes.Count(data, []byte{'\n'})
			if len(data) > 0 && data[len(data)-1] != '\n' {
				lines++
			}
			f.Lines = lines
			locByComponent[component] += lines
		}
	}
	for i := range m.Services {
		if loc, ok := locByComponent[m.Services[i].Name]; ok {
			m.Services[i].LOC = loc
		}
	}
}

// applyNestingDepth runs tree-sitter nesting analysis and populates
// MaxNesting and AvgNesting on each ArchService.
func applyNestingDepth(root, _ string, m *ArchModel) {
	ts := &oculus.TreeSitterAnalyzer{}
	results, err := ts.NestingDepth(root)
	if err != nil || len(results) == 0 {
		return
	}

	type nestAgg struct {
		max   int
		sum   int
		count int
	}
	byComponent := make(map[string]*nestAgg)
	for _, r := range results {
		pkg := r.Package
		if pkg == "(root)" {
			pkg = "."
		}
		agg := byComponent[pkg]
		if agg == nil {
			agg = &nestAgg{}
			byComponent[pkg] = agg
		}
		if r.MaxDepth > agg.max {
			agg.max = r.MaxDepth
		}
		agg.sum += r.MaxDepth
		agg.count++
	}

	for i := range m.Services {
		name := m.Services[i].Name
		if agg, ok := byComponent[name]; ok {
			m.Services[i].MaxNesting = agg.max
			if agg.count > 0 {
				m.Services[i].AvgNesting = float64(agg.sum) / float64(agg.count)
			}
		}
	}
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
