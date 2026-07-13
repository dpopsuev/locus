package mcp

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dpopsuev/locus/internal/store"
	oculuscache "github.com/dpopsuev/oculus/v3/cache"
	"github.com/dpopsuev/oculus/v3/engine"
	"github.com/dpopsuev/oculus/v3/lsp"
)

func TestCollectWarmPaths_CapAndDedupe(t *testing.T) {
	a := t.TempDir()
	b := t.TempDir()
	c := t.TempDir()
	missing := filepath.Join(t.TempDir(), "nope")

	got := CollectWarmPaths(
		[]string{a, a, missing},
		[]store.ProjectInfo{
			{Path: b, LastScan: time.Now().Add(-time.Hour)},
			{Path: c, LastScan: time.Now()},
			{Path: a, LastScan: time.Now().Add(-2 * time.Hour)},
		},
		2,
	)
	if len(got) != 2 {
		t.Fatalf("got %d paths, want 2: %v", len(got), got)
	}
	if filepath.Clean(got[0]) != filepath.Clean(a) {
		t.Fatalf("first should be workspace %q, got %q", a, got[0])
	}
	if filepath.Clean(got[1]) != filepath.Clean(c) {
		t.Fatalf("second should be newest project %q, got %q", c, got[1])
	}
}

func TestWarmRecentProjects_InvokesWarm(t *testing.T) {
	dir := t.TempDir()
	sc := oculuscache.New(t.TempDir())
	db := store.NewFilesystem(sc, t.TempDir())
	pool := lsp.NewPoolWithConfig(lsp.PoolConfig{MaxActive: 1})
	t.Cleanup(func() { _ = pool.Shutdown(context.Background()) })
	eng := engine.New(db, []string{dir}, pool)

	var called atomic.Int32
	var gotPath atomic.Value
	WarmRecentProjects(eng, []string{dir}, func(_ context.Context, path string) error {
		called.Add(1)
		gotPath.Store(path)
		return nil
	})
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if called.Load() >= 1 {
			if p, _ := gotPath.Load().(string); filepath.Clean(p) != filepath.Clean(dir) {
				t.Fatalf("warm path=%q want %q", p, dir)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("warmFn not called")
}

func TestWarmRecentProjects_NilPoolNoOp(t *testing.T) {
	sc := oculuscache.New(t.TempDir())
	db := store.NewFilesystem(sc, t.TempDir())
	eng := engine.New(db, []string{t.TempDir()})
	var called atomic.Bool
	WarmRecentProjects(eng, []string{t.TempDir()}, func(context.Context, string) error {
		called.Store(true)
		return nil
	})
	time.Sleep(50 * time.Millisecond)
	if called.Load() {
		t.Fatal("must not warm without pool")
	}
}
