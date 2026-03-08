package survey_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dpopsuev/locus/internal/model"
	"github.com/dpopsuev/locus/internal/survey"
)

func setupCrate(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestRustScanSingleCrate(t *testing.T) {
	dir := setupCrate(t, map[string]string{
		"Cargo.toml": `[package]
name = "my-crate"
version = "0.1.0"
edition = "2021"

[dependencies]
serde = "1"
`,
		"src/lib.rs": `pub fn hello() -> String {
    "hello".to_string()
}

pub struct Config {
    pub name: String,
}

fn internal() {}
`,
	})

	sc := &survey.RustScanner{}
	proj, err := sc.Scan(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	if proj.Path != "my-crate" {
		t.Errorf("path = %q, want my-crate", proj.Path)
	}
	if proj.Language != model.LangRust {
		t.Errorf("language = %v, want Rust", proj.Language)
	}
	if len(proj.Namespaces) != 1 {
		t.Fatalf("namespaces = %d, want 1", len(proj.Namespaces))
	}

	ns := proj.Namespaces[0]
	if ns.Name != "my-crate" {
		t.Errorf("ns.name = %q, want my-crate", ns.Name)
	}

	symMap := make(map[string]*model.Symbol)
	for _, s := range ns.Symbols {
		symMap[s.Name] = s
	}

	if _, ok := symMap["hello"]; !ok {
		t.Error("missing pub fn hello")
	}
	if _, ok := symMap["Config"]; !ok {
		t.Error("missing pub struct Config")
	}
	if _, ok := symMap["internal"]; ok {
		t.Error("internal (non-pub) fn should not be extracted")
	}

	if proj.DependencyGraph == nil {
		t.Fatal("dependency graph is nil")
	}
	edges := proj.DependencyGraph.EdgesFrom("my-crate")
	foundSerde := false
	for _, e := range edges {
		if e.To == "serde" && e.External {
			foundSerde = true
		}
	}
	if !foundSerde {
		t.Error("missing external edge to serde")
	}
}

func TestRustScanWorkspace(t *testing.T) {
	dir := setupCrate(t, map[string]string{
		"Cargo.toml": `[workspace]
members = ["crates/core", "crates/server"]
`,
		"crates/core/Cargo.toml": `[package]
name = "my-core"
version = "0.1.0"

[dependencies]
serde = "1"
`,
		"crates/core/src/lib.rs": `pub trait Handler {
    fn handle(&self);
}

pub enum Status {
    Active,
    Inactive,
}
`,
		"crates/server/Cargo.toml": `[package]
name = "my-server"
version = "0.1.0"

[dependencies]
my-core = { path = "../core" }
tokio = "1"
`,
		"crates/server/src/main.rs": `pub fn start() {}

pub struct Server {
    port: u16,
}
`,
	})

	sc := &survey.RustScanner{}
	proj, err := sc.Scan(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	if len(proj.Namespaces) != 2 {
		t.Fatalf("namespaces = %d, want 2", len(proj.Namespaces))
	}

	nsMap := make(map[string]*model.Namespace)
	for _, ns := range proj.Namespaces {
		nsMap[ns.Name] = ns
	}

	core, ok := nsMap["my-core"]
	if !ok {
		t.Fatal("missing namespace my-core")
	}
	coreSyms := make(map[string]bool)
	for _, s := range core.Symbols {
		coreSyms[s.Name] = true
	}
	if !coreSyms["Handler"] {
		t.Error("missing pub trait Handler in core")
	}
	if !coreSyms["Status"] {
		t.Error("missing pub enum Status in core")
	}

	server, ok := nsMap["my-server"]
	if !ok {
		t.Fatal("missing namespace my-server")
	}
	serverSyms := make(map[string]bool)
	for _, s := range server.Symbols {
		serverSyms[s.Name] = true
	}
	if !serverSyms["start"] {
		t.Error("missing pub fn start in server")
	}
	if !serverSyms["Server"] {
		t.Error("missing pub struct Server in server")
	}

	// Internal edge: server -> core
	serverEdges := proj.DependencyGraph.EdgesFrom("my-server")
	foundCore := false
	foundTokio := false
	for _, e := range serverEdges {
		if e.To == "my-core" && !e.External {
			foundCore = true
		}
		if e.To == "tokio" && e.External {
			foundTokio = true
		}
	}
	if !foundCore {
		t.Error("missing internal edge server -> core")
	}
	if !foundTokio {
		t.Error("missing external edge server -> tokio")
	}
}
