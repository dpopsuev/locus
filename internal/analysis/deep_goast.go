package analysis

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"github.com/dpopsuev/locus/internal/model"
	"github.com/dpopsuev/locus/internal/survey"
)

// GoASTDeepAnalyzer uses go/ast for call graph, data flow, and state machine
// analysis. More accurate than regex, no external tools required.
type GoASTDeepAnalyzer struct {
	root string
}

// NewGoASTDeep creates a GoASTDeepAnalyzer for the given root directory.
// Returns nil if the root is not a Go project.
func NewGoASTDeep(root string) *GoASTDeepAnalyzer {
	if survey.DetectLanguage(root) != model.LangGo {
		return nil
	}
	return &GoASTDeepAnalyzer{root: root}
}

type goFunc struct {
	name         string
	pkg          string
	file         string
	line         int
	receiverType string   // non-empty for methods (e.g., "*APIDriver")
	callees      []string // function names called in the body
	body         *ast.BlockStmt
}

func (a *GoASTDeepAnalyzer) CallGraph(_ string, opts CallGraphOpts) (*CallGraph, error) {
	depth := opts.Depth
	if depth <= 0 {
		depth = DefaultCallGraphDepth
	}

	funcs, err := a.parseFunctions(opts.Scope)
	if err != nil {
		return nil, err
	}

	// Build index by function name.
	funcIndex := make(map[string]*goFunc)
	for i := range funcs {
		funcIndex[funcs[i].name] = &funcs[i]
	}

	// Determine root functions.
	var roots []string
	if opts.Entry != "" {
		roots = []string{opts.Entry}
	} else {
		for _, f := range funcs {
			if opts.Scope != "" && !strings.HasPrefix(f.pkg, opts.Scope) {
				continue
			}
			if opts.ExportedOnly && !ast.IsExported(f.name) {
				continue
			}
			if ast.IsExported(f.name) {
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
			calleeFn, ok := funcIndex[callee]
			if !ok {
				continue
			}
			calleeKey := calleeFn.pkg + "." + calleeFn.name
			nodeSet[calleeKey] = FuncNode{Name: calleeFn.name, Package: calleeFn.pkg, Line: calleeFn.line}
			edges = append(edges, CallEdge{
				Caller:       fn.name,
				Callee:       calleeFn.name,
				CallerPkg:    fn.pkg,
				CalleePkg:    calleeFn.pkg,
				Line:         fn.line,
				File:         fn.file,
				ReceiverType: fn.receiverType,
				CrossPkg:     fn.pkg != calleeFn.pkg,
			})
			walk(callee, d+1)
		}
	}

	for _, root := range roots {
		walk(root, 0)
	}

	nodes := make([]FuncNode, 0, len(nodeSet))
	for _, n := range nodeSet {
		nodes = append(nodes, n)
	}
	return &CallGraph{Nodes: nodes, Edges: edges, Layer: LayerGoAST}, nil
}

func (a *GoASTDeepAnalyzer) DataFlowTrace(_, entry string, maxDepth int) (*DataFlow, error) {
	if maxDepth <= 0 {
		maxDepth = DefaultDataFlowDepth
	}

	funcs, err := a.parseFunctions("") // DataFlowTrace needs full graph
	if err != nil {
		return nil, err
	}

	nf := make([]namedFunc, len(funcs))
	for i, f := range funcs {
		nf[i] = namedFunc{name: f.name, pkg: f.pkg, line: f.line, callees: f.callees}
	}
	return dataFlowTrace(nf, entry, maxDepth, LayerGoAST), nil
}

func (a *GoASTDeepAnalyzer) DetectStateMachines(_ string) ([]StateMachine, error) {
	fset := token.NewFileSet()
	absRoot, _ := filepath.Abs(a.root)

	var machines []StateMachine

	_ = filepath.WalkDir(absRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if survey.ShouldSkipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != extGo || strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}

		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return nil
		}

		rel, _ := filepath.Rel(absRoot, path)
		pkg := filepath.ToSlash(filepath.Dir(rel))
		if pkg == "." {
			pkg = pkgRoot
		}

		for _, decl := range f.Decls {
			if sm := parseIotaConstGroup(decl, f, pkg); sm != nil {
				machines = append(machines, *sm)
			}
		}
		return nil
	})

	return machines, nil
}

// parseIotaConstGroup checks if a declaration is an iota-based const group
// and returns a StateMachine if so.
func parseIotaConstGroup(decl ast.Decl, f *ast.File, pkg string) *StateMachine {
	gd, ok := decl.(*ast.GenDecl)
	if !ok || gd.Tok != token.CONST || len(gd.Specs) < 3 {
		return nil
	}

	var typeName string
	var values []string
	hasIota := false

	for _, spec := range gd.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for _, name := range vs.Names {
			values = append(values, name.Name)
		}
		if vs.Type != nil {
			if ident, ok := vs.Type.(*ast.Ident); ok {
				typeName = ident.Name
			}
		}
		for _, v := range vs.Values {
			if ident, ok := v.(*ast.Ident); ok && ident.Name == "iota" {
				hasIota = true
			}
		}
	}

	if !hasIota || len(values) < 3 {
		return nil
	}
	if typeName == "" {
		typeName = values[0] + "Type"
	}

	transitions := findASTSwitchTransitions(f, values)
	return &StateMachine{
		Name:        typeName,
		Package:     pkg,
		States:      values,
		Transitions: transitions,
		Initial:     values[0],
	}
}

func findASTSwitchTransitions(f *ast.File, states []string) []StateTransition {
	stateSet := make(map[string]bool, len(states))
	for _, s := range states {
		stateSet[s] = true
	}

	var transitions []StateTransition
	ast.Inspect(f, func(n ast.Node) bool {
		sw, ok := n.(*ast.SwitchStmt)
		if !ok {
			return true
		}
		// Check cases for state references.
		for _, stmt := range sw.Body.List {
			cc, ok := stmt.(*ast.CaseClause)
			if !ok {
				continue
			}
			for _, expr := range cc.List {
				if ident, ok := expr.(*ast.Ident); ok && stateSet[ident.Name] {
					// Look for assignments to the same type in the case body.
					for _, bs := range cc.Body {
						as, ok := bs.(*ast.AssignStmt)
						if !ok {
							continue
						}
						for _, rhs := range as.Rhs {
							if ri, ok := rhs.(*ast.Ident); ok && stateSet[ri.Name] && ri.Name != ident.Name {
								transitions = append(transitions, StateTransition{
									From: ident.Name,
									To:   ri.Name,
								})
							}
						}
					}
				}
			}
		}
		return true
	})
	return transitions
}

// parseFunctions walks the Go source tree and extracts all function declarations
// with their callees.
func (a *GoASTDeepAnalyzer) parseFunctions(scope string) ([]goFunc, error) {
	fset := token.NewFileSet()
	absRoot, err := filepath.Abs(a.root)
	if err != nil {
		return nil, err
	}

	var funcs []goFunc

	err = filepath.WalkDir(absRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if survey.ShouldSkipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != extGo || strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}

		rel, _ := filepath.Rel(absRoot, path)
		pkg := filepath.ToSlash(filepath.Dir(rel))
		if pkg == "." {
			pkg = pkgRoot
		}

		// Scope filter: skip files outside the requested scope prefix.
		if scope != "" && !strings.HasPrefix(pkg, scope) {
			return nil
		}

		f, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return nil
		}

		relFile := filepath.ToSlash(rel)
		for _, decl := range f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			name := fd.Name.Name
			var recvType string
			if fd.Recv != nil && len(fd.Recv.List) > 0 {
				recvType = receiverTypeName(fd.Recv.List[0].Type)
				name = recvType + "." + name // index as ReceiverType.Method
			}
			callees := extractCallees(fd.Body)
			funcs = append(funcs, goFunc{
				name:         name,
				pkg:          pkg,
				file:         relFile,
				line:         fset.Position(fd.Pos()).Line,
				receiverType: recvType,
				callees:      callees,
				body:         fd.Body,
			})
		}
		return nil
	})

	return funcs, err
}

// extractCallees walks a function body and returns all function names called.
func extractCallees(body *ast.BlockStmt) []string {
	seen := make(map[string]bool)
	var callees []string

	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		var name string
		switch fn := call.Fun.(type) {
		case *ast.Ident:
			name = fn.Name
		case *ast.SelectorExpr:
			name = fn.Sel.Name
		}
		if name != "" && !seen[name] {
			seen[name] = true
			callees = append(callees, name)
		}
		return true
	})
	return callees
}

// receiverTypeName extracts the type name from a method receiver expression.
// Handles both value (*ast.Ident) and pointer (*ast.StarExpr → *ast.Ident) receivers.
func receiverTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			return "*" + id.Name
		}
	}
	return ""
}
