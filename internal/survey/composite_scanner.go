package survey

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/dpopsuev/locus/internal/model"
)

// CompositeScanner detects multiple sub-projects within a root directory
// and merges their scan results into a single Project. This handles
// polyglot repositories (e.g. Rust backend + TypeScript frontend).
type CompositeScanner struct{}

type subProject struct {
	relPath string
	lang    model.Language
}

func (s *CompositeScanner) Scan(root string) (*model.Project, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	subs := discoverSubProjects(absRoot)
	if len(subs) == 0 {
		return (&AutoScanner{}).Scan(root)
	}

	proj := &model.Project{
		Path:            filepath.Base(absRoot),
		DependencyGraph: model.NewDependencyGraph(),
	}

	if len(subs) == 1 && subs[0].relPath == "." {
		proj.Language = subs[0].lang
	}

	for _, sub := range subs {
		subRoot := filepath.Join(absRoot, sub.relPath)
		sc := ScannerForLang(sub.lang)
		subProj, err := sc.Scan(subRoot)
		if err != nil {
			continue
		}

		prefix := sub.relPath
		for _, ns := range subProj.Namespaces {
			merged := &model.Namespace{
				Name:       ns.Name,
				ImportPath: prefixImportPath(prefix, ns.ImportPath),
				Files:      ns.Files,
				Symbols:    ns.Symbols,
			}
			proj.AddNamespace(merged)
		}

		if subProj.DependencyGraph != nil {
			for _, edge := range subProj.DependencyGraph.Edges {
				proj.DependencyGraph.AddEdge(
					prefixImportPath(prefix, edge.From),
					prefixImportPath(prefix, edge.To),
					edge.External,
				)
			}
		}
	}

	return proj, nil
}

func discoverSubProjects(root string) []subProject {
	var subs []subProject
	seen := make(map[string]bool)

	for _, m := range RootProjectMarkers {
		if _, err := os.Stat(filepath.Join(root, m.File)); err == nil {
			subs = append(subs, subProject{relPath: ".", lang: m.Lang})
			seen["."] = true
		}
	}

	tsMarkers := []string{"package.json", "tsconfig.json"}
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if ShouldSkipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		for _, tsm := range tsMarkers {
			if d.Name() == tsm {
				rel, relErr := filepath.Rel(root, filepath.Dir(path))
				if relErr != nil {
					break
				}
				rel = filepath.ToSlash(rel)
				if seen[rel] {
					break
				}
				if hasNodeModulesParent(rel) {
					break
				}
				seen[rel] = true
				subs = append(subs, subProject{relPath: rel, lang: model.LangTypeScript})
				break
			}
		}

		return nil
	})

	return subs
}

func hasNodeModulesParent(rel string) bool {
	for _, seg := range strings.Split(rel, "/") {
		if seg == "node_modules" {
			return true
		}
	}
	return false
}


func prefixImportPath(prefix, importPath string) string {
	if prefix == "." || prefix == "" {
		return importPath
	}
	return prefix + "/" + importPath
}
