package arch

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanAndBuild_IntentLevels(t *testing.T) {
	// Use the locus repo itself as a fixture.
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Skip("cannot resolve repo root")
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Skip("not in a Go repo")
	}

	tests := []struct {
		intent         ScanIntent
		wantCycles     bool
		wantHotSpots   bool
		wantNesting    bool
		wantGitHistory bool
	}{
		{IntentArchitecture, false, false, false, false},
		{IntentCoupling, true, true, false, false},
		{IntentHealth, true, true, true, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.intent), func(t *testing.T) {
			report, err := ScanAndBuild(root, ScanOpts{
				ExcludeTests: true,
				Depth:        2,
				ChurnDays:    7,
				Intent:       tt.intent,
			})
			if err != nil {
				t.Fatalf("scan: %v", err)
			}
			if len(report.Architecture.Services) == 0 {
				t.Error("expected at least one service")
			}
			if report.ModulePath == "" {
				t.Error("expected non-empty module path")
			}

			hasCycles := report.Cycles != nil
			if hasCycles != tt.wantCycles {
				t.Errorf("cycles: got %v, want %v", hasCycles, tt.wantCycles)
			}

			hasHotSpots := report.HotSpots != nil
			if hasHotSpots != tt.wantHotSpots {
				t.Errorf("hot spots: got %v, want %v", hasHotSpots, tt.wantHotSpots)
			}

			hasNesting := hasAnyNesting(report)
			if hasNesting != tt.wantNesting {
				t.Errorf("nesting: got %v, want %v", hasNesting, tt.wantNesting)
			}

			hasGitHistory := len(report.RecentCommits) > 0
			if hasGitHistory != tt.wantGitHistory {
				t.Errorf("git history: got %v, want %v", hasGitHistory, tt.wantGitHistory)
			}
		})
	}
}

func hasAnyNesting(r *ContextReport) bool {
	for i := range r.Architecture.Services {
		if r.Architecture.Services[i].MaxNesting > 0 {
			return true
		}
	}
	return false
}

func TestScanIntentLevel(t *testing.T) {
	tests := []struct {
		intent ScanIntent
		level  int
	}{
		{IntentArchitecture, 0},
		{IntentCoupling, 1},
		{IntentHealth, 2},
		{IntentFull, 3},
		{"", 2},        // default
		{"unknown", 2}, // unknown defaults to health
	}
	for _, tt := range tests {
		got := tt.intent.ScanLevel()
		if got != tt.level {
			t.Errorf("ScanIntent(%q).ScanLevel() = %d, want %d", tt.intent, got, tt.level)
		}
	}
}
