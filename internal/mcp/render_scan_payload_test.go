package mcp

import (
	"context"
	"strings"
	"testing"

	"github.com/dpopsuev/oculus/v3/arch"
	"github.com/dpopsuev/oculus/v3/engine"
)

func TestAppendCacheKeyLine(t *testing.T) {
	t.Parallel()
	const key = "/repo@deadbeef-full"

	got := appendCacheKeyLine("body", key)
	if want := "body\n\ncache_key: " + key; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}

	already := "Scanned x\ncache_key: " + key
	if got := appendCacheKeyLine(already, key); got != already {
		t.Fatalf("should not duplicate: got %q", got)
	}

	if got := appendCacheKeyLine("body", ""); got != "body" {
		t.Fatalf("empty key should leave body unchanged: %q", got)
	}
}

func TestRenderScanPayload_AllFormatsIncludeCacheKey(t *testing.T) {
	t.Parallel()
	const key = "/tmp/demo@abc123-full"
	payload := &sfPayload{
		scanResult: &engine.ScanResult{
			Report:   &arch.ContextReport{},
			CacheKey: key,
		},
		driftText: "Scanned demo: 0 components, 0 edges, 0 cycles, survey=none\ncache_key: " + key,
	}

	for _, format := range []string{FormatSummary, FormatJSON, ""} {
		res, err := renderScanPayload(payload, format)
		if err != nil {
			t.Fatalf("format=%q: %v", format, err)
		}
		text := extractText(res)
		if !strings.Contains(text, "cache_key: "+key) {
			t.Errorf("format=%q missing cache_key line\n%s", format, text)
		}
		if strings.Count(text, "cache_key:") != 1 {
			t.Errorf("format=%q: want exactly one cache_key line, got %d\n%s",
				format, strings.Count(text, "cache_key:"), text)
		}
	}
}

// TestScanLocal_SummaryAndJSONIncludeCacheKey covers the dogfood reproduction:
// format=summary / format=json must surface cache_key in the MCP text payload.
func TestScanLocal_SummaryAndJSONIncludeCacheKey(t *testing.T) {
	dir := monorepoFixture(t)
	h := newHandlerWithWorkspace(t, dir)
	ctx := context.Background()

	for _, format := range []string{FormatSummary, FormatJSON} {
		res, _, err := h.handleScanProject(ctx, nil, &codographActionInput{
			Path:   dir,
			Intent: "architecture",
			Format: format,
		})
		if err != nil {
			t.Fatalf("format=%q scan: %v", format, err)
		}
		text := extractText(res)
		_, _, cacheKey := parseScanSummary(text)
		if cacheKey == "" {
			t.Fatalf("format=%q: missing cache_key\n%s", format, text)
		}
		if !strings.Contains(cacheKey, "@") {
			t.Fatalf("format=%q: cache_key missing @: %q", format, cacheKey)
		}
	}
}
