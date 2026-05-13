//go:build e2e

package locus_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestBug56_TypeScriptMissingLanguageServer verifies the LCS-BUG-56 fix.
//
// Expected behavior after the fix:
//   - component-level analysis succeeds (scan_local + coupling)
//   - symbol-level analysis also succeeds via non-LSP fallback when
//     typescript-language-server is not on PATH.
func TestBug56_TypeScriptMissingLanguageServer(t *testing.T) {
	binary := "./locus"
	if _, err := exec.LookPath(binary); err != nil {
		t.Skip("locus binary not found (run 'make build')")
	}

	repo := setupBug56TypeScriptFixture(t)
	emptyPath := t.TempDir() // intentionally no typescript-language-server

	ctx := context.Background()
	client := sdkmcp.NewClient(
		&sdkmcp.Implementation{Name: "bug56-repro", Version: "1.0"},
		nil,
	)
	cmd := exec.Command(binary, "serve", "--workspace", repo)
	cmd.Env = withPATH(os.Environ(), emptyPath)
	transport := &sdkmcp.CommandTransport{Command: cmd}

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer session.Close()

	scan := callToolOutcome(t, session, ctx, "codograph", map[string]any{
		"action": "scan_local",
		"path":   repo,
		"intent": "health",
	})
	if scan.isError {
		t.Fatalf("scan_local should succeed without TS LSP, got error: %s", truncate(scan.text, 400))
	}

	coupling := callToolOutcome(t, session, ctx, "analysis", map[string]any{
		"action": "coupling",
		"path":   repo,
		"top_n":  5,
	})
	if coupling.isError {
		t.Fatalf("coupling should succeed without TS LSP, got error: %s", truncate(coupling.text, 400))
	}

	mesh := callToolOutcome(t, session, ctx, "analysis", map[string]any{
		"action": "mesh",
		"path":   repo,
	})
	if mesh.isError {
		t.Fatalf("expected mesh fallback success without typescript-language-server, got error: %s", truncate(mesh.text, 400))
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
	if strings.Contains(mesh.text, "typescript-language-server --stdio") {
		t.Fatalf("unexpected missing-LSP guidance after fallback fix: %s", truncate(mesh.text, 400))
	}
}

type toolOutcome struct {
	text    string
	isError bool
}

func callToolOutcome(t *testing.T, s *sdkmcp.ClientSession, ctx context.Context, tool string, args map[string]any) toolOutcome {
	t.Helper()
	result, err := s.CallTool(ctx, &sdkmcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		t.Fatalf("call %s: %v", tool, err)
	}
	if len(result.Content) == 0 {
		t.Fatalf("%s returned empty content", tool)
	}
	tc, ok := result.Content[0].(*sdkmcp.TextContent)
	if !ok {
		t.Fatalf("%s returned non-text content: %T", tool, result.Content[0])
	}
	return toolOutcome{text: tc.Text, isError: result.IsError}
}

func setupBug56TypeScriptFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	files := map[string]string{
		"package.json": `{
  "name": "bug56-repro",
  "version": "1.0.0",
  "private": true
}
`,
		"tsconfig.json": `{
  "compilerOptions": {
    "target": "ES2020",
    "module": "commonjs",
    "strict": true,
    "esModuleInterop": true,
    "skipLibCheck": true
  },
  "include": ["src/**/*"]
}
`,
		"src/index.ts": `import { decorate } from "./util";

export function run(input: string): string {
  return decorate(input);
}
`,
		"src/util.ts": `export function decorate(input: string): string {
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

	// Keep fixture behavior stable across scan heuristics.
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

func withPATH(env []string, path string) []string {
	out := make([]string, 0, len(env)+1)
	for _, kv := range env {
		if strings.HasPrefix(kv, "PATH=") {
			continue
		}
		out = append(out, kv)
	}
	out = append(out, "PATH="+path)
	return out
}
