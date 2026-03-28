package survey

import (
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/dpopsuev/locus/internal/model"
)

var (
	errNoPackages = errors.New("go/packages returned no packages")
	errGoPackages = errors.New("go/packages error")
)

// PackagesScanner extracts structural metadata from Go source using
// golang.org/x/tools/go/packages. It provides richer data than GoScanner
// (resolved types, symbol-level dependencies, build-tag awareness).
// Falls back to GoScanner when go/packages loading fails.
type PackagesScanner struct {
	Fallback *GoScanner
}

func (s *PackagesScanner) Scan(root string) (*model.Project, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	// Ensure Go modules are available. Handles container scans where the
	// module cache is empty but the workspace is mounted read-only.
	ensureGoModules(absRoot)

	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedImports |
			packages.NeedTypes | packages.NeedTypesInfo | packages.NeedSyntax |
			packages.NeedModule,
		Dir: absRoot,
	}

	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		return s.fallback(root, err)
	}

	if len(pkgs) == 0 {
		return s.fallback(root, errNoPackages)
	}

	// Check for load errors that indicate a broken environment.
	for _, pkg := range pkgs {
		for _, e := range pkg.Errors {
			if strings.Contains(e.Msg, "cannot find module") ||
				strings.Contains(e.Msg, "not a module") {
				return s.fallback(root, fmt.Errorf("%w: %s", errGoPackages, e.Msg))
			}
		}
	}

	modPath := detectModulePath(pkgs)
	proj := model.NewProject(modPath)
	proj.Language = model.LangGo
	proj.DependencyGraph = model.NewDependencyGraph()

	for _, pkg := range pkgs {
		if pkg.Name == "" {
			continue
		}

		ns := model.NewNamespace(pkg.Name, pkg.PkgPath)

		for _, f := range pkg.GoFiles {
			rel, relErr := filepath.Rel(absRoot, f)
			if relErr != nil {
				rel = f
			}
			ns.AddFile(model.NewFile(filepath.ToSlash(rel), pkg.Name))
		}

		extractTypedSymbols(pkg, ns)

		usageCounts := countSymbolUsages(pkg)
		for impPath := range pkg.Imports {
			external := !strings.HasPrefix(impPath, modPath)
			count := usageCounts[impPath]
			if count == 0 {
				count = 1
			}
			for range count {
				proj.DependencyGraph.AddEdge(pkg.PkgPath, impPath, external)
			}
		}

		coupling := computeCoupling(pkg)
		for impPath, c := range coupling {
			proj.DependencyGraph.SetEdgeCoupling(pkg.PkgPath, impPath, c.callSites, c.locSurface)
		}

		proj.AddNamespace(ns)
	}

	return proj, nil
}

func (s *PackagesScanner) fallback(root string, _ error) (*model.Project, error) {
	fb := s.Fallback
	if fb == nil {
		fb = &GoScanner{}
	}
	return fb.Scan(root)
}

func detectModulePath(pkgs []*packages.Package) string {
	for _, pkg := range pkgs {
		if pkg.Module != nil {
			return pkg.Module.Path
		}
	}
	if len(pkgs) > 0 {
		return pkgs[0].PkgPath
	}
	return ""
}

func extractTypedSymbols(pkg *packages.Package, ns *model.Namespace) {
	seen := make(map[string]bool)

	// Collect symbol-level dependencies from type info.
	symbolDeps := collectSymbolDeps(pkg)

	for _, f := range pkg.Syntax {
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
				sym := &model.Symbol{
					Name:     name,
					Kind:     model.SymbolFunction,
					Exported: ast.IsExported(name),
				}
				if deps, ok := symbolDeps[name]; ok {
					for dep := range deps {
						sym.Dependencies = append(sym.Dependencies, dep)
					}
				}
				ns.AddSymbol(sym)

			case *ast.GenDecl:
				extractGenDeclSymbols(d, ns, seen)
			}
		}
	}
}

// collectSymbolDeps returns per-function external package dependencies.
func collectSymbolDeps(pkg *packages.Package) map[string]map[string]bool {
	symbolDeps := make(map[string]map[string]bool)
	if pkg.TypesInfo == nil {
		return symbolDeps
	}
	for ident, obj := range pkg.TypesInfo.Uses {
		if obj.Pkg() == nil || obj.Pkg() == pkg.Types {
			continue
		}
		enclosing := enclosingFuncName(pkg, ident.Pos())
		if enclosing == "" {
			continue
		}
		if symbolDeps[enclosing] == nil {
			symbolDeps[enclosing] = make(map[string]bool)
		}
		symbolDeps[enclosing][obj.Pkg().Path()] = true
	}
	return symbolDeps
}

type couplingInfo struct {
	callSites  int
	locSurface int
}

// computeCoupling counts total call sites (every reference to an external
// symbol, including duplicates) and LOC surface (distinct source lines that
// reference an external package) per dependency.
func computeCoupling(pkg *packages.Package) map[string]*couplingInfo {
	result := make(map[string]*couplingInfo)
	if pkg.TypesInfo == nil {
		return result
	}

	linesSeen := make(map[string]map[int]bool)

	for ident, obj := range pkg.TypesInfo.Uses {
		if obj.Pkg() == nil || obj.Pkg() == pkg.Types {
			continue
		}
		impPath := obj.Pkg().Path()

		if result[impPath] == nil {
			result[impPath] = &couplingInfo{}
			linesSeen[impPath] = make(map[int]bool)
		}

		result[impPath].callSites++

		pos := pkg.Fset.Position(ident.Pos())
		if pos.IsValid() && !linesSeen[impPath][pos.Line] {
			linesSeen[impPath][pos.Line] = true
			result[impPath].locSurface++
		}
	}

	return result
}

// countSymbolUsages counts how many distinct symbols are used from each
// imported package. Returns map[importPath] -> usageCount.
func countSymbolUsages(pkg *packages.Package) map[string]int {
	counts := make(map[string]int)
	if pkg.TypesInfo == nil {
		return counts
	}
	seen := make(map[string]bool)
	for ident, obj := range pkg.TypesInfo.Uses {
		if obj.Pkg() == nil || obj.Pkg() == pkg.Types {
			continue
		}
		key := obj.Pkg().Path() + "." + ident.Name
		if seen[key] {
			continue
		}
		seen[key] = true
		counts[obj.Pkg().Path()]++
	}
	return counts
}

// enclosingFuncName returns the name of the top-level function containing pos,
// or "" if pos is not inside a function body.
func enclosingFuncName(pkg *packages.Package, pos token.Pos) string {
	for _, f := range pkg.Syntax {
		for _, decl := range f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Body == nil || fd.Recv != nil {
				continue
			}
			if fd.Body.Pos() <= pos && pos <= fd.Body.End() {
				return fd.Name.Name
			}
		}
	}
	return ""
}

// ensureGoModules runs go mod download (or go work download) to populate
// the module cache. Handles container deployments where the workspace is
// :ro but Go toolchain can write to its own module cache.
// Errors are silently ignored — PackagesScanner falls back to GoScanner.
func ensureGoModules(root string) {
	// Vendor mode — modules already local.
	if _, err := os.Stat(filepath.Join(root, "vendor")); err == nil {
		return
	}
	// go.work — multi-module workspace.
	if _, err := os.Stat(filepath.Join(root, "go.work")); err == nil {
		cmd := exec.Command("go", "work", "download")
		cmd.Dir = root
		_ = cmd.Run()
		return
	}
	// go.mod — standard module.
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
		cmd := exec.Command("go", "mod", "download")
		cmd.Dir = root
		_ = cmd.Run()
	}
}
