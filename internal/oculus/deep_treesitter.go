package oculus

import (
	"strings"
	"sync"

	sitter "github.com/smacker/go-tree-sitter"
)

// TreeSitterDeepAnalyzer uses a pre-parsed ParsedProject (built once)
// for all three DeepAnalyzer methods, avoiding redundant filesystem walks.
type TreeSitterDeepAnalyzer struct {
	project *ParsedProject
}

// NewTreeSitterDeep builds a ParsedProject from root and returns
// a ready-to-use deep analyzer. All subsequent queries reuse the
// cached ASTs and source bytes.
func NewTreeSitterDeep(root string) (*TreeSitterDeepAnalyzer, error) {
	pp, err := BuildParsedProject(root)
	if err != nil {
		return nil, err
	}
	return &TreeSitterDeepAnalyzer{project: pp}, nil
}

// CallGraph implements DeepAnalyzer using Divide-and-Conquer by package:
//  1. Divide: group ParsedProject.Files by Package
//  2. Conquer: extract function defs + call expressions per package
//  3. Combine: merge per-package graphs, mark cross-package edges
type cgFuncDef struct {
	name string
	pkg  string
	body *sitter.Node
	src  []byte
	line int
}

func (a *TreeSitterDeepAnalyzer) CallGraph(_ string, opts CallGraphOpts) (*CallGraph, error) {
	depth := opts.Depth
	if depth <= 0 {
		depth = 10
	}

	allFuncs, nodeSet := a.extractCallGraphFuncs(opts)
	edges := walkCallGraph(allFuncs, nodeSet, opts, depth)

	nodes := make([]FuncNode, 0, len(nodeSet))
	for _, n := range nodeSet {
		nodes = append(nodes, n)
	}

	return &CallGraph{Nodes: nodes, Edges: edges, Layer: LayerTreeSitter}, nil
}

// extractCallGraphFuncs extracts function definitions grouped by package.
func (a *TreeSitterDeepAnalyzer) extractCallGraphFuncs(opts CallGraphOpts) (allFuncs map[string]cgFuncDef, nodeSet map[string]FuncNode) {
	allFuncs = make(map[string]cgFuncDef)
	nodeSet = make(map[string]FuncNode)

	for _, f := range a.project.Files {
		pkg := f.Package
		if opts.Scope != "" && !strings.HasPrefix(pkg, opts.Scope) {
			continue
		}
		root := f.Tree.RootNode()
		for i := 0; i < int(root.ChildCount()); i++ {
			child := root.Child(i)
			nameNode := funcOrMethodName(child)
			if nameNode == nil {
				continue
			}
			name := nameNode.Content(f.Source)
			if opts.ExportedOnly && !isExported(name) {
				continue
			}
			body := child.ChildByFieldName("body")
			if body == nil {
				continue
			}
			key := pkg + "." + name
			line := int(nameNode.StartPoint().Row) + 1
			allFuncs[key] = cgFuncDef{name: name, pkg: pkg, body: body, src: f.Source, line: line}
			nodeSet[key] = FuncNode{Name: name, Package: pkg, Line: line}
		}
	}
	return allFuncs, nodeSet
}

// resolveCallee finds the fully qualified key and package for a callee.
func resolveCallee(callee, callerPkg string, allFuncs map[string]cgFuncDef) (key, pkg string) {
	calleeKey := callerPkg + "." + callee
	calleePkg := callerPkg
	if _, found := allFuncs[calleeKey]; !found {
		for k, f := range allFuncs {
			if f.name == callee {
				return k, f.pkg
			}
		}
	}
	return calleeKey, calleePkg
}

// walkCallGraph walks the call graph from roots determined by opts.
func walkCallGraph(allFuncs map[string]cgFuncDef, nodeSet map[string]FuncNode,
	opts CallGraphOpts, depth int,
) []CallEdge {
	var edges []CallEdge
	visited := make(map[string]bool)

	var walk func(key string, d int)
	walk = func(key string, d int) {
		if d > depth || visited[key] {
			return
		}
		visited[key] = true
		fd, ok := allFuncs[key]
		if !ok {
			return
		}
		extractCalls(fd.body, fd.src, func(callee string, line int) {
			calleeKey, calleePkg := resolveCallee(callee, fd.pkg, allFuncs)
			edges = append(edges, CallEdge{
				Caller:    fd.name,
				Callee:    callee,
				CallerPkg: fd.pkg,
				CalleePkg: calleePkg,
				Line:      line,
				CrossPkg:  fd.pkg != calleePkg,
			})
			if _, exists := allFuncs[calleeKey]; exists {
				nodeSet[calleeKey] = FuncNode{Name: callee, Package: calleePkg}
				walk(calleeKey, d+1)
			}
		})
	}

	if opts.Entry != "" {
		for key := range allFuncs {
			if allFuncs[key].name == opts.Entry {
				walk(key, 0)
				break
			}
		}
	} else {
		for key, fd := range allFuncs {
			if isExported(fd.name) {
				walk(key, 0)
			}
		}
	}
	return edges
}

// DataFlowTrace implements DeepAnalyzer using memoized recursive DFS.
// It traces data flow from an entry point, detecting data stores via
// import heuristics and trust boundaries via auth middleware patterns.
func (a *TreeSitterDeepAnalyzer) DataFlowTrace(_, entry string, maxDepth int) (*DataFlow, error) {
	if maxDepth <= 0 {
		maxDepth = 8
	}

	funcIndex := a.buildFuncIndex()
	dataStores := a.detectDataStores()

	nodeMap, edges := traceDataFlow(funcIndex, dataStores, entry, maxDepth)
	boundaries := detectTrustBoundaries(funcIndex, nodeMap)

	nodes := make([]DataFlowNode, 0, len(nodeMap))
	for _, n := range nodeMap {
		nodes = append(nodes, n)
	}

	return &DataFlow{
		Nodes:      nodes,
		Edges:      edges,
		Boundaries: boundaries,
		Layer:      LayerTreeSitter,
	}, nil
}

type tsFuncDef struct {
	name string
	pkg  string
	body *sitter.Node
	src  []byte
}

// buildFuncIndex extracts all function/method definitions from the parsed project.
func (a *TreeSitterDeepAnalyzer) buildFuncIndex() map[string]tsFuncDef {
	funcIndex := make(map[string]tsFuncDef)
	for _, f := range a.project.Files {
		root := f.Tree.RootNode()
		for i := 0; i < int(root.ChildCount()); i++ {
			child := root.Child(i)
			nameNode := funcOrMethodName(child)
			if nameNode == nil {
				continue
			}
			body := child.ChildByFieldName("body")
			if body == nil {
				continue
			}
			name := nameNode.Content(f.Source)
			funcIndex[name] = tsFuncDef{name: name, pkg: f.Package, body: body, src: f.Source}
		}
	}
	return funcIndex
}

// storeImports maps import paths to human-readable data store names.
var storeImports = map[string]string{
	"database/sql": "SQL Database",
	"go.mongodb":   "MongoDB",
	"redis":        "Redis",
	"bolt":         "BoltDB",
	"badger":       "BadgerDB",
	"sqlite":       "SQLite",
	"os":           "Filesystem",
}

// detectDataStores finds data store dependencies from import declarations.
func (a *TreeSitterDeepAnalyzer) detectDataStores() map[string]bool {
	dataStores := make(map[string]bool)
	for _, f := range a.project.Files {
		root := f.Tree.RootNode()
		for i := 0; i < int(root.ChildCount()); i++ {
			child := root.Child(i)
			if child.Type() != "import_declaration" {
				continue
			}
			content := child.Content(f.Source)
			for imp, storeName := range storeImports {
				if strings.Contains(content, imp) {
					dataStores[storeName] = true
				}
			}
		}
	}
	return dataStores
}

// isStoreAccess checks if a function name looks like a data store access pattern.
func isStoreAccess(callee string) bool {
	lc := strings.ToLower(callee)
	storePatterns := []string{
		"query", "exec", "read", "write", "get", "set",
		"open", "close", "save", "load", "store", "fetch",
	}
	for _, p := range storePatterns {
		if strings.Contains(lc, p) {
			return true
		}
	}
	return false
}

// traceDataFlow builds the data flow graph by walking from the entry point.
func traceDataFlow(funcIndex map[string]tsFuncDef, dataStores map[string]bool,
	entry string, maxDepth int,
) (nodes map[string]DataFlowNode, edges []DataFlowEdge) {
	nodeMap := make(map[string]DataFlowNode)
	memo := make(map[string]bool)

	nodeMap[entry] = DataFlowNode{Name: entry, Kind: "entry"}

	var trace func(name string, depth int)
	trace = func(name string, depth int) {
		if depth > maxDepth || memo[name] {
			return
		}
		memo[name] = true

		fd, ok := funcIndex[name]
		if !ok {
			return
		}

		if _, exists := nodeMap[name]; !exists {
			nodeMap[name] = DataFlowNode{Name: name, Kind: "process", Pkg: fd.pkg}
		}

		extractCalls(fd.body, fd.src, func(callee string, _ int) {
			if isStoreAccess(callee) && len(dataStores) > 0 {
				for store := range dataStores {
					if _, exists := nodeMap[store]; !exists {
						nodeMap[store] = DataFlowNode{Name: store, Kind: "data_store"}
					}
					edges = append(edges, DataFlowEdge{From: name, To: store, Label: callee})
					break
				}
			}

			if _, exists := funcIndex[callee]; exists {
				if _, inMap := nodeMap[callee]; !inMap {
					nodeMap[callee] = DataFlowNode{Name: callee, Kind: "process", Pkg: funcIndex[callee].pkg}
				}
				edges = append(edges, DataFlowEdge{From: name, To: callee})
				trace(callee, depth+1)
			}
		})
	}

	trace(entry, 0)
	return nodeMap, edges
}

// detectTrustBoundaries identifies auth and public API boundaries from function metadata.
func detectTrustBoundaries(funcIndex map[string]tsFuncDef, nodeMap map[string]DataFlowNode) []TrustBoundary {
	var boundaries []TrustBoundary

	authList := filterBoundaryNodes(funcIndex, nodeMap, isAuthFunc)
	if len(authList) > 0 {
		boundaries = append(boundaries, TrustBoundary{Name: "Auth Boundary", Nodes: authList})
	}

	pubList := filterBoundaryNodes(funcIndex, nodeMap, isPublicFunc)
	if len(pubList) > 0 {
		boundaries = append(boundaries, TrustBoundary{Name: "Public API", Nodes: pubList})
	}

	return boundaries
}

// filterBoundaryNodes returns function names that match the predicate and exist in nodeMap.
func filterBoundaryNodes(funcIndex map[string]tsFuncDef, nodeMap map[string]DataFlowNode,
	pred func(name string, fd tsFuncDef) bool,
) []string {
	var result []string
	for name, fd := range funcIndex {
		if pred(name, fd) {
			if _, exists := nodeMap[name]; exists {
				result = append(result, name)
			}
		}
	}
	return result
}

// isAuthFunc checks if a function body contains authentication-related patterns.
func isAuthFunc(_ string, fd tsFuncDef) bool {
	bodyContent := strings.ToLower(string(fd.src[fd.body.StartByte():fd.body.EndByte()]))
	return strings.Contains(bodyContent, "auth") || strings.Contains(bodyContent, "token") ||
		strings.Contains(bodyContent, "middleware") || strings.Contains(bodyContent, "jwt") ||
		strings.Contains(bodyContent, "session") || strings.Contains(bodyContent, "permission")
}

// isPublicFunc checks if a function name matches public API patterns.
func isPublicFunc(name string, _ tsFuncDef) bool {
	return strings.Contains(name, "Handle") || strings.Contains(name, "Serve") ||
		strings.HasPrefix(name, "API") || strings.HasSuffix(name, "Handler")
}

// funcOrMethodName returns the name node for a function or method declaration.
func funcOrMethodName(child *sitter.Node) *sitter.Node {
	switch child.Type() {
	case nodeFuncDecl, nodeMethodDecl:
		return child.ChildByFieldName("name")
	default:
		return nil
	}
}

// DetectStateMachines implements DeepAnalyzer using file-level parallelism.
// For each ParsedFile it:
//  1. Extracts const blocks with iota (Go state candidates)
//  2. Finds switch statements on those types
//  3. Builds transitions from case arms
func (a *TreeSitterDeepAnalyzer) DetectStateMachines(_ string) ([]StateMachine, error) {
	type perFileResult struct {
		machines []StateMachine
	}

	results := make([]perFileResult, len(a.project.Files))
	var wg sync.WaitGroup

	for idx, f := range a.project.Files {
		wg.Add(1)
		go func(i int, pf ParsedFile) {
			defer wg.Done()
			results[i] = perFileResult{machines: extractStateMachines(pf)}
		}(idx, f)
	}
	wg.Wait()

	var machines []StateMachine
	seen := make(map[string]bool)
	for _, r := range results {
		for _, m := range r.machines {
			if !seen[m.Name] {
				seen[m.Name] = true
				machines = append(machines, m)
			}
		}
	}
	return machines, nil
}

// extractStateMachines finds iota-based const groups and switch statements
// that reference those types, building StateMachine structures.
func extractStateMachines(pf ParsedFile) []StateMachine {
	root := pf.Tree.RootNode()

	// Phase 1: find const blocks with iota
	type constGroup struct {
		typeName string
		values   []string
	}
	var groups []constGroup

	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(i)
		if child.Type() != "const_declaration" {
			continue
		}

		content := child.Content(pf.Source)
		if !strings.Contains(content, "iota") {
			continue
		}

		var typeName string
		var values []string

		for j := 0; j < int(child.ChildCount()); j++ {
			spec := child.Child(j)
			if spec.Type() != "const_spec" {
				continue
			}
			nameNode := spec.ChildByFieldName("name")
			if nameNode == nil {
				continue
			}
			name := nameNode.Content(pf.Source)
			values = append(values, name)

			typeNode := spec.ChildByFieldName("type")
			if typeNode != nil && typeName == "" {
				typeName = typeNode.Content(pf.Source)
			}
		}

		if typeName != "" && len(values) >= 2 {
			groups = append(groups, constGroup{typeName: typeName, values: values})
		}
	}

	if len(groups) == 0 {
		return nil
	}

	// Phase 2: find switch statements and build transitions
	machines := make([]StateMachine, 0, len(groups))
	for _, g := range groups {
		transitions := findSwitchTransitions(root, pf.Source, g.typeName, g.values)
		initial := g.values[0]
		for _, v := range g.values {
			lv := strings.ToLower(v)
			if strings.Contains(lv, "initial") || strings.Contains(lv, "new") ||
				strings.Contains(lv, "start") || strings.Contains(lv, "idle") ||
				strings.Contains(lv, "pending") {
				initial = v
				break
			}
		}

		machines = append(machines, StateMachine{
			Name:        g.typeName,
			Package:     pf.Package,
			States:      g.values,
			Transitions: transitions,
			Initial:     initial,
		})
	}
	return machines
}

// findSwitchTransitions searches for switch statements that reference
// the given type's values and extracts transitions between states.
func findSwitchTransitions(root *sitter.Node, src []byte, _ string, values []string) []StateTransition {
	valueSet := make(map[string]bool)
	for _, v := range values {
		valueSet[v] = true
	}

	var transitions []StateTransition
	walkForSwitches(root, src, valueSet, &transitions)
	return transitions
}

func walkForSwitches(node *sitter.Node, src []byte, valueSet map[string]bool, transitions *[]StateTransition) {
	if node == nil {
		return
	}

	if node.Type() == "expression_switch_statement" || node.Type() == "type_switch_statement" {
		caseValues := extractCaseValues(node, src, valueSet)
		if len(caseValues) >= 2 {
			buildSwitchTransitions(node.Content(src), caseValues, transitions)
		}
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		walkForSwitches(node.Child(i), src, valueSet, transitions)
	}
}

// extractCaseValues collects state values referenced in switch case clauses.
func extractCaseValues(node *sitter.Node, src []byte, valueSet map[string]bool) []string {
	var caseValues []string
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() != "expression_case" && child.Type() != "type_case" {
			continue
		}
		caseContent := child.Content(src)
		for v := range valueSet {
			if strings.Contains(caseContent, v) {
				caseValues = append(caseValues, v)
			}
		}
	}
	return caseValues
}

// buildSwitchTransitions finds state transitions by checking if case bodies
// for one state reference another state.
func buildSwitchTransitions(bodyContent string, caseValues []string, transitions *[]StateTransition) {
	for _, fromState := range caseValues {
		for _, toState := range caseValues {
			if fromState == toState {
				continue
			}
			segment := caseSegment(bodyContent, fromState)
			if segment != "" && strings.Contains(segment, toState) {
				*transitions = append(*transitions, StateTransition{
					From:    fromState,
					To:      toState,
					Trigger: "switch",
				})
			}
		}
	}
}

// caseSegment extracts the text segment for a specific case clause from
// the full switch body content.
func caseSegment(bodyContent, state string) string {
	caseIdx := strings.Index(bodyContent, "case "+state)
	if caseIdx < 0 {
		return ""
	}
	nextCase := strings.Index(bodyContent[caseIdx+1:], "case ")
	if nextCase >= 0 {
		return bodyContent[caseIdx : caseIdx+1+nextCase]
	}
	return bodyContent[caseIdx:]
}
