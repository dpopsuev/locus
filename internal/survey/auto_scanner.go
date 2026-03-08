package survey

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/dpopsuev/locus/internal/model"
)

// AutoScanner selects the best available scanner for a project root.
// Detection order for Go: PackagesScanner -> LSPScanner(gopls) -> GoScanner.
// For non-Go languages: LSPScanner(detected-server) with no offline fallback.
type AutoScanner struct {
	// Override forces a specific scanner backend. Valid values:
	// "auto" (default), "go", "packages", "lsp".
	Override string
	// LSPCmd overrides the LSP server command (e.g. "rust-analyzer").
	LSPCmd string
}

func (s *AutoScanner) Scan(root string) (*model.Project, error) {
	scanner := s.resolve(root)

	if s.Override == "" || s.Override == "auto" {
		absRoot, _ := filepath.Abs(root)
		subs := discoverSubProjects(absRoot)
		if len(subs) > 1 {
			scanner = &CompositeScanner{}
		}
	}

	return scanner.Scan(root)
}

func (s *AutoScanner) resolve(root string) Scanner {
	switch s.Override {
	case "go":
		return &GoScanner{}
	case "packages":
		return &PackagesScanner{Fallback: &GoScanner{}}
	case "lsp":
		cmd := s.LSPCmd
		if cmd == "" {
			lang := DetectLanguage(root)
			cmd = DefaultLSPServer(lang)
		}
		return &LSPScanner{ServerCmd: cmd}
	case "ctags":
		return &CtagsScanner{}
	case "rust":
		return &RustScanner{}
	case "typescript":
		return &TypeScriptScanner{}
	case "python":
		return &PythonScanner{}
	case "composite":
		return &CompositeScanner{}
	}

	lang := DetectLanguage(root)

	switch lang {
	case model.LangGo:
		return &PackagesScanner{Fallback: &GoScanner{}}
	case model.LangRust:
		return &RustScanner{}
	case model.LangTypeScript:
		return &TypeScriptScanner{}
	case model.LangPython:
		return &PythonScanner{}
	case model.LangC, model.LangCpp:
		if _, err := exec.LookPath("ctags"); err == nil {
			return &CtagsScanner{}
		}
		cmd := DefaultLSPServer(lang)
		if cmd != "" {
			if _, err := exec.LookPath(splitFirst(cmd)); err == nil {
				return &LSPScanner{ServerCmd: cmd}
			}
		}
		return &CtagsScanner{}
	default:
		cmd := s.LSPCmd
		if cmd == "" {
			cmd = DefaultLSPServer(lang)
		}
		if cmd != "" {
			if _, err := exec.LookPath(splitFirst(cmd)); err == nil {
				return &LSPScanner{ServerCmd: cmd}
			}
		}
		return &GoScanner{}
	}
}

// DetectLanguage inspects marker files in root to determine the project language.
func DetectLanguage(root string) model.Language {
	markers := []struct {
		file string
		lang model.Language
	}{
		{"go.mod", model.LangGo},
		{"Cargo.toml", model.LangRust},
		{"CMakeLists.txt", model.LangCpp},
		{"pyproject.toml", model.LangPython},
		{"setup.py", model.LangPython},
		{"tsconfig.json", model.LangTypeScript},
		{"package.json", model.LangTypeScript},
		{"Makefile", model.LangC},
	}
	for _, m := range markers {
		if _, err := os.Stat(filepath.Join(root, m.file)); err == nil {
			return m.lang
		}
	}
	return model.LangUnknown
}

// DefaultLSPServer returns the conventional LSP server command for a language.
func DefaultLSPServer(lang model.Language) string {
	switch lang {
	case model.LangGo:
		return "gopls serve"
	case model.LangRust:
		return "rust-analyzer"
	case model.LangPython:
		return "pyright-langserver --stdio"
	case model.LangTypeScript:
		return "typescript-language-server --stdio"
	case model.LangC, model.LangCpp:
		return "clangd"
	default:
		return ""
	}
}

func splitFirst(cmd string) string {
	for i, c := range cmd {
		if c == ' ' {
			return cmd[:i]
		}
	}
	return cmd
}
