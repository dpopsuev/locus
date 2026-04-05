package survey

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dpopsuev/locus/internal/model"
)

// buildFixture writes files into dir. Keys are relative paths, values are contents.
func buildFixture(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		abs := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(abs), err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
}

func TestResolvePackageImport(t *testing.T) {
	tests := []struct {
		name   string
		dirNS  map[string]*model.Namespace
		match  []string
		expect string
	}{
		{
			name: "exact match",
			dirNS: map[string]*model.Namespace{
				"domain": model.NewNamespace("domain", "domain"),
			},
			match:  []string{"import domain.Entity;", "domain"},
			expect: "domain",
		},
		{
			name: "nested path matches via filepath.Base",
			dirNS: map[string]*model.Namespace{
				"src/main/java/domain": model.NewNamespace("domain", "src/main/java/domain"),
			},
			match:  []string{"import domain.Entity;", "domain"},
			expect: "src/main/java/domain",
		},
		{
			name: "no match returns empty",
			dirNS: map[string]*model.Namespace{
				"domain": model.NewNamespace("domain", "domain"),
			},
			match:  []string{"import unknown.Foo;", "unknown"},
			expect: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolvePackageImport(tt.match, tt.dirNS)
			if got != tt.expect {
				t.Errorf("resolvePackageImport(%q) = %q, want %q", tt.match[1], got, tt.expect)
			}
		})
	}
}

func TestResolveModuleImport(t *testing.T) {
	tests := []struct {
		name   string
		dirNS  map[string]*model.Namespace
		match  []string
		expect string
	}{
		{
			name: "match via filepath.Base",
			dirNS: map[string]*model.Namespace{
				"Sources/Domain": model.NewNamespace("Domain", "Sources/Domain"),
			},
			match:  []string{"import Domain", "Domain"},
			expect: "Sources/Domain",
		},
		{
			name: "no match returns empty",
			dirNS: map[string]*model.Namespace{
				"Sources/Domain": model.NewNamespace("Domain", "Sources/Domain"),
			},
			match:  []string{"import Unknown", "Unknown"},
			expect: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveModuleImport(tt.match, tt.dirNS)
			if got != tt.expect {
				t.Errorf("resolveModuleImport(%q) = %q, want %q", tt.match[1], got, tt.expect)
			}
		})
	}
}

func TestExtractLanguageImports_Java(t *testing.T) {
	dir := t.TempDir()

	buildFixture(t, dir, map[string]string{
		"pom.xml":                "<project></project>",
		"src/domain/Entity.java": "package domain;\npublic class Entity {}\n",
		"src/adapter/Repo.java":  "package adapter;\nimport domain.Entity;\npublic class Repo {}\n",
	})

	dirNS := map[string]*model.Namespace{
		"src/domain": {
			Name:       "src/domain",
			ImportPath: "src/domain",
			Files:      []*model.File{model.NewFile("src/domain/Entity.java", "src/domain")},
		},
		"src/adapter": {
			Name:       "src/adapter",
			ImportPath: "src/adapter",
			Files:      []*model.File{model.NewFile("src/adapter/Repo.java", "src/adapter")},
		},
	}

	graph := extractLanguageImports(dir, model.LangJava, dirNS)
	if graph == nil {
		t.Fatal("extractLanguageImports returned nil")
	}

	edges := graph.EdgesFrom("src/adapter")
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge from src/adapter, got %d", len(edges))
	}
	if edges[0].To != "src/domain" {
		t.Errorf("edge target = %q, want %q", edges[0].To, "src/domain")
	}
	if edges[0].External {
		t.Error("edge should not be external")
	}
}

func TestExtractLanguageImports_Kotlin(t *testing.T) {
	dir := t.TempDir()

	buildFixture(t, dir, map[string]string{
		"build.gradle.kts":     "plugins { kotlin(\"jvm\") }",
		"src/domain/Entity.kt": "package domain\ndata class Entity(val id: String)\n",
		"src/adapter/Repo.kt":  "package adapter\nimport domain.Entity\nclass Repo {}\n",
	})

	dirNS := map[string]*model.Namespace{
		"src/domain": {
			Name:       "src/domain",
			ImportPath: "src/domain",
			Files:      []*model.File{model.NewFile("src/domain/Entity.kt", "src/domain")},
		},
		"src/adapter": {
			Name:       "src/adapter",
			ImportPath: "src/adapter",
			Files:      []*model.File{model.NewFile("src/adapter/Repo.kt", "src/adapter")},
		},
	}

	graph := extractLanguageImports(dir, model.LangKotlin, dirNS)
	if graph == nil {
		t.Fatal("extractLanguageImports returned nil")
	}

	edges := graph.EdgesFrom("src/adapter")
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge from src/adapter, got %d", len(edges))
	}
	if edges[0].To != "src/domain" {
		t.Errorf("edge target = %q, want %q", edges[0].To, "src/domain")
	}
	if edges[0].External {
		t.Error("edge should not be external")
	}
}

func TestExtractLanguageImports_CSharp(t *testing.T) {
	dir := t.TempDir()

	buildFixture(t, dir, map[string]string{
		"Project.csproj":   "<Project></Project>",
		"Domain/Entity.cs": "namespace Domain { public class Entity {} }\n",
		"Adapter/Repo.cs":  "using Domain;\nnamespace Adapter { public class Repo {} }\n",
	})

	dirNS := map[string]*model.Namespace{
		"Domain": {
			Name:       "Domain",
			ImportPath: "Domain",
			Files:      []*model.File{model.NewFile("Domain/Entity.cs", "Domain")},
		},
		"Adapter": {
			Name:       "Adapter",
			ImportPath: "Adapter",
			Files:      []*model.File{model.NewFile("Adapter/Repo.cs", "Adapter")},
		},
	}

	graph := extractLanguageImports(dir, model.LangCSharp, dirNS)
	if graph == nil {
		t.Fatal("extractLanguageImports returned nil")
	}

	edges := graph.EdgesFrom("Adapter")
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge from Adapter, got %d", len(edges))
	}
	if edges[0].To != "Domain" {
		t.Errorf("edge target = %q, want %q", edges[0].To, "Domain")
	}
	if edges[0].External {
		t.Error("edge should not be external")
	}
}

func TestResolveToNamespace_Python(t *testing.T) {
	pkgSet := map[string]bool{
		"domain": true,
	}

	tests := []struct {
		name      string
		importKey string
		expect    string
	}{
		{
			name:      "exact match",
			importKey: "domain",
			expect:    "domain",
		},
		{
			name:      "prefix match",
			importKey: "domain/entity",
			expect:    "domain",
		},
		{
			name:      "no match returns input with dots",
			importKey: "unknown",
			expect:    "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveToNamespace(tt.importKey, pkgSet)
			if got != tt.expect {
				t.Errorf("resolveToNamespace(%q) = %q, want %q", tt.importKey, got, tt.expect)
			}
		})
	}
}

func TestRequireEdgeDetection_JavaScript(t *testing.T) {
	dir := t.TempDir()

	buildFixture(t, dir, map[string]string{
		"package.json":           `{"name":"test"}`,
		"src/domain/entity.js":   "class Entity {}\nmodule.exports = { Entity };\n",
		"src/adapter/handler.js": "const { Entity } = require('../domain/entity');\nclass Handler {}\n",
	})

	sc := &TypeScriptScanner{}
	proj, err := sc.Scan(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	if proj.DependencyGraph == nil {
		t.Fatal("dependency graph is nil")
	}

	edges := proj.DependencyGraph.EdgesFrom("src/adapter")
	found := false
	for _, e := range edges {
		if e.To == "src/domain" && !e.External {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("missing internal edge src/adapter -> src/domain; edges: %+v", edges)
	}
}
