package survey_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dpopsuev/locus/internal/model"
	"github.com/dpopsuev/locus/internal/survey"
	"github.com/dpopsuev/locus/internal/testkit"
)

func TestCompositeScanMergesRustAndTS(t *testing.T) {
	dir := t.TempDir()

	files := map[string]string{
		"Cargo.toml": `[workspace]
members = ["crates/core"]
`,
		"crates/core/Cargo.toml": `[package]
name = "core"
version = "0.1.0"

[dependencies]
serde = "1"
`,
		"crates/core/src/lib.rs": `pub fn process() {}
pub struct Engine {}
`,
		"client/package.json": `{"name": "client-app", "dependencies": {"three": "1.0"}}`,
		"client/src/main.ts": `import { Scene } from 'three'
export function init() {}
`,
	}

	for name, content := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	sc := &survey.CompositeScanner{}
	proj, err := sc.Scan(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	nsMap := make(map[string]*model.Namespace)
	for _, ns := range proj.Namespaces {
		nsMap[ns.ImportPath] = ns
	}

	if _, ok := nsMap["core"]; !ok {
		t.Error("missing Rust crate namespace 'core'")
	}

	if _, ok := nsMap["client/src"]; !ok {
		allPaths := make([]string, 0, len(nsMap))
		for k := range nsMap {
			allPaths = append(allPaths, k)
		}
		t.Errorf("missing TypeScript namespace 'client/src'; have: %v", allPaths)
	}

	if proj.DependencyGraph == nil {
		t.Fatal("dependency graph is nil")
	}

	coreEdges := proj.DependencyGraph.EdgesFrom("core")
	foundSerde := false
	for _, e := range coreEdges {
		if e.To == "serde" && e.External {
			foundSerde = true
		}
	}
	if !foundSerde {
		t.Error("missing Rust external edge core -> serde")
	}

	clientEdges := proj.DependencyGraph.EdgesFrom("client/src")
	foundThree := false
	for _, e := range clientEdges {
		if e.To == "client/three" || e.To == "three" {
			foundThree = true
		}
	}
	if !foundThree {
		t.Error("missing TypeScript external edge client/src -> three")
	}
}

func TestCompositeScanMergesPythonAndTS(t *testing.T) {
	dir := t.TempDir()

	files := map[string]string{
		"pyproject.toml":   "[project]\nname = \"backend\"\n",
		"backend/main.py":  "def serve():\n    pass\n",
		"web/package.json": `{"name": "web-ui"}`,
		"web/src/index.ts": `export function mount() {}`,
	}

	for name, content := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	sc := &survey.CompositeScanner{}
	proj, err := sc.Scan(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	nsMap := make(map[string]*model.Namespace)
	for _, ns := range proj.Namespaces {
		nsMap[ns.ImportPath] = ns
	}

	if _, ok := nsMap["backend"]; !ok {
		allPaths := make([]string, 0, len(nsMap))
		for k := range nsMap {
			allPaths = append(allPaths, k)
		}
		t.Errorf("missing Python namespace 'backend'; have: %v", allPaths)
	}

	if _, ok := nsMap["web/src"]; !ok {
		allPaths := make([]string, 0, len(nsMap))
		for k := range nsMap {
			allPaths = append(allPaths, k)
		}
		t.Errorf("missing TypeScript namespace 'web/src'; have: %v", allPaths)
	}
}

func TestCompositeScan_GoAndRust(t *testing.T) {
	dir := t.TempDir()

	files := map[string]string{
		// Go sub-project at root.
		"go.mod":      "module example.com/goapp\n\ngo 1.21",
		"main.go":     "package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println(\"hello\") }",
		"pkg/util.go": "package pkg\n\nfunc Helper() string { return \"help\" }",
		// Rust sub-project at root (workspace layout).
		"Cargo.toml":         "[workspace]\nmembers = [\"rustlib\"]",
		"rustlib/Cargo.toml": "[package]\nname = \"rustlib\"\nversion = \"0.1.0\"\nedition = \"2021\"",
		"rustlib/src/lib.rs": "pub fn greet() -> String { String::from(\"hello\") }",
	}

	if err := testkit.BuildFixture(dir, files); err != nil {
		t.Fatal(err)
	}

	sc := &survey.CompositeScanner{}
	proj, err := sc.Scan(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	nsMap := make(map[string]*model.Namespace)
	for _, ns := range proj.Namespaces {
		nsMap[ns.ImportPath] = ns
	}

	if len(nsMap) < 2 {
		t.Fatalf("expected at least 2 namespaces, got %d", len(nsMap))
	}

	// Verify Go namespaces carry the sub-project module path.
	// GoScanner produces import paths like "example.com/goapp" and
	// "example.com/goapp/pkg" from the module directive in go.mod.
	foundGoNS := false
	for path := range nsMap {
		if strings.Contains(path, "goapp") {
			foundGoNS = true
			break
		}
	}
	if !foundGoNS {
		allPaths := make([]string, 0, len(nsMap))
		for k := range nsMap {
			allPaths = append(allPaths, k)
		}
		t.Errorf("no Go namespace containing 'goapp'; have: %v", allPaths)
	}

	// Verify the pkg sub-package is prefixed with the module path.
	foundPkg := false
	for path := range nsMap {
		if strings.HasSuffix(path, "/pkg") || strings.HasSuffix(path, "goapp/pkg") {
			foundPkg = true
			break
		}
	}
	if !foundPkg {
		allPaths := make([]string, 0, len(nsMap))
		for k := range nsMap {
			allPaths = append(allPaths, k)
		}
		t.Errorf("no namespace ending with /pkg; have: %v", allPaths)
	}

	// Verify Rust namespace is present.
	if _, ok := nsMap["rustlib"]; !ok {
		allPaths := make([]string, 0, len(nsMap))
		for k := range nsMap {
			allPaths = append(allPaths, k)
		}
		t.Errorf("missing Rust namespace 'rustlib'; have: %v", allPaths)
	}

	// Both languages must be represented in the scan results.
	if !foundGoNS {
		t.Error("Go language not represented in scan results")
	}
	if _, ok := nsMap["rustlib"]; !ok {
		t.Error("Rust language not represented in scan results")
	}
}

func TestCompositeScanAutoDetectsMultipleLanguages(t *testing.T) {
	dir := t.TempDir()

	files := map[string]string{
		"Cargo.toml": `[package]
name = "backend"
version = "0.1.0"
`,
		"src/lib.rs":       `pub fn serve() {}`,
		"web/package.json": `{"name": "web-ui"}`,
		"web/src/index.ts": `export function mount() {}`,
	}

	for name, content := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	sc := &survey.AutoScanner{Override: "auto"}
	proj, err := sc.Scan(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	if len(proj.Namespaces) < 2 {
		t.Errorf("expected at least 2 namespaces from composite scan, got %d", len(proj.Namespaces))
	}
}
