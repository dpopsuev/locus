package survey_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dpopsuev/locus/internal/model"
	"github.com/dpopsuev/locus/internal/survey"
)

func TestDetectLanguageGo(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n"), 0o644)

	lang := survey.DetectLanguage(dir)
	if lang != model.LangGo {
		t.Errorf("language = %v, want LangGo", lang)
	}
}

func TestDetectLanguageRust(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte("[package]\n"), 0o644)

	lang := survey.DetectLanguage(dir)
	if lang != model.LangRust {
		t.Errorf("language = %v, want LangRust", lang)
	}
}

func TestDetectLanguagePython(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[tool]\n"), 0o644)

	lang := survey.DetectLanguage(dir)
	if lang != model.LangPython {
		t.Errorf("language = %v, want LangPython", lang)
	}
}

func TestDetectLanguageTypeScript(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "tsconfig.json"), []byte("{}\n"), 0o644)

	lang := survey.DetectLanguage(dir)
	if lang != model.LangTypeScript {
		t.Errorf("language = %v, want LangTypeScript", lang)
	}
}

func TestDetectLanguageUnknown(t *testing.T) {
	dir := t.TempDir()

	lang := survey.DetectLanguage(dir)
	if lang != model.LangUnknown {
		t.Errorf("language = %v, want LangUnknown", lang)
	}
}

func TestAutoScannerOverrideGo(t *testing.T) {
	dir := setupModule(t, map[string]string{
		"go.mod":  "module example.com/auto\n\ngo 1.21\n",
		"main.go": "package main\n\nfunc main() {}\n",
	})

	sc := &survey.AutoScanner{Override: "go"}
	proj, err := sc.Scan(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if proj.Path != "example.com/auto" {
		t.Errorf("path = %q", proj.Path)
	}
}

func TestAutoScannerAutoDetectsGo(t *testing.T) {
	dir := setupModule(t, map[string]string{
		"go.mod":  "module example.com/detect\n\ngo 1.21\n",
		"main.go": "package main\n\nfunc main() {}\n",
	})

	sc := &survey.AutoScanner{}
	proj, err := sc.Scan(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if proj.Language != model.LangGo {
		t.Errorf("language = %v, want LangGo", proj.Language)
	}
}

func TestDefaultLSPServer(t *testing.T) {
	tests := []struct {
		lang model.Language
		want string
	}{
		{model.LangGo, "gopls serve"},
		{model.LangRust, "rust-analyzer"},
		{model.LangPython, "pyright-langserver --stdio"},
		{model.LangTypeScript, "typescript-language-server --stdio"},
		{model.LangUnknown, ""},
	}

	for _, tt := range tests {
		got := survey.DefaultLSPServer(tt.lang)
		if got != tt.want {
			t.Errorf("DefaultLSPServer(%v) = %q, want %q", tt.lang, got, tt.want)
		}
	}
}
