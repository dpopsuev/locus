package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDogfoodAlef_Live reproduces the original Cursor MCP dogfood against the
// alef monorepo using the in-process handler (fixed code path).
// Opt-in: LOCUS_DOGFOOD_ALEF=1 (slow; ~25s full typescript scan).
func TestDogfoodAlef_Live(t *testing.T) {
	if os.Getenv("LOCUS_DOGFOOD_ALEF") == "" {
		t.Skip("set LOCUS_DOGFOOD_ALEF=1 to run live alef dogfood")
	}
	alef := os.Getenv("ALEF_ROOT")
	if alef == "" {
		alef = filepath.Join(os.Getenv("HOME"), "Workspace", "alef")
	}
	if st, err := os.Stat(alef); err != nil || !st.IsDir() {
		t.Skipf("alef not found at %s", alef)
	}

	h := newHandlerWithWorkspace(t, alef)
	ctx := context.Background()

	t.Run("scan_local_summary_cache_key", func(t *testing.T) {
		res, _, err := h.handleScanProject(ctx, nil, &codographActionInput{
			Path:    alef,
			Intent:  "coupling",
			Scanner: "typescript",
			Format:  FormatSummary,
		})
		if err != nil {
			t.Fatalf("scan_local: %v", err)
		}
		text := extractText(res)
		_, _, cacheKey := parseScanSummary(text)
		if cacheKey == "" {
			t.Fatalf("scan_local format=summary omitted cache_key\n%s", text[:min(800, len(text))])
		}
		t.Logf("cache_key=%s", cacheKey)
		if !strings.Contains(cacheKey, "@") {
			t.Fatalf("cache_key missing @: %q", cacheKey)
		}
	})

	t.Run("type_usages_symbol_DiscussionRef", func(t *testing.T) {
		// Ensure scan exists for sticky/path resolve.
		_, _, err := h.handleScanProject(ctx, nil, &codographActionInput{
			Path:    alef,
			Intent:  "full",
			Scanner: "typescript",
		})
		if err != nil {
			t.Fatalf("scan: %v", err)
		}
		res, _, err := h.handleAnalysis(ctx, nil, analysisInput{
			Action: ActionTypeUsages,
			Symbol: "DiscussionRef",
			Path:   alef,
		})
		if err != nil {
			t.Fatalf("type_usages: %v", err)
		}
		text := extractText(res)
		var report struct {
			TypeName string `json:"type_name"`
			Files    []struct {
				File      string `json:"file"`
				Component string `json:"component"`
			} `json:"files"`
			Summary string `json:"summary"`
		}
		if err := json.Unmarshal([]byte(text), &report); err != nil {
			t.Fatalf("parse: %v\n%s", err, text)
		}
		if report.TypeName != "DiscussionRef" {
			t.Fatalf("symbol= ignored: type_name=%q\n%s", report.TypeName, text)
		}
		t.Logf("type_usages: %s files=%d", report.Summary, len(report.Files))
		for _, f := range report.Files {
			t.Logf("  %s", f.Component)
		}
		if len(report.Files) == 0 {
			t.Fatal("expected at least packages/core/kernel for DiscussionRef")
		}
	})
}
