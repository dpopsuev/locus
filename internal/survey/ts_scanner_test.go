package survey_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dpopsuev/locus/internal/model"
	"github.com/dpopsuev/locus/internal/survey"
)

func setupTSProject(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestTSScanExtractsNamespacesAndSymbols(t *testing.T) {
	dir := setupTSProject(t, map[string]string{
		"package.json": `{"name": "my-app", "dependencies": {"three": "^0.172.0"}}`,
		"src/main.ts": `import { Scene } from 'three'
import { helper } from './utils'

export function init() {}
export class App {}
`,
		"src/utils.ts": `export const VERSION = "1.0"
export interface Config {
  name: string
}

export type ID = string
`,
	})

	sc := &survey.TypeScriptScanner{}
	proj, err := sc.Scan(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	if proj.Path != "my-app" {
		t.Errorf("path = %q, want my-app", proj.Path)
	}
	if proj.Language != model.LangTypeScript {
		t.Errorf("language = %v, want TypeScript", proj.Language)
	}

	nsMap := make(map[string]*model.Namespace)
	for _, ns := range proj.Namespaces {
		nsMap[ns.Name] = ns
	}

	src, ok := nsMap["src"]
	if !ok {
		t.Fatal("missing namespace src")
	}

	symMap := make(map[string]*model.Symbol)
	for _, s := range src.Symbols {
		symMap[s.Name] = s
	}

	if s, ok := symMap["init"]; !ok {
		t.Error("missing export function init")
	} else if s.Kind != model.SymbolFunction {
		t.Errorf("init.kind = %v, want function", s.Kind)
	}

	if s, ok := symMap["App"]; !ok {
		t.Error("missing export class App")
	} else if s.Kind != model.SymbolClass {
		t.Errorf("App.kind = %v, want class", s.Kind)
	}

	if s, ok := symMap["VERSION"]; !ok {
		t.Error("missing export const VERSION")
	} else if s.Kind != model.SymbolVariable {
		t.Errorf("VERSION.kind = %v, want variable", s.Kind)
	}

	if s, ok := symMap["Config"]; !ok {
		t.Error("missing export interface Config")
	} else if s.Kind != model.SymbolInterface {
		t.Errorf("Config.kind = %v, want interface", s.Kind)
	}

	if s, ok := symMap["ID"]; !ok {
		t.Error("missing export type ID")
	} else if s.Kind != model.SymbolTypeParameter {
		t.Errorf("ID.kind = %v, want type-parameter", s.Kind)
	}
}

func TestTSScanBuildsImportGraph(t *testing.T) {
	dir := setupTSProject(t, map[string]string{
		"package.json": `{"name": "graph-test", "dependencies": {"three": "1.0", "@msgpack/msgpack": "3.0"}}`,
		"src/main.ts": `import { Scene } from 'three'
import { helper } from './utils'
import { pack } from '@msgpack/msgpack'

export function run() {}
`,
		"src/utils.ts": `export function helper() { return 42 }
`,
		"src/modes/gameplay.ts": `import { run } from '../main'

export function startGame() {}
`,
	})

	sc := &survey.TypeScriptScanner{}
	proj, err := sc.Scan(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	if proj.DependencyGraph == nil {
		t.Fatal("dependency graph is nil")
	}

	srcEdges := proj.DependencyGraph.EdgesFrom("src")
	foundThree := false
	foundMsgpack := false
	for _, e := range srcEdges {
		if e.To == "three" && e.External {
			foundThree = true
		}
		if e.To == "@msgpack/msgpack" && e.External {
			foundMsgpack = true
		}
	}
	if !foundThree {
		t.Error("missing external edge src -> three")
	}
	if !foundMsgpack {
		t.Error("missing external edge src -> @msgpack/msgpack")
	}

	modesEdges := proj.DependencyGraph.EdgesFrom("src/modes")
	foundSrc := false
	for _, e := range modesEdges {
		if e.To == "src" && !e.External {
			foundSrc = true
		}
	}
	if !foundSrc {
		t.Error("missing internal edge src/modes -> src")
	}
}

func TestTSScanSkipsImportType(t *testing.T) {
	dir := setupTSProject(t, map[string]string{
		"package.json": `{"name": "type-import-test"}`,
		"src/core/types.ts": `import type { GlobeRenderer } from '../globe'

export interface ModeContext {
  globe: GlobeRenderer
}
`,
		"src/globe/index.ts": `import type { Vec3 } from '../types'

export class GlobeRenderer {}
`,
		"src/types.ts": `export type Vec3 = [number, number, number]
`,
	})

	sc := &survey.TypeScriptScanner{}
	proj, err := sc.Scan(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	// core/types.ts uses `import type` from globe — this should NOT create
	// a dependency edge because type-only imports are erased at compile time.
	coreEdges := proj.DependencyGraph.EdgesFrom("src/core")
	for _, e := range coreEdges {
		if e.To == "src/globe" {
			t.Errorf("import type should not create dependency edge: src/core -> src/globe")
		}
	}

	// globe/index.ts uses `import type` from types — also should not create edge.
	globeEdges := proj.DependencyGraph.EdgesFrom("src/globe")
	for _, e := range globeEdges {
		if e.To == "src" || e.To == "(root)" {
			t.Errorf("import type should not create dependency edge: src/globe -> %s", e.To)
		}
	}
}

func TestTSScanSkipsNodeModules(t *testing.T) {
	dir := setupTSProject(t, map[string]string{
		"package.json":              `{"name": "skip-test"}`,
		"src/main.ts":               `export function main() {}`,
		"node_modules/foo/index.js": `export function foo() {}`,
		"dist/bundle.js":            `export function bundled() {}`,
	})

	sc := &survey.TypeScriptScanner{}
	proj, err := sc.Scan(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	if len(proj.Namespaces) != 1 {
		t.Errorf("namespaces = %d, want 1 (only src)", len(proj.Namespaces))
	}
}
