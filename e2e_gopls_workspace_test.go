//go:build e2e

package locus_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestServe_GoplsWorkspaceSwitch verifies that gopls correctly switches
// between Go workspaces: call graph requests must succeed after scanning
// a different Go repository in the same server session.
//
// Run:
//
//	go test -tags=e2e -run TestServe_GoplsWorkspaceSwitch -timeout 240s -v .
func TestServe_GoplsWorkspaceSwitch(t *testing.T) {
	requireDocker(t)
	requireImage(t)

	repoA := setupGoRepoFixture(t, "example.com/bug65a")
	repoB := setupGoRepoFixture(t, "example.com/bug65b")
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	session := connectContainerForRepos(t, ctx, repoA, repoB)
	defer session.Close()

	scanRepo(t, session, ctx, repoA)
	if err := waitForSymbolGraph(ctx, session, repoA); err != nil {
		t.Fatalf("call graph baseline unavailable on repo A: %v", err)
	}

	scanRepo(t, session, ctx, repoB)

	// Reproduction point: on bugged builds this fails because gopls is
	// still bound to repo A and a server for repo B is not started/reused.
	if err := waitForSymbolGraph(ctx, session, repoB); err != nil {
		t.Fatalf("call graph unavailable after workspace switch: %v", err)
	}
}

func setupGoRepoFixture(t *testing.T, module string) string {
	t.Helper()

	dir := t.TempDir()
	files := map[string]string{
		"go.mod": fmt.Sprintf("module %s\ngo 1.21\n", module),
		"main.go": `package main

import "fmt"

func main() {
	fmt.Println(run("input"))
}

func run(v string) string {
	return decorate(v)
}

func decorate(v string) string {
	return "[" + v + "]"
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

	// gopls workspace initialization is more reliable inside git repos.
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

func connectContainerForRepos(t *testing.T, ctx context.Context, repoA, repoB string) *sdkmcp.ClientSession {
	t.Helper()
	client := sdkmcp.NewClient(
		&sdkmcp.Implementation{Name: "bug65-workspace-switch", Version: "1.0"},
		nil,
	)
	transport := &sdkmcp.CommandTransport{
		Command: exec.Command(
			"docker", "run", "--rm", "-i",
			"-v", repoA+":"+repoA+":z",
			"-v", repoB+":"+repoB+":z",
			"-w", repoA,
			containerImage,
			"serve", "--workspace", repoA,
		),
	}
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		t.Fatalf("connect container: %v", err)
	}
	return session
}

func scanRepo(t *testing.T, session *sdkmcp.ClientSession, ctx context.Context, path string) {
	t.Helper()
	callToolTextStrict(t, session, ctx, "codograph", map[string]any{
		"action": "scan_local",
		"path":   path,
		"intent": "health",
	})
}

func waitForSymbolGraph(ctx context.Context, session *sdkmcp.ClientSession, path string) error {
	deadline := time.Now().Add(20 * time.Second)
	last := "no response"
	for time.Now().Before(deadline) {
		result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
			Name: "analysis",
			Arguments: map[string]any{
				"action": "symbol_graph",
				"path":   path,
			},
		})
		if err != nil {
			last = fmt.Sprintf("transport error: %v", err)
			time.Sleep(500 * time.Millisecond)
			continue
		}
		if len(result.Content) == 0 {
			last = "empty tool content"
			time.Sleep(500 * time.Millisecond)
			continue
		}
		tc, ok := result.Content[0].(*sdkmcp.TextContent)
		if !ok {
			last = fmt.Sprintf("non-text content: %T", result.Content[0])
			time.Sleep(500 * time.Millisecond)
			continue
		}
		if result.IsError {
			last = tc.Text
			time.Sleep(500 * time.Millisecond)
			continue
		}

		var payload struct {
			Nodes []any `json:"nodes"`
			Edges []any `json:"edges"`
		}
		if err := json.Unmarshal([]byte(tc.Text), &payload); err != nil {
			last = fmt.Sprintf("non-JSON payload: %v (%s)", err, truncate(tc.Text, 200))
			time.Sleep(500 * time.Millisecond)
			continue
		}
		if len(payload.Nodes) > 0 && len(payload.Edges) > 0 {
			return nil
		}
		last = tc.Text
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("%s", truncate(last, 400))
}

func callToolTextStrict(t *testing.T, s *sdkmcp.ClientSession, ctx context.Context, tool string, args map[string]any) string {
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
	if result.IsError {
		t.Fatalf("%s returned tool error: %s", tool, truncate(tc.Text, 400))
	}
	return tc.Text
}
