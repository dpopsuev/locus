// Package mcp contains the MCP server implementation for Locus.
// This file tests the concurrent scan_local deduplication contract and the
// independence of analysis tools from the singleflight group.
package mcp

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dpopsuev/locus/internal/store"
	"github.com/dpopsuev/oculus/v3/arch"
	oculuscache "github.com/dpopsuev/oculus/v3/cache"
	"github.com/dpopsuev/oculus/v3/engine"
)

// --- fake scanProto ---

// fakeScanProto is a controllable implementation of scanProto for unit tests.
// Each call to ScanProject increments calls and then blocks until ready is closed.
type fakeScanProto struct {
	calls atomic.Int32
	ready chan struct{} // close to unblock all in-flight ScanProject calls
}

func newFakeScanProto() *fakeScanProto {
	return &fakeScanProto{ready: make(chan struct{})}
}

func (f *fakeScanProto) unblock() { close(f.ready) }

func (f *fakeScanProto) ScanProject(_ context.Context, _ string, _ engine.ScanOpts) (*engine.ScanResult, error) {
	f.calls.Add(1)
	<-f.ready
	return &engine.ScanResult{Report: &arch.ContextReport{}}, nil
}

func (f *fakeScanProto) CheckDriftOnScan(_ context.Context, _ string, _ *arch.ContextReport) string {
	return ""
}

// --- helpers ---

// newTestHandler builds a handler wired to a fake sproto. proto is left nil
// because none of the paths exercised by these tests need it.
func newTestHandler(sp scanProto) *handler {
	return &handler{sproto: sp}
}

// --- Test 1: singleflight dedup ---

// TestScanLocal_ConcurrentCalls_Deduplicated verifies that N simultaneous
// scan_local requests for the same path+intent share one ScanProject execution.
//
// Given N goroutines calling scan_local with identical path and intent
// When all calls arrive before the in-flight scan completes
// Then the underlying ScanProject is invoked exactly once and all callers receive the result
func TestScanLocal_ConcurrentCalls_Deduplicated(t *testing.T) {
	const n = 8
	fake := newFakeScanProto()
	h := newTestHandler(fake)

	var wg sync.WaitGroup
	errs := make(chan error, n)

	// Fire n goroutines before unblocking fake so they all pile into the group.
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := h.handleScanProject(context.Background(), &codographActionInput{
				Path:   "/workspace/proj",
				Intent: "health",
			})
			errs <- err
		}()
	}

	// Give goroutines time to reach singleflight.Do before we unblock.
	time.Sleep(20 * time.Millisecond)
	fake.unblock()
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("unexpected error from handleScanProject: %v", err)
		}
	}

	if got := fake.calls.Load(); got != 1 {
		t.Errorf("ScanProject called %d time(s), want exactly 1 — concurrent deduplication is broken", got)
	}
}

// TestScanLocal_DifferentKeys_NotDeduplicated verifies that calls with distinct
// path+intent+scanner keys are each executed independently.
//
// Given 3 goroutines with different path/intent combinations
// When all calls arrive before any scan completes
// Then ScanProject is invoked once per distinct key
func TestScanLocal_DifferentKeys_NotDeduplicated(t *testing.T) {
	fake := newFakeScanProto()
	h := newTestHandler(fake)

	var wg sync.WaitGroup
	inputs := []codographActionInput{
		{Path: "/workspace/proj", Intent: "health"},
		{Path: "/workspace/proj", Intent: "architecture"}, // same path, different intent
		{Path: "/workspace/other", Intent: "health"},      // different path
	}

	errs := make(chan error, len(inputs))
	for _, in := range inputs {
		wg.Add(1)
		go func(in codographActionInput) {
			defer wg.Done()
			_, _, err := h.handleScanProject(context.Background(), &in)
			errs <- err
		}(in)
	}

	time.Sleep(20 * time.Millisecond)
	fake.unblock()
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	want := int32(len(inputs)) //nolint:gosec // len(inputs) is a small constant, no overflow
	if got := fake.calls.Load(); got != want {
		t.Errorf("ScanProject called %d time(s), want %d (one per unique key)", got, want)
	}
}

// --- Test 2: analysis tools bypass the singleflight group ---

// TestAnalysis_GetCycles_IndependentOfScanGroup documents and proves that
// analysis tool calls (GetCycles, SearchSymbols, etc.) with no cache_key and a
// cold cache execute their own ScanAndBuild, independently of any in-flight
// scan_local singleflight.
//
// Given scan_local is in-flight and blocked inside a gate that never opens
// When GetCycles is called on the same path with a cold cache and no cache_key
// Then GetCycles returns without waiting for the scan_local singleflight
//
// This test will need updating once the engine routes cache-miss analysis
// calls through the same deduplication group as scan_local.
func TestAnalysis_GetCycles_IndependentOfScanGroup(t *testing.T) {
	// fixturePath is a tiny Go project included in the test corpus.
	// The Go scanner requires no external tools, so the test is hermetic.
	fixturePath := "../../testdata/testkit/go"

	// A fake sproto whose ScanProject blocks forever (gate never closed).
	// This simulates a long-running scan on the scan_local path.
	gate := make(chan struct{}) // intentionally never closed in this test
	var scanLocalCalls atomic.Int32
	blocked := &fakeScanProtoFunc{
		scanFn: func(_ context.Context, _ string, _ engine.ScanOpts) (*engine.ScanResult, error) {
			scanLocalCalls.Add(1)
			<-gate // block indefinitely
			return nil, nil
		},
		driftFn: func(_ context.Context, _ string, _ *arch.ContextReport) string { return "" },
	}

	// Real engine backed by a cold temp store.
	sc := oculuscache.New(t.TempDir())
	db := store.NewFilesystem(sc, t.TempDir())
	proto := engine.New(db, nil)
	h := &handler{proto: proto, sproto: blocked}

	// Start scan_local — it enters the singleflight and blocks in scanFn.
	scanLocalDone := make(chan error, 1)
	go func() {
		in := &codographActionInput{Path: fixturePath, Intent: "health"}
		_, _, err := h.handleScanProject(context.Background(), in)
		scanLocalDone <- err
	}()

	// Wait until scan_local has entered the singleflight.
	for scanLocalCalls.Load() == 0 {
		time.Sleep(5 * time.Millisecond)
	}

	// GetCycles with no cache_key on a cold cache. If it were gated by
	// scanGroup it would block forever and this test would time out.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, cyclesErr := proto.GetCycles(ctx, fixturePath, nil)

	// The call must return — success or scan error are both acceptable.
	// The only unacceptable outcome is blocking until the test times out.
	t.Logf("GetCycles returned (err=%v) — did not block on in-flight scan_local", cyclesErr)

	// Sanity: scan_local is still blocked (gate was never closed).
	select {
	case err := <-scanLocalDone:
		t.Fatalf("scan_local should still be blocked, but it returned (err=%v)", err)
	default:
		// expected: still in-flight
	}
}

// fakeScanProtoFunc is a function-based scanProto for one-off cases.
type fakeScanProtoFunc struct {
	scanFn  func(ctx context.Context, path string, opts engine.ScanOpts) (*engine.ScanResult, error)
	driftFn func(ctx context.Context, path string, report *arch.ContextReport) string
}

func (f *fakeScanProtoFunc) ScanProject(ctx context.Context, path string, opts engine.ScanOpts) (*engine.ScanResult, error) {
	return f.scanFn(ctx, path, opts)
}

func (f *fakeScanProtoFunc) CheckDriftOnScan(ctx context.Context, path string, report *arch.ContextReport) string {
	return f.driftFn(ctx, path, report)
}
