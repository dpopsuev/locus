package analysis

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/typescript/typescript"

	"github.com/dpopsuev/locus/internal/model"
	"github.com/dpopsuev/locus/internal/survey"
)

// TypeScriptDeepAnalyzer uses tree-sitter-typescript for call graph analysis.
type TypeScriptDeepAnalyzer struct {
	root string
}

// NewTypeScriptDeep creates a TypeScriptDeepAnalyzer. Returns nil for non-TS projects.
func NewTypeScriptDeep(root string) *TypeScriptDeepAnalyzer {
	if survey.DetectLanguage(root) != model.LangTypeScript {
		return nil
	}
	return &TypeScriptDeepAnalyzer{root: root}
}

type tsFunc struct {
	name    string
	pkg     string
	line    int
	callees []string
}

func (a *TypeScriptDeepAnalyzer) CallGraph(_ string, opts CallGraphOpts) (*CallGraph, error) {
	depth := opts.Depth
	if depth <= 0 {
		depth = DefaultCallGraphDepth
	}

	funcs, err := a.parseFunctions()
	if err != nil {
		return nil, err
	}

	funcIndex := make(map[string]*tsFunc)
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
			roots = append(roots, f.name)
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
	return &CallGraph{Nodes: nodes, Edges: edges, Layer: LayerTypeScript}, nil
}

func (a *TypeScriptDeepAnalyzer) DataFlowTrace(_ string, entry string, maxDepth int) (*DataFlow, error) {
	if maxDepth <= 0 {
		maxDepth = DefaultDataFlowDepth
	}
	funcs, err := a.parseFunctions()
	if err != nil {
		return nil, err
	}
	funcIndex := make(map[string]*tsFunc)
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
	return &DataFlow{Nodes: nodes, Edges: edges, Layer: LayerTypeScript}, nil
}

func (a *TypeScriptDeepAnalyzer) DetectStateMachines(_ string) ([]StateMachine, error) {
	return nil, nil
}

func (a *TypeScriptDeepAnalyzer) parseFunctions() ([]tsFunc, error) {
	parser := sitter.NewParser()
	parser.SetLanguage(typescript.GetLanguage())

	absRoot, err := filepath.Abs(a.root)
	if err != nil {
		return nil, err
	}

	var funcs []tsFunc

	err = filepath.WalkDir(absRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if survey.ShouldSkipTSDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(d.Name())
		if ext != ".ts" && ext != ".tsx" && ext != ".js" && ext != ".jsx" {
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

		extractTSFunctions(tree.RootNode(), src, pkg, &funcs)
		return nil
	})
	return funcs, err
}

func extractTSFunctions(root *sitter.Node, src []byte, pkg string, funcs *[]tsFunc) {
	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(i)
		switch child.Type() {
		case "function_declaration", "method_definition":
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
				callees = extractTSCallees(body, src)
			}
			*funcs = append(*funcs, tsFunc{
				name: name, pkg: pkg,
				line:    int(child.StartPoint().Row) + 1,
				callees: callees,
			})
		case "export_statement", "lexical_declaration":
			// export function foo() or const foo = () =>
			extractTSFunctions(child, src, pkg, funcs)
		case "variable_declarator":
			// const foo = () => { ... }
			nameNode := child.ChildByFieldName("name")
			valueNode := child.ChildByFieldName("value")
			if nameNode != nil && valueNode != nil && isArrowOrFunction(valueNode) {
				name := nameNode.Content(src)
				body := valueNode.ChildByFieldName("body")
				var callees []string
				if body != nil {
					callees = extractTSCallees(body, src)
				}
				*funcs = append(*funcs, tsFunc{
					name: name, pkg: pkg,
					line:    int(child.StartPoint().Row) + 1,
					callees: callees,
				})
			}
		case "class_declaration":
			if bodyNode := child.ChildByFieldName("body"); bodyNode != nil {
				extractTSFunctions(bodyNode, src, pkg, funcs)
			}
		}
	}
}

func isArrowOrFunction(node *sitter.Node) bool {
	t := node.Type()
	return t == "arrow_function" || t == "function" || t == "function_expression"
}

func extractTSCallees(node *sitter.Node, src []byte) []string {
	seen := make(map[string]bool)
	var callees []string
	collectTSCalls(node, src, seen, &callees)
	return callees
}

func collectTSCalls(node *sitter.Node, src []byte, seen map[string]bool, callees *[]string) {
	if node.Type() == "call_expression" {
		fn := node.ChildByFieldName("function")
		if fn != nil {
			var name string
			switch fn.Type() {
			case "identifier":
				name = fn.Content(src)
			case "member_expression":
				if prop := fn.ChildByFieldName("property"); prop != nil {
					name = prop.Content(src)
				}
			}
			if name != "" && !seen[name] {
				seen[name] = true
				*callees = append(*callees, name)
			}
		}
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		collectTSCalls(node.Child(i), src, seen, callees)
	}
}
