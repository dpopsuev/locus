package survey

import (
	"bufio"
	"cmp"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/dpopsuev/locus/internal/model"
)

var errNoModuleDirective = errors.New("no module directive found")

// GoScanner extracts structural metadata from Go source trees.
type GoScanner struct{}

func (s *GoScanner) Scan(root string) (*model.Project, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	modPath, err := readModulePath(filepath.Join(absRoot, "go.mod"))
	if err != nil {
		return nil, err
	}

	mod := model.NewProject(modPath)
	mod.Language = model.LangGo
	mod.DependencyGraph = model.NewDependencyGraph()

	pkgs := make(map[string]*model.Namespace)

	err = filepath.WalkDir(absRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			if ShouldSkipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		if !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}

		rel, err := filepath.Rel(absRoot, path)
		if err != nil {
			return err
		}

		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return nil
		}

		dir := filepath.Dir(rel)
		pkgName := f.Name.Name

		var importPath string
		if dir == "." {
			importPath = modPath
		} else {
			importPath = modPath + "/" + filepath.ToSlash(dir)
		}

		pkg, ok := pkgs[importPath]
		if !ok {
			pkg = model.NewNamespace(pkgName, importPath)
			pkgs[importPath] = pkg
		}

		fileObj := model.NewFile(filepath.ToSlash(rel), pkgName)
		if tokFile := fset.File(f.Pos()); tokFile != nil {
			fileObj.Lines = tokFile.LineCount()
		}
		pkg.AddFile(fileObj)

		extractSymbols(f, pkg)
		extractImports(f, importPath, modPath, mod.DependencyGraph)

		return nil
	})
	if err != nil {
		return nil, err
	}

	for _, p := range slices.Sorted(maps.Keys(pkgs)) {
		pkg := pkgs[p]
		slices.SortFunc(pkg.Files, func(a, b *model.File) int {
			return cmp.Compare(a.Path, b.Path)
		})
		mod.AddNamespace(pkg)
	}

	return mod, nil
}

func extractSymbols(f *ast.File, pkg *model.Namespace) {
	seen := make(map[string]bool)
	for _, s := range pkg.Symbols {
		seen[s.Name] = true
	}

	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Recv != nil {
				continue
			}
			name := d.Name.Name
			if seen[name] {
				continue
			}
			seen[name] = true
			pkg.AddSymbol(&model.Symbol{
				Name:     name,
				Kind:     model.SymbolFunction,
				Exported: ast.IsExported(name),
			})

		case *ast.GenDecl:
			extractGenDeclSymbols(d, pkg, seen)
		}
	}
}

// extractGenDeclSymbols extracts type and value symbols from a GenDecl and
// adds them to the namespace. Shared between GoScanner and PackagesScanner.
func extractGenDeclSymbols(d *ast.GenDecl, ns *model.Namespace, seen map[string]bool) {
	for _, spec := range d.Specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			name := s.Name.Name
			if seen[name] {
				continue
			}
			seen[name] = true
			kind := model.SymbolStruct
			if _, ok := s.Type.(*ast.InterfaceType); ok {
				kind = model.SymbolInterface
			}
			ns.AddSymbol(&model.Symbol{
				Name:     name,
				Kind:     kind,
				Exported: ast.IsExported(name),
			})

		case *ast.ValueSpec:
			for _, ident := range s.Names {
				name := ident.Name
				if seen[name] {
					continue
				}
				seen[name] = true
				kind := model.SymbolVariable
				if d.Tok == token.CONST {
					kind = model.SymbolConstant
				}
				ns.AddSymbol(&model.Symbol{
					Name:     name,
					Kind:     kind,
					Exported: ast.IsExported(name),
				})
			}
		}
	}
}

func extractImports(f *ast.File, fromPkg, modPath string, graph *model.DependencyGraph) {
	for _, imp := range f.Imports {
		to := strings.Trim(imp.Path.Value, `"`)
		external := !strings.HasPrefix(to, modPath)
		graph.AddEdge(fromPkg, to, external)
	}
}

func readModulePath(goModPath string) (string, error) {
	f, err := os.Open(goModPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module ")), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("%s: %w", goModPath, errNoModuleDirective)
}
