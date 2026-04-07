package oculus_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dpopsuev/locus/internal/oculus"
)

func TestTypeScriptDeepCallGraph(t *testing.T) {
	dir := t.TempDir()

	files := map[string]string{
		"package.json": `{"name": "testapp"}`,
		"src/main.ts": `
export function main() {
    const data = fetchData()
    const result = processData(data)
    sendResult(result)
}

function fetchData(): number[] {
    return [1, 2, 3]
}

function processData(data: number[]): number[] {
    return transform(data)
}

function transform(data: number[]): number[] {
    return data.map(x => x * 2)
}

function sendResult(result: number[]) {
    console.log(result)
}
`,
	}

	for name, content := range files {
		p := filepath.Join(dir, name)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
	}

	a := oculus.NewTypeScriptDeep(dir)
	if a == nil {
		t.Fatal("expected TypeScriptDeepAnalyzer for TS project")
	}

	cg, err := a.CallGraph(dir, oculus.CallGraphOpts{Entry: "main", Depth: 5})
	if err != nil {
		t.Fatalf("CallGraph: %v", err)
	}
	if cg.Layer != oculus.LayerTypeScript {
		t.Errorf("layer = %q, want typescript", cg.Layer)
	}
	if len(cg.Nodes) == 0 {
		t.Error("expected nodes")
	}
	if len(cg.Edges) == 0 {
		t.Error("expected edges")
	}

	callees := make(map[string][]string)
	for _, e := range cg.Edges {
		callees[e.Caller] = append(callees[e.Caller], e.Callee)
	}

	if _, ok := callees["main"]; !ok {
		t.Error("expected main in call graph")
	}

	t.Logf("TypeScript CallGraph: %d nodes, %d edges", len(cg.Nodes), len(cg.Edges))
}

func TestTypeScriptDeep_ArrowFunctions(t *testing.T) {
	dir := t.TempDir()

	files := map[string]string{
		"tsconfig.json": `{}`,
		"index.ts": `
const greet = (name: string) => {
    return formatName(name)
}

const formatName = (name: string) => {
    return name.toUpperCase()
}
`,
	}

	for name, content := range files {
		p := filepath.Join(dir, name)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
	}

	a := oculus.NewTypeScriptDeep(dir)
	if a == nil {
		t.Fatal("expected TypeScriptDeepAnalyzer")
	}

	cg, err := a.CallGraph(dir, oculus.CallGraphOpts{Entry: "greet", Depth: 3})
	if err != nil {
		t.Fatalf("CallGraph: %v", err)
	}

	found := false
	for _, e := range cg.Edges {
		if e.Caller == "greet" && e.Callee == "formatName" {
			found = true
		}
	}
	if !found {
		t.Error("expected edge greet -> formatName")
	}
	t.Logf("Arrow function CallGraph: %d nodes, %d edges", len(cg.Nodes), len(cg.Edges))
}

func TestTypeScriptDeep_NonTSRepo(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n"), 0o644)

	a := oculus.NewTypeScriptDeep(dir)
	if a != nil {
		t.Error("expected nil for non-TS repo")
	}
}
