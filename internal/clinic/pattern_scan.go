package clinic

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dpopsuev/locus/internal/analysis"
	"github.com/dpopsuev/locus/internal/arch"
	"github.com/dpopsuev/locus/internal/graph"
	"github.com/dpopsuev/locus/internal/port"
)

// PatternKind distinguishes design patterns from code smells.
type PatternKind string

const (
	PatternKindPattern PatternKind = "pattern"
	PatternKindSmell   PatternKind = "smell"
)

// CodeExample provides before/after code snippets illustrating a refactoring.
type CodeExample struct {
	Before string `json:"before"`
	After  string `json:"after"`
	Label  string `json:"label"`
}

// CatalogEntry describes a known pattern or smell in the catalog.
type CatalogEntry struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Kind        PatternKind   `json:"kind"`
	Category    string        `json:"category"`
	Description string        `json:"description"`
	Indicators  []string      `json:"indicators"`
	Remediation string        `json:"remediation,omitempty"`
	Steps       []string      `json:"steps,omitempty"`
	Examples    []CodeExample `json:"examples,omitempty"`
}

// PatternDetection records a single detected pattern or smell in a component.
type PatternDetection struct {
	PatternID       string           `json:"pattern_id"`
	PatternName     string           `json:"pattern_name"`
	Kind            PatternKind      `json:"kind"`
	Component       string           `json:"component"`
	Confidence      port.Confidence  `json:"confidence"`
	Evidence        []string         `json:"evidence"`
	Severity        port.Severity    `json:"severity"`
	MoveTargets     []MoveTarget     `json:"move_targets,omitempty"`
	SplitSuggestion *SplitSuggestion `json:"split_suggestion,omitempty"`
}

// PatternScanReport is the result of scanning an architecture for patterns and smells.
type PatternScanReport struct {
	Detections    []PatternDetection `json:"detections"`
	PatternsFound int                `json:"patterns_found"`
	SmellsFound   int                `json:"smells_found"`
	Summary       string             `json:"summary"`
}

// PatternCatalogReport lists catalog entries, optionally filtered.
type PatternCatalogReport struct {
	Entries []CatalogEntry `json:"entries"`
	Summary string         `json:"summary"`
}

// Threshold constants for signal functions.
const (
	thresholdGodLOC             = 1000
	thresholdGodSymbols         = 30
	thresholdGodFan             = 5
	thresholdLazyLOC            = 20
	thresholdLazyFanIn          = 1
	thresholdShotgunChurn       = 10
	thresholdShotgunFanIn       = 5
	thresholdFeatureEnvyPct     = 0.5
	thresholdStrategyImpls      = 2
	fingerprintGodThreshold     = 0.5
	fingerprintHighThreshold    = 0.9
	thresholdCoverageGapFanIn   = 3
	thresholdFragileContractFan = 5
)

// patternCatalog is the compile-time catalog of known patterns and smells.
var patternCatalog = []CatalogEntry{
	// ── Patterns ──
	{
		ID: "factory", Name: "Factory", Kind: PatternKindPattern,
		Category: "creational", Description: "Creates objects without specifying exact types",
		Indicators: []string{"New* functions returning interfaces", "constructor dispatching on type"},
	},
	{
		ID: "strategy", Name: "Strategy", Kind: PatternKindPattern,
		Category: "behavioral", Description: "Family of interchangeable algorithms",
		Indicators: []string{"interface with 1-2 methods", "multiple implementations", "field of interface type on struct"},
	},
	{
		ID: "observer", Name: "Observer", Kind: PatternKindPattern,
		Category: "behavioral", Description: "One-to-many dependency notification",
		Indicators: []string{"Register/Subscribe methods", "Notify/Publish methods", "listener slice or channel field"},
	},
	{
		ID: "decorator", Name: "Decorator", Kind: PatternKindPattern,
		Category: "structural", Description: "Wraps objects to add behavior",
		Indicators: []string{"struct embedding interface it implements", "method forwarding to wrapped field"},
	},
	{
		ID: "adapter", Name: "Adapter", Kind: PatternKindPattern,
		Category: "structural", Description: "Converts one interface to another",
		Indicators: []string{"struct wrapping external type", "implements internal interface via delegation"},
	},
	{
		ID: "repository", Name: "Repository", Kind: PatternKindPattern,
		Category: "architectural", Description: "Abstracts data persistence",
		Indicators: []string{"Store/Repository interface", "CRUD method set", "domain types in signatures"},
	},
	{
		ID: "middleware", Name: "Middleware", Kind: PatternKindPattern,
		Category: "structural", Description: "Wraps handler chains",
		Indicators: []string{"func(Handler) Handler signature", "http.Handler wrapping"},
	},
	{
		ID: "builder", Name: "Builder", Kind: PatternKindPattern,
		Category: "creational", Description: "Constructs complex objects step by step",
		Indicators: []string{"With* methods returning same type", "Build() terminal method"},
	},
	{
		ID: "singleton", Name: "Singleton", Kind: PatternKindPattern,
		Category: "creational", Description: "Ensures single instance",
		Indicators: []string{"sync.Once usage", "package-level var with init/Get"},
	},
	{
		ID: "composite", Name: "Composite", Kind: PatternKindPattern,
		Category: "structural", Description: "Tree structure with uniform interface",
		Indicators: []string{"interface field holding slice of same interface", "recursive method calls"},
	},
	{
		ID: "state_machine_candidate", Name: "State Machine Candidate", Kind: PatternKindPattern,
		Category: "behavioral", Description: "Struct with state/status field and methods that switch on it",
		Indicators: []string{"field named state/status/phase/mode", "switch/if-else on state field in methods"},
	},
	// ── Smells ──
	{
		ID: "god_component", Name: "God Component", Kind: PatternKindSmell,
		Category: "smell", Description: "Component doing too much",
		Indicators:  []string{"high fan-in AND fan-out", "LOC > 1000", "symbol count > 30"},
		Remediation: "Extract responsibilities into focused packages",
		Steps: []string{
			"Identify distinct responsibilities by grouping exported symbols that share call targets",
			"For each group, create a new package with a clear domain name",
			"Move types and functions to the new package, preserving exported names",
			"Update import paths in all consumers (use goimports or IDE rename)",
			"If the original package's callers need polymorphism, introduce an interface at the boundary",
			"Verify no circular imports with 'locus analysis action=cycles'",
		},
		Examples: []CodeExample{{
			Label:  "Extract Module",
			Before: "// pkg/engine — 2000 LOC, 40 symbols\nfunc ParseConfig() {}\nfunc RunPipeline() {}\nfunc RenderOutput() {}",
			After:  "// pkg/engine/config — ParseConfig()\n// pkg/engine/pipeline — RunPipeline()\n// pkg/engine/render — RenderOutput()",
		}},
	},
	{
		ID: "feature_envy", Name: "Feature Envy", Kind: PatternKindSmell,
		Category: "smell", Description: "Component uses another's data more than its own",
		Indicators:  []string{"high call_sites to single target", "LOCSurface > own LOC"},
		Remediation: "Move logic to the component whose data it uses",
		Steps: []string{
			"Identify the envious functions — those with >50% of calls targeting another package",
			"Check if the function accesses state from its own package (receiver fields, package vars)",
			"If no local state is used, move the function to the target package",
			"If local state is needed, extract the remote-calling logic into a helper in the target, call it from the original",
			"Update callers to import the new location",
		},
		Examples: []CodeExample{{
			Label:  "Move Method",
			Before: "// pkg/handler\nfunc (h *Handler) Process(r *repo.Record) {\n    repo.Validate(r)\n    repo.Normalize(r)\n    repo.Save(r)\n}",
			After:  "// pkg/repo\nfunc ProcessRecord(r *Record) {\n    Validate(r)\n    Normalize(r)\n    Save(r)\n}",
		}},
	},
	{
		ID: "shotgun_surgery", Name: "Shotgun Surgery", Kind: PatternKindSmell,
		Category: "smell", Description: "Changes require touching many files",
		Indicators:  []string{"high churn AND high fan-in"},
		Remediation: "Consolidate related logic to reduce change blast radius",
		Steps: []string{
			"Identify the repeated change pattern — what feature or concern triggers multi-file edits",
			"Group the scattered logic by responsibility, not by layer",
			"Move related functions into a single package that owns that concern",
			"Introduce a Facade or Service if consumers need a stable entry point",
			"Verify churn drops with 'locus analysis action=coupling view=hot_spots'",
		},
	},
	{
		ID: "inappropriate_intimacy", Name: "Inappropriate Intimacy", Kind: PatternKindSmell,
		Category: "smell", Description: "Bidirectional coupling between components",
		Indicators:  []string{"mutual edges between 2 components", "high weight both ways"},
		Remediation: "Extract shared interface or merge components",
		Steps: []string{
			"Determine if the two components are genuinely separate concerns",
			"If they are one concern: merge into a single package",
			"If separate concerns: extract a shared interface (port) that both depend on",
			"Move the interface to a neutral package (e.g., internal/port/)",
			"Each component depends on the port, not on each other",
		},
		Examples: []CodeExample{{
			Label:  "Dependency Inversion",
			Before: "// A imports B, B imports A (cycle)\nimport \"project/internal/A\"\nimport \"project/internal/B\"",
			After:  "// port/contract.go — shared interface\n// A imports port, B imports port\n// no cycle",
		}},
	},
	{
		ID: "lazy_component", Name: "Lazy Component", Kind: PatternKindSmell,
		Category: "smell", Description: "Too little responsibility",
		Indicators:  []string{"LOC < 20", "0-1 symbols", "0 fan-in"},
		Remediation: "Merge into related component",
		Steps: []string{
			"Identify the nearest related package by import graph (highest edge weight)",
			"Move all symbols into that package",
			"Remove the empty package directory",
			"Update imports in any consumers",
		},
	},
	{
		ID: "data_clump", Name: "Data Clump", Kind: PatternKindSmell,
		Category: "smell", Description: "Groups of data that travel together",
		Indicators:  []string{"multiple structs sharing 3+ field types"},
		Remediation: "Extract common fields into shared struct",
		Steps: []string{
			"List the repeated parameter groups across function signatures",
			"Create a named struct (Parameter Object) with those fields",
			"Replace the individual parameters with the struct in all signatures",
			"Add a constructor (New*) if validation is needed",
		},
		Examples: []CodeExample{{
			Label:  "Extract Parameter Object",
			Before: "func Query(host string, port int, user string, pass string) {}\nfunc Connect(host string, port int, user string, pass string) {}",
			After:  "type ConnConfig struct {\n    Host string\n    Port int\n    User string\n    Pass string\n}\nfunc Query(c ConnConfig) {}\nfunc Connect(c ConnConfig) {}",
		}},
	},
	{
		ID: "long_parameter_list", Name: "Long Parameter List", Kind: PatternKindSmell,
		Category: "smell", Description: "Functions with too many parameters",
		Indicators:  []string{"methods with >5 parameters"},
		Remediation: "Introduce parameter object or options struct",
		Steps: []string{
			"Group parameters by concern — which travel together?",
			"Create an Options struct with sensible defaults",
			"Replace the long parameter list with the struct",
			"Add functional options (With*) if callers typically override only 1-2 fields",
		},
		Examples: []CodeExample{{
			Label:  "Options Pattern",
			Before: "func Scan(path string, depth int, churn int, ext bool, tests bool, cov bool) {}",
			After:  "type ScanOpts struct {\n    Depth int\n    ChurnDays int\n    IncludeExternal bool\n}\nfunc Scan(path string, opts ScanOpts) {}",
		}},
	},
	{
		ID: "dead_code", Name: "Dead Code", Kind: PatternKindSmell,
		Category: "smell", Description: "Exported symbols with zero callers",
		Indicators:  []string{"exported symbol", "0 fan-in", "not in cmd/"},
		Remediation: "Remove or unexport unused symbols",
		Steps: []string{
			"Verify the symbol truly has zero callers with 'locus analysis action=callers symbol=Name'",
			"Check if it is part of an interface contract (may be required but uncalled)",
			"If unused: unexport (lowercase) or delete entirely",
			"If part of a public API: document why it exists or mark as deprecated",
		},
	},
	{
		ID: "unstable_interface", Name: "Unstable Interface", Kind: PatternKindSmell,
		Category: "smell", Description: "Interface that changes frequently",
		Indicators:  []string{"interface package with high churn", "many implementors affected"},
		Remediation: "Freeze interface, version with new type",
		Steps: []string{
			"Freeze the current interface — no new methods",
			"Create a V2 interface with the new methods if evolution is needed",
			"Use embedding to compose V1 into V2 for backward compatibility",
			"Migrate implementors incrementally to V2",
		},
	},
	{
		ID: "circular_dependency", Name: "Circular Dependency", Kind: PatternKindSmell,
		Category: "smell", Description: "Mutual dependency cycle",
		Indicators:  []string{"cycle in dependency graph"},
		Remediation: "Break cycle with dependency inversion",
		Steps: []string{
			"Identify the weakest edge in the cycle (fewest call sites)",
			"Extract an interface for the symbols crossing that edge",
			"Place the interface in the downstream package (consumer defines the port)",
			"The upstream package implements the interface without importing the consumer",
			"Verify with 'locus analysis action=cycles'",
		},
		Examples: []CodeExample{{
			Label:  "Break Cycle via Port",
			Before: "// A imports B, B imports A\npackage A; import \"B\"\npackage B; import \"A\"",
			After:  "// A defines port, B implements it\npackage A; type Store interface { Get() }\npackage B; func (b *B) Get() {} // implements A.Store",
		}},
	},
	{
		ID: "coverage_gap", Name: "Coverage Gap", Kind: PatternKindSmell,
		Category: "testing", Description: "Component has high fan-in but lacks cross-package test coverage",
		Indicators:  []string{"has _test.go files", "no acceptance/ or integration/ test files reference this component"},
		Remediation: "Add integration tests that exercise this component's boundaries with its dependencies",
		Steps: []string{
			"Identify the component's public contract (exported functions + interfaces)",
			"Write integration tests that call through the real dependency chain",
			"Focus on boundary cases: error paths, empty inputs, concurrent access",
			"Use testcontainers or fixtures for external dependencies",
		},
	},
	{
		ID: "fragile_contract", Name: "Fragile Contract", Kind: PatternKindSmell,
		Category: "reliability", Description: "Widely-used component without a constructor — callers must know hidden initialization rules",
		Indicators:  []string{"high fan-in", "no New* constructor among exported symbols"},
		Remediation: "Make preconditions explicit via constructor validation, Option pattern, or Builder",
		Steps: []string{
			"Identify the hidden initialization rules (required fields, order of calls)",
			"Create a New* constructor that validates all preconditions",
			"Return an error if preconditions are not met",
			"Consider functional options (With*) for optional configuration",
			"Make the struct fields unexported so callers must use the constructor",
		},
		Examples: []CodeExample{{
			Label:  "Constructor Validation",
			Before: "type Server struct {\n    Addr string // must not be empty\n    TLS  bool   // must match cert presence\n}\n// callers: s := Server{Addr: \":8080\"}",
			After:  "func NewServer(addr string, opts ...Option) (*Server, error) {\n    if addr == \"\" {\n        return nil, errors.New(\"addr required\")\n    }\n    // ...\n}",
		}},
	},
	{
		ID: "missing_pattern", Name: "Missing Pattern", Kind: PatternKindSmell,
		Category: "smell", Description: "High-churn component with no recognized design pattern — may benefit from Strategy, Observer, or Facade",
		Indicators:  []string{"churn > threshold", "no positive pattern detected"},
		Remediation: "Analyze the change reasons — if multiple axes of change, introduce Strategy. If many dependents react to changes, introduce Observer.",
		Steps: []string{
			"Run 'git log --oneline path/to/package' to identify recurring change themes",
			"If changes are along multiple axes (e.g., format + transport): introduce Strategy",
			"If many consumers react to internal changes: introduce Observer or event bus",
			"If the component is a grab-bag of utilities: split by domain into focused packages",
		},
	},
}

// ── Signal functions ──
// Each returns (detected, confidence, evidence).

func signalHighFanIn(svcName string, edges []arch.ArchEdge, threshold int) (detected bool, confidence float64, evidence string) {
	count := 0
	for _, e := range edges {
		if e.To == svcName {
			count++
		}
	}
	if count >= threshold {
		conf := float64(count)/float64(threshold)*0.5 + 0.5
		if conf > 1.0 {
			conf = 1.0
		}
		return true, conf, fmt.Sprintf("fan-in=%d (threshold %d)", count, threshold)
	}
	return false, 0, ""
}

func signalHighFanOut(svcName string, edges []arch.ArchEdge, threshold int) (detected bool, confidence float64, evidence string) {
	count := 0
	for _, e := range edges {
		if e.From == svcName {
			count++
		}
	}
	if count >= threshold {
		conf := float64(count)/float64(threshold)*0.5 + 0.5
		if conf > 1.0 {
			conf = 1.0
		}
		return true, conf, fmt.Sprintf("fan-out=%d (threshold %d)", count, threshold)
	}
	return false, 0, ""
}

func signalHighLOC(svc arch.ArchService, threshold int) (detected bool, confidence float64, evidence string) {
	if svc.LOC >= threshold {
		conf := float64(svc.LOC)/float64(threshold)*0.5 + 0.5
		if conf > 1.0 {
			conf = 1.0
		}
		return true, conf, fmt.Sprintf("LOC=%d (threshold %d)", svc.LOC, threshold)
	}
	return false, 0, ""
}

func signalLowLOC(svc arch.ArchService, threshold int) (detected bool, confidence float64, evidence string) {
	if svc.LOC > 0 && svc.LOC < threshold {
		// Lower LOC → higher confidence
		conf := 1.0 - float64(svc.LOC)/float64(threshold)
		if conf < 0.5 {
			conf = 0.5
		}
		return true, conf, fmt.Sprintf("LOC=%d (threshold %d)", svc.LOC, threshold)
	}
	return false, 0, ""
}

func signalHighChurn(svc arch.ArchService, threshold int) (detected bool, confidence float64, evidence string) {
	if svc.Churn >= threshold {
		conf := float64(svc.Churn)/float64(threshold)*0.5 + 0.5
		if conf > 1.0 {
			conf = 1.0
		}
		return true, conf, fmt.Sprintf("churn=%d (threshold %d)", svc.Churn, threshold)
	}
	return false, 0, ""
}

func signalHighSymbolCount(svc arch.ArchService, threshold int) (detected bool, confidence float64, evidence string) {
	count := len(svc.Symbols)
	if count >= threshold {
		conf := float64(count)/float64(threshold)*0.5 + 0.5
		if conf > 1.0 {
			conf = 1.0
		}
		return true, conf, fmt.Sprintf("symbols=%d (threshold %d)", count, threshold)
	}
	return false, 0, ""
}

func signalCycleParticipant(svcName string, cycles []graph.Cycle) (detected bool, confidence float64, evidence string) {
	for _, cycle := range cycles {
		for _, node := range cycle {
			if node == svcName {
				return true, 1.0, fmt.Sprintf("participates in cycle: %s", strings.Join(cycle, " → "))
			}
		}
	}
	return false, 0, ""
}

func signalBidirectionalEdge(svcName string, edges []arch.ArchEdge) (detected bool, confidence float64, evidence string) {
	outgoing := make(map[string]bool)
	incoming := make(map[string]bool)
	for _, e := range edges {
		if e.From == svcName {
			outgoing[e.To] = true
		}
		if e.To == svcName {
			incoming[e.From] = true
		}
	}
	for target := range outgoing {
		if incoming[target] {
			return true, 0.9, fmt.Sprintf("bidirectional edge: %s <-> %s", svcName, target)
		}
	}
	return false, 0, ""
}

func signalNewFunctions(svc arch.ArchService) (detected bool, confidence float64, evidence string) {
	count := 0
	for _, sym := range svc.Symbols {
		if strings.HasPrefix(sym, "New") {
			count++
		}
	}
	if count > 0 {
		conf := 0.6
		if count >= 2 {
			conf = 0.8
		}
		return true, conf, fmt.Sprintf("%d New* functions found", count)
	}
	return false, 0, ""
}

func signalSingleMethodInterface(classes []analysis.ClassInfo, pkg string) (detected bool, confidence float64, evidence string) {
	for _, c := range classes {
		if c.Package == pkg && c.Kind == classKindInterface && len(c.Methods) == 1 {
			return true, 0.7, fmt.Sprintf("single-method interface: %s", c.Name)
		}
	}
	return false, 0, ""
}

func signalMultipleImplementors(classes []analysis.ClassInfo, impls []analysis.ImplEdge, pkg string) (detected bool, confidence float64, evidence string) {
	// Find interfaces in this package.
	ifaces := make(map[string]bool)
	for _, c := range classes {
		if c.Package == pkg && c.Kind == classKindInterface {
			ifaces[c.Name] = true
		}
	}
	if len(ifaces) == 0 {
		return false, 0, ""
	}
	// Count implementors per interface.
	implCount := make(map[string]int, len(ifaces))
	for _, edge := range impls {
		if ifaces[edge.To] {
			implCount[edge.To]++
		}
	}
	for iface, count := range implCount {
		if count >= thresholdStrategyImpls {
			return true, 0.8, fmt.Sprintf("interface %s has %d implementors", iface, count)
		}
	}
	return false, 0, ""
}

func signalLowFanIn(svcName string, edges []arch.ArchEdge, threshold int) (detected bool, confidence float64, evidence string) {
	count := 0
	for _, e := range edges {
		if e.To == svcName {
			count++
		}
	}
	if count < threshold {
		return true, 0.7, fmt.Sprintf("fan-in=%d (below threshold %d)", count, threshold)
	}
	return false, 0, ""
}

// signalNoConstructor checks whether a service has no New* constructor among its symbols.
func signalNoConstructor(svc arch.ArchService) (detected bool, confidence float64, evidence string) {
	for _, sym := range svc.Symbols {
		if strings.HasPrefix(sym, "New") {
			return false, 0, ""
		}
	}
	return true, 0.8, "no New* constructor found"
}

// stateFieldKeywords are substrings (case-insensitive) that indicate a state-like field.
var stateFieldKeywords = []string{"state", "status", "phase", "mode"}

// signalStateField checks whether any struct in the package has a field whose name
// contains a state-like keyword (state, status, phase, mode). This is a heuristic
// signal for state machine candidate detection.
func signalStateField(classes []analysis.ClassInfo, pkg string) (detected bool, confidence float64, evidence string) {
	for _, c := range classes {
		if c.Package != pkg || c.Kind != "struct" {
			continue
		}
		for _, f := range c.Fields {
			lower := strings.ToLower(f.Name)
			for _, kw := range stateFieldKeywords {
				if strings.Contains(lower, kw) {
					return true, 0.8, fmt.Sprintf("struct %s has state-like field: %s", c.Name, f.Name)
				}
			}
		}
	}
	return false, 0, ""
}

// signalNoExternalTestImporter checks whether NO edge coming into svcName originates
// from a test-related package (one whose name contains "test" or "acceptance").
// Returns true when the component is only tested by its own co-located _test.go files.
func signalNoExternalTestImporter(svcName string, edges []arch.ArchEdge) (detected bool, confidence float64, evidence string) {
	for _, e := range edges {
		if e.To != svcName {
			continue
		}
		from := strings.ToLower(e.From)
		if strings.HasSuffix(from, "_test") ||
			strings.Contains(from, "test") ||
			strings.Contains(from, "acceptance") ||
			strings.Contains(from, "integration") {
			return false, 0, ""
		}
	}
	return true, 0.7, "no cross-package test imports"
}

// ── Fingerprint engine ──

type fingerprintRule struct {
	signal    string
	weight    float64
	threshold int
}

type patternFingerprint struct {
	patternID string
	rules     []fingerprintRule
	threshold float64
}

var fingerprints = []patternFingerprint{
	// ── Smells ──
	{
		patternID: "god_component",
		rules: []fingerprintRule{
			{signal: "highFanIn", weight: 0.25, threshold: thresholdGodFan},
			{signal: "highFanOut", weight: 0.25, threshold: thresholdGodFan},
			{signal: "highLOC", weight: 0.25, threshold: thresholdGodLOC},
			{signal: "highSymbolCount", weight: 0.25, threshold: thresholdGodSymbols},
		},
		threshold: fingerprintGodThreshold,
	},
	{
		patternID: "circular_dependency",
		rules: []fingerprintRule{
			{signal: "cycleParticipant", weight: 1.0},
		},
		threshold: 0.8,
	},
	{
		patternID: "inappropriate_intimacy",
		rules: []fingerprintRule{
			{signal: "bidirectionalEdge", weight: 1.0},
		},
		threshold: 0.8,
	},
	{
		patternID: "lazy_component",
		rules: []fingerprintRule{
			{signal: "lowLOC", weight: 0.5, threshold: thresholdLazyLOC},
			{signal: "lowFanIn", weight: 0.5, threshold: thresholdLazyFanIn},
		},
		threshold: 0.5,
	},
	{
		patternID: "shotgun_surgery",
		rules: []fingerprintRule{
			{signal: "highChurn", weight: 0.5, threshold: thresholdShotgunChurn},
			{signal: "highFanIn", weight: 0.5, threshold: thresholdShotgunFanIn},
		},
		threshold: 0.6,
	},
	// feature_envy handled as special case in evaluateFeatureEnvy
	{
		patternID: "coverage_gap",
		rules: []fingerprintRule{
			{signal: "highFanIn", weight: 0.5, threshold: thresholdCoverageGapFanIn},
			{signal: "noExternalTestImporter", weight: 0.5},
		},
		threshold: 0.6,
	},
	{
		patternID: "fragile_contract",
		rules: []fingerprintRule{
			{signal: "highFanIn", weight: 0.5, threshold: thresholdFragileContractFan},
			{signal: "noConstructor", weight: 0.5},
		},
		threshold: 0.6,
	},

	// ── Patterns ──
	{
		patternID: "factory",
		rules: []fingerprintRule{
			{signal: "newFunctions", weight: 0.6},
			{signal: "singleMethodInterface", weight: 0.4},
		},
		threshold: 0.5,
	},
	{
		patternID: "strategy",
		rules: []fingerprintRule{
			{signal: "singleMethodInterface", weight: 0.4},
			{signal: "multipleImplementors", weight: 0.6},
		},
		threshold: 0.6,
	},
	// Patterns with high thresholds (rarely trigger without deep analysis)
	{patternID: "observer", rules: []fingerprintRule{{signal: "highFanIn", weight: 1.0, threshold: 8}}, threshold: fingerprintHighThreshold},
	{patternID: "decorator", rules: []fingerprintRule{{signal: "singleMethodInterface", weight: 1.0}}, threshold: fingerprintHighThreshold},
	{patternID: "adapter", rules: []fingerprintRule{{signal: "singleMethodInterface", weight: 1.0}}, threshold: fingerprintHighThreshold},
	{patternID: "repository", rules: []fingerprintRule{{signal: "multipleImplementors", weight: 1.0}}, threshold: fingerprintHighThreshold},
	{patternID: "middleware", rules: []fingerprintRule{{signal: "singleMethodInterface", weight: 1.0}}, threshold: fingerprintHighThreshold},
	{patternID: "builder", rules: []fingerprintRule{{signal: "newFunctions", weight: 1.0}}, threshold: fingerprintHighThreshold},
	{patternID: "singleton", rules: []fingerprintRule{{signal: "highFanIn", weight: 1.0, threshold: 10}}, threshold: fingerprintHighThreshold},
	{patternID: "composite", rules: []fingerprintRule{{signal: "singleMethodInterface", weight: 1.0}}, threshold: fingerprintHighThreshold},
	{
		patternID: "state_machine_candidate",
		rules: []fingerprintRule{
			{signal: "stateField", weight: 0.5},
			{signal: "highSymbolCount", weight: 0.5, threshold: 5},
		},
		threshold: 0.6,
	},
	// Smells disabled until proper AST signals are implemented (BUG-21).
	{patternID: "data_clump", rules: []fingerprintRule{{signal: "highSymbolCount", weight: 1.0, threshold: 20}}, threshold: 1.1},
	{patternID: "long_parameter_list", rules: []fingerprintRule{{signal: "highSymbolCount", weight: 1.0, threshold: 15}}, threshold: 1.1},
	{patternID: "dead_code", rules: []fingerprintRule{{signal: "lowFanIn", weight: 1.0, threshold: 1}}, threshold: fingerprintHighThreshold},
	{patternID: "unstable_interface", rules: []fingerprintRule{{signal: "highChurn", weight: 1.0, threshold: 15}}, threshold: fingerprintHighThreshold},
}

// catalogByID provides O(1) lookup into the catalog.
var catalogByID map[string]*CatalogEntry

func init() {
	catalogByID = make(map[string]*CatalogEntry, len(patternCatalog))
	for i := range patternCatalog {
		catalogByID[patternCatalog[i].ID] = &patternCatalog[i]
	}
}

// evaluateSignal dispatches a rule to the appropriate signal function and returns
// the detection result.
func evaluateSignal(
	rule fingerprintRule,
	svc arch.ArchService,
	edges []arch.ArchEdge,
	cycles []graph.Cycle,
	classes []analysis.ClassInfo,
	impls []analysis.ImplEdge,
) (detected bool, confidence float64, evidence string) {
	switch rule.signal {
	case "highFanIn":
		return signalHighFanIn(svc.Name, edges, rule.threshold)
	case "highFanOut":
		return signalHighFanOut(svc.Name, edges, rule.threshold)
	case "highLOC":
		return signalHighLOC(svc, rule.threshold)
	case "lowLOC":
		return signalLowLOC(svc, rule.threshold)
	case "highChurn":
		return signalHighChurn(svc, rule.threshold)
	case "highSymbolCount":
		return signalHighSymbolCount(svc, rule.threshold)
	case "cycleParticipant":
		return signalCycleParticipant(svc.Name, cycles)
	case "bidirectionalEdge":
		return signalBidirectionalEdge(svc.Name, edges)
	case "newFunctions":
		return signalNewFunctions(svc)
	case "singleMethodInterface":
		return signalSingleMethodInterface(classes, svc.Package)
	case "multipleImplementors":
		return signalMultipleImplementors(classes, impls, svc.Package)
	case "lowFanIn":
		return signalLowFanIn(svc.Name, edges, rule.threshold)
	case "noConstructor":
		return signalNoConstructor(svc)
	case "noExternalTestImporter":
		return signalNoExternalTestImporter(svc.Name, edges)
	case "stateField":
		return signalStateField(classes, svc.Package)
	default:
		return false, 0, ""
	}
}

// evaluateFingerprint checks a single fingerprint against a service and returns
// a detection if the weighted score meets the threshold.
// roleMultiplier scales integer thresholds in rules (>1.0 = more lenient).
func evaluateFingerprint(
	fp patternFingerprint,
	svc arch.ArchService,
	edges []arch.ArchEdge,
	cycles []graph.Cycle,
	classes []analysis.ClassInfo,
	impls []analysis.ImplEdge,
	roleMultiplier float64,
) *PatternDetection {
	entry := catalogByID[fp.patternID]
	if entry == nil {
		return nil
	}

	var weightedSum float64
	evidence := make([]string, 0, len(fp.rules))
	for _, rule := range fp.rules {
		// Scale integer thresholds by role multiplier for smell fingerprints.
		scaledRule := rule
		if roleMultiplier != 1.0 && scaledRule.threshold > 0 && entry.Kind == PatternKindSmell {
			scaledRule.threshold = int(float64(scaledRule.threshold) * roleMultiplier)
		}
		detected, conf, ev := evaluateSignal(scaledRule, svc, edges, cycles, classes, impls)
		if detected {
			weightedSum += rule.weight * conf
			evidence = append(evidence, ev)
		}
	}

	if weightedSum < fp.threshold {
		return nil
	}

	severity := severityForDetection(entry.Kind, weightedSum)

	return &PatternDetection{
		PatternID:   entry.ID,
		PatternName: entry.Name,
		Kind:        entry.Kind,
		Component:   svc.Name,
		Confidence:  port.Confidence(weightedSum),
		Evidence:    evidence,
		Severity:    severity,
	}
}

// evaluateFeatureEnvy checks for the feature envy smell using a special heuristic:
// if >50% of a component's outgoing CallSites go to a single target.
func evaluateFeatureEnvy(svc arch.ArchService, edges []arch.ArchEdge) *PatternDetection {
	entry := catalogByID["feature_envy"]
	if entry == nil {
		return nil
	}

	totalCallSites := 0
	targetCallSites := make(map[string]int)
	for _, e := range edges {
		if e.From == svc.Name && e.CallSites > 0 {
			totalCallSites += e.CallSites
			targetCallSites[e.To] += e.CallSites
		}
	}
	if totalCallSites == 0 {
		return nil
	}

	for target, cs := range targetCallSites {
		ratio := float64(cs) / float64(totalCallSites)
		if ratio > thresholdFeatureEnvyPct {
			return &PatternDetection{
				PatternID:   entry.ID,
				PatternName: entry.Name,
				Kind:        entry.Kind,
				Component:   svc.Name,
				Confidence:  port.Confidence(ratio),
				Evidence:    []string{fmt.Sprintf("%.0f%% of call sites target %s", ratio*100, target)},
				Severity:    severityForDetection(entry.Kind, ratio),
			}
		}
	}
	return nil
}

// severityForDetection maps pattern kind and confidence to a severity level.
func severityForDetection(kind PatternKind, confidence float64) port.Severity {
	if kind == PatternKindPattern {
		return port.SeverityInfo
	}
	// Smells: high confidence → error, otherwise warning.
	if confidence > 0.8 {
		return port.SeverityError
	}
	return port.SeverityWarning
}

// detectMissingPattern is a post-processing step that flags high-churn components
// with no recognized design pattern. If a component has churn > thresholdShotgunChurn
// and no positive pattern (Kind == PatternKindPattern) was detected, it emits a
// missing_pattern smell suggesting the component may benefit from a structural pattern.
func detectMissingPattern(
	services []arch.ArchService,
	detections []PatternDetection,
	accepted []port.AcceptedViolation,
) []PatternDetection {
	entry := catalogByID["missing_pattern"]
	if entry == nil {
		return nil
	}

	// Build a set of components that already have a positive pattern detected.
	hasPattern := make(map[string]bool)
	for i := range detections {
		if detections[i].Kind == PatternKindPattern {
			hasPattern[detections[i].Component] = true
		}
	}

	extra := make([]PatternDetection, 0, len(services))
	for i := range services {
		svc := &services[i]
		if svc.Churn < thresholdShotgunChurn {
			continue
		}
		if hasPattern[svc.Name] {
			continue
		}
		if IsAccepted(accepted, svc.Name, "missing_pattern") {
			continue
		}
		conf := float64(svc.Churn) / float64(thresholdShotgunChurn) * 0.5
		if conf > 1.0 {
			conf = 1.0
		}
		extra = append(extra, PatternDetection{
			PatternID:   entry.ID,
			PatternName: entry.Name,
			Kind:        entry.Kind,
			Component:   svc.Name,
			Confidence:  port.Confidence(conf),
			Evidence: []string{
				fmt.Sprintf("churn=%d (threshold %d)", svc.Churn, thresholdShotgunChurn),
				"no positive design pattern detected",
			},
			Severity: severityForDetection(entry.Kind, conf),
		})
	}
	return extra
}

// ComputePatternScan evaluates all fingerprints against the provided architecture
// data and returns a report of detected patterns and smells.
// Thresholds for smell fingerprints are scaled by role multiplier; accepted
// violations (matched by pattern_id as principle) are suppressed.
func ComputePatternScan(
	services []arch.ArchService,
	edges []arch.ArchEdge,
	cycles []graph.Cycle,
	classes []analysis.ClassInfo,
	impls []analysis.ImplEdge,
	roles map[string]HexaRole,
	accepted []port.AcceptedViolation,
) *PatternScanReport {
	var detections []PatternDetection

	for i := range services {
		svc := &services[i]
		mult := RoleMultiplier(roles[svc.Name])
		// Evaluate each fingerprint against this service.
		for _, fp := range fingerprints {
			det := evaluateFingerprint(fp, *svc, edges, cycles, classes, impls, mult)
			if det != nil {
				if !IsAccepted(accepted, det.Component, det.PatternID) {
					detections = append(detections, *det)
				}
			}
		}
		// Special case: feature envy.
		if det := evaluateFeatureEnvy(*svc, edges); det != nil {
			if !IsAccepted(accepted, det.Component, det.PatternID) {
				detections = append(detections, *det)
			}
		}
	}

	// Post-processing: flag high-churn components with no recognized pattern.
	detections = append(detections, detectMissingPattern(services, detections, accepted)...)

	// Sort: smells before patterns, then by confidence descending.
	sort.Slice(detections, func(i, j int) bool {
		if detections[i].Kind != detections[j].Kind {
			return detections[i].Kind == PatternKindSmell
		}
		return detections[i].Confidence > detections[j].Confidence
	})

	patternsFound := 0
	smellsFound := 0
	for i := range detections {
		if detections[i].Kind == PatternKindPattern {
			patternsFound++
		} else {
			smellsFound++
		}
	}

	summary := "No patterns or smells detected"
	if patternsFound > 0 || smellsFound > 0 {
		summary = fmt.Sprintf("%d pattern(s) detected, %d smell(s) flagged", patternsFound, smellsFound)
	}

	return &PatternScanReport{
		Detections:    detections,
		PatternsFound: patternsFound,
		SmellsFound:   smellsFound,
		Summary:       summary,
	}
}

// GetPatternCatalog returns catalog entries, optionally filtered by kind or substring.
// Steps and Examples are only included when the filter matches exactly one entry by ID,
// keeping multi-entry responses within token budgets.
func GetPatternCatalog(filter string) *PatternCatalogReport {
	var entries []CatalogEntry

	switch {
	case filter == "":
		entries = make([]CatalogEntry, len(patternCatalog))
		copy(entries, patternCatalog)
	case filter == "pattern" || filter == "smell":
		kind := PatternKind(filter)
		for i := range patternCatalog {
			if patternCatalog[i].Kind == kind {
				entries = append(entries, patternCatalog[i])
			}
		}
	default:
		lower := strings.ToLower(filter)
		for i := range patternCatalog {
			e := &patternCatalog[i]
			if strings.Contains(strings.ToLower(e.ID), lower) ||
				strings.Contains(strings.ToLower(e.Name), lower) ||
				strings.Contains(strings.ToLower(e.Category), lower) ||
				strings.Contains(strings.ToLower(e.Description), lower) {
				entries = append(entries, *e)
			}
		}
	}

	// Only include verbose fields (Steps, Examples) when a single entry is matched
	// by exact ID. Multi-entry listings stay concise for token budget compliance.
	singleExactMatch := len(entries) == 1 && strings.EqualFold(entries[0].ID, filter)
	if !singleExactMatch {
		for i := range entries {
			entries[i].Steps = nil
			entries[i].Examples = nil
		}
	}

	summary := fmt.Sprintf("%d catalog entries", len(entries))
	if filter != "" {
		summary = fmt.Sprintf("%d entries matching '%s'", len(entries), filter)
	}

	return &PatternCatalogReport{
		Entries: entries,
		Summary: summary,
	}
}
