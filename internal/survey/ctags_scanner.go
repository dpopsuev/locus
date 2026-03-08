package survey

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/dpopsuev/locus/internal/model"
)

// CtagsScanner uses Universal Ctags (--output-format=json) to extract
// symbols from C/C++ (or any ctags-supported language) projects.
// It populates model.Project with one Namespace per directory, one Symbol
// per tag, and extracts #include directives for dependency edges.
type CtagsScanner struct{}

type ctagsEntry struct {
	Type      string `json:"_type"`
	Name      string `json:"name"`
	Path      string `json:"path"`
	Language  string `json:"language"`
	Line      int    `json:"line"`
	Kind      string `json:"kind"`
	Scope     string `json:"scope"`
	ScopeKnd  string `json:"scopeKind"`
	Signature string `json:"signature"`
}

func (s *CtagsScanner) Scan(root string) (*model.Project, error) {
	if _, err := exec.LookPath("ctags"); err != nil {
		return nil, fmt.Errorf("ctags not found; install with: dnf install ctags")
	}

	cmd := exec.Command("ctags", "--output-format=json", "--fields=*", "-R", ".")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ctags: %w", err)
	}

	proj := &model.Project{
		Path:     root,
		Language: DetectLanguage(root),
	}

	dirNS := make(map[string]*model.Namespace)
	fileSet := make(map[string]bool)

	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		var entry ctagsEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry.Type != "tag" {
			continue
		}

		dir := filepath.Dir(entry.Path)
		if dir == "" {
			dir = "."
		}

		ns := dirNS[dir]
		if ns == nil {
			ns = model.NewNamespace(dir, dir)
			dirNS[dir] = ns
		}

		sym := &model.Symbol{
			Name:     entry.Name,
			Kind:     mapCtagsKind(entry.Kind),
			Exported: true,
		}
		ns.AddSymbol(sym)

		if !fileSet[entry.Path] {
			fileSet[entry.Path] = true
			ns.AddFile(model.NewFile(entry.Path, dir))
		}
	}

	for _, ns := range dirNS {
		proj.AddNamespace(ns)
	}

	deps := extractCIncludes(root)
	if deps != nil && len(deps.Edges) > 0 {
		proj.DependencyGraph = deps
	}

	return proj, nil
}

func mapCtagsKind(kind string) model.SymbolKind {
	switch kind {
	case "function":
		return model.SymbolFunction
	case "method":
		return model.SymbolMethod
	case "struct", "union":
		return model.SymbolStruct
	case "class":
		return model.SymbolClass
	case "enum":
		return model.SymbolEnum
	case "variable", "externvar":
		return model.SymbolVariable
	case "macro", "define":
		return model.SymbolConstant
	case "typedef":
		return model.SymbolTypeParameter
	case "member":
		return model.SymbolField
	default:
		return model.SymbolVariable
	}
}

// extractCIncludes scans .c and .h files for #include directives and
// builds a dependency graph mapping source directories to included header dirs.
func extractCIncludes(root string) *model.DependencyGraph {
	deps := model.NewDependencyGraph()

	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if info.Name() == ".git" || info.Name() == ".mos" || info.Name() == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(path)
		if ext != ".c" && ext != ".h" && ext != ".cpp" && ext != ".hpp" && ext != ".cc" {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()

		relPath, _ := filepath.Rel(root, path)
		srcDir := filepath.Dir(relPath)

		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if !strings.HasPrefix(line, "#include") {
				continue
			}
			inc := parseInclude(line)
			if inc == "" {
				continue
			}
			incDir := filepath.Dir(inc)
			if incDir == "." {
				incDir = srcDir
			}
			if incDir != srcDir {
				deps.AddEdge(srcDir, incDir, false)
			}
		}
		return nil
	})
	return deps
}

func parseInclude(line string) string {
	line = strings.TrimPrefix(line, "#include")
	line = strings.TrimSpace(line)
	if len(line) < 2 {
		return ""
	}
	if line[0] == '"' {
		if end := strings.Index(line[1:], "\""); end >= 0 {
			return line[1 : 1+end]
		}
	}
	return ""
}
