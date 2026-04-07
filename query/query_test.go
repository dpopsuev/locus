package query_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/dpopsuev/locus/internal/arch"
	"github.com/dpopsuev/locus/internal/cache"
	"github.com/dpopsuev/locus/internal/store"
	"github.com/dpopsuev/locus/query"
)

func locusRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// file = .../locus/query/query_test.go
	return filepath.Dir(filepath.Dir(file))
}

func newClient(t *testing.T) *query.Client {
	t.Helper()
	dir := t.TempDir()
	sc := cache.New(dir)
	fs := store.NewFilesystem(sc, filepath.Join(dir, "history"))
	root := locusRoot(t)
	return query.New(fs, []string{root})
}

func TestQuery_Batch(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping self-scan in -short mode")
	}

	client := newClient(t)
	ctx := context.Background()
	root := locusRoot(t)

	resp, err := client.Query(ctx, query.Request{
		Path:   root,
		Intent: "health",
		Actions: []query.Action{
			{Name: "hot_spots", Params: map[string]any{"top_n": 5}},
			{Name: "cycles"},
			{Name: "pattern_scan"},
		},
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}

	if resp.CacheKey == "" {
		t.Error("expected non-empty CacheKey")
	}
	if len(resp.Results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(resp.Results))
	}

	for i, r := range resp.Results {
		if !r.OK {
			t.Errorf("result[%d] %s failed: %s", i, r.Action, r.Err)
			continue
		}
		if !json.Valid(r.Data) {
			t.Errorf("result[%d] %s has invalid JSON", i, r.Action)
		}
	}
}

func TestQuery_CacheKeyReuse(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping self-scan in -short mode")
	}

	client := newClient(t)
	ctx := context.Background()
	root := locusRoot(t)

	// First query to get a cache key.
	resp1, err := client.Query(ctx, query.Request{
		Path:    root,
		Actions: []query.Action{{Name: "cycles"}},
	})
	if err != nil {
		t.Fatalf("Query 1: %v", err)
	}

	// Second query reuses cache key — should be faster, no rescan.
	resp2, err := client.Query(ctx, query.Request{
		Path:     root,
		CacheKey: resp1.CacheKey,
		Actions:  []query.Action{{Name: "violations"}},
	})
	if err != nil {
		t.Fatalf("Query 2: %v", err)
	}

	if resp2.CacheKey != resp1.CacheKey {
		t.Errorf("cache key changed: %s -> %s", resp1.CacheKey, resp2.CacheKey)
	}
	if !resp2.Results[0].OK {
		t.Errorf("violations failed: %s", resp2.Results[0].Err)
	}
}

func TestQuery_ErrorIsolation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping self-scan in -short mode")
	}

	client := newClient(t)
	ctx := context.Background()
	root := locusRoot(t)

	resp, err := client.Query(ctx, query.Request{
		Path: root,
		Actions: []query.Action{
			{Name: "cycles"},             // should succeed
			{Name: "nonexistent_action"}, // should fail
			{Name: "pattern_scan"},       // should succeed despite prior failure
		},
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}

	if !resp.Results[0].OK {
		t.Errorf("cycles should have succeeded: %s", resp.Results[0].Err)
	}
	if resp.Results[1].OK {
		t.Error("nonexistent_action should have failed")
	}
	if resp.Results[1].Err == "" {
		t.Error("nonexistent_action should have an error message")
	}
	if !resp.Results[2].OK {
		t.Errorf("pattern_scan should have succeeded despite prior failure: %s", resp.Results[2].Err)
	}
}

func TestQuery_HotSpotsShape(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping self-scan in -short mode")
	}

	client := newClient(t)
	ctx := context.Background()
	root := locusRoot(t)

	resp, err := client.Query(ctx, query.Request{
		Path:    root,
		Actions: []query.Action{{Name: "hot_spots", Params: map[string]any{"top_n": 3}}},
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}

	var spots []arch.HotSpot
	if err := json.Unmarshal(resp.Results[0].Data, &spots); err != nil {
		t.Fatalf("unmarshal hot_spots: %v", err)
	}
	if len(spots) == 0 {
		t.Error("expected at least 1 hot spot from self-scan")
	}
	if len(spots) > 3 {
		t.Errorf("requested top_n=3 but got %d", len(spots))
	}
}
