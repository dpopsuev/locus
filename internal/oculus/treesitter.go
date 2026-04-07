package oculus

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/golang"

	olang "github.com/dpopsuev/locus/internal/oculus/lang"
)

// ErrUnsupportedLanguage is returned when tree-sitter does not support the detected language.
var ErrUnsupportedLanguage = errors.New("tree-sitter: unsupported language")

// TreeSitterAnalyzer extracts type-level metadata by parsing source files
// with tree-sitter grammars. Accuracy is ~70% (syntactic, not semantic).
type TreeSitterAnalyzer struct{}

func (a *TreeSitterAnalyzer) Classes(root string) ([]ClassInfo, error) {
	lang := olang.DetectLanguage(root)
	switch lang {
	case olang.Go:
		return a.goClasses(root)
	default:
		return nil, fmt.Errorf("%w: %v (classes)", ErrUnsupportedLanguage, lang)
	}
}

func (a *TreeSitterAnalyzer) Implements(root string) ([]ImplEdge, error) {
	lang := olang.DetectLanguage(root)
	switch lang {
	case olang.Go:
		return a.goImplements(root)
	default:
		return nil, fmt.Errorf("%w: %v (implements)", ErrUnsupportedLanguage, lang)
	}
}

func (a *TreeSitterAnalyzer) FieldRefs(root string) ([]FieldRef, error) {
	lang := olang.DetectLanguage(root)
	switch lang {
	case olang.Go:
		return a.goFieldRefs(root)
	default:
		return nil, fmt.Errorf("%w: %v (field refs)", ErrUnsupportedLanguage, lang)
	}
}

func (a *TreeSitterAnalyzer) CallChain(root, entry string, depth int) ([]Call, error) {
	lang := olang.DetectLanguage(root)
	switch lang {
	case olang.Go:
		return a.goCallChain(root, entry, depth)
	default:
		return nil, fmt.Errorf("%w: %v (call chain)", ErrUnsupportedLanguage, lang)
	}
}

func (a *TreeSitterAnalyzer) EntryPoints(root string) ([]EntryPoint, error) {
	lang := olang.DetectLanguage(root)
	switch lang {
	case olang.Go:
		return a.goEntryPoints(root)
	default:
		return nil, fmt.Errorf("%w: %v (entry points)", ErrUnsupportedLanguage, lang)
	}
}

func (a *TreeSitterAnalyzer) NestingDepth(root string) ([]NestingResult, error) {
	lang := olang.DetectLanguage(root)
	switch lang {
	case olang.Go:
		return a.goNestingDepth(root)
	default:
		return nil, fmt.Errorf("%w: %v (nesting)", ErrUnsupportedLanguage, lang)
	}
}

// --- Go-specific implementations ---

func (a *TreeSitterAnalyzer) goClasses(root string) ([]ClassInfo, error) {
	var classes []ClassInfo
	err := a.walkGoFiles(root, func(tree *sitter.Tree, src []byte, pkg, file string) {
		root := tree.RootNode()
		for i := 0; i < int(root.ChildCount()); i++ {
			child := root.Child(i)
			if child.Type() != "type_declaration" {
				continue
			}
			for j := 0; j < int(child.ChildCount()); j++ {
				spec := child.Child(j)
				if spec.Type() != "type_spec" {
					continue
				}
				nameNode := spec.ChildByFieldName("name")
				typeNode := spec.ChildByFieldName("type")
				if nameNode == nil || typeNode == nil {
					continue
				}
				name := nameNode.Content(src)
				ci := ClassInfo{
					Name:     name,
					Package:  pkg,
					Exported: isExported(name),
				}
				switch typeNode.Type() {
				case nodeStructType:
					ci.Kind = kindStruct
					ci.Fields = extractGoStructFields(typeNode, src)
				case nodeInterfaceType:
					ci.Kind = kindInterface
					ci.Methods = extractGoInterfaceMethods(typeNode, src)
				default:
					continue
				}
				classes = append(classes, ci)
			}
		}

		// Collect methods declared in this file
		for i := 0; i < int(root.ChildCount()); i++ {
			child := root.Child(i)
			if child.Type() != nodeMethodDecl {
				continue
			}
			nameNode := child.ChildByFieldName("name")
			recvNode := child.ChildByFieldName("receiver")
			if nameNode == nil || recvNode == nil {
				continue
			}
			methodName := nameNode.Content(src)
			recvType := extractGoReceiverType(recvNode, src)
			if recvType == "" {
				continue
			}
			params := child.ChildByFieldName("parameters")
			sig := methodName
			if params != nil {
				sig = methodName + params.Content(src)
			}
			for k := range classes {
				if classes[k].Name == recvType && classes[k].Package == pkg {
					classes[k].Methods = append(classes[k].Methods, MethodInfo{
						Name:      methodName,
						Signature: sig,
						Exported:  isExported(methodName),
					})
				}
			}
		}
	})
	return classes, err
}

//nolint:gocyclo // struct embedding detection requires iterating nested AST nodes
func (a *TreeSitterAnalyzer) goImplements(root string) ([]ImplEdge, error) {
	var edges []ImplEdge
	err := a.walkGoFiles(root, func(tree *sitter.Tree, src []byte, pkg, file string) {
		rootNode := tree.RootNode()
		for i := 0; i < int(rootNode.ChildCount()); i++ {
			child := rootNode.Child(i)
			if child.Type() != "type_declaration" {
				continue
			}
			for j := 0; j < int(child.ChildCount()); j++ {
				spec := child.Child(j)
				if spec.Type() != "type_spec" {
					continue
				}
				nameNode := spec.ChildByFieldName("name")
				typeNode := spec.ChildByFieldName("type")
				if nameNode == nil || typeNode == nil {
					continue
				}
				if typeNode.Type() != nodeStructType {
					continue
				}
				name := nameNode.Content(src)
				fieldList := typeNode.ChildByFieldName("field_list")
				if fieldList == nil {
					// try walking children for field_declaration_list
					for k := 0; k < int(typeNode.ChildCount()); k++ {
						c := typeNode.Child(k)
						if c.Type() == "field_declaration_list" {
							fieldList = c
							break
						}
					}
				}
				if fieldList == nil {
					continue
				}
				for k := 0; k < int(fieldList.ChildCount()); k++ {
					field := fieldList.Child(k)
					if field.Type() != "field_declaration" {
						continue
					}
					// Embedded field: no name, just a type
					nameCount := 0
					var typeContent string
					for m := 0; m < int(field.ChildCount()); m++ {
						fc := field.Child(m)
						if fc.Type() == "field_identifier" {
							nameCount++
						}
						if fc.Type() == nodeTypeID || fc.Type() == nodeQualifiedType || fc.Type() == nodePointerType {
							typeContent = fc.Content(src)
						}
					}
					if nameCount == 0 && typeContent != "" {
						typeContent = strings.TrimPrefix(typeContent, "*")
						edges = append(edges, ImplEdge{
							From: name,
							To:   typeContent,
							Kind: "embeds",
						})
					}
				}
			}
		}
	})
	return edges, err
}

func (a *TreeSitterAnalyzer) goFieldRefs(root string) ([]FieldRef, error) {
	classes, err := a.goClasses(root)
	if err != nil {
		return nil, err
	}
	typeSet := make(map[string]bool)
	for _, c := range classes {
		typeSet[c.Name] = true
	}
	var refs []FieldRef
	for _, c := range classes {
		if c.Kind != kindStruct {
			continue
		}
		for _, f := range c.Fields {
			refType := strings.TrimPrefix(f.Type, "*")
			refType = strings.TrimPrefix(refType, "[]")
			refType = strings.TrimPrefix(refType, "*")
			if idx := strings.LastIndex(refType, "."); idx >= 0 {
				refType = refType[idx+1:]
			}
			if typeSet[refType] && refType != c.Name {
				refs = append(refs, FieldRef{
					Owner:   c.Name,
					Field:   f.Name,
					RefType: refType,
				})
			}
		}
	}
	return refs, nil
}

func (a *TreeSitterAnalyzer) goCallChain(root, entry string, maxDepth int) ([]Call, error) {
	if maxDepth <= 0 {
		maxDepth = 5
	}

	type funcBody struct {
		pkg  string
		node *sitter.Node
		src  []byte
	}
	funcBodies := make(map[string]funcBody)

	err := a.walkGoFiles(root, func(tree *sitter.Tree, src []byte, pkg, file string) {
		rootNode := tree.RootNode()
		for i := 0; i < int(rootNode.ChildCount()); i++ {
			child := rootNode.Child(i)
			var nameNode *sitter.Node
			switch child.Type() {
			case nodeFuncDecl:
				nameNode = child.ChildByFieldName("name")
			case nodeMethodDecl:
				nameNode = child.ChildByFieldName("name")
			default:
				continue
			}
			if nameNode == nil {
				continue
			}
			name := nameNode.Content(src)
			body := child.ChildByFieldName("body")
			if body != nil {
				funcBodies[name] = funcBody{pkg: pkg, node: body, src: src}
			}
		}
	})
	if err != nil {
		return nil, err
	}

	var calls []Call
	visited := make(map[string]bool)

	var walk func(funcName string, depth int)
	walk = func(funcName string, depth int) {
		if depth > maxDepth || visited[funcName] {
			return
		}
		visited[funcName] = true
		fb, ok := funcBodies[funcName]
		if !ok {
			return
		}
		extractCalls(fb.node, fb.src, func(callee string, line int) {
			calls = append(calls, Call{
				Caller:  funcName,
				Callee:  callee,
				Package: fb.pkg,
				Line:    line,
			})
			walk(callee, depth+1)
		})
	}
	walk(entry, 0)
	return calls, nil
}

func (a *TreeSitterAnalyzer) goEntryPoints(root string) ([]EntryPoint, error) {
	var entries []EntryPoint
	err := a.walkGoFiles(root, func(tree *sitter.Tree, src []byte, pkg, file string) {
		rootNode := tree.RootNode()
		for i := 0; i < int(rootNode.ChildCount()); i++ {
			child := rootNode.Child(i)
			if child.Type() != nodeFuncDecl {
				continue
			}
			nameNode := child.ChildByFieldName("name")
			if nameNode == nil {
				continue
			}
			name := nameNode.Content(src)
			params := child.ChildByFieldName("parameters")

			switch {
			case name == "main":
				entries = append(entries, EntryPoint{
					Name: name, Kind: "main", Package: pkg, File: file,
					Line: int(nameNode.StartPoint().Row) + 1,
				})
			case name == "init":
				entries = append(entries, EntryPoint{
					Name: name, Kind: "init", Package: pkg, File: file,
					Line: int(nameNode.StartPoint().Row) + 1,
				})
			case strings.HasPrefix(name, "Test") && params != nil && isTestParam(params, src):
				entries = append(entries, EntryPoint{
					Name: name, Kind: "test", Package: pkg, File: file,
					Line: int(nameNode.StartPoint().Row) + 1,
				})
			case isHTTPHandlerSignature(params, src):
				entries = append(entries, EntryPoint{
					Name: name, Kind: "http_handler", Package: pkg, File: file,
					Line: int(nameNode.StartPoint().Row) + 1,
				})
			}
		}
	})
	return entries, err
}

func (a *TreeSitterAnalyzer) goNestingDepth(root string) ([]NestingResult, error) {
	var results []NestingResult
	err := a.walkGoFiles(root, func(tree *sitter.Tree, src []byte, pkg, file string) {
		rootNode := tree.RootNode()
		for i := 0; i < int(rootNode.ChildCount()); i++ {
			child := rootNode.Child(i)
			var nameNode *sitter.Node
			switch child.Type() {
			case nodeFuncDecl:
				nameNode = child.ChildByFieldName("name")
			case nodeMethodDecl:
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
			maxD := computeNesting(body, 0)
			results = append(results, NestingResult{
				Function: nameNode.Content(src),
				Package:  pkg,
				MaxDepth: maxD,
			})
		}
	})
	return results, err
}

// --- helpers ---

func (a *TreeSitterAnalyzer) walkGoFiles(root string, fn func(*sitter.Tree, []byte, string, string)) error {
	parser := sitter.NewParser()
	parser.SetLanguage(golang.GetLanguage())

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	return filepath.WalkDir(absRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			base := d.Name()
			if base == dirVendor || base == dirTestdata || strings.HasPrefix(base, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != extGo {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		tree, err := parser.ParseCtx(context.Background(), nil, src)
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(absRoot, path)
		pkg := filepath.Dir(rel)
		if pkg == "." {
			pkg = pkgRoot
		}
		pkg = filepath.ToSlash(pkg)
		fn(tree, src, pkg, rel)
		return nil
	})
}

func extractGoStructFields(structNode *sitter.Node, src []byte) []FieldInfo {
	var fields []FieldInfo
	var fieldList *sitter.Node
	for i := 0; i < int(structNode.ChildCount()); i++ {
		c := structNode.Child(i)
		if c.Type() == "field_declaration_list" {
			fieldList = c
			break
		}
	}
	if fieldList == nil {
		return nil
	}
	for i := 0; i < int(fieldList.ChildCount()); i++ {
		field := fieldList.Child(i)
		if field.Type() != "field_declaration" {
			continue
		}
		var names []string
		var typStr string
		var tag string
		for j := 0; j < int(field.ChildCount()); j++ {
			fc := field.Child(j)
			switch fc.Type() {
			case "field_identifier":
				names = append(names, fc.Content(src))
			case nodeTypeID, nodeQualifiedType, nodePointerType,
				"slice_type", "array_type", "map_type",
				"channel_type", "function_type", "interface_type",
				nodeStructType:
				typStr = fc.Content(src)
			case "raw_string_literal", "interpreted_string_literal":
				tag = fc.Content(src)
			}
		}
		if len(names) == 0 && typStr != "" {
			// Embedded field
			shortName := typStr
			shortName = strings.TrimPrefix(shortName, "*")
			if idx := strings.LastIndex(shortName, "."); idx >= 0 {
				shortName = shortName[idx+1:]
			}
			fields = append(fields, FieldInfo{
				Name:     shortName,
				Type:     typStr,
				Exported: isExported(shortName),
				Tag:      tag,
			})
			continue
		}
		for _, n := range names {
			fields = append(fields, FieldInfo{
				Name:     n,
				Type:     typStr,
				Exported: isExported(n),
				Tag:      tag,
			})
		}
	}
	return fields
}

func extractGoInterfaceMethods(ifaceNode *sitter.Node, src []byte) []MethodInfo {
	var methods []MethodInfo
	for i := 0; i < int(ifaceNode.ChildCount()); i++ {
		child := ifaceNode.Child(i)
		if child.Type() == "method_spec" {
			nameNode := child.ChildByFieldName("name")
			if nameNode == nil {
				continue
			}
			name := nameNode.Content(src)
			params := child.ChildByFieldName("parameters")
			sig := name
			if params != nil {
				sig = name + params.Content(src)
			}
			methods = append(methods, MethodInfo{
				Name:      name,
				Signature: sig,
				Exported:  isExported(name),
			})
		}
	}
	return methods
}

func extractGoReceiverType(recvNode *sitter.Node, src []byte) string {
	for i := 0; i < int(recvNode.ChildCount()); i++ {
		child := recvNode.Child(i)
		if child.Type() == "parameter_declaration" {
			for j := 0; j < int(child.ChildCount()); j++ {
				fc := child.Child(j)
				switch fc.Type() {
				case nodeTypeID:
					return fc.Content(src)
				case nodePointerType:
					inner := fc.Content(src)
					return strings.TrimPrefix(inner, "*")
				}
			}
		}
	}
	return ""
}

func extractCalls(node *sitter.Node, src []byte, emit func(callee string, line int)) {
	if node == nil {
		return
	}
	if node.Type() == "call_expression" {
		fn := node.ChildByFieldName("function")
		if fn != nil {
			callee := fn.Content(src)
			if idx := strings.LastIndex(callee, "."); idx >= 0 {
				callee = callee[idx+1:]
			}
			emit(callee, int(fn.StartPoint().Row)+1)
		}
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		extractCalls(node.Child(i), src, emit)
	}
}

var nestingTypes = map[string]bool{
	"if_statement":          true,
	"for_statement":         true,
	"switch_statement":      true,
	"select_statement":      true,
	"type_switch_statement": true,
}

func computeNesting(node *sitter.Node, depth int) int {
	maxD := depth
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		childDepth := depth
		if nestingTypes[child.Type()] {
			childDepth = depth + 1
		}
		if d := computeNesting(child, childDepth); d > maxD {
			maxD = d
		}
	}
	return maxD
}

func isTestParam(params *sitter.Node, src []byte) bool {
	content := params.Content(src)
	return strings.Contains(content, "*testing.T") || strings.Contains(content, "*testing.B") ||
		strings.Contains(content, "*testing.F") || strings.Contains(content, "*testing.M")
}

func isHTTPHandlerSignature(params *sitter.Node, src []byte) bool {
	if params == nil {
		return false
	}
	content := params.Content(src)
	return strings.Contains(content, "http.ResponseWriter") && strings.Contains(content, "*http.Request")
}

func isExported(name string) bool {
	if name == "" {
		return false
	}
	return name[0] >= 'A' && name[0] <= 'Z'
}
