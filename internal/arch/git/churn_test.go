package git

import (
	"testing"
)

func TestAggregateChurn_ExcludesTestFiles(t *testing.T) {
	gitOutput := `internal/arch/scan.go
internal/arch/scan_test.go
internal/arch/churn.go
internal/arch/churn_test.go
internal/protocol/pattern_scan.go
internal/protocol/pattern_scan_test.go
internal/protocol/protocol.go
`
	result := aggregateChurn(gitOutput, "/repo")

	// Two production files in internal/arch, two test files excluded.
	if got := result["internal/arch"]; got != 2 {
		t.Errorf("internal/arch churn = %d, want 2 (test files should be excluded)", got)
	}

	// Two production files in internal/protocol, one test file excluded.
	if got := result["internal/protocol"]; got != 2 {
		t.Errorf("internal/protocol churn = %d, want 2 (test files should be excluded)", got)
	}
}

func TestAggregateChurn_AllTestFiles(t *testing.T) {
	// A package with only test file changes should have zero churn.
	gitOutput := `pkg/foo/foo_test.go
pkg/foo/bar_test.go
`
	result := aggregateChurn(gitOutput, "/repo")

	if got := result["pkg/foo"]; got != 0 {
		t.Errorf("pkg/foo churn = %d, want 0 (only test files changed)", got)
	}
}

func TestAggregateChurn_RootFilesSkipped(t *testing.T) {
	gitOutput := `main.go
go.mod
internal/arch/scan.go
`
	result := aggregateChurn(gitOutput, "/repo")

	// Root-level files (dir == ".") are skipped.
	if _, ok := result["."]; ok {
		t.Error("root-level files should be skipped")
	}
	if got := result["internal/arch"]; got != 1 {
		t.Errorf("internal/arch churn = %d, want 1", got)
	}
}

func TestAggregateChurn_EmptyOutput(t *testing.T) {
	result := aggregateChurn("", "/repo")
	if len(result) != 0 {
		t.Errorf("expected empty result for empty output, got %v", result)
	}
}

func TestAggregateChurn_NonGoTestFilesIncluded(t *testing.T) {
	// Only _test.go files are excluded — other test-like files should count.
	gitOutput := `pkg/foo/testdata/fixture.json
pkg/foo/foo.go
pkg/foo/test_helper.go
`
	result := aggregateChurn(gitOutput, "/repo")

	// foo.go + test_helper.go = 2 (testdata is a different dir)
	if got := result["pkg/foo"]; got != 2 {
		t.Errorf("pkg/foo churn = %d, want 2", got)
	}
	if got := result["pkg/foo/testdata"]; got != 1 {
		t.Errorf("pkg/foo/testdata churn = %d, want 1", got)
	}
}
