package store_test

import (
	"context"
	"testing"

	"github.com/dpopsuev/locus/internal/store"
	"github.com/dpopsuev/oculus/v3/arch"
	"github.com/dpopsuev/oculus/v3/cache"
	"github.com/dpopsuev/oculus/v3/port"
)

func newFS(t *testing.T) *store.FilesystemStore {
	t.Helper()
	return store.NewFilesystem(cache.New(t.TempDir()), t.TempDir())
}

func newReport(modulePath string) *arch.ContextReport {
	return &arch.ContextReport{ScanCore: arch.ScanCore{ModulePath: modulePath, Scanner: "go"}}
}

// --- FilesystemStore: report round-trip ---

func TestFilesystem_PutGet_RoundTrip(t *testing.T) {
	fs := newFS(t)
	ctx := context.Background()
	r := newReport("example.com/mymod")

	if err := fs.PutReport(ctx, "/repo", "abc123", r); err != nil {
		t.Fatalf("PutReport: %v", err)
	}

	got, hit, err := fs.GetReport(ctx, "/repo", "abc123")
	if err != nil || !hit {
		t.Fatalf("GetReport: hit=%v err=%v", hit, err)
	}
	if got.ModulePath != "example.com/mymod" {
		t.Errorf("ModulePath = %q, want example.com/mymod", got.ModulePath)
	}
}

func TestFilesystem_Get_Miss(t *testing.T) {
	fs := newFS(t)
	_, hit, err := fs.GetReport(context.Background(), "/repo", "nosuchsha")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hit {
		t.Error("expected miss for unknown sha")
	}
}

func TestFilesystem_Invalidate_RemovesReport(t *testing.T) {
	fs := newFS(t)
	ctx := context.Background()

	_ = fs.PutReport(ctx, "/repo", "sha1", newReport("mod"))
	if err := fs.Invalidate(ctx, "/repo"); err != nil {
		t.Fatalf("Invalidate: %v", err)
	}

	_, hit, _ := fs.GetReport(ctx, "/repo", "sha1")
	if hit {
		t.Error("report should be gone after Invalidate")
	}
}

func TestFilesystem_DifferentSHAs_Independent(t *testing.T) {
	fs := newFS(t)
	ctx := context.Background()

	_ = fs.PutReport(ctx, "/repo", "sha-a", newReport("mod-a"))
	_ = fs.PutReport(ctx, "/repo", "sha-b", newReport("mod-b"))

	a, hitA, _ := fs.GetReport(ctx, "/repo", "sha-a")
	b, hitB, _ := fs.GetReport(ctx, "/repo", "sha-b")

	if !hitA || a.ModulePath != "mod-a" {
		t.Errorf("sha-a: hit=%v mod=%q", hitA, a.ModulePath)
	}
	if !hitB || b.ModulePath != "mod-b" {
		t.Errorf("sha-b: hit=%v mod=%q", hitB, b.ModulePath)
	}
}

// --- FilesystemStore: desired state ---

func TestFilesystem_DesiredState_RoundTrip(t *testing.T) {
	fs := newFS(t)
	ctx := context.Background()

	ds := &port.DesiredState{Layers: []string{"domain", "adapter", "infra"}}
	if err := fs.PutDesiredState(ctx, "/repo", ds); err != nil {
		t.Fatalf("PutDesiredState: %v", err)
	}

	got, err := fs.GetDesiredState(ctx, "/repo")
	if err != nil {
		t.Fatalf("GetDesiredState: %v", err)
	}
	if len(got.Layers) != 3 || got.Layers[1] != "adapter" {
		t.Errorf("layers = %v", got.Layers)
	}
}

func TestFilesystem_GetDesiredState_Missing(t *testing.T) {
	fs := newFS(t)
	got, err := fs.GetDesiredState(context.Background(), "/no-such-repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for missing desired state, got %+v", got)
	}
}

// --- FilesystemStore: /tmp project filter ---

// TestFilesystem_TmpProject_NotRegistered verifies that a scan of a path under
// os.TempDir() is never written to projects.json.
//
// Given a PutReport call for a path under /tmp
// When ListProjects is called
// Then the /tmp entry is absent from the result
func TestFilesystem_TmpProject_NotRegistered(t *testing.T) {
	fs := newFS(t)
	ctx := context.Background()

	tmpPath := t.TempDir() // always under os.TempDir()
	if err := fs.PutReport(ctx, tmpPath, "sha1", newReport("tmp-mod")); err != nil {
		t.Fatalf("PutReport: %v", err)
	}

	projects, err := fs.ListProjects(ctx)
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	for _, p := range projects {
		if p.Path == tmpPath {
			t.Errorf("tmp project %q should not appear in ListProjects", tmpPath)
		}
	}
}

// TestFilesystem_TmpProject_FilteredOnRead verifies that stale /tmp entries
// already persisted in projects.json are filtered out on read.
//
// Given projects.json already contains a /tmp entry (legacy pollution)
// When ListProjects is called
// Then the /tmp entry is excluded from the returned slice
func TestFilesystem_TmpProject_FilteredOnRead(t *testing.T) {
	fs := newFS(t)
	ctx := context.Background()

	// Register a real project first so we can check the file exists.
	_ = fs.PutReport(ctx, "/real/project", "sha1", newReport("real-mod"))

	// Directly inject a stale /tmp entry by calling UpsertProject.
	tmpPath := t.TempDir()
	_ = fs.UpsertProject(ctx, store.ProjectInfo{Path: tmpPath, Name: "stale-tmp"})

	projects, err := fs.ListProjects(ctx)
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}

	foundReal := false
	for _, p := range projects {
		if p.Path == tmpPath {
			t.Errorf("stale tmp entry %q should be filtered on read", tmpPath)
		}
		if p.Path == "/real/project" {
			foundReal = true
		}
	}
	if !foundReal {
		t.Error("real project should still appear after /tmp filtering")
	}
}

// --- LRUStore: snapshot and invalidate ---

func TestLRU_Snapshot_ReflectsWarmEntries(t *testing.T) {
	inner := store.NewFilesystem(cache.New(t.TempDir()), t.TempDir())
	lru := store.NewLRU(inner, 8)
	ctx := context.Background()

	_ = lru.PutReport(ctx, "/r1", "s1", newReport("mod1"))
	_ = lru.PutReport(ctx, "/r2", "s2", newReport("mod2"))

	snap := lru.Snapshot()
	if len(snap) != 2 {
		t.Fatalf("snapshot len = %d, want 2", len(snap))
	}
	// Most-recently-used is first.
	if snap[0].SHA != "s2" {
		t.Errorf("MRU entry SHA = %q, want s2", snap[0].SHA)
	}
}

func TestLRU_Invalidate_EvictsFromCache(t *testing.T) {
	inner := store.NewFilesystem(cache.New(t.TempDir()), t.TempDir())
	lru := store.NewLRU(inner, 8)
	ctx := context.Background()

	_ = lru.PutReport(ctx, "/repo", "sha1", newReport("mod"))

	if err := lru.Invalidate(ctx, "/repo"); err != nil {
		t.Fatalf("Invalidate: %v", err)
	}

	// Snapshot must be empty — entry evicted from LRU.
	if snap := lru.Snapshot(); len(snap) != 0 {
		t.Errorf("snapshot after invalidate: %d entries, want 0", len(snap))
	}

	// On-disk entry also gone (delegated to FilesystemStore.Invalidate).
	_, hit, _ := lru.GetReport(ctx, "/repo", "sha1")
	if hit {
		t.Error("report should be gone from disk after Invalidate")
	}
}

func TestLRU_MRU_Promoted_On_Get(t *testing.T) {
	inner := store.NewFilesystem(cache.New(t.TempDir()), t.TempDir())
	lru := store.NewLRU(inner, 2)
	ctx := context.Background()

	_ = lru.PutReport(ctx, "/r1", "s1", newReport("mod1"))
	_ = lru.PutReport(ctx, "/r2", "s2", newReport("mod2"))

	// Promote r1 to MRU by accessing it.
	_, _, _ = lru.GetReport(ctx, "/r1", "s1")

	// Now add r3 — capacity 2 means r2 (LRU) is evicted, not r1.
	_ = lru.PutReport(ctx, "/r3", "s3", newReport("mod3"))

	snap := lru.Snapshot()
	for _, e := range snap {
		if e.SHA == "s2" {
			t.Error("s2 should have been evicted (it was LRU), but it's still in cache")
		}
	}

	// r1 must still be in the LRU (it was promoted).
	found := false
	for _, e := range snap {
		if e.SHA == "s1" {
			found = true
		}
	}
	if !found {
		t.Error("s1 should still be in LRU after MRU promotion, but it was evicted")
	}
}
