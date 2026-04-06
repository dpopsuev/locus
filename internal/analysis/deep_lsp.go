package analysis

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/dpopsuev/locus/internal/oculus/lsp"
)

// LSPDeepAnalyzer uses a single gopls connection for all DeepAnalyzer
// methods. The connection is started lazily on first call and reused.
type LSPDeepAnalyzer struct {
	root    string
	timeout time.Duration
	pool    lsp.Pool
}

// NewLSPDeep creates a deep analyzer that will start gopls on first use.
func NewLSPDeep(root string) *LSPDeepAnalyzer {
	return &LSPDeepAnalyzer{root: root, timeout: 30 * time.Second}
}

// NewLSPDeepWithPool creates a deep analyzer backed by a connection pool.
func NewLSPDeepWithPool(root string, pool lsp.Pool) *LSPDeepAnalyzer {
	return &LSPDeepAnalyzer{root: root, timeout: 30 * time.Second, pool: pool}
}

func (a *LSPDeepAnalyzer) startConn() (*lspConn, func(), error) {
	analyzer := &LSPAnalyzer{Timeout: a.timeout, pool: a.pool}
	return analyzer.startServer(a.root)
}

// CallGraph uses callHierarchy/outgoingCalls recursively from all
// exported functions (or a single entry if opts.Entry is set).
func (a *LSPDeepAnalyzer) CallGraph(_ string, opts CallGraphOpts) (*CallGraph, error) {
	conn, cleanup, err := a.startConn()
	if err != nil {
		return nil, fmt.Errorf("lsp deep call graph: %w", err)
	}
	defer cleanup()

	depth := opts.Depth
	if depth <= 0 {
		depth = DefaultCallGraphDepth
	}

	roots := lspCallGraphRoots(opts, conn, a.root)

	nodeSet := make(map[string]FuncNode)
	var edges []CallEdge
	visited := make(map[string]bool)

	for _, entry := range roots {
		item, err := conn.findCallHierarchyItem(a.root, entry)
		if err != nil || item == nil {
			continue
		}

		var walk func(it *callHierarchyItem, d int)
		walk = func(it *callHierarchyItem, d int) {
			if d > depth || visited[it.Name] {
				return
			}
			visited[it.Name] = true

			pkg := uriToPackage(it.URI, a.root)
			nodeSet[pkg+"."+it.Name] = FuncNode{
				Name: it.Name, Package: pkg, Line: it.Range.Start.Line + 1,
			}

			outgoing, err := conn.Request("callHierarchy/outgoingCalls", map[string]any{"item": it})
			if err != nil {
				return
			}
			var outs []struct {
				To callHierarchyItem `json:"to"`
			}
			if json.Unmarshal(outgoing, &outs) != nil {
				return
			}
			for _, out := range outs {
				calleePkg := uriToPackage(out.To.URI, a.root)
				nodeSet[calleePkg+"."+out.To.Name] = FuncNode{
					Name: out.To.Name, Package: calleePkg,
					Line: out.To.Range.Start.Line + 1,
				}
				edges = append(edges, CallEdge{
					Caller:    it.Name,
					Callee:    out.To.Name,
					CallerPkg: pkg,
					CalleePkg: calleePkg,
					Line:      out.To.Range.Start.Line + 1,
					CrossPkg:  pkg != calleePkg,
				})
				walk(&out.To, d+1)
			}
		}
		walk(item, 0)
	}

	nodes := make([]FuncNode, 0, len(nodeSet))
	for _, n := range nodeSet {
		nodes = append(nodes, n)
	}
	return &CallGraph{Nodes: nodes, Edges: edges, Layer: LayerLSP}, nil
}

// DataFlowTrace uses callHierarchy to trace data flow from an entry,
// detecting data stores via workspace/symbol heuristics.
func (a *LSPDeepAnalyzer) DataFlowTrace(_, entry string, maxDepth int) (*DataFlow, error) {
	conn, cleanup, err := a.startConn()
	if err != nil {
		return nil, fmt.Errorf("lsp deep dataflow: %w", err)
	}
	defer cleanup()

	if maxDepth <= 0 {
		maxDepth = DefaultDataFlowDepth
	}

	nodeMap := make(map[string]DataFlowNode)
	var edges []DataFlowEdge
	visited := make(map[string]bool)

	nodeMap[entry] = DataFlowNode{Name: entry, Kind: "entry"}

	item, err := conn.findCallHierarchyItem(a.root, entry)
	if err != nil || item == nil {
		return &DataFlow{
			Nodes: []DataFlowNode{{Name: entry, Kind: "entry"}},
			Layer: LayerLSP,
		}, nil
	}

	var trace func(it *callHierarchyItem, d int)
	trace = func(it *callHierarchyItem, d int) {
		if d > maxDepth || visited[it.Name] {
			return
		}
		visited[it.Name] = true

		pkg := uriToPackage(it.URI, a.root)
		if _, exists := nodeMap[it.Name]; !exists {
			nodeMap[it.Name] = DataFlowNode{Name: it.Name, Kind: "process", Pkg: pkg}
		}

		outgoing, err := conn.Request("callHierarchy/outgoingCalls", map[string]any{"item": it})
		if err != nil {
			return
		}
		var outs []struct {
			To callHierarchyItem `json:"to"`
		}
		if json.Unmarshal(outgoing, &outs) != nil {
			return
		}
		for _, out := range outs {
			lc := strings.ToLower(out.To.Name)
			isStore := strings.Contains(lc, "query") || strings.Contains(lc, "exec") ||
				strings.Contains(lc, "readfile") || strings.Contains(lc, "writefile")

			calleePkg := uriToPackage(out.To.URI, a.root)
			if isStore {
				storeName := calleePkg + " Store"
				if _, exists := nodeMap[storeName]; !exists {
					nodeMap[storeName] = DataFlowNode{Name: storeName, Kind: "data_store", Pkg: calleePkg}
				}
				edges = append(edges, DataFlowEdge{From: it.Name, To: storeName, Label: out.To.Name})
			} else {
				if _, exists := nodeMap[out.To.Name]; !exists {
					nodeMap[out.To.Name] = DataFlowNode{Name: out.To.Name, Kind: "process", Pkg: calleePkg}
				}
				edges = append(edges, DataFlowEdge{From: it.Name, To: out.To.Name})
			}
			trace(&out.To, d+1)
		}
	}

	trace(item, 0)

	nodes := make([]DataFlowNode, 0, len(nodeMap))
	for _, n := range nodeMap {
		nodes = append(nodes, n)
	}
	return &DataFlow{Nodes: nodes, Edges: edges, Layer: LayerLSP}, nil
}

// DetectStateMachines uses documentSymbol to find const groups and
// then workspace/symbol + textDocument/references to find switch contexts.
func (a *LSPDeepAnalyzer) DetectStateMachines(_ string) ([]StateMachine, error) {
	conn, cleanup, err := a.startConn()
	if err != nil {
		return nil, fmt.Errorf("lsp deep state machines: %w", err)
	}
	defer cleanup()

	files := findSrcFiles(a.root)
	var machines []StateMachine
	seen := make(map[string]bool)

	for _, f := range files {
		syms, err := conn.documentSymbols(f, a.root)
		if err != nil {
			continue
		}

		// Look for const groups that might represent state enums
		for _, sym := range syms {
			if sym.Kind != 14 {
				continue
			}
			if len(sym.Children) < 2 {
				continue
			}

			typeName := sym.Name
			if seen[typeName] {
				continue
			}
			seen[typeName] = true

			var states []string
			for _, ch := range sym.Children {
				states = append(states, ch.Name)
			}

			initial := states[0]
			for _, s := range states {
				ls := strings.ToLower(s)
				if strings.Contains(ls, "initial") || strings.Contains(ls, "new") ||
					strings.Contains(ls, "start") || strings.Contains(ls, "idle") {
					initial = s
					break
				}
			}

			machines = append(machines, StateMachine{
				Name:    typeName,
				Package: uriToPackage(pathToURI(f), a.root),
				States:  states,
				Initial: initial,
			})
		}
	}

	return machines, nil
}

// lspCallGraphRoots determines the root functions for call graph analysis.
func lspCallGraphRoots(opts CallGraphOpts, conn *lspConn, root string) []string {
	if opts.Entry != "" {
		return []string{opts.Entry}
	}
	files := findSrcFiles(root)
	seen := make(map[string]bool)
	var roots []string
	for _, f := range files {
		syms, err := conn.documentSymbols(f, root)
		if err != nil {
			continue
		}
		for _, sym := range syms {
			if !isExported(sym.Name) {
				continue
			}
			if sym.Kind != 12 && sym.Kind != 6 {
				continue
			}
			if !seen[sym.Name] {
				seen[sym.Name] = true
				roots = append(roots, sym.Name)
			}
		}
	}
	return roots
}
