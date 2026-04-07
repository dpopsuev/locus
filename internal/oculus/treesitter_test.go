package oculus

import (
	"os"
	"path/filepath"
	"testing"
)

func setupTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	// go.mod for language detection
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/test\n\ngo 1.21\n"), 0o644)

	src := `package main

import "net/http"

type Server struct {
	Addr string
	DB   *Database
}

type Database struct {
	Host string
	Port int
}

type Handler interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}

func main() {
	s := &Server{}
	s.Start()
}

func (s *Server) Start() {
	Listen(s.Addr)
}

func Listen(addr string) {}

func handleRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		for i := 0; i < 10; i++ {
			if i > 5 {
				switch i {
				case 6:
					return
				}
			}
		}
	}
}

func TestSomething(t *testing.T) {}

func init() {}
`
	os.WriteFile(filepath.Join(dir, "main.go"), []byte(src), 0o644)
	return dir
}

func TestTreeSitter_Classes(t *testing.T) {
	dir := setupTestRepo(t)
	ts := &TreeSitterAnalyzer{}
	classes, err := ts.Classes(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(classes) < 3 {
		t.Fatalf("expected at least 3 types, got %d", len(classes))
	}

	found := map[string]bool{}
	for _, c := range classes {
		found[c.Name] = true
		switch c.Name {
		case "Server":
			if c.Kind != "struct" {
				t.Errorf("Server: expected struct, got %s", c.Kind)
			}
			if len(c.Fields) < 2 {
				t.Errorf("Server: expected 2+ fields, got %d", len(c.Fields))
			}
			hasStart := false
			for _, m := range c.Methods {
				if m.Name == "Start" {
					hasStart = true
				}
			}
			if !hasStart {
				t.Error("Server: missing Start method")
			}
		case "Handler":
			if c.Kind != "interface" {
				t.Errorf("Handler: expected interface, got %s", c.Kind)
			}
		}
	}

	for _, name := range []string{"Server", "Database", "Handler"} {
		if !found[name] {
			t.Errorf("missing type %s", name)
		}
	}
}

func TestTreeSitter_Implements(t *testing.T) {
	dir := setupTestRepo(t)
	// Add a struct that embeds another
	src := `package main

type Base struct {
	ID int
}

type Derived struct {
	Base
	Name string
}
`
	os.WriteFile(filepath.Join(dir, "embed.go"), []byte(src), 0o644)

	ts := &TreeSitterAnalyzer{}
	edges, err := ts.Implements(dir)
	if err != nil {
		t.Fatal(err)
	}

	foundEmbed := false
	for _, e := range edges {
		if e.From == "Derived" && e.To == "Base" && e.Kind == "embeds" {
			foundEmbed = true
		}
	}
	if !foundEmbed {
		t.Error("expected Derived embeds Base edge")
	}
}

func TestTreeSitter_FieldRefs(t *testing.T) {
	dir := setupTestRepo(t)
	ts := &TreeSitterAnalyzer{}
	refs, err := ts.FieldRefs(dir)
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, r := range refs {
		if r.Owner == "Server" && r.RefType == "Database" {
			found = true
		}
	}
	if !found {
		t.Error("expected Server->Database field reference")
	}
}

func TestTreeSitter_EntryPoints(t *testing.T) {
	dir := setupTestRepo(t)
	ts := &TreeSitterAnalyzer{}
	entries, err := ts.EntryPoints(dir)
	if err != nil {
		t.Fatal(err)
	}

	kinds := map[string]bool{}
	for _, e := range entries {
		kinds[e.Kind] = true
	}
	if !kinds["main"] {
		t.Error("missing main entry point")
	}
	if !kinds["http_handler"] {
		t.Error("missing http_handler entry point")
	}
	if !kinds["init"] {
		t.Error("missing init entry point")
	}
}

func TestTreeSitter_NestingDepth(t *testing.T) {
	dir := setupTestRepo(t)
	ts := &TreeSitterAnalyzer{}
	results, err := ts.NestingDepth(dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(results) == 0 {
		t.Fatal("no nesting results")
	}

	maxDepth := 0
	for _, r := range results {
		if r.Function == "handleRequest" {
			maxDepth = r.MaxDepth
		}
	}
	if maxDepth < 3 {
		t.Errorf("handleRequest nesting: expected >= 3, got %d", maxDepth)
	}
}

func TestTreeSitter_CallChain(t *testing.T) {
	dir := setupTestRepo(t)
	ts := &TreeSitterAnalyzer{}
	calls, err := ts.CallChain(dir, "main", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) == 0 {
		t.Fatal("no calls from main")
	}
	found := false
	for _, c := range calls {
		if c.Caller == "main" && c.Callee == "Start" {
			found = true
		}
	}
	if !found {
		t.Error("expected main->Start call")
	}
}
