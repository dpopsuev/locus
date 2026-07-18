package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// dogfoodFixture copies testdata/mcp_dogfood_cache_key_type_usages into a temp
// git repo. Mirrors the alef dogfood shape: kernel exports DiscussionRef,
// foundry imports it.
func dogfoodFixture(t *testing.T) string {
	t.Helper()
	src := filepath.Join("..", "..", "testdata", "mcp_dogfood_cache_key_type_usages")
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("fixture missing at %s: %v", src, err)
	}
	dir := t.TempDir()
	if err := copyTree(src, dir); err != nil {
		t.Fatalf("copy fixture: %v", err)
	}
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...) //nolint:gosec // test helper: fixed git argv
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test")
	run("add", "-A")
	run("commit", "-q", "-m", "init")
	return dir
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == ".git" {
			return fs.SkipDir
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o750)
		}
		in, err := os.Open(path) //nolint:gosec // test fixture copy; path from WalkDir under src
		if err != nil {
			return err
		}
		defer in.Close()
		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600) //nolint:gosec // test fixture copy into TempDir
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, in)
		return err
	})
}

// RCA break 1: format=summary|json used RenderMarkdown/RenderJSON only —
// sticky rememberSticky ran, but agents could not copy cache_key from the payload.
func TestDogfood_ScanLocal_SummaryAndJSONSurfaceCacheKey(t *testing.T) {
	dir := dogfoodFixture(t)
	h := newHandlerWithWorkspace(t, dir)
	ctx := context.Background()

	for _, format := range []string{FormatSummary, FormatJSON, ""} {
		res, _, err := h.handleScanProject(ctx, nil, &codographActionInput{
			Path:    dir,
			Intent:  "coupling",
			Scanner: "typescript",
			Format:  format,
		})
		if err != nil {
			t.Fatalf("format=%q scan_local: %v", format, err)
		}
		text := extractText(res)
		_, _, cacheKey := parseScanSummary(text)
		if cacheKey == "" {
			t.Fatalf("format=%q: scan_local omitted cache_key (RCA: renderScanPayload)\n%s", format, text)
		}
		if !strings.Contains(cacheKey, "@") {
			t.Fatalf("format=%q: cache_key missing @: %q", format, cacheKey)
		}
		if strings.Count(text, "cache_key:") != 1 {
			t.Fatalf("format=%q: want one cache_key line, got %d\n%s",
				format, strings.Count(text, "cache_key:"), text)
		}
	}
}

// RCA break 2: opTypeUsages passed in.Query only — symbol=DiscussionRef → type_name="".
func TestDogfood_TypeUsages_SymbolDiscussionRef(t *testing.T) {
	dir := dogfoodFixture(t)
	h := newHandlerWithWorkspace(t, dir)
	ctx := context.Background()

	_, _, err := h.handleScanProject(ctx, nil, &codographActionInput{
		Path:    dir,
		Intent:  "full",
		Scanner: "typescript",
	})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	t.Run("symbol=", func(t *testing.T) {
		res, _, err := h.handleAnalysis(ctx, nil, analysisInput{
			Action: ActionTypeUsages,
			Symbol: "DiscussionRef",
			Path:   dir,
		})
		if err != nil {
			t.Fatalf("type_usages symbol=: %v", err)
		}
		text := extractText(res)
		var report struct {
			TypeName string `json:"type_name"`
			Files    []any  `json:"files"`
		}
		if err := json.Unmarshal([]byte(text), &report); err != nil {
			t.Fatalf("parse: %v\n%s", err, text)
		}
		if report.TypeName != "DiscussionRef" {
			t.Fatalf("symbol= ignored: type_name=%q (want DiscussionRef)\n%s", report.TypeName, text)
		}
		if len(report.Files) < 2 {
			t.Fatalf("expected kernel decl + foundry import-type consumer, got %d\n%s",
				len(report.Files), text)
		}
		blob := text
		if !strings.Contains(blob, "kernel") || !strings.Contains(blob, "foundry") {
			t.Fatalf("want kernel + foundry in type_usages:\n%s", text)
		}
	})

	t.Run("query=", func(t *testing.T) {
		res, _, err := h.handleAnalysis(ctx, nil, analysisInput{
			Action: ActionTypeUsages,
			Query:  "DiscussionRef",
			Path:   dir,
		})
		if err != nil {
			t.Fatalf("type_usages query=: %v", err)
		}
		if !strings.Contains(extractText(res), `"type_name":"DiscussionRef"`) {
			t.Fatalf("query= failed:\n%s", extractText(res))
		}
	})

	t.Run("empty_errors", func(t *testing.T) {
		raw, _ := json.Marshal(analysisInput{Action: ActionTypeUsages, Path: dir})
		_, err := opTypeUsages.Run(ctx, h, raw)
		if !errors.Is(err, ErrTypeUsagesLocatorRequired) {
			t.Fatalf("got %v, want ErrTypeUsagesLocatorRequired", err)
		}
	})
}
