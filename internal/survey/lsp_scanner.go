package survey

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/dpopsuev/locus/internal/model"
)

var errEmptyServerCmd = errors.New("lsp scanner: empty ServerCmd")

// extToLanguageID maps file extensions to LSP language identifiers.
// Language-agnostic: covers all languages Locus supports.
var extToLanguageID = map[string]string{
	".go":    "go",
	".rs":    "rust",
	".py":    "python",
	".ts":    "typescript",
	".tsx":   "typescriptreact",
	".js":    "javascript",
	".jsx":   "javascriptreact",
	".java":  "java",
	".kt":    "kotlin",
	".kts":   "kotlin",
	".c":     "c",
	".h":     "c",
	".cpp":   "cpp",
	".cc":    "cpp",
	".hpp":   "cpp",
	".cs":    "csharp",
	".swift": "swift",
	".zig":   "zig",
}

// LSPScanner extracts structural metadata by communicating with an
// external LSP server. It is language-agnostic: the same code works
// with gopls, rust-analyzer, pyright, or any LSP-compliant server.
type LSPScanner struct {
	ServerCmd string // e.g. "gopls serve", "rust-analyzer", "pyright-langserver --stdio"
}

func (s *LSPScanner) Scan(root string) (*model.Project, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	parts := strings.Fields(s.ServerCmd)
	if len(parts) == 0 {
		return nil, errEmptyServerCmd
	}

	bin, err := exec.LookPath(parts[0])
	if err != nil {
		return nil, fmt.Errorf("lsp scanner: %s not found on PATH: %w", parts[0], err)
	}

	cmd := exec.Command(bin, parts[1:]...)
	cmd.Dir = absRoot
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("lsp scanner: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("lsp scanner: stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("lsp scanner: start %s: %w", parts[0], err)
	}

	client := newLSPClient(stdout, stdin)

	proj, scanErr := s.runProtocol(client, absRoot)

	// Always attempt clean shutdown.
	_ = shutdownLSP(client)
	stdin.Close()
	_ = cmd.Wait()

	if scanErr != nil {
		return nil, scanErr
	}
	return proj, nil
}

func (s *LSPScanner) runProtocol(client *lspClient, root string) (*model.Project, error) {
	rootURI := pathToURI(root)

	initParams := map[string]any{
		"processId": os.Getpid(),
		"rootUri":   rootURI,
		"capabilities": map[string]any{
			"textDocument": map[string]any{
				"documentSymbol": map[string]any{
					"hierarchicalDocumentSymbolSupport": true,
				},
			},
		},
	}

	_, err := client.Request("initialize", initParams)
	if err != nil {
		return nil, fmt.Errorf("initialize: %w", err)
	}

	if err := client.Notify("initialized", struct{}{}); err != nil {
		return nil, fmt.Errorf("initialized notification: %w", err)
	}

	goFiles, err := findSourceFiles(root)
	if err != nil {
		return nil, fmt.Errorf("find source files: %w", err)
	}

	proj := model.NewProject(filepath.Base(root))
	proj.DependencyGraph = model.NewDependencyGraph()

	nsMap := make(map[string]*model.Namespace)

	for _, f := range goFiles {
		fileURI := pathToURI(f)
		content, readErr := os.ReadFile(f)
		if readErr != nil {
			continue
		}

		langID := extToLanguageID[filepath.Ext(f)]
		if langID == "" {
			langID = "plaintext"
		}
		err := client.Notify("textDocument/didOpen", map[string]any{
			"textDocument": map[string]any{
				"uri":        fileURI,
				"languageId": langID,
				"version":    1,
				"text":       string(content),
			},
		})
		if err != nil {
			continue
		}

		result, err := client.Request("textDocument/documentSymbol", map[string]any{
			"textDocument": map[string]string{"uri": fileURI},
		})
		if err != nil {
			continue
		}

		var symbols []lspDocumentSymbol
		if json.Unmarshal(result, &symbols) != nil {
			var flat []lspSymbolInformation
			if json.Unmarshal(result, &flat) == nil {
				for _, sym := range flat {
					addSymbolToNS(nsMap, sym.ContainerName, sym.Name, sym.Kind, "", 0)
				}
			}
			continue
		}

		rel, relErr := filepath.Rel(root, f)
		if relErr != nil {
			rel = f
		}
		dir := filepath.Dir(rel)
		nsKey := filepath.ToSlash(dir)
		if nsKey == "." {
			nsKey = nsRoot
		}

		for _, sym := range symbols {
			line := 0
			if sym.Range.Start.Line > 0 {
				line = sym.Range.Start.Line + 1 // LSP lines are 0-based
			}
			addSymbolToNS(nsMap, nsKey, sym.Name, sym.Kind, filepath.ToSlash(rel), line)
		}
	}

	for _, ns := range nsMap {
		proj.AddNamespace(ns)
	}

	return proj, nil
}

func addSymbolToNS(nsMap map[string]*model.Namespace, nsKey, name string, kind int, filePath string, line int) {
	if nsKey == "" {
		nsKey = nsRoot
	}
	ns := nsMap[nsKey]
	if ns == nil {
		ns = model.NewNamespace(nsKey, nsKey)
		nsMap[nsKey] = ns
	}
	ns.AddSymbol(&model.Symbol{
		Name:     name,
		Kind:     model.SymbolKind(kind),
		Exported: isExportedSymbol(name),
		File:     filePath,
		Line:     line,
	})
}

func findSourceFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			base := d.Name()
			if ShouldSkipDir(base) {
				return filepath.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(d.Name())
		if _, ok := extToLanguageID[ext]; ok {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

func shutdownLSP(client *lspClient) error {
	_, err := client.Request("shutdown", nil)
	if err != nil {
		return err
	}
	return client.Notify("exit", nil)
}

func pathToURI(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	slash := filepath.ToSlash(abs)
	if !strings.HasPrefix(slash, "/") {
		slash = "/" + slash
	}
	return "file://" + slash
}

func isExportedSymbol(name string) bool {
	if name == "" {
		return false
	}
	c := name[0]
	return c >= 'A' && c <= 'Z'
}

type lspSymbolInformation struct {
	Name          string `json:"name"`
	Kind          int    `json:"kind"`
	ContainerName string `json:"containerName"`
	Location      struct {
		URI string `json:"uri"`
	} `json:"location"`
}

type lspDocumentSymbol struct {
	Name     string              `json:"name"`
	Kind     int                 `json:"kind"`
	Range    lspRange            `json:"range"`
	Children []lspDocumentSymbol `json:"children,omitempty"`
}

type lspRange struct {
	Start lspPosition `json:"start"`
	End   lspPosition `json:"end"`
}

type lspPosition struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}
