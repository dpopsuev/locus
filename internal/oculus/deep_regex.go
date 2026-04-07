package oculus

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// RegexDeepAnalyzer provides best-effort deep analysis using regular
// expressions. It supports multiple languages and never returns errors,
// mirroring the RegexAnalyzer pattern from Tier 2.
type RegexDeepAnalyzer struct{}

var (
	reGoFunc     = regexp.MustCompile(`(?m)^func\s+(?:\([^)]*\)\s+)?(\w+)\s*\(`)
	reGoCall     = regexp.MustCompile(`(\w+)\s*\(`)
	reGoConst    = regexp.MustCompile(`(?m)^const\s*\(`)
	reGoIota     = regexp.MustCompile(`\biota\b`)
	reGoConstVal = regexp.MustCompile(`(?m)^\s+(\w+)`)
	reGoType     = regexp.MustCompile(`(?m)^type\s+(\w+)\s+`)
	reGoImport   = regexp.MustCompile(`"([^"]+)"`)
)

func (a *RegexDeepAnalyzer) CallGraph(root string, opts CallGraphOpts) (*CallGraph, error) {
	depth := opts.Depth
	if depth <= 0 {
		depth = DefaultCallGraphDepth
	}

	funcIndex := make(map[string]regexFuncDef)
	nodeSet := make(map[string]FuncNode)

	walkSourceFiles(root, func(content, pkg, relPath string) {
		for _, m := range reGoFunc.FindAllStringSubmatchIndex(content, -1) {
			name := content[m[2]:m[3]]
			if opts.ExportedOnly && !isExported(name) {
				continue
			}
			if opts.Scope != "" && !strings.HasPrefix(pkg, opts.Scope) {
				continue
			}
			// Extract a rough function body (until next func declaration or EOF)
			start := m[0]
			endIdx := len(content)
			nextFunc := reGoFunc.FindStringIndex(content[start+1:])
			if nextFunc != nil {
				endIdx = start + 1 + nextFunc[0]
			}
			line := strings.Count(content[:start], "\n") + 1
			key := pkg + "." + name
			funcIndex[key] = regexFuncDef{name: name, pkg: pkg, body: content[start:endIdx], line: line}
			nodeSet[key] = FuncNode{Name: name, Package: pkg, Line: line}
		}
	})

	var edges []CallEdge
	visited := make(map[string]bool)

	var walk func(key string, d int)
	walk = func(key string, d int) {
		if d > depth || visited[key] {
			return
		}
		visited[key] = true
		fd, ok := funcIndex[key]
		if !ok {
			return
		}
		for _, m := range reGoCall.FindAllStringSubmatch(fd.body, -1) {
			callee := m[1]
			if isRegexKeyword(callee, fd.name) {
				continue
			}
			calleeKey, calleePkg := resolveRegexCallee(callee, fd.pkg, funcIndex)
			edges = append(edges, CallEdge{
				Caller:    fd.name,
				Callee:    callee,
				CallerPkg: fd.pkg,
				CalleePkg: calleePkg,
				CrossPkg:  fd.pkg != calleePkg,
			})
			if _, exists := funcIndex[calleeKey]; exists {
				walk(calleeKey, d+1)
			}
		}
	}

	if opts.Entry != "" {
		for key, fd := range funcIndex {
			if fd.name == opts.Entry {
				walk(key, 0)
				break
			}
		}
	} else {
		for key, fd := range funcIndex {
			if isExported(fd.name) {
				walk(key, 0)
			}
		}
	}

	nodes := make([]FuncNode, 0, len(nodeSet))
	for _, n := range nodeSet {
		nodes = append(nodes, n)
	}
	return &CallGraph{Nodes: nodes, Edges: edges, Layer: LayerRegex}, nil
}

//nolint:gocyclo // data flow tracing with import heuristics requires multiple branches
func (a *RegexDeepAnalyzer) DataFlowTrace(root, entry string, maxDepth int) (*DataFlow, error) {
	if maxDepth <= 0 {
		maxDepth = DefaultDataFlowDepth
	}

	funcBodies := make(map[string]string)
	funcPkgs := make(map[string]string)
	var dataStores []string

	walkSourceFiles(root, func(content, pkg, _ string) {
		for _, m := range reGoFunc.FindAllStringSubmatchIndex(content, -1) {
			name := content[m[2]:m[3]]
			start := m[0]
			endIdx := len(content)
			nextFunc := reGoFunc.FindStringIndex(content[start+1:])
			if nextFunc != nil {
				endIdx = start + 1 + nextFunc[0]
			}
			funcBodies[name] = content[start:endIdx]
			funcPkgs[name] = pkg
		}
		for _, im := range reGoImport.FindAllStringSubmatch(content, -1) {
			imp := im[1]
			switch {
			case strings.Contains(imp, "database/sql"):
				dataStores = append(dataStores, "SQL Database")
			case strings.Contains(imp, "redis"):
				dataStores = append(dataStores, "Redis")
			case strings.Contains(imp, "bolt") || strings.Contains(imp, "bbolt"):
				dataStores = append(dataStores, "BoltDB")
			case strings.Contains(imp, "sqlite"):
				dataStores = append(dataStores, "SQLite")
			}
		}
	})

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
		body, ok := funcBodies[name]
		if !ok {
			return
		}
		for _, m := range reGoCall.FindAllStringSubmatch(body, -1) {
			callee := m[1]
			if callee == name || callee == "func" {
				continue
			}
			if _, exists := funcBodies[callee]; exists {
				if _, exists := nodeMap[callee]; !exists {
					nodeMap[callee] = DataFlowNode{Name: callee, Kind: "process", Pkg: funcPkgs[callee]}
				}
				edges = append(edges, DataFlowEdge{From: name, To: callee})
				trace(callee, d+1)
			}
		}
	}
	trace(entry, 0)

	// Add detected data stores
	for _, store := range dataStores {
		if _, exists := nodeMap[store]; !exists {
			nodeMap[store] = DataFlowNode{Name: store, Kind: "data_store"}
		}
	}

	nodes := make([]DataFlowNode, 0, len(nodeMap))
	for _, n := range nodeMap {
		nodes = append(nodes, n)
	}
	return &DataFlow{Nodes: nodes, Edges: edges, Layer: LayerRegex}, nil
}

func (a *RegexDeepAnalyzer) DetectStateMachines(root string) ([]StateMachine, error) {
	var machines []StateMachine

	walkSourceFiles(root, func(content, pkg, _ string) {
		blocks := reGoConst.FindAllStringIndex(content, -1)
		for _, block := range blocks {
			// Find the matching closing paren
			start := block[1]
			depth := 1
			end := start
			for end < len(content) && depth > 0 {
				if content[end] == '(' {
					depth++
				} else if content[end] == ')' {
					depth--
				}
				end++
			}
			constBlock := content[block[0]:end]
			if !reGoIota.MatchString(constBlock) {
				continue
			}

			// Extract type name and values
			typeName, values := parseConstBlock(constBlock)

			if typeName == "" && len(values) > 0 {
				typeName = values[0] + "Type"
			}
			if len(values) >= 2 {
				initial := values[0]
				for _, v := range values {
					lv := strings.ToLower(v)
					if strings.Contains(lv, "initial") || strings.Contains(lv, "new") ||
						strings.Contains(lv, "start") || strings.Contains(lv, "idle") {
						initial = v
						break
					}
				}
				machines = append(machines, StateMachine{
					Name:    typeName,
					Package: pkg,
					States:  values,
					Initial: initial,
				})
			}
		}
	})

	return machines, nil
}

func walkSourceFiles(root string, fn func(content, pkg, relPath string)) {
	absRoot, _ := filepath.Abs(root)
	_ = filepath.WalkDir(absRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			if d != nil && d.IsDir() {
				base := d.Name()
				if base == dirVendor || base == dirTestdata || strings.HasPrefix(base, ".") {
					return filepath.SkipDir
				}
			}
			return nil
		}
		ext := filepath.Ext(path)
		if ext != extGo && ext != extRust && ext != extPy && ext != extTS && ext != extJS && ext != extJava {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(absRoot, path)
		pkg := filepath.Dir(rel)
		if pkg == "." {
			pkg = pkgRoot
		}
		pkg = filepath.ToSlash(pkg)
		fn(string(data), pkg, rel)
		return nil
	})
}

// parseConstBlock extracts a type name and const values from a Go const block.
func parseConstBlock(constBlock string) (typeName string, values []string) {
	for _, line := range strings.Split(constBlock, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, "iota") {
			typeName, values = parseIotaLine(trimmed, typeName, values)
		} else if matches := reGoConstVal.FindStringSubmatch(trimmed); len(matches) > 1 {
			name := matches[1]
			if name != "" && name != ")" && name != "//" && !strings.HasPrefix(name, "//") {
				values = append(values, name)
			}
		}
	}
	return typeName, values
}

// parseIotaLine parses a single line containing iota and returns the updated type name and values.
func parseIotaLine(trimmed, typeName string, values []string) (resultType string, resultValues []string) {
	parts := strings.Fields(trimmed)
	if len(parts) < 2 {
		return typeName, values
	}
	values = append(values, parts[0])
	if typeName != "" {
		return typeName, values
	}
	// Look for type before = or iota
	for _, p := range parts {
		if reGoType.MatchString("type "+p+" ") || (p != parts[0] && p != "=" && p != "iota") {
			return p, values
		}
	}
	return typeName, values
}

// regexKeywords are Go keywords that look like function calls in regex matching.
var regexKeywords = map[string]bool{
	"func": true, "if": true, "for": true,
	"switch": true, "return": true,
}

// isRegexKeyword checks if a callee name is a Go keyword or the caller itself.
func isRegexKeyword(callee, callerName string) bool {
	return callee == callerName || regexKeywords[callee]
}

type regexFuncDef struct {
	name string
	pkg  string
	body string
	line int
}

// resolveRegexCallee finds the package-qualified key for a callee in the regex index.
func resolveRegexCallee(callee, callerPkg string, funcIndex map[string]regexFuncDef) (key, pkg string) {
	calleeKey := callerPkg + "." + callee
	calleePkg := callerPkg
	if _, found := funcIndex[calleeKey]; !found {
		for k, f := range funcIndex {
			if f.name == callee {
				return k, f.pkg
			}
		}
	}
	return calleeKey, calleePkg
}
