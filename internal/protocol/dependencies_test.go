package protocol

import (
	"os"
	"path/filepath"
	"testing"
)

const testGoMod = `module example.com/myapp

go 1.22.0

require (
	github.com/foo/bar v1.2.3
	github.com/baz/qux v0.5.0
	golang.org/x/text v0.14.0 // indirect
)

replace github.com/foo/bar => ../local-bar
`

func TestComputeDependencies_Basic(t *testing.T) {
	dir := t.TempDir()
	goModPath := filepath.Join(dir, "go.mod")
	if err := os.WriteFile(goModPath, []byte(testGoMod), 0o644); err != nil {
		t.Fatal(err)
	}

	report, err := ComputeDependencies(goModPath)
	if err != nil {
		t.Fatalf("ComputeDependencies: %v", err)
	}

	if report.Module != "example.com/myapp" {
		t.Errorf("module = %q, want example.com/myapp", report.Module)
	}
	if report.GoVersion != "1.22.0" {
		t.Errorf("go_version = %q, want 1.22.0", report.GoVersion)
	}
	if len(report.Direct) != 2 {
		t.Errorf("direct deps = %d, want 2", len(report.Direct))
	}
	if len(report.Indirect) != 1 {
		t.Errorf("indirect deps = %d, want 1", len(report.Indirect))
	}
	if len(report.Replaces) != 1 {
		t.Errorf("replaces = %d, want 1", len(report.Replaces))
	}

	// Verify direct deps.
	directPaths := make(map[string]bool)
	for _, d := range report.Direct {
		directPaths[d.Path] = true
		if d.Indirect {
			t.Errorf("direct dep %q marked as indirect", d.Path)
		}
	}
	if !directPaths["github.com/foo/bar"] {
		t.Error("missing direct dep github.com/foo/bar")
	}
	if !directPaths["github.com/baz/qux"] {
		t.Error("missing direct dep github.com/baz/qux")
	}

	// Verify indirect dep.
	if report.Indirect[0].Path != "golang.org/x/text" {
		t.Errorf("indirect[0].Path = %q, want golang.org/x/text", report.Indirect[0].Path)
	}
	if !report.Indirect[0].Indirect {
		t.Error("indirect dep not marked as indirect")
	}

	// Verify replace.
	if report.Replaces[0].Path != "github.com/foo/bar" {
		t.Errorf("replace[0].Path = %q, want github.com/foo/bar", report.Replaces[0].Path)
	}
	if report.Replaces[0].Replace != "../local-bar" {
		t.Errorf("replace[0].Replace = %q, want ../local-bar", report.Replaces[0].Replace)
	}

	// Verify summary.
	want := "2 direct, 1 indirect, 1 replaces"
	if report.Summary != want {
		t.Errorf("summary = %q, want %q", report.Summary, want)
	}
}

func TestComputeDependencies_MissingFile(t *testing.T) {
	_, err := ComputeDependencies("/nonexistent/go.mod")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestComputeDependencies_EmptyModule(t *testing.T) {
	dir := t.TempDir()
	goModPath := filepath.Join(dir, "go.mod")
	content := `module example.com/empty

go 1.21.0
`
	if err := os.WriteFile(goModPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	report, err := ComputeDependencies(goModPath)
	if err != nil {
		t.Fatalf("ComputeDependencies: %v", err)
	}

	if report.Module != "example.com/empty" {
		t.Errorf("module = %q, want example.com/empty", report.Module)
	}
	if len(report.Direct) != 0 {
		t.Errorf("direct = %d, want 0", len(report.Direct))
	}
	if len(report.Indirect) != 0 {
		t.Errorf("indirect = %d, want 0", len(report.Indirect))
	}
	if len(report.Replaces) != 0 {
		t.Errorf("replaces = %d, want 0", len(report.Replaces))
	}
	if report.Summary != "0 direct, 0 indirect, 0 replaces" {
		t.Errorf("summary = %q", report.Summary)
	}
}

func TestComputeDependencies_ReplaceWithVersion(t *testing.T) {
	dir := t.TempDir()
	goModPath := filepath.Join(dir, "go.mod")
	content := `module example.com/versioned

go 1.22.0

require github.com/old/pkg v1.0.0

replace github.com/old/pkg v1.0.0 => github.com/new/pkg v2.0.0
`
	if err := os.WriteFile(goModPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	report, err := ComputeDependencies(goModPath)
	if err != nil {
		t.Fatalf("ComputeDependencies: %v", err)
	}

	if len(report.Replaces) != 1 {
		t.Fatalf("replaces = %d, want 1", len(report.Replaces))
	}
	rep := report.Replaces[0]
	if rep.Replace != "github.com/new/pkg@v2.0.0" {
		t.Errorf("replace = %q, want github.com/new/pkg@v2.0.0", rep.Replace)
	}
}
