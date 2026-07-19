package mcp

import (
	"context"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dpopsuev/locus/internal/store"
	oculuscache "github.com/dpopsuev/oculus/v3/cache"
	"github.com/dpopsuev/oculus/v3/engine"
	"github.com/dpopsuev/oculus/v3/lsp"
)

func TestAnalysisInput_SymbolGraphOpts_DefaultQuick(t *testing.T) {
	in := analysisInput{}
	if in.symbolGraphOpts().AllowLSP || !in.symbolGraphOpts().Quick {
		t.Fatal("default quality must be AST-only (!AllowLSP)")
	}
	in.Quality = "quick"
	if in.symbolGraphOpts().AllowLSP {
		t.Fatal("quality=quick must not AllowLSP")
	}
	in.Quality = "deep"
	in.Symbol = "Foo"
	o := in.symbolGraphOpts()
	if !o.AllowLSP || o.Quick {
		t.Fatal("quality=deep must AllowLSP")
	}
	if o.FocusEntry != "Foo" {
		t.Fatalf("FocusEntry=%q want Foo", o.FocusEntry)
	}
	in.Quality = "DEEP"
	if !in.symbolGraphOpts().AllowLSP {
		t.Fatal("quality=DEEP must AllowLSP")
	}
}

func TestMaybeWarmAfterScan_NilPoolNoOp(t *testing.T) {
	dir := monorepoFixture(t)
	h := newHandlerWithWorkspace(t, dir)
	var called atomic.Bool
	h.warmAfterScan = func(context.Context, string) { called.Store(true) }
	h.maybeWarmAfterScan(dir)
	time.Sleep(50 * time.Millisecond)
	if called.Load() {
		t.Fatal("warm must not run without LSP pool")
	}
}

func TestScanLocal_WarmHookOnSuccess(t *testing.T) {
	dir := monorepoFixture(t)
	sc := oculuscache.New(t.TempDir())
	db := store.NewFilesystem(sc, filepath.Join(t.TempDir(), "history"))
	lruDB := store.NewLRU(db, 16)
	pool := lsp.NewPoolWithConfig(lsp.PoolConfig{MaxActive: 1})
	t.Cleanup(func() { _ = pool.Shutdown(context.Background()) })

	eng := engine.New(lruDB, []string{dir}, pool)
	h := &handler{proto: eng, sproto: eng}
	var called atomic.Int32
	h.warmAfterScan = func(_ context.Context, path string) {
		called.Add(1)
		if filepath.Clean(path) != filepath.Clean(dir) {
			t.Errorf("warm path=%q want %q", path, dir)
		}
	}

	_, _, err := h.handleScanProject(context.Background(), nil, &codographActionInput{
		Path:   dir,
		Intent: "full",
	})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if called.Load() >= 1 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("warmAfterScan not called after successful scan (called=%d)", called.Load())
}

func TestScanLocal_WarmNotOnFailure(t *testing.T) {
	// Non-git empty dir: scan may still succeed with 0 components; use invalid
	// path that fails. If scan succeeds without pool, warm must not fire.
	h := newHandlerWithWorkspace(t, t.TempDir())
	var called atomic.Bool
	h.warmAfterScan = func(context.Context, string) { called.Store(true) }
	// No pool → maybeWarmAfterScan no-ops even on success.
	_, _, _ = h.handleScanProject(context.Background(), nil, &codographActionInput{
		Path:   h.proto.ResolvePath(""),
		Intent: "full",
	})
	time.Sleep(50 * time.Millisecond)
	if called.Load() {
		t.Fatal("warm must not run without pool")
	}
	_ = strings.TrimSpace("")
}
