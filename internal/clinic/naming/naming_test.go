package naming

import (
	"strings"
	"testing"

	"github.com/dpopsuev/locus/internal/arch"
	"github.com/dpopsuev/locus/internal/model"
	"github.com/dpopsuev/locus/internal/oculus/lang"
	"github.com/dpopsuev/locus/internal/port"
)

func TestComputeSymbolQuality_Abbreviation(t *testing.T) {
	services := []arch.ArchService{
		{Name: "svc/config", Package: "pkg/config", Symbols: model.SymbolsFromNames("Cfg")},
	}

	report := ComputeSymbolQuality(services, nil)

	if len(report.Issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(report.Issues))
	}
	issue := report.Issues[0]
	if issue.Issue != "abbreviation" {
		t.Errorf("expected issue type abbreviation, got %s", issue.Issue)
	}
	if issue.Symbol != "Cfg" {
		t.Errorf("expected symbol Cfg, got %s", issue.Symbol)
	}
	if issue.Severity != port.SeverityWarning {
		t.Errorf("expected severity %s, got %s", port.SeverityWarning, issue.Severity)
	}
	if !strings.Contains(issue.Suggestion, "Config") {
		t.Errorf("suggestion should mention Config, got %q", issue.Suggestion)
	}
}

func TestComputeSymbolQuality_AbbreviationSuffix(t *testing.T) {
	services := []arch.ArchService{
		{Name: "svc/server", Package: "pkg/server", Symbols: model.SymbolsFromNames("AppSrv")},
	}

	report := ComputeSymbolQuality(services, nil)

	found := false
	for _, issue := range report.Issues {
		if issue.Issue == "abbreviation" && issue.Symbol == "AppSrv" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected AppSrv to be flagged as abbreviation, issues: %+v", report.Issues)
	}
}

func TestComputeSymbolQuality_StdlibIdiomNotFlagged(t *testing.T) {
	services := []arch.ArchService{
		{Name: "svc/db", Package: "pkg/db", Symbols: model.SymbolsFromNames("DB", "HTTP", "URL")},
	}

	report := ComputeSymbolQuality(services, nil)

	for _, issue := range report.Issues {
		if issue.Issue == "abbreviation" {
			t.Errorf("stdlib idiom %q should not be flagged as abbreviation", issue.Symbol)
		}
	}
}

func TestComputeSymbolQuality_GenericName(t *testing.T) {
	services := []arch.ArchService{
		{Name: "svc/core", Package: "pkg/core", Symbols: model.SymbolsFromNames("Manager", "SessionManager")},
	}

	report := ComputeSymbolQuality(services, nil)

	// "Manager" (bare) should be flagged, "SessionManager" should not.
	found := false
	for _, issue := range report.Issues {
		if issue.Issue == "generic_name" {
			if issue.Symbol == "Manager" {
				found = true
			}
			if issue.Symbol == "SessionManager" {
				t.Error("SessionManager should not be flagged as generic (has domain qualifier)")
			}
		}
	}
	if !found {
		t.Error("bare Manager should be flagged as generic_name")
	}
}

func TestComputeSymbolQuality_Clean(t *testing.T) {
	services := []arch.ArchService{
		{Name: "svc/user", Package: "pkg/user", Symbols: model.SymbolsFromNames("GetUser", "ParseConfig")},
	}

	report := ComputeSymbolQuality(services, nil)

	if len(report.Issues) != 0 {
		t.Errorf("expected 0 issues for clean symbols, got %d: %+v", len(report.Issues), report.Issues)
	}
	if report.TotalChecked != 2 {
		t.Errorf("expected 2 total checked, got %d", report.TotalChecked)
	}
	if report.Score != 100 {
		t.Errorf("expected score 100, got %.0f", report.Score)
	}
	if !strings.Contains(report.Summary, "all 2 symbols clean") {
		t.Errorf("unexpected summary: %s", report.Summary)
	}
}

func TestComputeSymbolQuality_FanInWeighting(t *testing.T) {
	services := []arch.ArchService{
		{Name: "svc/core", Package: "pkg/core", Symbols: model.SymbolsFromNames("Cfg")},
	}
	// Create enough edges to push fan-in to the escalation threshold.
	edges := []arch.ArchEdge{
		{From: "a", To: "svc/core", Weight: 2},
		{From: "b", To: "svc/core", Weight: 2},
		{From: "c", To: "svc/core", Weight: 1},
	}

	report := ComputeSymbolQuality(services, edges)

	if len(report.Issues) == 0 {
		t.Fatal("expected at least 1 issue")
	}
	issue := report.Issues[0]
	if issue.Severity != port.SeverityError {
		t.Errorf("expected severity %s for high fan-in, got %s", port.SeverityError, issue.Severity)
	}
	if issue.FanIn != 5 {
		t.Errorf("expected fan_in 5, got %d", issue.FanIn)
	}
}

func TestComputeSymbolQuality_VerblessExport(t *testing.T) {
	services := []arch.ArchService{
		{Name: "svc/ops", Package: "pkg/ops", Symbols: model.SymbolsFromNames("Frobnicate")},
	}

	// With GoRules, verbless exports are detected.
	report := ComputeSymbolQuality(services, nil, &lang.GoRules{})

	found := false
	for _, issue := range report.Issues {
		if issue.Issue == "verbless_export" && issue.Symbol == "Frobnicate" {
			found = true
			if issue.Severity != port.SeverityInfo {
				t.Errorf("expected severity %s, got %s", port.SeverityInfo, issue.Severity)
			}
		}
	}
	if !found {
		t.Error("Frobnicate should be flagged as verbless_export")
	}
}

func TestComputeSymbolQuality_VerblessExport_GenericRulesNeverFlags(t *testing.T) {
	services := []arch.ArchService{
		{Name: "svc/ops", Package: "pkg/ops", Symbols: model.SymbolsFromNames("Frobnicate")},
	}

	// Without rules (GenericRules default), no verbless violations are reported.
	report := ComputeSymbolQuality(services, nil)

	for _, issue := range report.Issues {
		if issue.Issue == "verbless_export" {
			t.Errorf("GenericRules should not flag verbless exports, got %q", issue.Symbol)
		}
	}
}

func TestComputeSymbolQuality_TypeSuffixNotFlagged(t *testing.T) {
	services := []arch.ArchService{
		{Name: "svc/http", Package: "pkg/http", Symbols: model.SymbolsFromNames(
			"UserHandler", "TokenValidator", "SessionStore",
		)},
	}

	// Even with GoRules, type-suffix symbols should not be flagged.
	report := ComputeSymbolQuality(services, nil, &lang.GoRules{})

	for _, issue := range report.Issues {
		if issue.Issue == "verbless_export" {
			t.Errorf("type-like symbol %q should not be flagged as verbless_export", issue.Symbol)
		}
	}
}

func TestComputeSymbolQuality_SortOrder(t *testing.T) {
	services := []arch.ArchService{
		{Name: "svc/a", Package: "pkg/a", Symbols: model.SymbolsFromNames("Cfg")},
		{Name: "svc/b", Package: "pkg/b", Symbols: model.SymbolsFromNames("Manager")},
	}
	edges := []arch.ArchEdge{
		{From: "x", To: "svc/a", Weight: 6}, // high fan-in → error
	}

	report := ComputeSymbolQuality(services, edges)

	if len(report.Issues) < 2 {
		t.Fatalf("expected at least 2 issues, got %d", len(report.Issues))
	}
	// First issue should be error severity (escalated due to fan-in).
	if report.Issues[0].Severity != port.SeverityError {
		t.Errorf("first issue should be error, got %s", report.Issues[0].Severity)
	}
	// Second issue should be warning.
	if report.Issues[1].Severity != port.SeverityWarning {
		t.Errorf("second issue should be warning, got %s", report.Issues[1].Severity)
	}
}

func TestComputeVocabMap_SynonymDrift(t *testing.T) {
	services := []arch.ArchService{
		{Name: "svc/a", Package: "pkg/a", Symbols: model.SymbolsFromNames("GetUser")},
		{Name: "svc/b", Package: "pkg/b", Symbols: model.SymbolsFromNames("FetchUser")},
	}

	report := ComputeVocabMap(services)

	if len(report.Groups) == 0 {
		t.Fatal("expected at least one synonym group for get/fetch drift")
	}

	found := false
	for _, g := range report.Groups {
		if g.Canonical != "get" {
			continue
		}
		found = true
		if len(g.Variants) < 2 {
			t.Errorf("expected at least 2 variants, got %d: %v", len(g.Variants), g.Variants)
		}
		hasFetch := false
		hasGet := false
		for _, v := range g.Variants {
			if v == "fetch" {
				hasFetch = true
			}
			if v == "get" {
				hasGet = true
			}
		}
		if !hasFetch || !hasGet {
			t.Errorf("expected both get and fetch in variants, got %v", g.Variants)
		}
		if len(g.Packages) < 2 {
			t.Errorf("expected at least 2 packages, got %d: %v", len(g.Packages), g.Packages)
		}
		break
	}
	if !found {
		t.Error("expected synonym group with canonical 'get'")
	}

	if report.Consistency >= 100 {
		t.Errorf("consistency should be below 100 with drift, got %.0f", report.Consistency)
	}
}

func TestComputeVocabMap_Consistent(t *testing.T) {
	services := []arch.ArchService{
		{Name: "svc/a", Package: "pkg/a", Symbols: model.SymbolsFromNames("GetUser")},
		{Name: "svc/b", Package: "pkg/b", Symbols: model.SymbolsFromNames("GetOrder")},
	}

	report := ComputeVocabMap(services)

	// "Get" used consistently in both packages — no drift.
	for _, g := range report.Groups {
		if g.Canonical == "get" {
			t.Errorf("should not have drift for consistently used 'get', got variants %v", g.Variants)
		}
	}

	if report.Consistency != 100 {
		t.Errorf("expected 100%% consistency, got %.0f%%", report.Consistency)
	}
	if !strings.Contains(report.Summary, "100%") {
		t.Errorf("summary should mention 100%%, got %q", report.Summary)
	}
}

func TestSplitCamelCase(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{input: "GetUserByID", want: []string{"Get", "User", "By", "ID"}},
		{input: "HTTPServer", want: []string{"HTTP", "Server"}},
		{input: "parseJSON", want: []string{"parse", "JSON"}},
		{input: "simple", want: []string{"simple"}},
		{input: "A", want: []string{"A"}},
		{input: "", want: nil},
		{input: "XMLParser", want: []string{"XML", "Parser"}},
		{input: "getHTTPResponse", want: []string{"get", "HTTP", "Response"}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := splitCamelCase(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("splitCamelCase(%q) = %v, want %v", tt.input, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("splitCamelCase(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}
