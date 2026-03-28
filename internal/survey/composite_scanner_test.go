package survey_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dpopsuev/locus/internal/model"
	"github.com/dpopsuev/locus/internal/survey"
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
