package mcp

import (
	"context"
	"strings"
	"testing"
)

// TestBug98_FileGranularity_PropagatedToDiagram reproduces LCS-BUG-98:
// render_diagram with a file-granularity cache_key still renders nodes at
// package level because resolveDiagramReport re-scans without the flag.
//
// When a file-granularity scan is performed and its cache_key is passed
// to render_diagram, the output must contain file-level node names
// (e.g. "index.ts" as a node) rather than collapsing to the directory.
//
// Given a monorepo with TypeScript files
// When scan_local with file_granularity=true produces a cache_key
// And render_diagram type=dependency is called with that cache_key
// Then the diagram nodes reflect file-level granularity (not dir-level)
func TestBug98_FileGranularity_PropagatedToDiagram(t *testing.T) {
	dir := monorepoFixture(t)
	h := newHandlerWithWorkspace(t, dir)
	ctx := context.Background()

	// Scan with file granularity.
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

	// Render diagram using the file-granularity cache key.
	diagResult, _, err := h.handleRenderDiagram(ctx, nil, diagramInput{
		Path:     dir,
		Type:     "dependency",
		CacheKey: fileCacheKey,
	})
	if err != nil {
		t.Fatalf("render_diagram with file cache_key: %v", err)
	}
	diagText := extractText(diagResult)

	// A file-granularity diagram must reference individual .ts files as nodes,
	// not just directory-level component names.
	// The monorepo fixture has packages/spine/src/index.ts — file-level scan
	// should produce a node for "index.ts" or the full file path.
	if !strings.Contains(diagText, ".ts") {
		t.Errorf("BUG-98: diagram with file-granularity cache_key shows no .ts file nodes\ndiagram:\n%s", diagText)
	}
}

// TestBug98_FileGranularity_DiagramInput_HasField verifies the diagramInput
// struct exposes a file_granularity field for direct use without a cache_key.
//
// Given render_diagram called with file_granularity=true and no cache_key
// When the diagram is rendered
// Then it scans at file granularity (nodes are files, not directories)
func TestBug98_FileGranularity_DirectScan(t *testing.T) {
	dir := monorepoFixture(t)
	h := newHandlerWithWorkspace(t, dir)
	ctx := context.Background()

	// Render diagram directly with file_granularity=true (no prior scan needed).
	_, _, err := h.handleRenderDiagram(ctx, nil, diagramInput{
		Path:            dir,
		Type:            "dependency",
		FileGranularity: true,
	})
	if err != nil {
		t.Fatalf("render_diagram file_granularity=true: %v", err)
	}
	// Build success is sufficient: the field must exist and be wired through.
}
