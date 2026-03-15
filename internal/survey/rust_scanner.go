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

// RustScanner extracts structural metadata from Rust projects by parsing
// Cargo.toml manifests and scanning source files for pub declarations.
// It handles both single-crate and workspace layouts.
type RustScanner struct{}

type cargoWorkspace struct {
	Members []string `toml:"members"`
}

type cargoPackage struct {
	Name string `toml:"name"`
}

type cargoDep struct {
	Path    string
	Version string
}

type cargoManifest struct {
	Workspace *cargoWorkspace        `toml:"workspace"`
	Package   *cargoPackage          `toml:"package"`
	Deps      map[string]interface{} `toml:"dependencies"`
}

func (s *RustScanner) Scan(root string) (*model.Project, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	var manifest cargoManifest
	if _, err := toml.DecodeFile(filepath.Join(absRoot, "Cargo.toml"), &manifest); err != nil {
		return nil, err
	}

	proj := &model.Project{
		Path:            projectName(manifest, absRoot),
		Language:        model.LangRust,
		DependencyGraph: model.NewDependencyGraph(),
	}

	if manifest.Workspace != nil {
		return s.scanWorkspace(absRoot, manifest, proj)
	}
	return s.scanSingleCrate(absRoot, manifest, proj)
}

func (s *RustScanner) scanWorkspace(root string, manifest cargoManifest, proj *model.Project) (*model.Project, error) {
	crateNames := make(map[string]bool)
	type crateInfo struct {
		name string
		dir  string
	}
	var crates []crateInfo

	for _, member := range manifest.Workspace.Members {
		memberDir := filepath.Join(root, member)
		var cm cargoManifest
		if _, err := toml.DecodeFile(filepath.Join(memberDir, "Cargo.toml"), &cm); err != nil {
			continue
		}
		if cm.Package == nil {
			continue
		}
		crateNames[cm.Package.Name] = true
		crates = append(crates, crateInfo{name: cm.Package.Name, dir: memberDir})
	}

	for _, c := range crates {
		var cm cargoManifest
		_, _ = toml.DecodeFile(filepath.Join(c.dir, "Cargo.toml"), &cm)

		ns := model.NewNamespace(c.name, c.name)
		s.extractRustSymbols(c.dir, ns)
		proj.AddNamespace(ns)

		for depName, depVal := range cm.Deps {
			dep := parseCargoDep(depVal)
			if dep.Path != "" || crateNames[depName] {
				proj.DependencyGraph.AddEdge(c.name, depName, false)
			} else {
				proj.DependencyGraph.AddEdge(c.name, depName, true)
			}
		}
	}

	sort.Slice(proj.Namespaces, func(i, j int) bool {
		return proj.Namespaces[i].Name < proj.Namespaces[j].Name
	})
	return proj, nil
}

func (s *RustScanner) scanSingleCrate(root string, manifest cargoManifest, proj *model.Project) (*model.Project, error) {
	name := proj.Path
	ns := model.NewNamespace(name, name)
	s.extractRustSymbols(root, ns)
	proj.AddNamespace(ns)

	for depName, depVal := range manifest.Deps {
		dep := parseCargoDep(depVal)
		if dep.Path != "" {
			proj.DependencyGraph.AddEdge(name, depName, false)
		} else {
			proj.DependencyGraph.AddEdge(name, depName, true)
		}
	}

	return proj, nil
}

var (
	rePubFn     = regexp.MustCompile(`^\s*pub(?:\(crate\))?\s+(?:async\s+)?fn\s+(\w+)`)
	rePubStruct = regexp.MustCompile(`^\s*pub(?:\(crate\))?\s+struct\s+(\w+)`)
	rePubEnum   = regexp.MustCompile(`^\s*pub(?:\(crate\))?\s+enum\s+(\w+)`)
	rePubTrait  = regexp.MustCompile(`^\s*pub(?:\(crate\))?\s+trait\s+(\w+)`)
	rePubConst  = regexp.MustCompile(`^\s*pub(?:\(crate\))?\s+const\s+(\w+)`)
	rePubType   = regexp.MustCompile(`^\s*pub(?:\(crate\))?\s+type\s+(\w+)`)
)

func (s *RustScanner) extractRustSymbols(crateDir string, ns *model.Namespace) {
	seen := make(map[string]bool)
	_ = filepath.WalkDir(crateDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if ShouldSkipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".rs") {
			return nil
		}

		rel, relErr := filepath.Rel(crateDir, path)
		if relErr != nil {
			rel = path
		}
		ns.AddFile(model.NewFile(filepath.ToSlash(rel), ns.Name))

		f, fErr := os.Open(path)
		if fErr != nil {
			return nil
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Text()
			if m := rePubFn.FindStringSubmatch(line); m != nil {
				addRustSymbol(ns, seen, m[1], model.SymbolFunction)
			} else if m := rePubStruct.FindStringSubmatch(line); m != nil {
				addRustSymbol(ns, seen, m[1], model.SymbolStruct)
			} else if m := rePubEnum.FindStringSubmatch(line); m != nil {
				addRustSymbol(ns, seen, m[1], model.SymbolEnum)
			} else if m := rePubTrait.FindStringSubmatch(line); m != nil {
				addRustSymbol(ns, seen, m[1], model.SymbolInterface)
			} else if m := rePubConst.FindStringSubmatch(line); m != nil {
				addRustSymbol(ns, seen, m[1], model.SymbolConstant)
			} else if m := rePubType.FindStringSubmatch(line); m != nil {
				addRustSymbol(ns, seen, m[1], model.SymbolTypeParameter)
			}
		}
		return nil
	})
}

func addRustSymbol(ns *model.Namespace, seen map[string]bool, name string, kind model.SymbolKind) {
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

func parseCargoDep(v interface{}) cargoDep {
	switch val := v.(type) {
	case string:
		return cargoDep{Version: val}
	case map[string]interface{}:
		d := cargoDep{}
		if p, ok := val["path"].(string); ok {
			d.Path = p
		}
		if ver, ok := val["version"].(string); ok {
			d.Version = ver
		}
		return d
	default:
		return cargoDep{}
	}
}

func projectName(m cargoManifest, root string) string {
	if m.Package != nil && m.Package.Name != "" {
		return m.Package.Name
	}
	return filepath.Base(root)
}
