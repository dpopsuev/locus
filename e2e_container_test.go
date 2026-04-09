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

const containerImage = "locus"

// TestContainer_E2E validates the full MCP flow through a Docker container
// against a fixture with known architecture:
//
//	3 components: (root), internal/config, internal/handler
//	2 edges: . → internal/config, internal/handler → internal/config
//	0 cycles
//
// Run: make test-container
func TestContainer_E2E(t *testing.T) {
	requireDocker(t)
	requireImage(t)

	dir := setupGoFixture(t)
	ctx := context.Background()
	session := connectContainer(t, ctx, dir)
	defer session.Close()

	t.Run("tools", func(t *testing.T) {
		tools, err := session.ListTools(ctx, nil)
		if err != nil {
			t.Fatalf("list tools: %v", err)
		}
		if len(tools.Tools) < 5 {
			t.Errorf("expected >= 5 tools, got %d", len(tools.Tools))
		}
	})

	t.Run("architecture_review", func(t *testing.T) {
		text := callToolText(t, session, ctx, "analysis", map[string]any{
			"action": "preset",
			"preset": "architecture_review",
			"path":   dir,
		})

		assertContains(t, text, "3 components")
		assertContains(t, text, "2 edges")
		assertContains(t, text, "0 cycles")
	})

	t.Run("cycles", func(t *testing.T) {
		text := callToolText(t, session, ctx, "analysis", map[string]any{
			"action": "cycles",
			"path":   dir,
		})

		var result struct {
			Cycles      []any          `json:"cycles"`
			ImportDepth map[string]int `json:"import_depth"`
		}
		if err := json.Unmarshal([]byte(text), &result); err != nil {
			t.Fatalf("parse cycles: %v", err)
		}
		if len(result.Cycles) != 0 {
			t.Errorf("expected 0 cycles, got %d", len(result.Cycles))
		}
		// internal/config has import depth 1 (imported by root and handler)
		if d, ok := result.ImportDepth["internal/config"]; !ok || d != 1 {
			t.Errorf("expected internal/config depth=1, got %d", d)
		}
	})

	t.Run("deps", func(t *testing.T) {
		text := callToolText(t, session, ctx, "analysis", map[string]any{
			"action":    "deps",
			"path":      dir,
			"component": "internal/config",
		})

		var result struct {
			Component string `json:"component"`
			FanIn     []struct {
				From string `json:"from"`
				To   string `json:"to"`
			} `json:"fan_in"`
		}
		if err := json.Unmarshal([]byte(text), &result); err != nil {
			t.Fatalf("parse deps: %v", err)
		}
		if result.Component != "internal/config" {
			t.Errorf("expected component internal/config, got %s", result.Component)
		}
		if len(result.FanIn) != 2 {
			t.Errorf("expected 2 fan_in edges, got %d", len(result.FanIn))
		}
		// Exact callers: root (.) and internal/handler
		callers := make(map[string]bool)
		for _, e := range result.FanIn {
			callers[e.From] = true
		}
		if !callers["."] {
			t.Error("expected . in fan_in")
		}
		if !callers["internal/handler"] {
			t.Error("expected internal/handler in fan_in")
		}
	})

	t.Run("diagram", func(t *testing.T) {
		text := callToolText(t, session, ctx, "render_diagram", map[string]any{
			"type": "dependency",
			"path": dir,
		})

		// Mermaid output must contain exact node and edge declarations
		assertContains(t, text, "graph TD")
		assertContains(t, text, `internal_config["internal/config"]`)
		assertContains(t, text, `internal_handler["internal/handler"]`)
		assertContains(t, text, "internal_handler -->")
	})
}

// --- helpers ---

func connectContainer(t *testing.T, ctx context.Context, dir string) *sdkmcp.ClientSession {
	t.Helper()
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "e2e-test", Version: "1.0"}, nil)
	transport := &sdkmcp.CommandTransport{
		Command: exec.Command("docker", "run", "--rm", "-i",
			"-v", dir+":"+dir+":ro,z",
			"-w", dir,
			containerImage,
			"serve", "--workspace", dir,
		),
	}
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Logf("connected to %s", session.InitializeResult().ServerInfo.Name)
	return session
}

func callToolText(t *testing.T, s *sdkmcp.ClientSession, ctx context.Context, tool string, args map[string]any) string {
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
	return tc.Text
}

func assertContains(t *testing.T, text, substr string) {
	t.Helper()
	if !strings.Contains(text, substr) {
		t.Errorf("expected %q in output, got:\n%s", substr, truncate(text, 300))
	}
}

func requireDocker(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not found")
	}
}

func requireImage(t *testing.T) {
	t.Helper()
	if out, err := exec.Command("docker", "image", "inspect", containerImage).CombinedOutput(); err != nil {
		t.Skipf("image %s not found (run 'make docker'): %s", containerImage, string(out))
	}
}

func setupGoFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	os.Chmod(dir, 0o755)
	files := map[string]string{
		"go.mod":                      "module example.com/e2etest\ngo 1.21\n",
		"main.go":                     "package main\n\nimport \"example.com/e2etest/internal/config\"\n\nfunc main() {\n\tcfg := config.Load(\"app.yaml\")\n\t_ = cfg\n}\n",
		"internal/config/config.go":   "package config\n\ntype Config struct{ Name string }\n\nfunc Load(path string) *Config { return &Config{Name: path} }\n",
		"internal/handler/handler.go": "package handler\n\nimport \"example.com/e2etest/internal/config\"\n\ntype Handler struct{ cfg *config.Config }\n\nfunc New(cfg *config.Config) *Handler { return &Handler{cfg: cfg} }\n",
	}
	for rel, content := range files {
		abs := filepath.Join(dir, rel)
		os.MkdirAll(filepath.Dir(abs), 0o755)
		os.WriteFile(abs, []byte(content), 0o644)
	}
	return dir
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
