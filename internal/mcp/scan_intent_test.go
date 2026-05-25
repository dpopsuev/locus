// Package mcp — regression tests for LCS-BUG-77.
//
// scan_local with intent=full must forward ScannerOverride="lsp" to the engine
// so that the symbol graph, call graph, and coupling hot spots are populated.
// Without this, TypeScriptScanner (regex-only) is used and analysis actions
// (risk_scores, symbol_search, cycles, coupling) all return empty/null.
package mcp

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dpopsuev/oculus/v3/arch"
	"github.com/dpopsuev/oculus/v3/engine"
)

// capturingScanProto records every ScanOpts passed to ScanProject.
type capturingScanProto struct {
	calls []engine.ScanOpts
}

func (c *capturingScanProto) ScanProject(_ context.Context, _ string, opts engine.ScanOpts) (*engine.ScanResult, error) {
	c.calls = append(c.calls, opts)
	return &engine.ScanResult{Report: &arch.ContextReport{}}, nil
}

func (c *capturingScanProto) CheckDriftOnScan(_ context.Context, _ string, _ *arch.ContextReport) string {
	return ""
}

func (c *capturingScanProto) last() engine.ScanOpts {
	return c.calls[len(c.calls)-1]
}

// --- LCS-BUG-77 reproduction ---

// TestScanLocal_IntentFull_UsesAutoScanner verifies that intent=full with no
// explicit scanner leaves ScannerOverride empty so AutoScanner selects the
// right language scanner (e.g. TypeScriptScanner for TS → correct import
// edges). Deep analysis uses the LSP pool independently (LCS-BUG-78).
func TestScanLocal_IntentFull_UsesAutoScanner(t *testing.T) {
	sp := &capturingScanProto{}
	h := newTestHandler(sp)

	_, _, err := h.handleScanProject(context.Background(), &codographActionInput{
		Path:   "/workspace/ts-project",
		Intent: "full",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := sp.last().Scanner
	if got != "" {
		t.Errorf("intent=full with no explicit scanner: ScannerOverride=%q, want \"\" (auto)\n"+
			"(LCS-BUG-78: forcing lsp for survey breaks import edges; deep analysis uses pool)", got)
	}
}

// TestScanLocal_ExplicitScanner_Forwarded verifies that an explicit scanner
// field on the MCP input is forwarded verbatim to ScannerOverride. This covers
// the case where a caller wants a specific scanner regardless of intent.
func TestScanLocal_ExplicitScanner_Forwarded(t *testing.T) {
	cases := []struct {
		scanner string
		intent  string
		want    string
	}{
		{"lsp", "health", "lsp"},
		{"typescript", "full", "typescript"}, // explicit overrides the full→lsp default
		{"go", "architecture", "go"},
		{"ctags", "health", "ctags"},
	}

	for _, tc := range cases {
		t.Run(tc.scanner+"/"+tc.intent, func(t *testing.T) {
			sp := &capturingScanProto{}
			h := newTestHandler(sp)

			_, _, err := h.handleScanProject(context.Background(), &codographActionInput{
				Path:    "/workspace/proj",
				Intent:  tc.intent,
				Scanner: tc.scanner,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := sp.last().Scanner; got != tc.want {
				t.Errorf("scanner=%q intent=%q: ScannerOverride=%q, want %q",
					tc.scanner, tc.intent, got, tc.want)
			}
		})
	}
}

// TestScanLocal_IntentBelowFull_AutoScanner verifies that intents below "full"
// leave ScannerOverride empty so AutoScanner picks the right language scanner
// automatically. Upgrading to LSP for non-full intents would be wasteful.
func TestScanLocal_IntentBelowFull_AutoScanner(t *testing.T) {
	for _, intent := range []string{"architecture", "coupling", "health", ""} {
		t.Run("intent="+intent, func(t *testing.T) {
			sp := &capturingScanProto{}
			h := newTestHandler(sp)

			_, _, err := h.handleScanProject(context.Background(), &codographActionInput{
				Path:   "/workspace/proj",
				Intent: intent,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := sp.last().Scanner; got != "" {
				t.Errorf("intent=%q: ScannerOverride=%q, want \"\" (auto)", intent, got)
			}
		})
	}
}

// TestScanLocal_IntentFull_ScannerInSingleflightKey verifies that the
// singleflight key is sensitive to the effective scanner so that a concurrent
// intent=full call and an intent=health call are NOT merged into the same
// in-flight scan (different intents → different DB cache keys → different results).
func TestScanLocal_IntentFull_ScannerInSingleflightKey(t *testing.T) {
	var calls atomic.Int32
	gate := make(chan struct{})

	sp := &fakeScanProtoFunc{
		scanFn: func(_ context.Context, _ string, _ engine.ScanOpts) (*engine.ScanResult, error) {
			calls.Add(1)
			<-gate
			return &engine.ScanResult{Report: &arch.ContextReport{}}, nil
		},
		driftFn: func(_ context.Context, _ string, _ *arch.ContextReport) string { return "" },
	}
	h := newTestHandler(sp)

	done := make(chan struct{}, 2)

	// Two concurrent calls: same path, same intent=full.
	// They MUST be deduplicated — only 1 scan invocation.
	for range 2 {
		go func() {
			_, _, _ = h.handleScanProject(context.Background(), &codographActionInput{
				Path: "/workspace/proj", Intent: "full",
			})
			done <- struct{}{}
		}()
	}

	// A third concurrent call: same path, intent=health (different effective scanner).
	// It must NOT be merged with the intent=full pair.
	go func() {
		_, _, _ = h.handleScanProject(context.Background(), &codographActionInput{
			Path: "/workspace/proj", Intent: "health",
		})
		done <- struct{}{}
	}()

	// Give all goroutines time to block inside scanGroup.Do before unblocking.
	time.Sleep(20 * time.Millisecond)
	close(gate)
	for range 3 {
		<-done
	}

	// intent=full pair → 1 scan; intent=health → 1 scan = 2 total.
	if got := calls.Load(); got != 2 {
		t.Errorf("scan called %d time(s), want 2 (one per distinct intent+scanner key)", got)
	}
}
