package survey

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/dpopsuev/locus/internal/model"
	"github.com/dpopsuev/oculus/lang"
)

// LanguageSupport describes how to scan a particular language.
type LanguageSupport struct {
	Language       model.Language
	Markers        []string                  // project marker files (go.mod, Cargo.toml, etc.)
	ScannerFactory func(root string) Scanner // creates a scanner for this language
	SkipDirs       map[string]bool           // language-specific directories to skip
	Rules          lang.Rules                // per-language naming rules for symbol quality
}

var (
	langRegistry   = make(map[model.Language]*LanguageSupport)
	langRegistryMu sync.RWMutex
)

// Register adds a language to the scanner registry.
func Register(ls *LanguageSupport) {
	langRegistryMu.Lock()
	defer langRegistryMu.Unlock()
	langRegistry[ls.Language] = ls
}

// GetLanguageSupport returns the registered support for a language, or nil.
func GetLanguageSupport(lang model.Language) *LanguageSupport {
	langRegistryMu.RLock()
	defer langRegistryMu.RUnlock()
	return langRegistry[lang]
}

// DetectFromMarkers checks for project marker files and returns the detected language.
// It checks markers in registration order — first match wins.
func DetectFromMarkers(root string) model.Language {
	langRegistryMu.RLock()
	defer langRegistryMu.RUnlock()

	// Check in a deterministic order: Go first (most common in this project).
	order := []model.Language{
		model.LangGo, model.LangRust, model.LangPython, model.LangTypeScript,
		model.LangJava, model.LangKotlin, model.LangSwift, model.LangCSharp,
		model.LangZig, model.LangLua, model.LangC, model.LangCpp,
		model.LangJavaScript, model.LangProto, model.LangShell,
	}
	for _, lang := range order {
		ls, ok := langRegistry[lang]
		if !ok {
			continue
		}
		for _, marker := range ls.Markers {
			if _, err := os.Stat(filepath.Join(root, marker)); err == nil {
				return lang
			}
		}
	}
	return model.LangUnknown
}

// ScannerFromRegistry creates a scanner for the given language using the registry.
// Returns nil if the language is not registered.
func ScannerFromRegistry(lang model.Language, root string) Scanner {
	ls := GetLanguageSupport(lang)
	if ls == nil || ls.ScannerFactory == nil {
		return nil
	}
	return ls.ScannerFactory(root)
}

func init() {
	Register(&LanguageSupport{
		Language: model.LangGo,
		Markers:  []string{"go.mod"},
		ScannerFactory: func(_ string) Scanner {
			return &PackagesScanner{Fallback: &GoScanner{}}
		},
		Rules: &lang.GoRules{},
	})
	Register(&LanguageSupport{
		Language: model.LangRust,
		Markers:  []string{"Cargo.toml"},
		ScannerFactory: func(_ string) Scanner {
			return &RustScanner{}
		},
		Rules: &lang.RustRules{},
	})
	Register(&LanguageSupport{
		Language: model.LangPython,
		Markers:  []string{"pyproject.toml", "setup.py", "setup.cfg", "requirements.txt"},
		ScannerFactory: func(_ string) Scanner {
			return &PythonScanner{}
		},
		SkipDirs: PythonSkipDirs,
		Rules:    &lang.PythonRules{},
	})
	Register(&LanguageSupport{
		Language: model.LangTypeScript,
		Markers:  []string{"tsconfig.json"},
		ScannerFactory: func(_ string) Scanner {
			return &TypeScriptScanner{}
		},
		SkipDirs: TSSkipDirs,
		Rules:    &lang.TypeScriptRules{},
	})
	Register(&LanguageSupport{
		Language: model.LangC,
		Markers:  []string{"CMakeLists.txt"},
		ScannerFactory: func(_ string) Scanner {
			return &CtagsScanner{}
		},
		Rules: &lang.GenericRules{},
	})
	Register(&LanguageSupport{
		Language: model.LangCpp,
		Markers:  []string{"CMakeLists.txt"},
		ScannerFactory: func(_ string) Scanner {
			return &CtagsScanner{}
		},
		Rules: &lang.GenericRules{},
	})
	Register(&LanguageSupport{
		Language: model.LangJava,
		Markers:  []string{"pom.xml", "build.gradle"},
		ScannerFactory: func(_ string) Scanner {
			return &CtagsScanner{}
		},
		Rules: &lang.GenericRules{},
	})
	Register(&LanguageSupport{
		Language: model.LangJavaScript,
		Markers:  []string{"package.json"},
		ScannerFactory: func(_ string) Scanner {
			return &TypeScriptScanner{}
		},
		SkipDirs: TSSkipDirs,
		Rules:    &lang.TypeScriptRules{},
	})
	Register(&LanguageSupport{
		Language: model.LangKotlin,
		Markers:  []string{"build.gradle.kts"},
		ScannerFactory: func(_ string) Scanner {
			return &CtagsScanner{}
		},
		Rules: &lang.GenericRules{},
	})
	Register(&LanguageSupport{
		Language: model.LangZig,
		Markers:  []string{"build.zig"},
		ScannerFactory: func(_ string) Scanner {
			return &CtagsScanner{}
		},
		Rules: &lang.GenericRules{},
	})
	Register(&LanguageSupport{
		Language: model.LangCSharp,
		Markers:  []string{"global.json", "Directory.Build.props"},
		ScannerFactory: func(_ string) Scanner {
			return &CtagsScanner{}
		},
		Rules: &lang.GenericRules{},
	})
	Register(&LanguageSupport{
		Language: model.LangSwift,
		Markers:  []string{"Package.swift"},
		ScannerFactory: func(_ string) Scanner {
			return &CtagsScanner{}
		},
		Rules: &lang.GenericRules{},
	})
}
