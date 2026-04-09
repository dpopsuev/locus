//go:build e2e

package locus_test

import (
	"context"
	"os/exec"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestSmoke_LocalServer verifies the locally built binary works as a
// long-running MCP server: connect, call a tool, call again (stays alive),
// disconnect.
//
// Run: go test -tags=e2e -run TestSmoke_LocalServer -timeout 60s -v .
func TestSmoke_LocalServer(t *testing.T) {
	binary := "./locus"
	if _, err := exec.LookPath(binary); err != nil {
		t.Skip("locus binary not found (run 'make build')")
	}

	dir := setupGoFixture(t)
	ctx := context.Background()

	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "smoke-test", Version: "1.0"}, nil)
	transport := &sdkmcp.CommandTransport{
		Command: exec.Command(binary, "serve", "--workspace", dir),
	}
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer session.Close()

	info := session.InitializeResult().ServerInfo
	t.Logf("server: %s %s", info.Name, info.Version)

	// First call — architecture review
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

	// Second call — server must stay alive between calls
	t.Run("cycles", func(t *testing.T) {
		text := callToolText(t, session, ctx, "analysis", map[string]any{
			"action": "cycles",
			"path":   dir,
		})
		assertContains(t, text, `"cycles": []`)
	})
}
