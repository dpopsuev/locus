package survey

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/dpopsuev/locus/internal/model"
)

// TypeScriptScanner extracts structural metadata from TypeScript/JavaScript
// projects by parsing package.json and scanning source files for import/export
// declarations via regex.
type TypeScriptScanner struct{}

type packageJSON struct {
	Name         string            `json:"name"`
	Dependencies map[string]string `json:"dependencies"`
	DevDeps      map[string]string `json:"devDependencies"`
}

func (s *TypeScriptScanner) Scan(root string) (*model.Project, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	pkg := readPackageJSON(absRoot)

	projName := pkg.Name
	if projName == "" {
		projName = filepath.Base(absRoot)
	}

	proj := &model.Project{
		Path:            projName,
		Language:        model.LangTypeScript,
		DependencyGraph: model.NewDependencyGraph(),
	}

	externalPkgs := make(map[string]bool)
	for dep := range pkg.Dependencies {
		externalPkgs[dep] = true
	}
	for dep := range pkg.DevDeps {
		externalPkgs[dep] = true
	}

	nsMap := make(map[string]*model.Namespace)
	seen := make(map[string]map[string]bool)

	err = filepath.WalkDir(absRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			base := d.Name()
			if base == "node_modules" || base == "dist" || base == "build" ||
				base == ".next" || base == "coverage" || strings.HasPrefix(base, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !isTSFile(d.Name()) {
			return nil
		}

		rel, relErr := filepath.Rel(absRoot, path)
		if relErr != nil {
			rel = path
		}
		rel = filepath.ToSlash(rel)

		dir := filepath.ToSlash(filepath.Dir(rel))
		if dir == "." {
			dir = "(root)"
		}

		ns := nsMap[dir]
		if ns == nil {
			ns = model.NewNamespace(dir, dir)
			nsMap[dir] = ns
			seen[dir] = make(map[string]bool)
		}
		ns.AddFile(model.NewFile(rel, dir))

		f, fErr := os.Open(path)
		if fErr != nil {
			return nil
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Text()
			s.extractExports(line, ns, seen[dir])
			s.extractImportEdge(line, dir, absRoot, externalPkgs, proj.DependencyGraph)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	keys := make([]string, 0, len(nsMap))
	for k := range nsMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		proj.AddNamespace(nsMap[k])
	}

	return proj, nil
}

var (
	reExportFunc      = regexp.MustCompile(`^\s*export\s+(?:async\s+)?function\s+(\w+)`)
	reExportClass     = regexp.MustCompile(`^\s*export\s+(?:abstract\s+)?class\s+(\w+)`)
	reExportInterface = regexp.MustCompile(`^\s*export\s+(?:type\s+)?interface\s+(\w+)`)
	reExportConst     = regexp.MustCompile(`^\s*export\s+(?:const|let|var)\s+(\w+)`)
	reExportType      = regexp.MustCompile(`^\s*export\s+type\s+(\w+)\s*=`)
	reExportEnum      = regexp.MustCompile(`^\s*export\s+(?:const\s+)?enum\s+(\w+)`)

	reImportFrom = regexp.MustCompile(`(?:import|export)\s+.*?\s+from\s+['"]([^'"]+)['"]`)
	reImportSide = regexp.MustCompile(`^\s*import\s+['"]([^'"]+)['"]`)
)

func (s *TypeScriptScanner) extractExports(line string, ns *model.Namespace, seen map[string]bool) {
	if m := reExportFunc.FindStringSubmatch(line); m != nil {
		addTSSymbol(ns, seen, m[1], model.SymbolFunction)
	} else if m := reExportClass.FindStringSubmatch(line); m != nil {
		addTSSymbol(ns, seen, m[1], model.SymbolClass)
	} else if m := reExportEnum.FindStringSubmatch(line); m != nil {
		addTSSymbol(ns, seen, m[1], model.SymbolEnum)
	} else if m := reExportInterface.FindStringSubmatch(line); m != nil {
		addTSSymbol(ns, seen, m[1], model.SymbolInterface)
	} else if m := reExportType.FindStringSubmatch(line); m != nil {
		addTSSymbol(ns, seen, m[1], model.SymbolTypeParameter)
	} else if m := reExportConst.FindStringSubmatch(line); m != nil {
		addTSSymbol(ns, seen, m[1], model.SymbolVariable)
	}
}

func (s *TypeScriptScanner) extractImportEdge(line, fromDir, root string, externalPkgs map[string]bool, graph *model.DependencyGraph) {
	var spec string
	if m := reImportFrom.FindStringSubmatch(line); m != nil {
		spec = m[1]
	} else if m := reImportSide.FindStringSubmatch(line); m != nil {
		spec = m[1]
	}
	if spec == "" {
		return
	}

	if strings.HasPrefix(spec, "./") || strings.HasPrefix(spec, "../") {
		resolved := resolveRelativeImport(fromDir, spec)
		if resolved != fromDir {
			graph.AddEdge(fromDir, resolved, false)
		}
	} else {
		pkgName := barePackageName(spec)
		graph.AddEdge(fromDir, pkgName, true)
	}
}

func resolveRelativeImport(fromDir, spec string) string {
	base := fromDir
	if base == "(root)" {
		base = "."
	}
	resolved := filepath.ToSlash(filepath.Clean(filepath.Join(base, spec)))
	dir := filepath.ToSlash(filepath.Dir(resolved))
	if dir == "." {
		return "(root)"
	}
	return dir
}

func barePackageName(spec string) string {
	if strings.HasPrefix(spec, "@") {
		parts := strings.SplitN(spec, "/", 3)
		if len(parts) >= 2 {
			return parts[0] + "/" + parts[1]
		}
		return spec
	}
	parts := strings.SplitN(spec, "/", 2)
	return parts[0]
}

func addTSSymbol(ns *model.Namespace, seen map[string]bool, name string, kind model.SymbolKind) {
	if seen[name] {
		return
	}
	seen[name] = true
	ns.AddSymbol(&model.Symbol{
		Name:     name,
		Kind:     kind,
		Exported: true,
	})
}

func isTSFile(name string) bool {
	ext := filepath.Ext(name)
	switch ext {
	case ".ts", ".tsx", ".js", ".jsx", ".mts", ".mjs":
		return true
	}
	return false
}

func readPackageJSON(root string) packageJSON {
	var pkg packageJSON
	data, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		return pkg
	}
	_ = json.Unmarshal(data, &pkg)
	return pkg
}
