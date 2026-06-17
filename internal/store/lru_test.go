package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/dpopsuev/locus/internal/store"
	"github.com/dpopsuev/oculus/v3/arch"
	"github.com/dpopsuev/oculus/v3/cache"
)

const testModulePath = "test/lru"

func TestLRU_HitAvoidsDisk(t *testing.T) {
	inner := store.NewFilesystem(cache.New(t.TempDir()), t.TempDir())
	lru := store.NewLRU(inner, 4)

	ctx := context.Background()
	report := &arch.ContextReport{ScanCore: arch.ScanCore{ModulePath: "test/repo", Scanner: "go"}}

	_ = lru.PutReport(ctx, "/repo", "sha1", report)

	// First read: populates LRU from disk.
	got, hit, err := lru.GetReport(ctx, "/repo", "sha1")
	if err != nil || !hit {
		t.Fatalf("first get: hit=%v err=%v", hit, err)
	}
	if got.ModulePath != "test/repo" {
		t.Errorf("module path = %q", got.ModulePath)
	}

	// Second read: should be <1ms (from LRU, no disk I/O).
	start := time.Now()
	for i := 0; i < 1000; i++ {
		_, _, _ = lru.GetReport(ctx, "/repo", "sha1")
	}
	elapsed := time.Since(start)
	if elapsed > time.Millisecond*100 {
		t.Errorf("1000 LRU reads took %v, expected <100ms", elapsed)
	}
	t.Logf("1000 LRU reads: %v (%.2f µs/read)", elapsed, float64(elapsed.Microseconds())/1000)
}

func TestLRU_EvictionAtCapacity(t *testing.T) {
	inner := store.NewFilesystem(cache.New(t.TempDir()), t.TempDir())
	lru := store.NewLRU(inner, 2) // capacity 2

	ctx := context.Background()
	r1 := &arch.ContextReport{ScanCore: arch.ScanCore{ModulePath: "repo1"}}
	r2 := &arch.ContextReport{ScanCore: arch.ScanCore{ModulePath: "repo2"}}
	r3 := &arch.ContextReport{ScanCore: arch.ScanCore{ModulePath: "repo3"}}

	_ = lru.PutReport(ctx, "/r1", "s1", r1)
	_ = lru.PutReport(ctx, "/r2", "s2", r2)
	_ = lru.PutReport(ctx, "/r3", "s3", r3) // evicts r1

	// r1 should still be on disk but evicted from LRU.
	// r3 and r2 should be in LRU.
	got, hit, _ := lru.GetReport(ctx, "/r1", "s1")
	if !hit || got.ModulePath != "repo1" {
		t.Errorf("r1 should still be on disk: hit=%v", hit)
	}

	// r3 should be in LRU (fast).
	got, hit, _ = lru.GetReport(ctx, "/r3", "s3")
	if !hit || got.ModulePath != "repo3" {
		t.Errorf("r3 should be in LRU: hit=%v", hit)
	}
}

func TestLRU_MissReturnsFalse(t *testing.T) {
	inner := store.NewFilesystem(cache.New(t.TempDir()), t.TempDir())
	lru := store.NewLRU(inner, 4)

	_, hit, err := lru.GetReport(context.Background(), "/nonexistent", "sha1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hit {
		t.Error("expected miss for nonexistent key")
	}
}

func TestLRU_LenAndCapacity(t *testing.T) {
	inner := store.NewFilesystem(cache.New(t.TempDir()), t.TempDir())
	lru := store.NewLRU(inner, 4)

	if lru.Capacity() != 4 {
		t.Errorf("Capacity() = %d, want 4", lru.Capacity())
	}
	if lru.Len() != 0 {
		t.Errorf("Len() = %d, want 0", lru.Len())
	}

	ctx := context.Background()
	r := &arch.ContextReport{ScanCore: arch.ScanCore{ModulePath: testModulePath}}
	_ = lru.PutReport(ctx, "/r1", "s1", r)
	_ = lru.PutReport(ctx, "/r2", "s2", r)

	if lru.Len() != 2 {
		t.Errorf("Len() = %d, want 2", lru.Len())
	}
}

func TestLRU_EvictOldest(t *testing.T) {
	inner := store.NewFilesystem(cache.New(t.TempDir()), t.TempDir())
	lru := store.NewLRU(inner, 4)

	ctx := context.Background()
	r := &arch.ContextReport{ScanCore: arch.ScanCore{ModulePath: testModulePath}}
	_ = lru.PutReport(ctx, "/r1", "s1", r)
	_ = lru.PutReport(ctx, "/r2", "s2", r)
	_ = lru.PutReport(ctx, "/r3", "s3", r)

	evicted := lru.EvictOldest(2)
	if evicted != 2 {
		t.Errorf("EvictOldest(2) = %d, want 2", evicted)
	}
	if lru.Len() != 1 {
		t.Errorf("Len() = %d, want 1", lru.Len())
	}
}

func TestLRU_EvictOldest_MoreThanAvailable(t *testing.T) {
	inner := store.NewFilesystem(cache.New(t.TempDir()), t.TempDir())
	lru := store.NewLRU(inner, 4)

	ctx := context.Background()
	r := &arch.ContextReport{ScanCore: arch.ScanCore{ModulePath: testModulePath}}
	_ = lru.PutReport(ctx, "/r1", "s1", r)

	evicted := lru.EvictOldest(5)
	if evicted != 1 {
		t.Errorf("EvictOldest(5) = %d, want 1", evicted)
	}
	if lru.Len() != 0 {
		t.Errorf("Len() = %d, want 0", lru.Len())
	}
}
