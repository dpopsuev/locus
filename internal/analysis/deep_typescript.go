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

	nf := make([]namedFunc, len(funcs))
	for i, f := range funcs {
		nf[i] = namedFunc(f)
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

	return buildSimpleCallGraph(nf, roots, depth, LayerTypeScript), nil
}

func (a *TypeScriptDeepAnalyzer) DataFlowTrace(_, entry string, maxDepth int) (*DataFlow, error) {
	if maxDepth <= 0 {
		maxDepth = DefaultDataFlowDepth
	}
	funcs, err := a.parseFunctions()
	if err != nil {
		return nil, err
	}

	nf := make([]namedFunc, len(funcs))
	for i, f := range funcs {
		nf[i] = namedFunc(f)
	}
	return dataFlowTrace(nf, entry, maxDepth, LayerTypeScript), nil
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
		if ext != extTS && ext != extTSX && ext != extJS && ext != extJSX {
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
			pkg = pkgRoot
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
	collectTreeSitterCalls(node, src, "call_expression", "function", tsNameExtractor, seen, callees)
}

func tsNameExtractor(fn *sitter.Node, src []byte) string {
	switch fn.Type() {
	case "identifier":
		return fn.Content(src)
	case "member_expression":
		if prop := fn.ChildByFieldName("property"); prop != nil {
			return prop.Content(src)
		}
	}
	return ""
}
