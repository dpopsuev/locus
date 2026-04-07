package anchors

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// AnchorKind classifies the role of a semantic anchor.
type AnchorKind string

const pkgMain = "main"

const (
	AnchorEntryPoint   AnchorKind = "entry_point"
	AnchorHTTPHandler  AnchorKind = "http_handler"
	AnchorCLICommand   AnchorKind = "cli_command"
	AnchorConfigSchema AnchorKind = "config_schema"
	AnchorTestSuite    AnchorKind = "test_suite"
)

// SemanticAnchor represents a structurally significant code point.
type SemanticAnchor struct {
	Kind      AnchorKind `json:"kind"`
	Name      string     `json:"name"`
	Package   string     `json:"package"`
	File      string     `json:"file,omitempty"`
	Line      int        `json:"line,omitempty"`
	Signature string     `json:"signature,omitempty"`
}

// ExtractAnchors scans Go source files in the given directory and returns
// detected semantic anchors. This is a heuristic-based extraction that
// identifies structurally important code points without requiring type info.
func ExtractAnchors(root, pkgPath string) []SemanticAnchor {
	absRoot, _ := filepath.Abs(root)
	fset := token.NewFileSet()
	var anchors []SemanticAnchor

	entries, err := os.ReadDir(absRoot)
	if err != nil {
		return nil
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}

		fullPath := filepath.Join(absRoot, entry.Name())
		f, err := parser.ParseFile(fset, fullPath, nil, parser.ParseComments)
		if err != nil {
			continue
		}

		fileAnchors := extractFileAnchors(f, fset, pkgPath, entry.Name())
		anchors = append(anchors, fileAnchors...)
	}

	sort.Slice(anchors, func(i, j int) bool {
		if anchors[i].Kind != anchors[j].Kind {
			return anchors[i].Kind < anchors[j].Kind
		}
		return anchors[i].Name < anchors[j].Name
	})
	return anchors
}

func extractFileAnchors(f *ast.File, fset *token.FileSet, pkgPath, fileName string) []SemanticAnchor {
	var anchors []SemanticAnchor
	pkgName := f.Name.Name
	isTestFile := strings.HasSuffix(fileName, "_test.go")

	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			anchors = append(anchors, extractFuncAnchors(d, fset, pkgPath, pkgName, fileName, isTestFile)...)
		case *ast.GenDecl:
			if d.Tok == token.TYPE {
				anchors = append(anchors, extractTypeAnchors(d, fset, pkgPath, fileName)...)
			}
			if d.Tok == token.VAR {
				anchors = append(anchors, extractVarAnchors(d, fset, pkgPath, fileName)...)
			}
		}
	}
	return anchors
}

func extractFuncAnchors(d *ast.FuncDecl, fset *token.FileSet, pkgPath, pkgName, fileName string, isTestFile bool) []SemanticAnchor {
	var anchors []SemanticAnchor
	pos := fset.Position(d.Pos())
	name := d.Name.Name

	// Entry points: main() and init() in package main
	if d.Recv == nil && pkgName == pkgMain && (name == "main" || name == "init") {
		anchors = append(anchors, SemanticAnchor{
			Kind:    AnchorEntryPoint,
			Name:    name,
			Package: pkgPath,
			File:    fileName,
			Line:    pos.Line,
		})
	}

	// init() in any package
	if d.Recv == nil && name == "init" && pkgName != "main" {
		anchors = append(anchors, SemanticAnchor{
			Kind:    AnchorEntryPoint,
			Name:    pkgName + ".init",
			Package: pkgPath,
			File:    fileName,
			Line:    pos.Line,
		})
	}

	// Test suites: Test*, Benchmark*, Example*, Fuzz*
	if isTestFile && d.Recv == nil {
		if strings.HasPrefix(name, "Test") || strings.HasPrefix(name, "Benchmark") ||
			strings.HasPrefix(name, "Example") || strings.HasPrefix(name, "Fuzz") {
			anchors = append(anchors, SemanticAnchor{
				Kind:    AnchorTestSuite,
				Name:    name,
				Package: pkgPath,
				File:    fileName,
				Line:    pos.Line,
			})
		}
	}

	// HTTP handler heuristics: functions that take (http.ResponseWriter, *http.Request)
	if d.Recv == nil && d.Type.Params != nil && len(d.Type.Params.List) >= 2 {
		if looksLikeHTTPHandler(d.Type.Params.List) {
			anchors = append(anchors, SemanticAnchor{
				Kind:    AnchorHTTPHandler,
				Name:    name,
				Package: pkgPath,
				File:    fileName,
				Line:    pos.Line,
			})
		}
	}

	// Scan function body for handler registrations and cobra commands
	if d.Body != nil {
		anchors = append(anchors, scanBodyForAnchors(d.Body, fset, pkgPath, fileName)...)
	}

	return anchors
}

func extractTypeAnchors(d *ast.GenDecl, fset *token.FileSet, pkgPath, fileName string) []SemanticAnchor {
	var anchors []SemanticAnchor
	for _, spec := range d.Specs {
		ts, ok := spec.(*ast.TypeSpec)
		if !ok {
			continue
		}
		st, ok := ts.Type.(*ast.StructType)
		if !ok || st.Fields == nil {
			continue
		}

		hasConfigTags := false
		for _, field := range st.Fields.List {
			if field.Tag != nil {
				tag := field.Tag.Value
				if strings.Contains(tag, `json:"`) || strings.Contains(tag, `yaml:"`) ||
					strings.Contains(tag, `env:"`) || strings.Contains(tag, `mapstructure:"`) {
					hasConfigTags = true
					break
				}
			}
		}

		if hasConfigTags && ast.IsExported(ts.Name.Name) {
			pos := fset.Position(ts.Pos())
			anchors = append(anchors, SemanticAnchor{
				Kind:    AnchorConfigSchema,
				Name:    ts.Name.Name,
				Package: pkgPath,
				File:    fileName,
				Line:    pos.Line,
			})
		}
	}
	return anchors
}

func extractVarAnchors(d *ast.GenDecl, fset *token.FileSet, pkgPath, fileName string) []SemanticAnchor {
	var anchors []SemanticAnchor
	for _, spec := range d.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for _, val := range vs.Values {
			if isCobraCommand(val) {
				for _, name := range vs.Names {
					pos := fset.Position(vs.Pos())
					anchors = append(anchors, SemanticAnchor{
						Kind:    AnchorCLICommand,
						Name:    name.Name,
						Package: pkgPath,
						File:    fileName,
						Line:    pos.Line,
					})
				}
			}
		}
	}
	return anchors
}

func scanBodyForAnchors(body *ast.BlockStmt, fset *token.FileSet, pkgPath, fileName string) []SemanticAnchor {
	var anchors []SemanticAnchor

	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		funcName := callFuncName(call)
		if isHTTPRegistration(funcName) && len(call.Args) >= 2 {
			pos := fset.Position(call.Pos())
			routeName := extractStringLit(call.Args[0])
			if routeName == "" {
				routeName = funcName
			}
			anchors = append(anchors, SemanticAnchor{
				Kind:    AnchorHTTPHandler,
				Name:    funcName + "(" + routeName + ")",
				Package: pkgPath,
				File:    fileName,
				Line:    pos.Line,
			})
		}
		return true
	})

	return anchors
}

func looksLikeHTTPHandler(params []*ast.Field) bool {
	if len(params) < 2 {
		return false
	}
	first := typeString(params[0].Type)
	second := typeString(params[1].Type)
	return (first == "http.ResponseWriter" || first == "ResponseWriter") &&
		(second == "*http.Request" || second == "*Request")
}

func isHTTPRegistration(name string) bool {
	registrations := []string{
		"HandleFunc", "Handle",
		"http.HandleFunc", "http.Handle",
		"mux.HandleFunc", "mux.Handle",
		"r.HandleFunc", "r.Handle",
		"router.HandleFunc", "router.Handle",
		"GET", "POST", "PUT", "DELETE", "PATCH",
		"r.GET", "r.POST", "r.PUT", "r.DELETE", "r.PATCH",
		"e.GET", "e.POST", "e.PUT", "e.DELETE", "e.PATCH",
	}
	for _, r := range registrations {
		if name == r {
			return true
		}
	}
	return false
}

func isCobraCommand(expr ast.Expr) bool {
	unary, ok := expr.(*ast.UnaryExpr)
	if !ok {
		return false
	}
	comp, ok := unary.X.(*ast.CompositeLit)
	if !ok {
		return false
	}
	return typeString(comp.Type) == "cobra.Command"
}

func callFuncName(call *ast.CallExpr) string {
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		return fn.Name
	case *ast.SelectorExpr:
		if x, ok := fn.X.(*ast.Ident); ok {
			return x.Name + "." + fn.Sel.Name
		}
		return fn.Sel.Name
	}
	return ""
}

func typeString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		if x, ok := t.X.(*ast.Ident); ok {
			return x.Name + "." + t.Sel.Name
		}
	case *ast.StarExpr:
		return "*" + typeString(t.X)
	}
	return ""
}

func extractStringLit(expr ast.Expr) string {
	if lit, ok := expr.(*ast.BasicLit); ok && lit.Kind == token.STRING {
		return strings.Trim(lit.Value, `"`)
	}
	return ""
}
