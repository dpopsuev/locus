package naming

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/dpopsuev/locus/internal/arch"
	"github.com/dpopsuev/locus/internal/port"
	"github.com/dpopsuev/oculus/lang"
)

// Fan-in threshold above which severity is escalated from warning to error.
const fanInEscalationThreshold = 5

// Score scaling factor: issues / total * 100.
const scoreScale = 100

// Minimum remaining length after stripping a generic suffix for a symbol to be considered qualified.
const minDomainPrefixLen = 3

// SymbolIssue records a single naming problem found in an exported symbol.
type SymbolIssue struct {
	Symbol     string        `json:"symbol"`
	Package    string        `json:"package"`
	Issue      string        `json:"issue"` // "abbreviation", "generic_name", "verbless_export"
	Severity   port.Severity `json:"severity"`
	FanIn      int           `json:"fan_in"`
	Suggestion string        `json:"suggestion,omitempty"`
}

// SymbolQualityReport summarizes naming quality across all exported symbols.
type SymbolQualityReport struct {
	Issues       []SymbolIssue `json:"issues"`
	TotalChecked int           `json:"total_checked"`
	Score        port.Score    `json:"score"`
	Summary      string        `json:"summary"`
}

// SynonymGroup collects variants of the same canonical term across packages.
type SynonymGroup struct {
	Canonical string   `json:"canonical"`
	Variants  []string `json:"variants"`
	Packages  []string `json:"packages"`
}

// VocabMapReport summarizes vocabulary consistency across the codebase.
type VocabMapReport struct {
	Groups      []SynonymGroup `json:"synonym_groups"`
	Consistency float64        `json:"consistency"` // 0-100
	Summary     string         `json:"summary"`
}

// badAbbreviations maps short abbreviations to their full-word expansions.
var badAbbreviations = map[string]string{
	"Cfg":  "Config",
	"Srv":  "Server",
	"Mgr":  "Manager",
	"Impl": "Implementation",
	"Util": "Utility",
	"Hlpr": "Helper",
	"Ctx":  "Context",
	"Req":  "Request",
	"Resp": "Response",
	"Btn":  "Button",
	"Msg":  "Message",
	"Pkg":  "Package",
	"Env":  "Environment",
	"Tmp":  "Temporary",
	"Buf":  "Buffer",
}

// defaultStdlibIdioms is the fallback set when no language-specific rules are provided.
var defaultStdlibIdioms = map[string]bool{
	"DB": true, "HTTP": true, "URL": true, "API": true, "ID": true,
	"IO": true, "IP": true, "TCP": true, "UDP": true, "JSON": true,
	"XML": true, "SQL": true, "CSS": true, "HTML": true, "OK": true,
	"EOF": true,
}

// genericSuffixes are names too vague without a domain qualifier.
var genericSuffixes = []string{
	"Manager", "Helper", "Util", "Utils", "Data", "Info",
	"Object", "Base", "Common", "General", "Misc",
}

// knownSynonyms maps variant terms to a canonical form for vocabulary consistency checks.
var knownSynonyms = map[string]string{
	"fetch": "get", "retrieve": "get", "obtain": "get", "acquire": "get",
	"create": "new", "build": "new", "construct": "new", "make": "new",
	"delete": "remove", "destroy": "remove", "drop": "remove", "erase": "remove",
	"update": "set", "modify": "set", "change": "set", "alter": "set", "mutate": "set",
	"find": "search", "query": "search", "lookup": "search", "seek": "search",
	"config": "configuration", "cfg": "configuration", "conf": "configuration",
	"init": "initialize", "setup": "initialize",
	"err": "error", "fail": "error",
	"msg":  "message",
	"info": "information",
	"ctx":  "context",
	"req":  "request",
	"resp": "response", "res": "response",
}

// ComputeSymbolQuality analyzes exported symbol names for naming quality issues.
// If rules is nil, GenericRules is used (no verbless violations reported).
func ComputeSymbolQuality(services []arch.ArchService, edges []arch.ArchEdge, rules ...lang.Rules) *SymbolQualityReport {
	var r lang.Rules
	if len(rules) > 0 && rules[0] != nil {
		r = rules[0]
	} else {
		r = &lang.GenericRules{}
	}

	fanIn := buildFanInMap(edges)

	// Resolve idioms for abbreviation checks.
	idioms := r.StdlibIdioms()
	if idioms == nil {
		idioms = defaultStdlibIdioms
	}

	var issues []SymbolIssue
	totalChecked := 0

	for i := range services {
		svc := &services[i]
		svcFanIn := fanIn[svc.Name]
		for _, sym := range svc.Symbols {
			totalChecked++
			issues = append(issues, checkAbbreviation(sym.Name, svc.Package, svcFanIn, idioms)...)
			issues = append(issues, checkGenericName(sym.Name, svc.Package, svcFanIn)...)
			issues = append(issues, checkVerblessExport(sym.Name, sym.Kind.String(), svc.Package, svcFanIn, r)...)
		}
	}

	sortIssues(issues)

	score := float64(scoreScale)
	if totalChecked > 0 {
		score = float64(scoreScale) - float64(len(issues))/float64(totalChecked)*float64(scoreScale)
		if score < 0 {
			score = 0
		}
	}

	summary := fmt.Sprintf("Symbol quality: %.0f/100 — %d issue(s) in %d symbols checked", score, len(issues), totalChecked)
	if len(issues) == 0 {
		summary = fmt.Sprintf("Symbol quality: 100/100 — all %d symbols clean", totalChecked)
	}

	return &SymbolQualityReport{
		Issues:       issues,
		TotalChecked: totalChecked,
		Score:        port.Score(score),
		Summary:      summary,
	}
}

// buildFanInMap computes the number of incoming edges for each target service.
func buildFanInMap(edges []arch.ArchEdge) map[string]int {
	fanIn := make(map[string]int, len(edges))
	for _, e := range edges {
		w := e.Weight
		if w == 0 {
			w = 1
		}
		fanIn[e.To] += w
	}
	return fanIn
}

// checkAbbreviation flags symbols that use common abbreviations instead of full words.
func checkAbbreviation(sym, pkg string, svcFanIn int, idioms map[string]bool) []SymbolIssue {
	for abbr, expansion := range badAbbreviations {
		if sym == abbr || strings.HasSuffix(sym, abbr) {
			if idioms[sym] {
				continue
			}
			severity := port.SeverityWarning
			if svcFanIn >= fanInEscalationThreshold {
				severity = port.SeverityError
			}
			return []SymbolIssue{{
				Symbol:     sym,
				Package:    pkg,
				Issue:      "abbreviation",
				Severity:   severity,
				FanIn:      svcFanIn,
				Suggestion: fmt.Sprintf("Use full word: %s → %s", abbr, expansion),
			}}
		}
	}
	return nil
}

// checkGenericName flags symbols that are or end with generic suffixes without a domain qualifier.
func checkGenericName(sym, pkg string, svcFanIn int) []SymbolIssue {
	for _, suffix := range genericSuffixes {
		if sym == suffix || strings.HasSuffix(sym, suffix) {
			remaining := strings.TrimSuffix(sym, suffix)
			if len(remaining) < minDomainPrefixLen {
				severity := port.SeverityWarning
				if svcFanIn >= fanInEscalationThreshold {
					severity = port.SeverityError
				}
				return []SymbolIssue{{
					Symbol:     sym,
					Package:    pkg,
					Issue:      "generic_name",
					Severity:   severity,
					FanIn:      svcFanIn,
					Suggestion: "Add domain-specific prefix",
				}}
			}
		}
	}
	return nil
}

// checkVerblessExport flags exported symbols that lack a verb prefix and don't look like types.
// Delegates to language-specific Rules for the actual check. If rules reports no violation,
// the symbol is clean. GenericRules (the default) never reports violations — conservative default.
func checkVerblessExport(sym, kind, pkg string, svcFanIn int, rules lang.Rules) []SymbolIssue {
	if !rules.IsVerblessViolation(sym, kind) {
		return nil
	}

	severity := port.SeverityInfo
	if svcFanIn >= fanInEscalationThreshold {
		severity = port.SeverityWarning
	}

	return []SymbolIssue{{
		Symbol:     sym,
		Package:    pkg,
		Issue:      "verbless_export",
		Severity:   severity,
		FanIn:      svcFanIn,
		Suggestion: "Consider adding a verb prefix for clarity",
	}}
}

// symbolSeverityOrder returns a numeric rank for sorting: error(0) < warning(1) < info(2).
func symbolSeverityOrder(s port.Severity) int {
	switch s {
	case port.SeverityError:
		return 0
	case port.SeverityWarning:
		return 1
	case port.SeverityInfo:
		return 2
	default:
		return 3
	}
}

// sortIssues orders issues by severity (errors first), then by fan-in descending.
func sortIssues(issues []SymbolIssue) {
	sort.Slice(issues, func(i, j int) bool {
		ri, rj := symbolSeverityOrder(issues[i].Severity), symbolSeverityOrder(issues[j].Severity)
		if ri != rj {
			return ri < rj
		}
		return issues[i].FanIn > issues[j].FanIn
	})
}

// ComputeVocabMap analyzes vocabulary consistency across packages by detecting synonym drift.
func ComputeVocabMap(services []arch.ArchService) *VocabMapReport {
	// canonical -> variant -> set of packages
	type variantInfo struct {
		packages map[string]bool
	}
	canonMap := make(map[string]map[string]*variantInfo)

	for i := range services {
		svc := &services[i]
		for _, sym := range svc.Symbols {
			tokens := splitCamelCase(sym.Name)
			for _, tok := range tokens {
				lower := strings.ToLower(tok)
				canonical := lower
				if mapped, ok := knownSynonyms[lower]; ok {
					canonical = mapped
				}

				variants, ok := canonMap[canonical]
				if !ok {
					variants = make(map[string]*variantInfo)
					canonMap[canonical] = variants
				}
				vi, ok := variants[lower]
				if !ok {
					vi = &variantInfo{packages: make(map[string]bool)}
					variants[lower] = vi
				}
				vi.packages[svc.Package] = true
			}
		}
	}

	// Collect groups with drift: more than 1 variant across more than 1 package.
	totalCanonicals := len(canonMap)
	groups := make([]SynonymGroup, 0, totalCanonicals)

	for canonical, variants := range canonMap {
		if len(variants) <= 1 {
			continue
		}
		// Check if variants span more than 1 package.
		allPkgs := make(map[string]bool)
		for _, vi := range variants {
			for pkg := range vi.packages {
				allPkgs[pkg] = true
			}
		}
		if len(allPkgs) <= 1 {
			continue
		}

		var variantNames []string
		for v := range variants {
			variantNames = append(variantNames, v)
		}
		sort.Strings(variantNames)

		var pkgNames []string
		for pkg := range allPkgs {
			pkgNames = append(pkgNames, pkg)
		}
		sort.Strings(pkgNames)

		groups = append(groups, SynonymGroup{
			Canonical: canonical,
			Variants:  variantNames,
			Packages:  pkgNames,
		})
	}

	// Sort by number of variants descending.
	sort.Slice(groups, func(i, j int) bool {
		return len(groups[i].Variants) > len(groups[j].Variants)
	})

	consistency := float64(scoreScale)
	if totalCanonicals > 0 {
		consistency = (1 - float64(len(groups))/float64(totalCanonicals)) * float64(scoreScale)
	}

	summary := fmt.Sprintf("Vocabulary consistency: %.0f%% — %d synonym drift(s) across %d terms",
		consistency, len(groups), totalCanonicals)
	if len(groups) == 0 {
		summary = fmt.Sprintf("Vocabulary: 100%% consistent across %d terms", totalCanonicals)
	}

	return &VocabMapReport{
		Groups:      groups,
		Consistency: consistency,
		Summary:     summary,
	}
}

// splitCamelCase splits a PascalCase/camelCase string into tokens.
//
// Examples:
//
//	"GetUserByID"  → ["Get", "User", "By", "ID"]
//	"HTTPServer"   → ["HTTP", "Server"]
//	"parseJSON"    → ["parse", "JSON"]
func splitCamelCase(s string) []string {
	runes := []rune(s)
	if len(runes) == 0 {
		return nil
	}

	var tokens []string
	start := 0

	for i := 1; i < len(runes); i++ {
		prev := runes[i-1]
		cur := runes[i]

		// Split on lower→upper transition: "getUser" → split before 'U'
		if unicode.IsLower(prev) && unicode.IsUpper(cur) {
			tokens = append(tokens, string(runes[start:i]))
			start = i
			continue
		}

		// Split on upper→upper→lower transition for acronyms: "HTTPServer" → split before 'S'
		if i+1 < len(runes) && unicode.IsUpper(prev) && unicode.IsUpper(cur) && unicode.IsLower(runes[i+1]) {
			if start < i {
				tokens = append(tokens, string(runes[start:i]))
				start = i
			}
		}
	}

	// Append the last token.
	if start < len(runes) {
		tokens = append(tokens, string(runes[start:]))
	}

	return tokens
}
