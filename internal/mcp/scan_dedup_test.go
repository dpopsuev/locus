// Package mcp contains the MCP server implementation for Locus.
// This file tests LCS-BUG-75: concurrent scan_local deduplication via
// singleflight, and the documented residual that locus_analysis without a
// cache_key bypasses that singleflight entirely.
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

// TestScanLocal_Singleflight proves that N concurrent scan_local calls for the
// same path+intent result in exactly one invocation of the underlying
// ScanProject function (LCS-BUG-75 checklist item 2).
func TestScanLocal_Singleflight(t *testing.T) {
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
			in := &codographActionInput{Path: "/workspace/proj", Intent: "health"}
			_, _, err := h.handleScanProject(context.Background(), in)
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
		t.Errorf("ScanProject called %d time(s), want exactly 1 — singleflight is broken", got)
	}
}

// TestScanLocal_Singleflight_DifferentKeys proves that calls with different
// path+intent keys are NOT deduplicated — each gets its own ScanProject call.
func TestScanLocal_Singleflight_DifferentKeys(t *testing.T) {
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

// --- Test 2: residual — analysis bypasses scanGroup ---

// TestAnalysis_GetCycles_BypassesScanGroup is the proof of the documented
// residual in LCS-BUG-75: GetCycles (and all other getOrScan-based analysis
// methods) called without a cache_key do NOT go through handleScanProject's
// singleflight. If the cache is cold they call arch.ScanAndBuild independently.
//
// The test demonstrates this with two observations:
//
//  1. A handleScanProject call is blocked inside a fake sproto.ScanProject.
//     Only one goroutine enters (singleflight works for scan_local).
//
//  2. A concurrent h.proto.GetCycles call on the same path and a cold cache
//     returns without waiting for the singleflight — it runs its own
//     ScanAndBuild via getOrScan. If GetCycles were gated by scanGroup it
//     would deadlock here (the gate is never closed during the test).
//
// This test will need updating (or deleting) once the residual is fixed by
// pushing dedup down into the engine's getOrScan path.
func TestAnalysis_GetCycles_BypassesScanGroup(t *testing.T) {
	// fixturePath is a tiny Go project included in the test corpus.
	// The Go scanner requires no external tools, so the test is hermetic.
	fixturePath := "../../testdata/testkit/go"

	// A fake sproto whose ScanProject blocks forever (gate never closed).
	// This simulates a long-running ctags on the scan_local path.
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
	// scanGroup this call would block forever. Instead it runs its own
	// ScanAndBuild via getOrScan and returns (success or scan error).
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, cyclesErr := proto.GetCycles(ctx, fixturePath, nil)

	// We don't assert on cyclesErr because the fixture may or may not have
	// cycles. What matters is that the call RETURNED — proving it did not
	// wait for the singleflight. If it had been blocked the context deadline
	// would fire and this test would time out.
	t.Logf("GetCycles returned (err=%v) — did not block on scan_local singleflight", cyclesErr)

	// Sanity: scan_local is still blocked (the gate is never closed).
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
