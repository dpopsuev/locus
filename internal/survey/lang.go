package survey

import (
	"strings"

	"github.com/dpopsuev/locus/internal/model"
)

// LanguageMarker maps a project manifest file to its language.
type LanguageMarker struct {
	File string
	Lang model.Language
}

// LanguageMarkers is the canonical list of file→language mappings,
// ordered by specificity (most specific first, ambiguous markers last).
var LanguageMarkers = []LanguageMarker{
	{"go.mod", model.LangGo},
	{"Cargo.toml", model.LangRust},
	{"CMakeLists.txt", model.LangCpp},
	{"pyproject.toml", model.LangPython},
	{"setup.py", model.LangPython},
	{"tsconfig.json", model.LangTypeScript},
	{"package.json", model.LangTypeScript},
	{"Makefile", model.LangC},
}

// RootProjectMarkers is the subset of LanguageMarkers used for
// discovering sub-projects at the root of a polyglot repo.
// TypeScript is excluded here because it's discovered via directory walk.
var RootProjectMarkers = []LanguageMarker{
	{"go.mod", model.LangGo},
	{"Cargo.toml", model.LangRust},
	{"pyproject.toml", model.LangPython},
	{"setup.py", model.LangPython},
}

// CommonSkipDirs are directories skipped by all scanners.
var CommonSkipDirs = map[string]bool{
	"vendor":       true,
	"testdata":     true,
	"node_modules": true,
	"dist":         true,
	"build":        true,
	"target":       true,
	".git":         true,
	".hg":          true,
	".svn":         true,
	".mos":         true,
}

// ShouldSkipDir returns true if the directory should be skipped during scanning.
// It checks common skip dirs and hidden directories (dot-prefixed).
func ShouldSkipDir(name string) bool {
	if CommonSkipDirs[name] {
		return true
	}
	return strings.HasPrefix(name, ".")
}

// PythonSkipDirs are additional directories skipped for Python projects.
var PythonSkipDirs = map[string]bool{
	"__pycache__":   true,
	".tox":          true,
	".nox":          true,
	".mypy_cache":   true,
	".pytest_cache": true,
	".ruff_cache":   true,
	"venv":          true,
	".venv":         true,
	"env":           true,
	".env":          true,
	".eggs":         true,
}

// ShouldSkipPythonDir returns true if the directory should be skipped for Python scanning.
func ShouldSkipPythonDir(name string) bool {
	if PythonSkipDirs[name] || strings.HasSuffix(name, ".egg-info") {
		return true
	}
	return ShouldSkipDir(name)
}

// TSSkipDirs are additional directories skipped for TypeScript projects.
var TSSkipDirs = map[string]bool{
	".next":    true,
	"coverage": true,
}

// ShouldSkipTSDir returns true if the directory should be skipped for TypeScript scanning.
func ShouldSkipTSDir(name string) bool {
	if TSSkipDirs[name] {
		return true
	}
	return ShouldSkipDir(name)
}

// ScannerForLang returns the appropriate scanner for a detected language.
func ScannerForLang(lang model.Language) Scanner {
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
		return &CtagsScanner{}
	default:
		return &CtagsScanner{}
	}
}

// DefaultLSPServers maps languages to their conventional LSP server commands.
var DefaultLSPServers = map[model.Language]string{
	model.LangGo:         "gopls serve",
	model.LangRust:       "rust-analyzer",
	model.LangPython:     "pyright-langserver --stdio",
	model.LangTypeScript: "typescript-language-server --stdio",
	model.LangC:          "clangd",
	model.LangCpp:        "clangd",
}
