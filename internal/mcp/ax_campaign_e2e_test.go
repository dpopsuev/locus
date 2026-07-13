package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dpopsuev/locus/internal/store"
	oculuscache "github.com/dpopsuev/oculus/v3/cache"
	"github.com/dpopsuev/oculus/v3/engine"
)

// TestAXCampaign_E2E is the AX campaign dogfood harness (ROI #2):
//
//	scan_local(intent=full) → hotspots → architecture_review → probe/scenario
//
// All analysis calls pass cache_key only (empty Path) so pathFromCacheKey
// regressions (HOME scan / RSS balloon) fail this test.
//
// Fixture preference: sibling ../oculus when present (campaign success criteria);
// otherwise the TS monorepo fixture with spineCore.
func TestAXCampaign_E2E(t *testing.T) {
	dir, symbol := axCampaignFixture(t)
	decoy := t.TempDir()
	scanH, decoyH := newSharedStoreHandlers(t, dir, decoy)

	ctx := context.Background()

	scanResult, _, err := scanH.handleScanProject(ctx, nil, &codographActionInput{
		Path:   dir,
		Intent: "full",
	})
	if err != nil {
		t.Fatalf("scan_local: %v", err)
	}
	scanText := extractText(scanResult)
	_, _, cacheKey := parseScanSummary(scanText)
	if cacheKey == "" {
		t.Fatalf("scan returned no cache_key: %s", scanText)
	}
	if !strings.Contains(cacheKey, "@") {
		t.Fatalf("cache_key missing @: %q", cacheKey)
	}
	boundPath := pathFromCacheKey(cacheKey)
	if filepath.Clean(boundPath) != filepath.Clean(dir) {
		t.Fatalf("pathFromCacheKey=%q, want %q", boundPath, dir)
	}
	t.Logf("cache_key=%s bound=%s decoy=%s", cacheKey, boundPath, decoy)

	t.Run("hot_spots_cache_key_only", func(t *testing.T) {
		r, _, err := decoyH.handleAnalysis(ctx, nil, analysisInput{
			Action:   ActionCoupling,
			View:     "hot_spots",
			TopN:     20,
			CacheKey: cacheKey,
		})
		if err != nil {
			t.Fatalf("coupling hot_spots: %v", err)
		}
		body := extractText(r)
		t.Logf("hot_spots (truncated): %.400s", body)
		n := hotspotCount(body)
		if n < 1 {
			t.Errorf("hotspot count=%d, want ≥1; body=%.300s", n, body)
		}
	})

	t.Run("architecture_review_cache_key_only", func(t *testing.T) {
		r, _, err := decoyH.handleAnalysis(ctx, nil, analysisInput{
			Action:   ActionPreset,
			Preset:   "architecture_review",
			CacheKey: cacheKey,
		})
		if err != nil {
			t.Fatalf("architecture_review: %v", err)
		}
		body := extractText(r)
		t.Logf("architecture_review (truncated): %.500s", body)
		if !strings.Contains(body, "## Coupling") {
			t.Errorf("missing ## Coupling section")
		}
		if !strings.Contains(body, "## Hot Spots") {
			t.Errorf("missing ## Hot Spots section")
		}
	})

	t.Run("probe_cache_key_only", func(t *testing.T) {
		start := time.Now()
		r, _, err := decoyH.handleAnalysis(ctx, nil, analysisInput{
			Action:   ActionProbe,
			Symbol:   symbol,
			CacheKey: cacheKey,
		})
		elapsed := time.Since(start)
		if err != nil {
			t.Fatalf("probe: %v", err)
		}
		body := extractText(r)
		t.Logf("probe elapsed=%v (truncated): %.400s", elapsed, body)
		if r != nil && r.IsError {
			t.Fatalf("probe isError: %s", body)
		}
		if elapsed > 10*time.Second {
			t.Errorf("probe took %v, want <10s (Quick SG path)", elapsed)
		}
		if strings.Contains(body, decoy) {
			t.Errorf("probe body references decoy workspace %q", decoy)
		}
		// Type pivots (engine.Engine) should suggest method pivots after FQN fix.
		if symbol == "engine.Engine" && !strings.Contains(body, "suggested_pivots") {
			t.Log("note: no suggested_pivots in body (may be empty JSON omitempty if uncovered)")
		}
	})

	t.Run("hybrid_query_cache_key_only", func(t *testing.T) {
		r, _, err := decoyH.handleAnalysis(ctx, nil, analysisInput{
			Action:   ActionQuery,
			Query:    "where is GetSymbolGraph defined",
			CacheKey: cacheKey,
		})
		if err != nil {
			t.Fatalf("query hybrid: %v", err)
		}
		body := extractText(r)
		t.Logf("hybrid query: %.500s", body)
		if !strings.Contains(body, "hybrid") {
			t.Errorf("expected hybrid action; got %.300s", body)
		}
		// Prefer non-empty hits when SG is warm; allow empty on cold sparse indexes.
		if strings.Contains(body, `"hits":[]`) {
			t.Log("hybrid hits empty — package index miss; SG fallback may still be warming")
		} else if !strings.Contains(body, "GetSymbolGraph") && !strings.Contains(body, "SymbolGraph") {
			t.Logf("hybrid hits present but no GetSymbolGraph substring: %.400s", body)
		}
	})

	t.Run("quality_default_quick", func(t *testing.T) {
		in := analysisInput{Quality: ""}
		if !in.symbolGraphOpts().Quick {
			t.Fatal("empty quality must default to Quick")
		}
	})

	t.Run("scenario_cache_key_only", func(t *testing.T) {
		start := time.Now()
		r, _, err := decoyH.handleAnalysis(ctx, nil, analysisInput{
			Action:   ActionScenario,
			Symbol:   symbol,
			CacheKey: cacheKey,
		})
		elapsed := time.Since(start)
		if err != nil {
			t.Fatalf("scenario: %v", err)
		}
		body := extractText(r)
		t.Logf("scenario elapsed=%v (truncated): %.400s", elapsed, body)
		if r != nil && r.IsError {
			t.Fatalf("scenario isError: %s", body)
		}
		if elapsed > 10*time.Second {
			t.Errorf("scenario took %v, want <10s", elapsed)
		}
	})
}

// newSharedStoreHandlers builds two handlers that share one store/cache but
// have different workspace roots. Analysis with cache_key-only must bind to
// the scanned path even when the decoy engine's workspaces[0] is wrong.
func newSharedStoreHandlers(t *testing.T, scanRoot, decoyRoot string) (scanH, decoyH *handler) {
	t.Helper()
	sc := oculuscache.New(t.TempDir())
	db := store.NewFilesystem(sc, filepath.Join(t.TempDir(), "history"))
	lruDB := store.NewLRU(db, 16)
	scanEng := engine.New(lruDB, []string{scanRoot})
	decoyEng := engine.New(lruDB, []string{decoyRoot})
	return &handler{proto: scanEng, sproto: scanEng}, &handler{proto: decoyEng, sproto: decoyEng}
}

func axCampaignFixture(t *testing.T) (dir, symbol string) {
	t.Helper()
	candidates := []string{
		os.Getenv("OCULUS_FIXTURE"),
		filepath.Join("..", "..", "..", "oculus"),
		filepath.Join(os.Getenv("HOME"), "Workspace", "oculus"),
	}
	for _, c := range candidates {
		if c == "" {
			continue
		}
		abs, err := filepath.Abs(c)
		if err != nil {
			continue
		}
		if st, err := os.Stat(filepath.Join(abs, ".git")); err == nil && st.IsDir() {
			if _, err := os.Stat(filepath.Join(abs, "engine")); err == nil {
				t.Logf("using oculus fixture: %s", abs)
				return abs, "engine.Engine"
			}
		}
	}
	t.Log("oculus fixture not found; using TS monorepo fixture")
	return monorepoFixture(t), "spineCore"
}

func hotspotCount(body string) int {
	body = strings.TrimSpace(body)
	if body == "" || body == "null" || body == "[]" {
		return 0
	}
	var arr []any
	if err := json.Unmarshal([]byte(body), &arr); err == nil {
		return len(arr)
	}
	return strings.Count(body, `"component"`)
}
