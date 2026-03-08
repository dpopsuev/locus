package survey_test

import (
	"os/exec"
	"testing"

	"github.com/dpopsuev/locus/internal/survey"
)

func TestLSPScannerWithGopls(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping LSP integration test in short mode")
	}

	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not available on PATH")
	}

	dir := setupModule(t, map[string]string{
		"go.mod":  "module example.com/lsptest\n\ngo 1.21\n",
		"main.go": "package main\n\nfunc Hello() string { return \"hi\" }\n\nfunc main() {}\n",
	})

	sc := &survey.LSPScanner{ServerCmd: "gopls serve"}
	proj, err := sc.Scan(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	if len(proj.Namespaces) == 0 {
		t.Fatal("no namespaces extracted")
	}

	foundHello := false
	for _, ns := range proj.Namespaces {
		for _, s := range ns.Symbols {
			if s.Name == "Hello" {
				foundHello = true
				if !s.Exported {
					t.Error("Hello should be exported")
				}
			}
		}
	}
	if !foundHello {
		t.Error("missing symbol 'Hello'")
	}
}

func TestLSPScannerMissingBinary(t *testing.T) {
	sc := &survey.LSPScanner{ServerCmd: "nonexistent-lsp-server-xyz"}
	_, err := sc.Scan(".")
	if err == nil {
		t.Fatal("expected error for missing binary")
	}
}

func TestLSPScannerEmptyCmd(t *testing.T) {
	sc := &survey.LSPScanner{ServerCmd: ""}
	_, err := sc.Scan(".")
	if err == nil {
		t.Fatal("expected error for empty ServerCmd")
	}
}
