package mcp

import (
	"context"
	"testing"
)

func TestApplyStickyPathAndCacheKey(t *testing.T) {
	t.Parallel()
	h := &handler{}
	h.lastCacheKey.Store("/tmp/demo-repo@deadbeef-full")

	// No default project → sticky alone.
	in := analysisInput{}
	h.applyStickyPathAndCacheKey(context.Background(), &in)
	if in.CacheKey != "/tmp/demo-repo@deadbeef-full" {
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
	h.lastCacheKey.Store("/tmp/demo-repo@deadbeef-full")

	in := diagramInput{Type: "dependency"}
	h.fillStickyPathAndCacheKey(context.Background(), &in.Path, &in.CacheKey)
	if in.CacheKey != "/tmp/demo-repo@deadbeef-full" {
		t.Fatalf("CacheKey=%q", in.CacheKey)
	}
	if in.Path != "/tmp/demo-repo" {
		t.Fatalf("Path=%q", in.Path)
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
