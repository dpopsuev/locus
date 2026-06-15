package mcp

import (
	"context"
	"strings"
	"testing"
)

// TestFileGranularity_PropagatedToDiagram verifies that render_diagram with
// a file-granularity cache_key renders nodes at file level, not package level.
//
// When a file-granularity scan is performed and its cache_key is passed
// to render_diagram, the output must contain file-level node names
// (e.g. "index.ts") rather than collapsing to the directory.
//
// Given a monorepo with TypeScript files
// When scan_local with file_granularity=true produces a cache_key
// And render_diagram type=dependency is called with that cache_key
// Then the diagram nodes reflect file-level granularity
func TestFileGranularity_PropagatedToDiagram(t *testing.T) {
	dir := monorepoFixture(t)
	h := newHandlerWithWorkspace(t, dir)
	ctx := context.Background()

	scanResult, _, err := h.handleScanProject(ctx, nil, &codographActionInput{
		Path:            dir,
		Intent:          "full",
		FileGranularity: true,
	})
	if err != nil {
		t.Fatalf("file-granularity scan: %v", err)
	}
	scanText := extractText(scanResult)
	_, _, fileCacheKey := parseScanSummary(scanText)
	if !strings.HasSuffix(fileCacheKey, "-file") {
		t.Skipf("no file-granularity cache key (not a TS repo or ctags unavailable): %q", fileCacheKey)
	}

	diagResult, _, err := h.handleRenderDiagram(ctx, nil, diagramInput{
		Path:     dir,
		Type:     "dependency",
		CacheKey: fileCacheKey,
	})
	if err != nil {
		t.Fatalf("render_diagram with file cache_key: %v", err)
	}
	diagText := extractText(diagResult)

	if !strings.Contains(diagText, ".ts") {
		t.Errorf("diagram with file-granularity cache_key shows no .ts file nodes\ndiagram:\n%s", diagText)
	}
}

// TestFileGranularity_DirectScan verifies the diagramInput struct exposes
// a file_granularity field for direct use without a cache_key.
//
// Given render_diagram called with file_granularity=true and no cache_key
// When the diagram is rendered
// Then it scans at file granularity
func TestFileGranularity_DirectScan(t *testing.T) {
	dir := monorepoFixture(t)
	h := newHandlerWithWorkspace(t, dir)
	ctx := context.Background()

	_, _, err := h.handleRenderDiagram(ctx, nil, diagramInput{
		Path:            dir,
		Type:            "dependency",
		FileGranularity: true,
	})
	if err != nil {
		t.Fatalf("render_diagram file_granularity=true: %v", err)
	}
}
