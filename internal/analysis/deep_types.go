package analysis

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/golang"
)

// ParsedFile holds a pre-parsed source file with its AST, source bytes,
// package name, and relative path. Created once by BuildParsedProject
// and reused by all DeepAnalyzer queries without redundant I/O.
type ParsedFile struct {
	Tree    *sitter.Tree
	Source  []byte
	Package string
	RelPath string
}

// ParsedProject caches parsed ASTs for an entire Go repository.
// It enables "parse once, query many" — all DeepAnalyzer methods
// iterate over Files instead of re-walking the filesystem.
type ParsedProject struct {
	Root  string
	Files []ParsedFile
}

// BuildParsedProject walks root once, reads and parses every non-test .go
// file, and returns a ParsedProject. This is the "Divide" step in D&C:
// one filesystem walk, N parallel parses.
func BuildParsedProject(root string) (*ParsedProject, error) {
	parser := sitter.NewParser()
	parser.SetLanguage(golang.GetLanguage())

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	var files []ParsedFile
	err = filepath.WalkDir(absRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			base := d.Name()
			if base == "vendor" || base == "testdata" || strings.HasPrefix(base, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		tree, err := parser.ParseCtx(context.Background(), nil, src)
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(absRoot, path)
		pkg := filepath.Dir(rel)
		if pkg == "." {
			pkg = "(root)"
		}
		pkg = filepath.ToSlash(pkg)
		files = append(files, ParsedFile{
			Tree:    tree,
			Source:  src,
			Package: pkg,
			RelPath: rel,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &ParsedProject{Root: absRoot, Files: files}, nil
}

// CallGraphOpts configures call graph construction.
type CallGraphOpts struct {
	Entry        string // entry function name; empty = all exported
	Depth        int    // max recursion depth; 0 = default (10)
	ExportedOnly bool   // only include exported functions as roots
	Scope        string // limit to this package prefix
}

// CallEdge represents a single caller->callee edge in the call graph.
type CallEdge struct {
	Caller       string `json:"caller"`
	Callee       string `json:"callee"`
	CallerPkg    string `json:"caller_pkg"`
	CalleePkg    string `json:"callee_pkg"`
	Line         int    `json:"line,omitempty"`
	File         string `json:"file,omitempty"`
	ReceiverType string `json:"receiver_type,omitempty"`
	CrossPkg     bool   `json:"cross_pkg,omitempty"`
}

// FuncNode represents a function in the call graph.
type FuncNode struct {
	Name    string `json:"name"`
	Package string `json:"package"`
	Line    int    `json:"line,omitempty"`
}

// CallGraph is the result of call graph analysis.
type CallGraph struct {
	Nodes []FuncNode `json:"nodes"`
	Edges []CallEdge `json:"edges"`
	Layer string     `json:"layer,omitempty"`
}

// DataFlowNode represents a participant in a data flow.
type DataFlowNode struct {
	Name string `json:"name"`
	Kind string `json:"kind"` // "process", "data_store", "external", "entry"
	Pkg  string `json:"package,omitempty"`
}

// DataFlowEdge represents data moving between nodes.
type DataFlowEdge struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Label string `json:"label,omitempty"`
}

// TrustBoundary defines a security boundary containing nodes.
type TrustBoundary struct {
	Name  string   `json:"name"`
	Nodes []string `json:"nodes"`
}

// DataFlow is the result of data flow analysis.
type DataFlow struct {
	Nodes      []DataFlowNode  `json:"nodes"`
	Edges      []DataFlowEdge  `json:"edges"`
	Boundaries []TrustBoundary `json:"boundaries,omitempty"`
	Layer      string          `json:"layer,omitempty"`
}

// StateTransition represents a transition between states.
type StateTransition struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Trigger string `json:"trigger,omitempty"`
}

// StateMachine represents a detected state machine pattern.
type StateMachine struct {
	Name        string            `json:"name"`
	Package     string            `json:"package"`
	States      []string          `json:"states"`
	Transitions []StateTransition `json:"transitions"`
	Initial     string            `json:"initial,omitempty"`
}
