package survey

import (
	"github.com/dpopsuev/locus/internal/model"
	"github.com/dpopsuev/locus/internal/oculus/lang"
)

// LanguageMarker is re-exported from oculus/lang for backward compatibility.
type LanguageMarker = lang.LanguageMarker

// LanguageMarkers delegates to oculus/lang.
var LanguageMarkers = lang.LanguageMarkers

// RootProjectMarkers delegates to oculus/lang.
var RootProjectMarkers = lang.RootProjectMarkers

// CommonSkipDirs delegates to oculus/lang.
var CommonSkipDirs = lang.CommonSkipDirs

// PythonSkipDirs delegates to oculus/lang.
var PythonSkipDirs = lang.PythonSkipDirs

// TSSkipDirs delegates to oculus/lang.
var TSSkipDirs = lang.TSSkipDirs

// ShouldSkipDir delegates to oculus/lang.
func ShouldSkipDir(name string) bool { return lang.ShouldSkipDir(name) }

// ShouldSkipPythonDir delegates to oculus/lang.
func ShouldSkipPythonDir(name string) bool { return lang.ShouldSkipPythonDir(name) }

// ShouldSkipTSDir delegates to oculus/lang.
func ShouldSkipTSDir(name string) bool { return lang.ShouldSkipTSDir(name) }

// DefaultLSPServers delegates to oculus/lang (converted to model.Language keys).
var DefaultLSPServers = map[model.Language]string{
	model.LangGo:         lang.DefaultLSPServer(lang.Go),
	model.LangRust:       lang.DefaultLSPServer(lang.Rust),
	model.LangPython:     lang.DefaultLSPServer(lang.Python),
	model.LangTypeScript: lang.DefaultLSPServer(lang.TypeScript),
	model.LangC:          lang.DefaultLSPServer(lang.C),
	model.LangCpp:        lang.DefaultLSPServer(lang.Cpp),
}

// ScannerForLang returns the appropriate scanner for a detected language.
func ScannerForLang(l model.Language) Scanner {
	switch l {
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

// ToOculusLanguage converts a model.Language to an oculus/lang.Language.
func ToOculusLanguage(l model.Language) lang.Language {
	return lang.Language(l.String())
}

// ToModelLanguage converts an oculus/lang.Language to a model.Language.
func ToModelLanguage(l lang.Language) model.Language {
	switch l {
	case lang.Go:
		return model.LangGo
	case lang.Rust:
		return model.LangRust
	case lang.Python:
		return model.LangPython
	case lang.TypeScript:
		return model.LangTypeScript
	case lang.C:
		return model.LangC
	case lang.Cpp:
		return model.LangCpp
	case lang.Java:
		return model.LangJava
	case lang.JavaScript:
		return model.LangJavaScript
	case lang.Zig:
		return model.LangZig
	case lang.Kotlin:
		return model.LangKotlin
	case lang.Swift:
		return model.LangSwift
	case lang.CSharp:
		return model.LangCSharp
	default:
		return model.LangUnknown
	}
}
