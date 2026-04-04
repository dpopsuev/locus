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
			if ShouldSkipTSDir(d.Name()) {
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
			dir = nsRoot
		}

		ns := nsMap[dir]
		if ns == nil {
			ns = model.NewNamespace(dir, dir)
			nsMap[dir] = ns
			seen[dir] = make(map[string]bool)
		}
		fileObj := model.NewFile(rel, dir)

		f, fErr := os.Open(path)
		if fErr != nil {
			return nil
		}
		defer f.Close()

		lineCount := 0
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			lineCount++
			line := scanner.Text()
			s.extractExports(line, ns, seen[dir], rel, lineCount)
			s.extractImportEdge(line, dir, absRoot, externalPkgs, proj.DependencyGraph)
		}
		fileObj.Lines = lineCount
		ns.AddFile(fileObj)
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

var tsExportPatterns = []symbolPattern{
	{regexp.MustCompile(`^\s*export\s+(?:async\s+)?function\s+(\w+)`), model.SymbolFunction},
	{regexp.MustCompile(`^\s*export\s+(?:abstract\s+)?class\s+(\w+)`), model.SymbolClass},
	{regexp.MustCompile(`^\s*export\s+(?:const\s+)?enum\s+(\w+)`), model.SymbolEnum},
	{regexp.MustCompile(`^\s*export\s+(?:type\s+)?interface\s+(\w+)`), model.SymbolInterface},
	{regexp.MustCompile(`^\s*export\s+type\s+(\w+)\s*=`), model.SymbolTypeParameter},
	{regexp.MustCompile(`^\s*export\s+(?:const|let|var)\s+(\w+)`), model.SymbolVariable},
}

var (
	// Match value imports but NOT type-only imports.
	// `import type { X } from '...'` and `export type { X } from '...'` are
	// compile-time only (erased by TypeScript) and should not create dependency edges.
	reImportFrom     = regexp.MustCompile(`(?:import|export)\s+.*?\s+from\s+['"]([^'"]+)['"]`)
	reImportTypeOnly = regexp.MustCompile(`^\s*(?:import|export)\s+type\s+\{`)
	reImportSide     = regexp.MustCompile(`^\s*import\s+['"]([^'"]+)['"]`)
)

func (s *TypeScriptScanner) extractExports(line string, ns *model.Namespace, seen map[string]bool, filePath string, lineNum int) {
	matchSymbolPatterns(line, tsExportPatterns, ns, seen, true, filePath, lineNum)
}

func (s *TypeScriptScanner) extractImportEdge(line, fromDir, _ string, _ map[string]bool, graph *model.DependencyGraph) {
	// Skip type-only imports — they are erased at compile time and
	// don't create runtime dependencies. Prevents false-positive cycles.
	if reImportTypeOnly.MatchString(line) {
		return
	}

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
	if base == nsRoot {
		base = "."
	}
	resolved := filepath.ToSlash(filepath.Clean(filepath.Join(base, spec)))
	dir := filepath.ToSlash(filepath.Dir(resolved))
	if dir == "." {
		return nsRoot
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
