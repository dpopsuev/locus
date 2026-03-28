package protocol

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dpopsuev/locus/internal/analysis"
	"github.com/dpopsuev/locus/internal/arch"
)

// PatternKind distinguishes design patterns from code smells.
type PatternKind string

const (
	PatternKindPattern PatternKind = "pattern"
	PatternKindSmell   PatternKind = "smell"
)

// CatalogEntry describes a known pattern or smell in the catalog.
type CatalogEntry struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Kind        PatternKind `json:"kind"`
	Category    string      `json:"category"`
	Description string      `json:"description"`
	Indicators  []string    `json:"indicators"`
	Remediation string      `json:"remediation,omitempty"`
}

// PatternDetection records a single detected pattern or smell in a component.
type PatternDetection struct {
	PatternID   string      `json:"pattern_id"`
	PatternName string      `json:"pattern_name"`
	Kind        PatternKind `json:"kind"`
	Component   string      `json:"component"`
	Confidence  float64     `json:"confidence"`
	Evidence    []string    `json:"evidence"`
	Severity    string      `json:"severity"`
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
	thresholdGodLOC          = 1000
	thresholdGodSymbols      = 30
	thresholdGodFan          = 5
	thresholdLazyLOC         = 20
	thresholdLazyFanIn       = 1
	thresholdShotgunChurn    = 10
	thresholdShotgunFanIn    = 5
	thresholdFeatureEnvyPct  = 0.5
	thresholdStrategyImpls   = 2
	fingerprintGodThreshold  = 0.5
	fingerprintHighThreshold = 0.9
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
	// ── Smells ──
	{
		ID: "god_component", Name: "God Component", Kind: PatternKindSmell,
		Category: "smell", Description: "Component doing too much",
		Indicators:  []string{"high fan-in AND fan-out", "LOC > 1000", "symbol count > 30"},
		Remediation: "Extract responsibilities into focused packages",
	},
	{
		ID: "feature_envy", Name: "Feature Envy", Kind: PatternKindSmell,
		Category: "smell", Description: "Component uses another's data more than its own",
		Indicators:  []string{"high call_sites to single target", "LOCSurface > own LOC"},
		Remediation: "Move logic to the component whose data it uses",
	},
	{
		ID: "shotgun_surgery", Name: "Shotgun Surgery", Kind: PatternKindSmell,
		Category: "smell", Description: "Changes require touching many files",
		Indicators:  []string{"high churn AND high fan-in"},
		Remediation: "Consolidate related logic to reduce change blast radius",
	},
	{
		ID: "inappropriate_intimacy", Name: "Inappropriate Intimacy", Kind: PatternKindSmell,
		Category: "smell", Description: "Bidirectional coupling between components",
		Indicators:  []string{"mutual edges between 2 components", "high weight both ways"},
		Remediation: "Extract shared interface or merge components",
	},
	{
		ID: "lazy_component", Name: "Lazy Component", Kind: PatternKindSmell,
		Category: "smell", Description: "Too little responsibility",
		Indicators:  []string{"LOC < 20", "0-1 symbols", "0 fan-in"},
		Remediation: "Merge into related component",
	},
	{
		ID: "data_clump", Name: "Data Clump", Kind: PatternKindSmell,
		Category: "smell", Description: "Groups of data that travel together",
		Indicators:  []string{"multiple structs sharing 3+ field types"},
		Remediation: "Extract common fields into shared struct",
	},
	{
		ID: "long_parameter_list", Name: "Long Parameter List", Kind: PatternKindSmell,
		Category: "smell", Description: "Functions with too many parameters",
		Indicators:  []string{"methods with >5 parameters"},
		Remediation: "Introduce parameter object or options struct",
	},
	{
		ID: "dead_code", Name: "Dead Code", Kind: PatternKindSmell,
		Category: "smell", Description: "Exported symbols with zero callers",
		Indicators:  []string{"exported symbol", "0 fan-in", "not in cmd/"},
		Remediation: "Remove or unexport unused symbols",
	},
	{
		ID: "unstable_interface", Name: "Unstable Interface", Kind: PatternKindSmell,
		Category: "smell", Description: "Interface that changes frequently",
		Indicators:  []string{"interface package with high churn", "many implementors affected"},
		Remediation: "Freeze interface, version with new type",
	},
	{
		ID: "circular_dependency", Name: "Circular Dependency", Kind: PatternKindSmell,
		Category: "smell", Description: "Mutual dependency cycle",
		Indicators:  []string{"cycle in dependency graph"},
		Remediation: "Break cycle with dependency inversion",
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

func signalCycleParticipant(svcName string, cycles []arch.Cycle) (detected bool, confidence float64, evidence string) {
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
	// Smells with high thresholds
	{patternID: "data_clump", rules: []fingerprintRule{{signal: "highSymbolCount", weight: 1.0, threshold: 20}}, threshold: fingerprintHighThreshold},
	{patternID: "long_parameter_list", rules: []fingerprintRule{{signal: "highSymbolCount", weight: 1.0, threshold: 15}}, threshold: fingerprintHighThreshold},
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
	cycles []arch.Cycle,
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
	default:
		return false, 0, ""
	}
}

// evaluateFingerprint checks a single fingerprint against a service and returns
// a detection if the weighted score meets the threshold.
func evaluateFingerprint(
	fp patternFingerprint,
	svc arch.ArchService,
	edges []arch.ArchEdge,
	cycles []arch.Cycle,
	classes []analysis.ClassInfo,
	impls []analysis.ImplEdge,
) *PatternDetection {
	entry := catalogByID[fp.patternID]
	if entry == nil {
		return nil
	}

	var weightedSum float64
	evidence := make([]string, 0, len(fp.rules))
	for _, rule := range fp.rules {
		detected, conf, ev := evaluateSignal(rule, svc, edges, cycles, classes, impls)
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
		Confidence:  weightedSum,
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
				Confidence:  ratio,
				Evidence:    []string{fmt.Sprintf("%.0f%% of call sites target %s", ratio*100, target)},
				Severity:    severityForDetection(entry.Kind, ratio),
			}
		}
	}
	return nil
}

// severityForDetection maps pattern kind and confidence to a severity level.
func severityForDetection(kind PatternKind, confidence float64) string {
	if kind == PatternKindPattern {
		return SeverityInfo
	}
	// Smells: high confidence → error, otherwise warning.
	if confidence > 0.8 {
		return SeverityError
	}
	return SeverityWarning
}

// ComputePatternScan evaluates all fingerprints against the provided architecture
// data and returns a report of detected patterns and smells.
func ComputePatternScan(
	services []arch.ArchService,
	edges []arch.ArchEdge,
	cycles []arch.Cycle,
	classes []analysis.ClassInfo,
	impls []analysis.ImplEdge,
) *PatternScanReport {
	var detections []PatternDetection

	for i := range services {
		svc := &services[i]
		// Evaluate each fingerprint against this service.
		for _, fp := range fingerprints {
			det := evaluateFingerprint(fp, *svc, edges, cycles, classes, impls)
			if det != nil {
				detections = append(detections, *det)
			}
		}
		// Special case: feature envy.
		if det := evaluateFeatureEnvy(*svc, edges); det != nil {
			detections = append(detections, *det)
		}
	}

	// Sort: smells before patterns, then by confidence descending.
	sort.Slice(detections, func(i, j int) bool {
		if detections[i].Kind != detections[j].Kind {
			return detections[i].Kind == PatternKindSmell
		}
		return detections[i].Confidence > detections[j].Confidence
	})

	patternsFound := 0
	smellsFound := 0
	for _, d := range detections {
		if d.Kind == PatternKindPattern {
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
func GetPatternCatalog(filter string) *PatternCatalogReport {
	var entries []CatalogEntry

	switch {
	case filter == "":
		entries = make([]CatalogEntry, len(patternCatalog))
		copy(entries, patternCatalog)
	case filter == "pattern" || filter == "smell":
		kind := PatternKind(filter)
		for _, e := range patternCatalog {
			if e.Kind == kind {
				entries = append(entries, e)
			}
		}
	default:
		lower := strings.ToLower(filter)
		for _, e := range patternCatalog {
			if strings.Contains(strings.ToLower(e.ID), lower) ||
				strings.Contains(strings.ToLower(e.Name), lower) ||
				strings.Contains(strings.ToLower(e.Category), lower) ||
				strings.Contains(strings.ToLower(e.Description), lower) {
				entries = append(entries, e)
			}
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
