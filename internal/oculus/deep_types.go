package oculus

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

// namedFunc is a minimal function descriptor used by the shared helpers.
type namedFunc struct {
	name    string
	pkg     string
	line    int
	callees []string
}

// buildSimpleCallGraph constructs a call graph from a list of named functions.
// Shared between Python and TypeScript deep analyzers to avoid duplication.
func buildSimpleCallGraph(funcs []namedFunc, roots []string, depth int, layer string) *CallGraph {
	funcIndex := make(map[string]*namedFunc, len(funcs))
	for i := range funcs {
		funcIndex[funcs[i].name] = &funcs[i]
	}

	nodeSet := make(map[string]FuncNode)
	var edges []CallEdge
	visited := make(map[string]bool)

	var walk func(name string, d int)
	walk = func(name string, d int) {
		if d > depth || visited[name] {
			return
		}
		visited[name] = true
		fn, ok := funcIndex[name]
		if !ok {
			return
		}
		key := fn.pkg + "." + fn.name
		nodeSet[key] = FuncNode{Name: fn.name, Package: fn.pkg, Line: fn.line}
		for _, callee := range fn.callees {
			cf, ok := funcIndex[callee]
			if !ok {
				continue
			}
			ck := cf.pkg + "." + cf.name
			nodeSet[ck] = FuncNode{Name: cf.name, Package: cf.pkg, Line: cf.line}
			edges = append(edges, CallEdge{
				Caller:    fn.name,
				Callee:    cf.name,
				CallerPkg: fn.pkg,
				CalleePkg: cf.pkg,
				CrossPkg:  fn.pkg != cf.pkg,
			})
			walk(callee, d+1)
		}
	}
	for _, r := range roots {
		walk(r, 0)
	}

	nodes := make([]FuncNode, 0, len(nodeSet))
	for _, n := range nodeSet {
		nodes = append(nodes, n)
	}
	return &CallGraph{Nodes: nodes, Edges: edges, Layer: layer}
}

// dataFlowTrace is a shared implementation for DataFlowTrace across deep analyzers.
// It avoids duplication between GoAST, Python, and TypeScript deep analyzers.
func dataFlowTrace(funcs []namedFunc, entry string, maxDepth int, layer string) *DataFlow {
	funcIndex := make(map[string]*namedFunc, len(funcs))
	for i := range funcs {
		funcIndex[funcs[i].name] = &funcs[i]
	}

	nodeMap := make(map[string]DataFlowNode)
	var edges []DataFlowEdge
	visited := make(map[string]bool)
	nodeMap[entry] = DataFlowNode{Name: entry, Kind: "entry"}

	var trace func(name string, d int)
	trace = func(name string, d int) {
		if d > maxDepth || visited[name] {
			return
		}
		visited[name] = true
		fn, ok := funcIndex[name]
		if !ok {
			return
		}
		for _, callee := range fn.callees {
			if _, ok := funcIndex[callee]; !ok {
				continue
			}
			if _, exists := nodeMap[callee]; !exists {
				nodeMap[callee] = DataFlowNode{Name: callee, Kind: "process", Pkg: funcIndex[callee].pkg}
			}
			edges = append(edges, DataFlowEdge{From: name, To: callee})
			trace(callee, d+1)
		}
	}
	trace(entry, 0)

	nodes := make([]DataFlowNode, 0, len(nodeMap))
	for _, n := range nodeMap {
		nodes = append(nodes, n)
	}
	return &DataFlow{Nodes: nodes, Edges: edges, Layer: layer}
}

// collectTreeSitterCalls walks a tree-sitter node tree collecting function call names.
// Shared between Python and TypeScript deep analyzers.
func collectTreeSitterCalls(node *sitter.Node, src []byte, callType, funcField string,
	nameExtractor func(fn *sitter.Node, src []byte) string,
	seen map[string]bool, callees *[]string,
) {
	if node.Type() == callType {
		fn := node.ChildByFieldName(funcField)
		if fn != nil {
			name := nameExtractor(fn, src)
			if name != "" && !seen[name] {
				seen[name] = true
				*callees = append(*callees, name)
			}
		}
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		collectTreeSitterCalls(node.Child(i), src, callType, funcField, nameExtractor, seen, callees)
	}
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
			if base == dirVendor || base == dirTestdata || strings.HasPrefix(base, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != extGo || strings.HasSuffix(path, "_test.go") {
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
			pkg = pkgRoot
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

// Type definitions moved to internal/oculus/types.go.
// Re-exported via deep.go type aliases for backward compatibility.
