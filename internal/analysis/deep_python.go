package analysis

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/python"

	"github.com/dpopsuev/locus/internal/oculus/lang"
)

// PythonDeepAnalyzer uses tree-sitter-python for call graph analysis.
type PythonDeepAnalyzer struct {
	root string
}

// NewPythonDeep creates a PythonDeepAnalyzer. Returns nil for non-Python projects.
func NewPythonDeep(root string) *PythonDeepAnalyzer {
	if lang.DetectLanguage(root) != lang.Python {
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
			if opts.ExportedOnly && strings.HasPrefix(f.name, "_") {
				continue
			}
			if !strings.HasPrefix(f.name, "_") {
				roots = append(roots, f.name)
			}
		}
	}

	return buildSimpleCallGraph(nf, roots, depth, LayerPython), nil
}

func (a *PythonDeepAnalyzer) DataFlowTrace(_, entry string, maxDepth int) (*DataFlow, error) {
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
	return dataFlowTrace(nf, entry, maxDepth, LayerPython), nil
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
			if lang.ShouldSkipPythonDir(d.Name()) {
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
			pkg = pkgRoot
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
	collectTreeSitterCalls(node, src, "call", "function", pyNameExtractor, seen, callees)
}

func pyNameExtractor(fn *sitter.Node, src []byte) string {
	switch fn.Type() {
	case "identifier":
		return fn.Content(src)
	case "attribute":
		if attr := fn.ChildByFieldName("attribute"); attr != nil {
			return attr.Content(src)
		}
	}
	return ""
}
