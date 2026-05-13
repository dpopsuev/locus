//go:build e2e

package locus_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestTypeScriptMonorepo_ScanSucceedsWithoutRootProject verifies that a monorepo
// layout with no tsconfig at the root (TypeScript code lives in nested package
// directories, similar to Alef) succeeds for scan_local, coupling, and mesh.
// Symbol-level analysis must use the fallback path rather than failing with
// missing-LSP guidance.
func TestTypeScriptMonorepo_ScanSucceedsWithoutRootProject(t *testing.T) {
	requireDocker(t)
	requireImage(t)

	repo := setupMonorepoNoProjectFixture(t)
	ctx := context.Background()
	session := connectContainer(t, ctx, repo)
	defer session.Close()

	scan := callToolOutcome(t, session, ctx, "codograph", map[string]any{
		"action": "scan_local",
		"path":   repo,
		"intent": "full",
	})
	if scan.isError {
		t.Fatalf("scan_local should succeed, got: %s", truncate(scan.text, 400))
	}

	coupling := callToolOutcome(t, session, ctx, "analysis", map[string]any{
		"action": "coupling",
		"path":   repo,
		"top_n":  5,
	})
	if coupling.isError {
		t.Fatalf("coupling should succeed, got: %s", truncate(coupling.text, 400))
	}

	mesh := callToolOutcome(t, session, ctx, "analysis", map[string]any{
		"action": "mesh",
		"path":   repo,
	})
	if mesh.isError {
		t.Fatalf("expected mesh fallback success for monorepo root, got error: %s", truncate(mesh.text, 400))
	}

	var payload struct {
		Nodes map[string]any `json:"nodes"`
		Edges []any          `json:"edges"`
	}
	if err := json.Unmarshal([]byte(mesh.text), &payload); err != nil {
		t.Fatalf("mesh returned non-JSON: %v\n%s", err, truncate(mesh.text, 400))
	}
	if len(payload.Nodes) == 0 || len(payload.Edges) == 0 {
		t.Fatalf("expected non-empty mesh via fallback, got: %s", truncate(mesh.text, 400))
	}
}

func setupMonorepoNoProjectFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	files := map[string]string{
		"package.json": `{
  "name": "ts-monorepo-no-project",
  "private": true,
  "workspaces": ["packages/*"]
}
`,
		"packages/app/package.json": `{
  "name": "@ts-monorepo-no-project/app",
  "version": "1.0.0",
  "private": true
}
`,
		"packages/app/tsconfig.json": `{
  "compilerOptions": {
    "target": "ES2020",
    "module": "commonjs",
    "strict": true,
    "esModuleInterop": true
  },
  "include": ["src/**/*"]
}
`,
		"packages/app/src/index.ts": `import { decorate } from "./util";

export function run(input: string): string {
  return decorate(input);
}
`,
		"packages/app/src/util.ts": `export function decorate(input: string): string {
  return "[" + input + "]";
}
`,
	}
	for rel, content := range files {
		abs := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", abs, err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", abs, err)
		}
	}

	for _, args := range [][]string{
		{"init", "-q"},
		{"add", "-A"},
		{"commit", "-q", "-m", "init"},
	} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	return dir
}
