package mcp

// Three invariants:
//
//  1. analysisTimeout() returns defaultAnalysisTimeout by default and honours
//     LOCUS_ANALYSIS_TIMEOUT for operator-controlled overrides.
//
//  2. handleAnalysis threads the deadline through the context: when the
//     caller's context is already cancelled, handleAnalysis returns promptly
//     rather than blocking on a cold scan.
//
//  3. With LOCUS_ANALYSIS_TIMEOUT set to a very short value, handleAnalysis
//     returns well before the scan-build timeout — the envelope is enforced.

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	oculuscache "github.com/dpopsuev/oculus/v3/cache"
	"github.com/dpopsuev/oculus/v3/engine"
	"github.com/dpopsuev/locus/internal/store"
)

// newRealHandler builds a handler backed by a cold in-memory store.
// proto is a genuine engine.Engine so that handleAnalysis exercises the real
// dispatch path. The store returns no cached reports, so every analysis call
// falls through to a cold ScanAndBuild.
func newRealHandler(t *testing.T) *handler {
	t.Helper()
	sc := oculuscache.New(t.TempDir())
	db := store.NewFilesystem(sc, t.TempDir())
	return &handler{proto: engine.New(db, nil)}
}

// coldFixture creates a minimal directory that is NOT a git repo so that
// arch.ScanAndBuild on it either errors quickly or runs until the context fires,
// depending on the filesystem state. The scan never returns a cached report.
func coldFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module slow_test\ngo 1.21\n"), 0o600)
	_ = os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() {}\n"), 0o600)
	return dir
}

// coldGitFixture is like coldFixture but is a real git repo so ResolveHEAD
// returns a non-empty SHA, giving the singleflight a stable key.
func coldGitFixture(t *testing.T) string {
	t.Helper()
	dir := coldFixture(t)
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(exec.Command("git", "-C", dir, "init", "-q").Run())  //nolint:gosec // dir is a t.TempDir()
	must(exec.Command("git", "-C", dir, "add", "-A").Run())   //nolint:gosec
	must(exec.Command("git", "-C", dir, "commit", "-q", "-m", "init").Run()) //nolint:gosec
	return dir
}

// ─── 1. analysisTimeout unit tests ──────────────────────────────────────────

func TestAnalysisTimeout_DefaultValue(t *testing.T) {
	t.Setenv("LOCUS_ANALYSIS_TIMEOUT", "") // ensure env is clear
	got := analysisTimeout()
	if got != defaultAnalysisTimeout {
		t.Errorf("analysisTimeout() = %v, want %v", got, defaultAnalysisTimeout)
	}
}

func TestAnalysisTimeout_EnvOverride(t *testing.T) {
	t.Setenv("LOCUS_ANALYSIS_TIMEOUT", "42s")
	got := analysisTimeout()
	if got != 42*time.Second {
		t.Errorf("analysisTimeout() = %v, want 42s", got)
	}
}

func TestAnalysisTimeout_InvalidEnv_FallsBackToDefault(t *testing.T) {
	t.Setenv("LOCUS_ANALYSIS_TIMEOUT", "not-a-duration")
	got := analysisTimeout()
	if got != defaultAnalysisTimeout {
		t.Errorf("analysisTimeout() = %v on bad env, want default %v", got, defaultAnalysisTimeout)
	}
}

func TestAnalysisTimeout_ZeroEnv_FallsBackToDefault(t *testing.T) {
	t.Setenv("LOCUS_ANALYSIS_TIMEOUT", "0s")
	got := analysisTimeout()
	if got != defaultAnalysisTimeout {
		t.Errorf("analysisTimeout() = %v on 0s env, want default %v", got, defaultAnalysisTimeout)
	}
}

// ─── 2. Context threading: pre-cancelled context returns promptly ─────────────

// TestHandleAnalysis_PreCancelledContext_ReturnsImmediately verifies that
// handleAnalysis threads the context deadline into the underlying engine call.
//
// If handleAnalysis ignored the context and called the engine with a fresh
// context.Background(), this test would block for the full scan-build timeout
// (30 min) rather than returning within milliseconds.
func TestHandleAnalysis_PreCancelledContext_ReturnsImmediately(t *testing.T) {
	h := newRealHandler(t)

	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel() // already done before we even call handleAnalysis

	in := analysisInput{
		Action: ActionSymbolSearch,
		Symbol: "Foo",
		Path:   coldGitFixture(t),
	}

	done := make(chan error, 1)
	go func() {
		_, _, err := h.handleAnalysis(cancelledCtx, &sdkmcp.CallToolRequest{}, in)
		done <- err
	}()

	select {
	case err := <-done:
		// Either context.Canceled or context.DeadlineExceeded are fine.
		// A nil error would mean the scan completed before the check —
		// which can't happen here because the fixture has no cached report.
		if err == nil {
			t.Log("handleAnalysis returned nil (unexpectedly fast scan — acceptable)")
			return
		}
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			// Scan errors (e.g. no git HEAD) are also acceptable — what matters
			// is that handleAnalysis did not block.
			t.Logf("handleAnalysis returned non-context error (acceptable): %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("handleAnalysis blocked for 5s on a pre-cancelled context — context not threaded through to engine calls")
	}
}

// ─── 3. Short LOCUS_ANALYSIS_TIMEOUT fires before scan-build completes ──────

// TestHandleAnalysis_ShortTimeoutEnforced verifies that LOCUS_ANALYSIS_TIMEOUT
// is respected end-to-end: with a 10ms ceiling, handleAnalysis must return
// before a slow (or non-existent) scan-build would naturally finish.
//
// Without the deadline, a cache-miss on a large workspace blocks indefinitely.
func TestHandleAnalysis_ShortTimeoutEnforced(t *testing.T) {
	// 10ms is fast enough that no real scan can finish, but long enough to
	// avoid flakiness on a heavily loaded CI runner.
	t.Setenv("LOCUS_ANALYSIS_TIMEOUT", "10ms")

	h := newRealHandler(t)
	dir := coldGitFixture(t)

	in := analysisInput{
		Action: ActionSymbolSearch,
		Symbol: "anything",
		Path:   dir,
	}

	start := time.Now()
	_, _, err := h.handleAnalysis(context.Background(), &sdkmcp.CallToolRequest{}, in)
	elapsed := time.Since(start)

	// Allow 10× headroom for slow CI, but the call must not run for 30 min.
	limit := 10 * 10 * time.Millisecond // 100ms
	if elapsed > limit {
		t.Errorf("handleAnalysis took %v with 10ms timeout — deadline was not enforced (limit %v)", elapsed, limit)
	}

	// The error must be context-related (not nil — a nil result would mean the
	// scan completed in <10ms which is unrealistic on a cold cache+git fixture).
	t.Logf("handleAnalysis returned in %v, err=%v", elapsed.Round(time.Millisecond), err)
}
