package survey

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/dpopsuev/locus/internal/model"
)

func writePyFile(t *testing.T, dir, name, content string) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPythonScannerBasic(t *testing.T) {
	dir := t.TempDir()

	writePyFile(t, dir, "pyproject.toml", `
[project]
name = "myapp"
dependencies = ["requests>=2.0", "click"]
`)

	writePyFile(t, dir, "myapp/__init__.py", "")
	writePyFile(t, dir, "myapp/core.py", `
import os
import requests

from myapp.utils import helper

class Engine:
    pass

def run():
    pass

async def process():
    pass

def _internal():
    pass
`)
	writePyFile(t, dir, "myapp/utils.py", `
import json

def helper():
    pass

class Config:
    pass
`)

	sc := &PythonScanner{}
	proj, err := sc.Scan(dir)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if proj.Path != "myapp" {
		t.Errorf("project path = %q, want myapp", proj.Path)
	}
	if proj.Language != model.LangPython {
		t.Errorf("language = %v, want Python", proj.Language)
	}

	if len(proj.Namespaces) < 1 {
		t.Fatalf("expected at least 1 namespace, got %d", len(proj.Namespaces))
	}

	nsMap := make(map[string]*model.Namespace)
	for _, ns := range proj.Namespaces {
		nsMap[ns.ImportPath] = ns
	}

	core, ok := nsMap["myapp"]
	if !ok {
		t.Fatalf("missing namespace 'myapp', have: %v", keys(nsMap))
	}

	symMap := make(map[string]*model.Symbol)
	for _, s := range core.Symbols {
		symMap[s.Name] = s
	}

	if _, ok := symMap["Engine"]; !ok {
		t.Error("missing symbol Engine")
	}
	if _, ok := symMap["run"]; !ok {
		t.Error("missing symbol run")
	}
	if _, ok := symMap["process"]; !ok {
		t.Error("missing symbol process")
	}

	if s, ok := symMap["_internal"]; ok && s.Exported {
		t.Error("_internal should not be exported")
	}

	if proj.DependencyGraph == nil {
		t.Fatal("missing dependency graph")
	}
	if len(proj.DependencyGraph.Edges) == 0 {
		t.Error("expected at least one edge")
	}

	var externalEdges []string
	for _, e := range proj.DependencyGraph.Edges {
		if e.External {
			externalEdges = append(externalEdges, e.To)
		}
	}
	sort.Strings(externalEdges)
	if !contains(externalEdges, "requests") {
		t.Errorf("missing external dep 'requests', got %v", externalEdges)
	}
}

func TestPythonScannerDetectsLanguage(t *testing.T) {
	dir := t.TempDir()
	writePyFile(t, dir, "pyproject.toml", `[project]
name = "testapp"
`)
	writePyFile(t, dir, "testapp/__init__.py", "")
	writePyFile(t, dir, "testapp/main.py", "def main(): pass\n")

	lang := DetectLanguage(dir)
	if lang != model.LangPython {
		t.Errorf("DetectLanguage = %v, want Python", lang)
	}

	auto := &AutoScanner{}
	sc := auto.resolve(dir)
	if _, ok := sc.(*PythonScanner); !ok {
		t.Errorf("AutoScanner resolved to %T, want *PythonScanner", sc)
	}
}

func TestPythonScannerSetupPy(t *testing.T) {
	dir := t.TempDir()
	writePyFile(t, dir, "setup.py", `
from setuptools import setup
setup(name='legacy-app', version='1.0')
`)
	writePyFile(t, dir, "src/__init__.py", "")
	writePyFile(t, dir, "src/app.py", "class App: pass\n")

	sc := &PythonScanner{}
	proj, err := sc.Scan(dir)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if proj.Path != "legacy-app" {
		t.Errorf("project path = %q, want legacy-app", proj.Path)
	}
}

func keys(m map[string]*model.Namespace) []string {
	k := make([]string, 0, len(m))
	for key := range m {
		k = append(k, key)
	}
	sort.Strings(k)
	return k
}

func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
