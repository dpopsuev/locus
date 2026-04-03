package survey

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/dpopsuev/locus/internal/model"
)

// PythonScanner extracts structural metadata from Python projects by parsing
// pyproject.toml (or setup.py) for project metadata and dependencies, and
// scanning .py source files for import/def/class declarations. Zero external
// tool dependency.
type PythonScanner struct{}

func (s *PythonScanner) Scan(root string) (*model.Project, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	projName := detectPythonProjectName(absRoot)
	proj := &model.Project{
		Path:            projName,
		Language:        model.LangPython,
		DependencyGraph: model.NewDependencyGraph(),
	}

	externalDeps := detectPythonDependencies(absRoot)

	packages := discoverPythonPackages(absRoot)

	pkgSet := make(map[string]bool, len(packages))
	for _, pkg := range packages {
		pkgSet[pkg] = true
	}

	sort.Strings(packages)
	for _, pkgPath := range packages {
		relPath := pkgPath
		importPath := strings.ReplaceAll(relPath, "/", ".")
		ns := model.NewNamespace(filepath.Base(pkgPath), importPath)

		fullDir := filepath.Join(absRoot, pkgPath)
		s.extractPythonSymbols(fullDir, ns)
		s.extractPythonImports(fullDir, ns, importPath, pkgSet, externalDeps, proj)
		proj.AddNamespace(ns)
	}

	return proj, nil
}

// --- Project metadata detection ---

type pyprojectTOML struct {
	Project struct {
		Name         string   `toml:"name"`
		Dependencies []string `toml:"dependencies"`
	} `toml:"project"`
	Tool struct {
		Poetry struct {
			Name         string                 `toml:"name"`
			Dependencies map[string]interface{} `toml:"dependencies"`
		} `toml:"poetry"`
	} `toml:"tool"`
}

func detectPythonProjectName(root string) string {
	var pp pyprojectTOML
	if _, err := toml.DecodeFile(filepath.Join(root, "pyproject.toml"), &pp); err == nil {
		if pp.Project.Name != "" {
			return pp.Project.Name
		}
		if pp.Tool.Poetry.Name != "" {
			return pp.Tool.Poetry.Name
		}
	}

	if data, err := os.ReadFile(filepath.Join(root, "setup.py")); err == nil {
		if name := parseSetupPyName(string(data)); name != "" {
			return name
		}
	}

	return filepath.Base(root)
}

var reSetupName = regexp.MustCompile(`name\s*=\s*['"]([^'"]+)['"]`)

func parseSetupPyName(content string) string {
	if m := reSetupName.FindStringSubmatch(content); m != nil {
		return m[1]
	}
	return ""
}

func detectPythonDependencies(root string) map[string]bool {
	deps := make(map[string]bool)

	var pp pyprojectTOML
	if _, err := toml.DecodeFile(filepath.Join(root, "pyproject.toml"), &pp); err == nil {
		for _, dep := range pp.Project.Dependencies {
			name := normalizePipDep(dep)
			if name != "" {
				deps[name] = true
			}
		}
		for name := range pp.Tool.Poetry.Dependencies {
			if name != "python" {
				deps[normalizePkgName(name)] = true
			}
		}
	}

	if f, err := os.Open(filepath.Join(root, "requirements.txt")); err == nil {
		defer f.Close()
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "-") {
				continue
			}
			name := normalizePipDep(line)
			if name != "" {
				deps[name] = true
			}
		}
	}

	return deps
}

func normalizePipDep(dep string) string {
	dep = strings.TrimSpace(dep)
	for _, sep := range []string{">=", "<=", "==", "!=", "~=", ">", "<", "[", ";"} {
		if idx := strings.Index(dep, sep); idx > 0 {
			dep = dep[:idx]
		}
	}
	return normalizePkgName(strings.TrimSpace(dep))
}

func normalizePkgName(name string) string {
	return strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(name, "-", "_"), ".", "_"))
}

// --- Package discovery ---

func discoverPythonPackages(root string) []string {
	var packages []string

	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}

		base := d.Name()
		if ShouldSkipPythonDir(base) {
			return filepath.SkipDir
		}

		if path == root {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)

		if hasPythonFiles(path) {
			packages = append(packages, rel)
		}
		return nil
	})

	if len(packages) == 0 && hasPythonFiles(root) {
		packages = append(packages, ".")
	}

	return packages
}

func hasPythonFiles(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".py") {
			return true
		}
	}
	return false
}

// --- Symbol extraction ---

var (
	rePyDef      = regexp.MustCompile(`^def\s+(\w+)\s*\(`)
	rePyAsyncDef = regexp.MustCompile(`^async\s+def\s+(\w+)\s*\(`)
	rePyClass    = regexp.MustCompile(`^class\s+(\w+)[\s:(]`)
)

func (s *PythonScanner) extractPythonSymbols(dir string, ns *model.Namespace) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	seen := make(map[string]bool)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".py") {
			continue
		}

		fullPath := filepath.Join(dir, entry.Name())
		fileObj := model.NewFile(entry.Name(), ns.Name)

		f, err := os.Open(fullPath)
		if err != nil {
			continue
		}

		lineCount := 0
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			lineCount++
			line := sc.Text()
			if m := rePyDef.FindStringSubmatch(line); m != nil {
				addPythonSymbol(ns, seen, m[1], model.SymbolFunction)
			} else if m := rePyAsyncDef.FindStringSubmatch(line); m != nil {
				addPythonSymbol(ns, seen, m[1], model.SymbolFunction)
			} else if m := rePyClass.FindStringSubmatch(line); m != nil {
				addPythonSymbol(ns, seen, m[1], model.SymbolClass)
			}
		}
		f.Close()
		fileObj.Lines = lineCount
		ns.AddFile(fileObj)
	}
}

func addPythonSymbol(ns *model.Namespace, seen map[string]bool, name string, kind model.SymbolKind) {
	if seen[name] {
		return
	}
	seen[name] = true
	ns.AddSymbol(&model.Symbol{
		Name:     name,
		Kind:     kind,
		Exported: !strings.HasPrefix(name, "_"),
	})
}

// --- Import extraction ---

var (
	rePyImport     = regexp.MustCompile(`^import\s+(\S+)`)
	rePyFromImport = regexp.MustCompile(`^from\s+(\S+)\s+import\s+`)
)

func (s *PythonScanner) extractPythonImports(dir string, _ *model.Namespace, importPath string, internalPkgs, _ map[string]bool, proj *model.Project) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	seen := make(map[string]bool)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".py") {
			continue
		}

		fullPath := filepath.Join(dir, entry.Name())
		f, err := os.Open(fullPath)
		if err != nil {
			continue
		}

		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())

			var module string
			if m := rePyFromImport.FindStringSubmatch(line); m != nil {
				module = m[1]
			} else if m := rePyImport.FindStringSubmatch(line); m != nil {
				module = m[1]
			}

			if module == "" || strings.HasPrefix(module, ".") {
				continue
			}

			topLevel := strings.SplitN(module, ".", 2)[0]
			dotPath := module

			internalKey := strings.ReplaceAll(dotPath, ".", "/")
			if seen[internalKey] {
				continue
			}
			seen[internalKey] = true

			if matchesInternalPackage(internalKey, internalPkgs) {
				target := strings.ReplaceAll(internalKey, "/", ".")
				if target != importPath {
					proj.DependencyGraph.AddEdge(importPath, target, false)
				}
			} else {
				normalized := normalizePkgName(topLevel)
				if !seen["ext:"+normalized] {
					seen["ext:"+normalized] = true
					proj.DependencyGraph.AddEdge(importPath, normalized, true)
				}
			}
		}
		f.Close()
	}
}

func matchesInternalPackage(importKey string, pkgSet map[string]bool) bool {
	if pkgSet[importKey] {
		return true
	}
	for pkg := range pkgSet {
		if strings.HasPrefix(importKey, pkg+"/") || strings.HasPrefix(pkg, importKey+"/") {
			return true
		}
	}
	return false
}
