package lang

import (
	"os"
	"path/filepath"
)

// LanguageMarker maps a project manifest file to its language.
type LanguageMarker struct {
	File string
	Lang Language
}

// LanguageMarkers is the canonical list of file→language mappings,
// ordered by specificity (most specific first, ambiguous markers last).
var LanguageMarkers = []LanguageMarker{
	{"go.mod", Go},
	{"Cargo.toml", Rust},
	{"CMakeLists.txt", Cpp},
	{"pyproject.toml", Python},
	{"setup.py", Python},
	{"tsconfig.json", TypeScript},
	{"package.json", TypeScript},
	{"Makefile", C},
}

// RootProjectMarkers is the subset of LanguageMarkers used for
// discovering sub-projects at the root of a polyglot repo.
// TypeScript is excluded here because it's discovered via directory walk.
var RootProjectMarkers = []LanguageMarker{
	{"go.mod", Go},
	{"Cargo.toml", Rust},
	{"pyproject.toml", Python},
	{"setup.py", Python},
}

// DetectLanguage inspects marker files in root to determine the project language.
func DetectLanguage(root string) Language {
	for _, m := range LanguageMarkers {
		if _, err := os.Stat(filepath.Join(root, m.File)); err == nil {
			return m.Lang
		}
	}
	return Unknown
}
