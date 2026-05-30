// Package mcp — tests for scanner selection and intent routing in scan_local.
//
// Covers three invariants:
//   - intent=full does not override the survey scanner; auto-detection is preserved
//   - an explicit scanner field is forwarded verbatim to the engine
//   - the singleflight key is sensitive to the effective scanner so that
//     calls producing different cache entries are never merged
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

// TestScanLocal_IntentFull_UsesAutoScanner verifies that a scan with intent=full
// and no explicit scanner leaves ScannerOverride empty. AutoScanner then selects
// the correct language scanner (e.g. TypeScriptScanner → correct import edges).
// The LSP pool drives deep analysis independently via CachedDeepFallback.
//
// Given scan_local is called with intent=full and no scanner override
// When the request is forwarded to ScanProject
// Then ScannerOverride is empty (auto-detection preserved)
func TestScanLocal_IntentFull_UsesAutoScanner(t *testing.T) {
	sp := &capturingScanProto{}
	h := newTestHandler(sp)

	_, _, err := h.handleScanProject(context.Background(), nil, &codographActionInput{
		Path:   "/workspace/ts-project",
		Intent: "full",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := sp.last().Scanner
	if got != "" {
		t.Errorf("intent=full with no explicit scanner: ScannerOverride=%q, want \"\" (auto)\n"+
			"forcing lsp for survey breaks import edges; deep analysis uses the pool", got)
	}
}

// TestScanLocal_ExplicitScanner_ForwardedVerbatim verifies that an explicit
// scanner field is forwarded to the engine's ScannerOverride as-is, regardless
// of the intent. An explicit scanner always wins over any auto-selection logic.
//
// Given scan_local is called with an explicit scanner and any intent
// When the request is forwarded to ScanProject
// Then ScannerOverride matches the explicit scanner exactly
func TestScanLocal_ExplicitScanner_ForwardedVerbatim(t *testing.T) {
	cases := []struct {
		scanner string
		intent  string
		want    string
	}{
		{"lsp", "health", "lsp"},
		{"typescript", "full", "typescript"}, // explicit overrides auto for full intent too
		{"go", "architecture", "go"},
		{"ctags", "health", "ctags"},
	}

	for _, tc := range cases {
		t.Run(tc.scanner+"/"+tc.intent, func(t *testing.T) {
			sp := &capturingScanProto{}
			h := newTestHandler(sp)

			_, _, err := h.handleScanProject(context.Background(), nil, &codographActionInput{
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

// TestScanLocal_SubFullIntent_AutoScannerPreserved verifies that no intent below
// "full" causes a scanner override — all produce an empty ScannerOverride.
//
// Given scan_local is called with architecture, coupling, health, or no intent
// When the request is forwarded to ScanProject
// Then ScannerOverride is empty for every sub-full intent
func TestScanLocal_SubFullIntent_AutoScannerPreserved(t *testing.T) {
	for _, intent := range []string{"architecture", "coupling", "health", ""} {
		t.Run("intent="+intent, func(t *testing.T) {
			sp := &capturingScanProto{}
			h := newTestHandler(sp)

			_, _, err := h.handleScanProject(context.Background(), nil, &codographActionInput{
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

// TestScanLocal_Singleflight_KeyIncludesScanner verifies that two concurrent
// scan_local calls that produce different effective scanners are NOT merged by
// the singleflight group. They generate different DB cache entries and must be
// kept separate.
//
// Given 2 goroutines with intent=full and 1 goroutine with intent=health on the same path
// When all calls block on the same gate
// Then ScanProject is called exactly twice (one per distinct intent+scanner key)
func TestScanLocal_Singleflight_KeyIncludesScanner(t *testing.T) {
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

	done := make(chan struct{}, 3)

	// Two concurrent intent=full calls — must be deduplicated to 1 scan.
	for range 2 {
		go func() {
			_, _, _ = h.handleScanProject(context.Background(), nil, &codographActionInput{
				Path: "/workspace/proj", Intent: "full",
			})
			done <- struct{}{}
		}()
	}

	// A third concurrent call with a different intent — must NOT be merged.
	go func() {
		_, _, _ = h.handleScanProject(context.Background(), nil, &codographActionInput{
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
		t.Errorf("ScanProject called %d time(s), want 2 (one per distinct intent+scanner key)", got)
	}
}
