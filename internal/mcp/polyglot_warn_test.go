package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPolyglotScannerWarning(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte("[package]\nname=\"x\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"x"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	warn := polyglotScannerWarning(dir, "typescript")
	if warn == "" || !strings.Contains(warn, "polyglot") {
		t.Fatalf("expected polyglot warning, got %q", warn)
	}
	if polyglotScannerWarning(dir, "auto") != "" {
		t.Fatal("auto should not warn")
	}
	if polyglotScannerWarning(dir, "composite") != "" {
		t.Fatal("composite should not warn")
	}
	if polyglotScannerWarning(dir, "") != "" {
		t.Fatal("empty scanner should not warn")
	}
}
