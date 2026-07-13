package mcp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	oculuscache "github.com/dpopsuev/oculus/v3/cache"
	"github.com/dpopsuev/oculus/v3/engine"
	"github.com/dpopsuev/locus/internal/store"
)

// CWD-shift harness (locus-analysis-returns-wrong-project-when-cwd-shifts-during-58d5):
// parent workspace → scan sibling alef then utiqa → chdir under alef → analysis
// with empty path/cache_key must bind to alef, not lastScanned utiqa.
//
// Trees are synthetic under /var/tmp (outside os.TempDir so projects register).

type cwdShiftHarness struct {
	Parent   string
	Alef     string
	Utiqa    string
	AlefDeep string
	Handler  *handler
}

func setupCWDShiftHarness(t *testing.T) *cwdShiftHarness {
	t.Helper()
	// Outside os.TempDir so registered projects are not filtered as ephemeral.
	base := filepath.FromSlash("/var/tmp/locus-cwd-shift/" + t.Name())
	_ = os.RemoveAll(base)
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(base) })

	alef := filepath.Join(base, "alef")
	utiqa := filepath.Join(base, "utiqa")
	writeTree(t, alef, map[string]string{
		"go.mod":                            "module example.com/alef\n\ngo 1.22\n",
		"packages/profiles/profiles.go":     "package profiles\n\nfunc AlefProfilesMarker() {}\n",
		"packages/core/llm/llm.go":          "package llm\n\nfunc Run() {}\n",
	})
	writeTree(t, utiqa, map[string]string{
		"go.mod":                    "module example.com/utiqa\n\ngo 1.22\n",
		"internal/exec/exec.go":     "package exec\n\nfunc UtiqaExecMarker() {}\n",
		"internal/typedbus/bus.go":  "package typedbus\n\nfunc Bus() {}\n",
		"cmd/utiqa/main.go":         "package main\n\nfunc main() {}\n",
	})
	gitInit(t, alef)
	gitInit(t, utiqa)

	sc := oculuscache.New(t.TempDir())
	db := store.NewFilesystem(sc, filepath.Join(t.TempDir(), "history"))
	eng := engine.New(store.NewLRU(db, 16), []string{base})
	h := &handler{proto: eng, sproto: eng}
	ctx := context.Background()

	if _, _, err := h.handleScanProject(ctx, nil, &codographActionInput{Path: alef, Intent: "architecture"}); err != nil {
		t.Fatalf("scan alef: %v", err)
	}
	if _, _, err := h.handleScanProject(ctx, nil, &codographActionInput{Path: utiqa, Intent: "architecture"}); err != nil {
		t.Fatalf("scan utiqa: %v", err)
	}
	if p, _ := h.lastScannedPath.Load().(string); filepath.Clean(p) != filepath.Clean(utiqa) {
		t.Fatalf("lastScannedPath=%q, want utiqa", p)
	}

	return &cwdShiftHarness{
		Parent:   base,
		Alef:     alef,
		Utiqa:    utiqa,
		AlefDeep: filepath.Join(alef, "packages", "core", "llm"),
		Handler:  h,
	}
}

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, body := range files {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func analysisDefaultPath(ctx context.Context, h *handler) string {
	if resolved := h.resolveDefaultProject(ctx); resolved != "" {
		return resolved
	}
	if v := h.lastScannedPath.Load(); v != nil {
		if p, ok := v.(string); ok && p != "" {
			return p
		}
	}
	return h.proto.ResolvePath("")
}

func TestCWDShift_AnalysisBindsToEnclosingProject(t *testing.T) {
	h := setupCWDShiftHarness(t)
	ctx := context.Background()
	t.Chdir(h.AlefDeep)

	selected := analysisDefaultPath(ctx, h.Handler)
	r, _, err := h.Handler.handleAnalysis(ctx, nil, analysisInput{
		Action: ActionSymbolSearch,
		Symbol: "Marker",
		TopN:   20,
	})
	if err != nil {
		t.Fatalf("symbol_search: %v", err)
	}
	body := extractText(r)
	t.Logf("selected=%s\n%s", selected, body)

	if filepath.Clean(selected) != filepath.Clean(h.Alef) {
		t.Errorf("default path=%q, want alef %q", selected, h.Alef)
	}
	if strings.Contains(body, "UtiqaExecMarker") || strings.Contains(body, "internal/exec") {
		t.Errorf("bound to utiqa while CWD under alef: %.400s", body)
	}
	if !strings.Contains(body, "AlefProfilesMarker") {
		t.Errorf("expected AlefProfilesMarker in alef scan: %.400s", body)
	}
}

func TestCWDShift_ResolveDefaultUsesProcessCWD(t *testing.T) {
	h := setupCWDShiftHarness(t)
	t.Chdir(h.AlefDeep)
	got := h.Handler.resolveDefaultProject(context.Background())
	if filepath.Clean(got) != filepath.Clean(h.Alef) {
		t.Errorf("resolveDefaultProject=%q, want %q", got, h.Alef)
	}
}
