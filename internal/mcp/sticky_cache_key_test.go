package mcp

import (
	"context"
	"testing"
)

func TestApplyStickyPathAndCacheKey(t *testing.T) {
	t.Parallel()
	h := &handler{}
	h.rememberSticky("/tmp/demo-repo@deadbeef-full-sc:auto")

	// No default project → sticky alone.
	in := analysisInput{}
	h.applyStickyPathAndCacheKey(context.Background(), &in)
	if in.CacheKey != "/tmp/demo-repo@deadbeef-full-sc:auto" {
		t.Fatalf("CacheKey=%q", in.CacheKey)
	}
	if in.Path != "/tmp/demo-repo" {
		t.Fatalf("Path=%q", in.Path)
	}

	// Explicit fields win — sticky must not override.
	in2 := analysisInput{Path: "/other", CacheKey: "other@sha"}
	h.applyStickyPathAndCacheKey(context.Background(), &in2)
	if in2.Path != "/other" || in2.CacheKey != "other@sha" {
		t.Fatalf("sticky overrode explicit: %+v", in2)
	}
}

func TestFillStickyPathAndCacheKey_Diagram(t *testing.T) {
	t.Parallel()
	h := &handler{}
	h.rememberSticky("/tmp/demo-repo@deadbeef-full-sc:auto")

	in := diagramInput{Type: "dependency"}
	h.fillStickyPathAndCacheKey(context.Background(), &in.Path, &in.CacheKey)
	if in.CacheKey != "/tmp/demo-repo@deadbeef-full-sc:auto" {
		t.Fatalf("CacheKey=%q", in.CacheKey)
	}
	if in.Path != "/tmp/demo-repo" {
		t.Fatalf("Path=%q", in.Path)
	}
}

func TestStickyMap_PerPathIsolation(t *testing.T) {
	t.Parallel()
	h := &handler{}
	h.rememberSticky("/tmp/repo-a@aaa-full-sc:auto")
	h.rememberSticky("/tmp/repo-b@bbb-full-sc:auto")

	if got := h.stickyForPath("/tmp/repo-a"); got != "/tmp/repo-a@aaa-full-sc:auto" {
		t.Fatalf("stickyForPath(A)=%q, want A's key (not B's)", got)
	}
	if got := h.stickyForPath("/tmp/repo-b"); got != "/tmp/repo-b@bbb-full-sc:auto" {
		t.Fatalf("stickyForPath(B)=%q", got)
	}

	// Simulate CWD under A: resolveDefaultProject returns A → bind A's key.
	// We exercise fillSticky with an injected resolved path by setting Path
	// empty and stubbing via stickyForPath lookup path (same as fill when
	// resolveDefaultProject returns the path). Direct check:
	ck := h.stickyForPath("/tmp/repo-a")
	if ck == "" || pathFromCacheKey(ck) != "/tmp/repo-a" {
		t.Fatalf("expected A sticky, got %q", ck)
	}
	if ck == h.stickyForPath("/tmp/repo-b") {
		t.Fatal("A and B must not share sticky keys")
	}
}

func TestSameProjectPath(t *testing.T) {
	t.Parallel()
	if !sameProjectPath("/tmp/a", "/tmp/a/") {
		t.Fatal("expected same")
	}
	if sameProjectPath("/tmp/a", "/tmp/b") {
		t.Fatal("expected different")
	}
}
