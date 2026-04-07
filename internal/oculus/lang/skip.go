package lang

import "strings"

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
	".locus":       true,
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
