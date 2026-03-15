package analysis

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/python"

	"github.com/dpopsuev/locus/internal/model"
	"github.com/dpopsuev/locus/internal/survey"
)

// PythonDeepAnalyzer uses tree-sitter-python for call graph analysis.
type PythonDeepAnalyzer struct {
	root string
}

// NewPythonDeep creates a PythonDeepAnalyzer. Returns nil for non-Python projects.
func NewPythonDeep(root string) *PythonDeepAnalyzer {
	if survey.DetectLanguage(root) != model.LangPython {
		return nil
	}
	return &PythonDeepAnalyzer{root: root}
}

type pyFunc struct {
	name    string
	pkg     string
	line    int
	callees []string
}

func (a *PythonDeepAnalyzer) CallGraph(_ string, opts CallGraphOpts) (*CallGraph, error) {
	depth := opts.Depth
	if depth <= 0 {
		depth = DefaultCallGraphDepth
	}

	funcs, err := a.parseFunctions()
	if err != nil {
		return nil, err
	}

	funcIndex := make(map[string]*pyFunc)
	for i := range funcs {
		funcIndex[funcs[i].name] = &funcs[i]
	}

	var roots []string
	if opts.Entry != "" {
		roots = []string{opts.Entry}
	} else {
		for _, f := range funcs {
			if opts.Scope != "" && !strings.HasPrefix(f.pkg, opts.Scope) {
				continue
			}
			if opts.ExportedOnly && strings.HasPrefix(f.name, "_") {
				continue
			}
			if !strings.HasPrefix(f.name, "_") {
				roots = append(roots, f.name)
			}
		}
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
				Caller: fn.name, Callee: cf.name,
				CallerPkg: fn.pkg, CalleePkg: cf.pkg,
				CrossPkg: fn.pkg != cf.pkg,
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
	return &CallGraph{Nodes: nodes, Edges: edges, Layer: LayerPython}, nil
}

func (a *PythonDeepAnalyzer) DataFlowTrace(_ string, entry string, maxDepth int) (*DataFlow, error) {
	if maxDepth <= 0 {
		maxDepth = DefaultDataFlowDepth
	}
	funcs, err := a.parseFunctions()
	if err != nil {
		return nil, err
	}
	funcIndex := make(map[string]*pyFunc)
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
	return &DataFlow{Nodes: nodes, Edges: edges, Layer: LayerPython}, nil
}

func (a *PythonDeepAnalyzer) DetectStateMachines(_ string) ([]StateMachine, error) {
	return nil, nil
}

func (a *PythonDeepAnalyzer) parseFunctions() ([]pyFunc, error) {
	parser := sitter.NewParser()
	parser.SetLanguage(python.GetLanguage())

	absRoot, err := filepath.Abs(a.root)
	if err != nil {
		return nil, err
	}

	var funcs []pyFunc

	err = filepath.WalkDir(absRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if survey.ShouldSkipPythonDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".py") {
			return nil
		}
		src, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		tree, parseErr := parser.ParseCtx(context.Background(), nil, src)
		if parseErr != nil {
			return nil
		}

		rel, _ := filepath.Rel(absRoot, path)
		pkg := filepath.ToSlash(filepath.Dir(rel))
		if pkg == "." {
			pkg = "(root)"
		}

		extractPyFunctions(tree.RootNode(), src, pkg, &funcs)
		return nil
	})
	return funcs, err
}

func extractPyFunctions(root *sitter.Node, src []byte, pkg string, funcs *[]pyFunc) {
	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(i)
		if child.Type() == "function_definition" || child.Type() == "async_function_definition" {
			name := ""
			if nameNode := child.ChildByFieldName("name"); nameNode != nil {
				name = nameNode.Content(src)
			}
			if name == "" {
				continue
			}
			body := child.ChildByFieldName("body")
			var callees []string
			if body != nil {
				callees = extractPyCallees(body, src)
			}
			*funcs = append(*funcs, pyFunc{
				name:    name,
				pkg:     pkg,
				line:    int(child.StartPoint().Row) + 1,
				callees: callees,
			})
		}
		// Recurse into class definitions to find methods.
		if child.Type() == "class_definition" {
			if bodyNode := child.ChildByFieldName("body"); bodyNode != nil {
				extractPyFunctions(bodyNode, src, pkg, funcs)
			}
		}
	}
}

func extractPyCallees(node *sitter.Node, src []byte) []string {
	seen := make(map[string]bool)
	var callees []string
	collectPyCalls(node, src, seen, &callees)
	return callees
}

func collectPyCalls(node *sitter.Node, src []byte, seen map[string]bool, callees *[]string) {
	if node.Type() == "call" {
		fn := node.ChildByFieldName("function")
		if fn != nil {
			var name string
			switch fn.Type() {
			case "identifier":
				name = fn.Content(src)
			case "attribute":
				if attr := fn.ChildByFieldName("attribute"); attr != nil {
					name = attr.Content(src)
				}
			}
			if name != "" && !seen[name] {
				seen[name] = true
				*callees = append(*callees, name)
			}
		}
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		collectPyCalls(node.Child(i), src, seen, callees)
	}
}
