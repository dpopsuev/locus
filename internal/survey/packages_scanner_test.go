package survey_test

import (
	"testing"

	"github.com/dpopsuev/locus/internal/model"
	"github.com/dpopsuev/locus/internal/survey"
)

func TestPackagesScannerExtractsNamespaces(t *testing.T) {
	dir := setupModule(t, map[string]string{
		"go.mod":     "module example.com/ps\n\ngo 1.21\n",
		"main.go":    "package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println(\"hi\") }\n",
		"lib/lib.go": "package lib\n\nfunc Hello() string { return \"hi\" }\n",
	})

	sc := &survey.PackagesScanner{}
	proj, err := sc.Scan(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	if proj.Language != model.LangGo {
		t.Errorf("language = %v, want LangGo", proj.Language)
	}

	if len(proj.Namespaces) < 2 {
		t.Fatalf("namespaces = %d, want >= 2", len(proj.Namespaces))
	}

	nsMap := make(map[string]*model.Namespace)
	for _, ns := range proj.Namespaces {
		nsMap[ns.ImportPath] = ns
	}

	lib, ok := nsMap["example.com/ps/lib"]
	if !ok {
		t.Fatal("missing namespace example.com/ps/lib")
	}
	if lib.Name != "lib" {
		t.Errorf("lib.name = %q, want lib", lib.Name)
	}
}

func TestPackagesScannerSymbolDependencies(t *testing.T) {
	dir := setupModule(t, map[string]string{
		"go.mod": "module example.com/deps\n\ngo 1.21\n",
		"main.go": `package main

import "fmt"

func hello() { fmt.Println("hi") }

func noImport() {}
`,
	})

	sc := &survey.PackagesScanner{}
	proj, err := sc.Scan(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	if len(proj.Namespaces) == 0 {
		t.Fatal("no namespaces")
	}

	var helloSym, noImportSym *model.Symbol
	for _, ns := range proj.Namespaces {
		for _, s := range ns.Symbols {
			switch s.Name {
			case "hello":
				helloSym = s
			case "noImport":
				noImportSym = s
			}
		}
	}

	if helloSym == nil {
		t.Fatal("missing symbol 'hello'")
	}
	if len(helloSym.Dependencies) == 0 {
		t.Error("hello should have dependencies (uses fmt)")
	}
	hasFmt := false
	for _, dep := range helloSym.Dependencies {
		if dep == "fmt" {
			hasFmt = true
		}
	}
	if !hasFmt {
		t.Errorf("hello.Dependencies = %v, want to contain 'fmt'", helloSym.Dependencies)
	}

	if noImportSym == nil {
		t.Fatal("missing symbol 'noImport'")
	}
	if len(noImportSym.Dependencies) != 0 {
		t.Errorf("noImport.Dependencies = %v, want empty", noImportSym.Dependencies)
	}
}

func TestPackagesScannerBuildsDependencyGraph(t *testing.T) {
	dir := setupModule(t, map[string]string{
		"go.mod": "module example.com/graph\n\ngo 1.21\n",
		"a/a.go": "package a\n\nimport \"example.com/graph/b\"\n\nvar _ = b.Hello\n",
		"b/b.go": "package b\n\nimport \"fmt\"\n\nfunc Hello() { fmt.Println(\"hi\") }\n",
	})

	sc := &survey.PackagesScanner{}
	proj, err := sc.Scan(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	if proj.DependencyGraph == nil {
		t.Fatal("dependency graph is nil")
	}

	aEdges := proj.DependencyGraph.EdgesFrom("example.com/graph/a")
	found := false
	for _, e := range aEdges {
		if e.To == "example.com/graph/b" && !e.External {
			found = true
		}
	}
	if !found {
		t.Error("missing internal edge a -> b")
	}

	bEdges := proj.DependencyGraph.EdgesFrom("example.com/graph/b")
	foundFmt := false
	for _, e := range bEdges {
		if e.To == "fmt" && e.External {
			foundFmt = true
		}
	}
	if !foundFmt {
		t.Error("missing external edge b -> fmt")
	}
}

func TestPackagesScannerFallback(t *testing.T) {
	dir := setupModule(t, map[string]string{
		"go.mod":  "module example.com/fb\n\ngo 1.21\n",
		"main.go": "package main\n\nfunc main() {}\n",
	})

	sc := &survey.PackagesScanner{Fallback: &survey.GoScanner{}}
	proj, err := sc.Scan(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	if proj.Path != "example.com/fb" {
		t.Errorf("path = %q", proj.Path)
	}
}
