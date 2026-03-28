package analysis_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dpopsuev/locus/internal/analysis"
)

func TestPythonDeepCallGraph(t *testing.T) {
	dir := t.TempDir()

	files := map[string]string{
		"pyproject.toml":  "[project]\nname = \"testapp\"\n",
		"app/__init__.py": "",
		"app/main.py": `
def main():
    result = process_data()
    send_result(result)

def process_data():
    data = fetch_data()
    return transform(data)

def fetch_data():
    return [1, 2, 3]

def transform(data):
    return [x * 2 for x in data]

def send_result(result):
    print(result)
`,
	}

	for name, content := range files {
		p := filepath.Join(dir, name)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
	}

	a := analysis.NewPythonDeep(dir)
	if a == nil {
		t.Fatal("expected PythonDeepAnalyzer for Python project")
	}

	cg, err := a.CallGraph(dir, analysis.CallGraphOpts{Entry: "main", Depth: 5})
	if err != nil {
		t.Fatalf("CallGraph: %v", err)
	}
	if cg.Layer != analysis.LayerPython {
		t.Errorf("layer = %q, want python", cg.Layer)
	}
	if len(cg.Nodes) == 0 {
		t.Error("expected nodes")
	}
	if len(cg.Edges) == 0 {
		t.Error("expected edges")
	}

	// Verify the call chain: main -> process_data -> fetch_data, transform
	callees := make(map[string][]string)
	for _, e := range cg.Edges {
		callees[e.Caller] = append(callees[e.Caller], e.Callee)
	}

	if _, ok := callees["main"]; !ok {
		t.Error("expected main in call graph")
	}

	t.Logf("Python CallGraph: %d nodes, %d edges", len(cg.Nodes), len(cg.Edges))
}

func TestPythonDeep_NonPythonRepo(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n"), 0o644)

	a := analysis.NewPythonDeep(dir)
	if a != nil {
		t.Error("expected nil for non-Python repo")
	}
}
