//go:build e2e

package locus_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

const containerImage = "locus"

// TestContainer_E2E validates the full MCP flow through a Docker container:
// build image -> mount test repo read-only -> MCP initialize + tool calls -> assert.
//
// Run: make test-container
func TestContainer_E2E(t *testing.T) {
	requireDocker(t)
	requireImage(t)

	dir := setupGoFixture(t)
	ctx := context.Background()

	// Connect to Locus in a container via the MCP SDK's CommandTransport.
	client := sdkmcp.NewClient(&sdkmcp.Implementation{
		Name:    "e2e-test",
		Version: "1.0",
	}, nil)

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
	defer session.Close()

	t.Logf("connected, server: %+v", session.InitializeResult().ServerInfo)

	// List tools
	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	t.Logf("tools: %d", len(tools.Tools))
	if len(tools.Tools) == 0 {
		t.Fatal("expected at least 1 tool")
	}

	// Call analysis — architecture review of the mounted fixture
	result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name: "analysis",
		Arguments: map[string]any{
			"action": "preset",
			"preset": "architecture_review",
			"path":   dir,
		},
	})
	if err != nil {
		t.Fatalf("call analysis: %v", err)
	}
	if len(result.Content) == 0 {
		t.Fatal("analysis returned empty content")
	}

	var text string
	switch c := result.Content[0].(type) {
	case *sdkmcp.TextContent:
		text = c.Text
	default:
		t.Fatalf("unexpected content type: %T", result.Content[0])
	}
	t.Logf("analysis (%d chars): %s", len(text), truncate(text, 200))

	if !strings.Contains(text, "component") && !strings.Contains(text, "edge") {
		t.Error("analysis result doesn't mention components or edges")
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
		"go.mod": "module example.com/e2etest\ngo 1.21\n",
		"main.go": "package main\n\nimport \"example.com/e2etest/internal/config\"\n\nfunc main() {\n\tcfg := config.Load(\"app.yaml\")\n\t_ = cfg\n}\n",
		"internal/config/config.go": "package config\n\ntype Config struct{ Name string }\n\nfunc Load(path string) *Config { return &Config{Name: path} }\n",
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
