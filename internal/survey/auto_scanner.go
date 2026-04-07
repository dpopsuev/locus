package survey

import (
	"os/exec"
	"path/filepath"

	"github.com/dpopsuev/locus/internal/model"
	"github.com/dpopsuev/oculus/lang"
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

	// For languages with dedicated scanners, use the shared registry.
	if lang != model.LangUnknown {
		return ScannerForLang(lang)
	}

	// Unknown language: try LSP, fall back to ctags.
	cmd := s.LSPCmd
	if cmd == "" {
		cmd = DefaultLSPServer(lang)
	}
	if cmd != "" {
		if _, err := exec.LookPath(splitFirst(cmd)); err == nil {
			return &LSPScanner{ServerCmd: cmd}
		}
	}
	return &CtagsScanner{}
}

// DetectLanguage inspects marker files in root to determine the project language.
func DetectLanguage(root string) model.Language {
	return ToModelLanguage(lang.DetectLanguage(root))
}

// DefaultLSPServer returns the conventional LSP server command for a language.
func DefaultLSPServer(l model.Language) string {
	return DefaultLSPServers[l]
}

func splitFirst(cmd string) string {
	for i, c := range cmd {
		if c == ' ' {
			return cmd[:i]
		}
	}
	return cmd
}
