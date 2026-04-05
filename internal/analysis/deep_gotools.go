package analysis

import (
	"fmt"
	"go/token"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/callgraph/cha"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"

	"github.com/dpopsuev/locus/internal/model"
	"github.com/dpopsuev/locus/internal/survey"
)

const LayerGoTools = "gotools"

// GoToolsDeepAnalyzer uses golang.org/x/tools/go/callgraph for precise
// call graph analysis via Class Hierarchy Analysis (CHA). More accurate
// than AST walking for interface dispatch and method sets. Slower due to
// full type-checking.
type GoToolsDeepAnalyzer struct {
	root string
}

// NewGoToolsDeep creates a GoToolsDeepAnalyzer. Returns nil if not a Go project.
func NewGoToolsDeep(root string) *GoToolsDeepAnalyzer {
	if survey.DetectLanguage(root) != model.LangGo {
		return nil
	}
	return &GoToolsDeepAnalyzer{root: root}
}

func (a *GoToolsDeepAnalyzer) CallGraph(_ string, opts CallGraphOpts) (*CallGraph, error) {
	absRoot, err := filepath.Abs(a.root)
	if err != nil {
		return nil, err
	}

	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo | packages.NeedDeps,
		Dir: absRoot,
	}
	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		return nil, fmt.Errorf("gotools: load packages: %w", err)
	}

	// Build SSA for all packages.
	prog, created := ssautil.AllPackages(pkgs, ssa.InstantiateGenerics)
	prog.Build()

	// Run CHA (Class Hierarchy Analysis) — fast, doesn't need entry points.
	cg := cha.CallGraph(prog)

	// Convert gonum call graph to Locus format.
	nodeSet := make(map[string]FuncNode)
	var edges []CallEdge

	for fn, node := range cg.Nodes {
		if fn == nil || fn.Synthetic != "" {
			continue
		}
		pos := prog.Fset.Position(fn.Pos())
		callerPkg := funcPackage(fn, absRoot, created)

		if opts.Scope != "" && !strings.HasPrefix(callerPkg, opts.Scope) {
			continue
		}
		if opts.ExportedOnly && !token.IsExported(fn.Name()) {
			continue
		}

		callerKey := callerPkg + "." + fn.Name()
		nodeSet[callerKey] = FuncNode{
			Name:    fn.Name(),
			Package: callerPkg,
			Line:    pos.Line,
		}

		for _, edge := range node.Out {
			callee := edge.Callee.Func
			if callee == nil || callee.Synthetic != "" {
				continue
			}
			calleePkg := funcPackage(callee, absRoot, created)
			calleeKey := calleePkg + "." + callee.Name()
			calleePos := prog.Fset.Position(callee.Pos())

			nodeSet[calleeKey] = FuncNode{
				Name:    callee.Name(),
				Package: calleePkg,
				Line:    calleePos.Line,
			}
			edges = append(edges, CallEdge{
				Caller:    fn.Name(),
				Callee:    callee.Name(),
				CallerPkg: callerPkg,
				CalleePkg: calleePkg,
				Line:      pos.Line,
				CrossPkg:  callerPkg != calleePkg,
			})
		}
	}

	nodes := make([]FuncNode, 0, len(nodeSet))
	for _, n := range nodeSet {
		nodes = append(nodes, n)
	}
	return &CallGraph{Nodes: nodes, Edges: edges, Layer: LayerGoTools}, nil
}

func (a *GoToolsDeepAnalyzer) DataFlowTrace(root, entry string, maxDepth int) (*DataFlow, error) {
	// Delegate to GoAST for data flow — CHA doesn't provide data flow info.
	goast := NewGoASTDeep(a.root)
	if goast == nil {
		return &DataFlow{Layer: LayerGoTools}, nil
	}
	return goast.DataFlowTrace(root, entry, maxDepth)
}

func (a *GoToolsDeepAnalyzer) DetectStateMachines(root string) ([]StateMachine, error) {
	// Delegate to GoAST — state machines are AST pattern matching, not call graph.
	goast := NewGoASTDeep(a.root)
	if goast == nil {
		return nil, nil
	}
	return goast.DetectStateMachines(root)
}

// funcPackage derives the Locus-style relative package path for an SSA function.
func funcPackage(fn *ssa.Function, absRoot string, _ []*ssa.Package) string {
	if fn.Package() == nil {
		return pkgRoot
	}

	// Try to derive relative path from the function's file position.
	rel := funcRelDir(fn, absRoot)
	if rel != "" {
		return rel
	}

	// Fallback: use the last path component of the package path.
	pkgPath := fn.Package().Pkg.Path()
	parts := strings.Split(pkgPath, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return pkgRoot
}

// funcRelDir derives a relative directory from a function's source position.
func funcRelDir(fn *ssa.Function, absRoot string) string {
	for _, m := range fn.Package().Members {
		pos := fn.Prog.Fset.Position(m.Pos())
		if pos.Filename == "" {
			continue
		}
		dir := filepath.Dir(pos.Filename)
		if rel, err := filepath.Rel(absRoot, dir); err == nil {
			return filepath.ToSlash(rel)
		}
	}
	return ""
}
