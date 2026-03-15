package analysis

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
func (a *TreeSitterDeepAnalyzer) CallGraph(_ string, opts CallGraphOpts) (*CallGraph, error) {
	depth := opts.Depth
	if depth <= 0 {
		depth = 10
	}

	type funcDef struct {
		name string
		pkg  string
		body *sitter.Node
		src  []byte
		line int
	}

	// Phase 1 (Divide): group files by package
	pkgFiles := make(map[string][]ParsedFile)
	for _, f := range a.project.Files {
		pkgFiles[f.Package] = append(pkgFiles[f.Package], f)
	}

	// Phase 2 (Conquer): extract function definitions per package
	allFuncs := make(map[string]funcDef)
	nodeSet := make(map[string]FuncNode)

	for pkg, files := range pkgFiles {
		if opts.Scope != "" && !strings.HasPrefix(pkg, opts.Scope) {
			continue
		}
		for _, f := range files {
			root := f.Tree.RootNode()
			for i := 0; i < int(root.ChildCount()); i++ {
				child := root.Child(i)
				var nameNode *sitter.Node
				switch child.Type() {
				case "function_declaration":
					nameNode = child.ChildByFieldName("name")
				case "method_declaration":
					nameNode = child.ChildByFieldName("name")
				default:
					continue
				}
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
				allFuncs[key] = funcDef{
					name: name, pkg: pkg,
					body: body, src: f.Source,
					line: int(nameNode.StartPoint().Row) + 1,
				}
				nodeSet[key] = FuncNode{Name: name, Package: pkg, Line: int(nameNode.StartPoint().Row) + 1}
			}
		}
	}

	// Phase 3 (Combine): walk call graph from entry or all roots
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
			// Resolve callee to a fully-qualified key
			calleeKey := fd.pkg + "." + callee
			calleePkg := fd.pkg
			// Check if it exists in the same package first
			if _, found := allFuncs[calleeKey]; !found {
				// Try to find it in any other package
				for k, f := range allFuncs {
					if f.name == callee {
						calleeKey = k
						calleePkg = f.pkg
						break
					}
				}
			}

			edge := CallEdge{
				Caller:    fd.name,
				Callee:    callee,
				CallerPkg: fd.pkg,
				CalleePkg: calleePkg,
				Line:      line,
				CrossPkg:  fd.pkg != calleePkg,
			}
			edges = append(edges, edge)

			if _, exists := allFuncs[calleeKey]; exists {
				nodeSet[calleeKey] = FuncNode{Name: callee, Package: calleePkg}
				walk(calleeKey, d+1)
			}
		})
	}

	if opts.Entry != "" {
		// Find the entry function across all packages
		for key := range allFuncs {
			if allFuncs[key].name == opts.Entry {
				walk(key, 0)
				break
			}
		}
	} else {
		// Walk from all exported functions (or all if ExportedOnly is false)
		for key, fd := range allFuncs {
			if isExported(fd.name) {
				walk(key, 0)
			}
		}
	}

	nodes := make([]FuncNode, 0, len(nodeSet))
	for _, n := range nodeSet {
		nodes = append(nodes, n)
	}

	return &CallGraph{Nodes: nodes, Edges: edges, Layer: LayerTreeSitter}, nil
}

// DataFlowTrace implements DeepAnalyzer using memoized recursive DFS.
// It traces data flow from an entry point, detecting data stores via
// import heuristics and trust boundaries via auth middleware patterns.
func (a *TreeSitterDeepAnalyzer) DataFlowTrace(_ string, entry string, maxDepth int) (*DataFlow, error) {
	if maxDepth <= 0 {
		maxDepth = 8
	}

	type funcDef struct {
		name string
		pkg  string
		body *sitter.Node
		src  []byte
	}

	funcIndex := make(map[string]funcDef)
	for _, f := range a.project.Files {
		root := f.Tree.RootNode()
		for i := 0; i < int(root.ChildCount()); i++ {
			child := root.Child(i)
			var nameNode *sitter.Node
			switch child.Type() {
			case "function_declaration":
				nameNode = child.ChildByFieldName("name")
			case "method_declaration":
				nameNode = child.ChildByFieldName("name")
			default:
				continue
			}
			if nameNode == nil {
				continue
			}
			body := child.ChildByFieldName("body")
			if body == nil {
				continue
			}
			name := nameNode.Content(f.Source)
			funcIndex[name] = funcDef{name: name, pkg: f.Package, body: body, src: f.Source}
		}
	}

	// Detect data stores from imports
	storeImports := map[string]string{
		"database/sql": "SQL Database",
		"go.mongodb":   "MongoDB",
		"redis":        "Redis",
		"bolt":         "BoltDB",
		"badger":       "BadgerDB",
		"sqlite":       "SQLite",
		"os":           "Filesystem",
	}

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

	// Build flow graph
	nodeMap := make(map[string]DataFlowNode)
	var edges []DataFlowEdge
	memo := make(map[string]bool)

	// Add entry node
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
			// Check for data store access patterns
			lc := strings.ToLower(callee)
			isStore := strings.Contains(lc, "query") || strings.Contains(lc, "exec") ||
				strings.Contains(lc, "read") || strings.Contains(lc, "write") ||
				strings.Contains(lc, "get") || strings.Contains(lc, "set") ||
				strings.Contains(lc, "open") || strings.Contains(lc, "close") ||
				strings.Contains(lc, "save") || strings.Contains(lc, "load") ||
				strings.Contains(lc, "store") || strings.Contains(lc, "fetch")

			if isStore && len(dataStores) > 0 {
				for store := range dataStores {
					storeNode := store
					if _, exists := nodeMap[storeNode]; !exists {
						nodeMap[storeNode] = DataFlowNode{Name: storeNode, Kind: "data_store"}
					}
					edges = append(edges, DataFlowEdge{From: name, To: storeNode, Label: callee})
					break
				}
			}

			if _, exists := funcIndex[callee]; exists {
				if _, exists := nodeMap[callee]; !exists {
					nodeMap[callee] = DataFlowNode{Name: callee, Kind: "process", Pkg: funcIndex[callee].pkg}
				}
				edges = append(edges, DataFlowEdge{From: name, To: callee})
				trace(callee, depth+1)
			}
		})
	}

	trace(entry, 0)

	// Detect trust boundaries via auth patterns
	var boundaries []TrustBoundary
	authNodes := make(map[string]bool)
	publicNodes := make(map[string]bool)

	for name, fd := range funcIndex {
		bodyContent := strings.ToLower(string(fd.src[fd.body.StartByte():fd.body.EndByte()]))
		if strings.Contains(bodyContent, "auth") || strings.Contains(bodyContent, "token") ||
			strings.Contains(bodyContent, "middleware") || strings.Contains(bodyContent, "jwt") ||
			strings.Contains(bodyContent, "session") || strings.Contains(bodyContent, "permission") {
			authNodes[name] = true
		}
		if strings.Contains(name, "Handle") || strings.Contains(name, "Serve") ||
			strings.HasPrefix(name, "API") || strings.HasSuffix(name, "Handler") {
			publicNodes[name] = true
		}
	}

	if len(authNodes) > 0 {
		authList := make([]string, 0, len(authNodes))
		for n := range authNodes {
			if _, exists := nodeMap[n]; exists {
				authList = append(authList, n)
			}
		}
		if len(authList) > 0 {
			boundaries = append(boundaries, TrustBoundary{Name: "Auth Boundary", Nodes: authList})
		}
	}
	if len(publicNodes) > 0 {
		pubList := make([]string, 0, len(publicNodes))
		for n := range publicNodes {
			if _, exists := nodeMap[n]; exists {
				pubList = append(pubList, n)
			}
		}
		if len(pubList) > 0 {
			boundaries = append(boundaries, TrustBoundary{Name: "Public API", Nodes: pubList})
		}
	}

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
	var machines []StateMachine
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
func findSwitchTransitions(root *sitter.Node, src []byte, typeName string, values []string) []StateTransition {
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
		// Build transitions between consecutive case values (same switch = possible transitions)
		if len(caseValues) >= 2 {
			// Also look for assignments that indicate state changes
			bodyContent := node.Content(src)
			for _, fromState := range caseValues {
				for _, toState := range caseValues {
					if fromState != toState {
						// Check if the case body for fromState mentions toState
						caseIdx := strings.Index(bodyContent, "case "+fromState)
						if caseIdx >= 0 {
							nextCase := strings.Index(bodyContent[caseIdx+1:], "case ")
							var segment string
							if nextCase >= 0 {
								segment = bodyContent[caseIdx : caseIdx+1+nextCase]
							} else {
								segment = bodyContent[caseIdx:]
							}
							if strings.Contains(segment, toState) {
								*transitions = append(*transitions, StateTransition{
									From:    fromState,
									To:      toState,
									Trigger: "switch",
								})
							}
						}
					}
				}
			}
		}
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		walkForSwitches(node.Child(i), src, valueSet, transitions)
	}
}
