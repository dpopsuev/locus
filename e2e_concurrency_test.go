//go:build e2e

package locus_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestConcurrentContainers_Light reproduces LCS-BUG-50 with a small fixture:
// N containers start concurrently, connect, and call codograph status.
//
// Run: go test -tags=e2e -run TestConcurrentContainers_Light -timeout 120s -v .
func TestConcurrentContainers_Light(t *testing.T) {
	requireDocker(t)
	requireImage(t)

	dir := setupGoFixture(t)
	runConcurrentContainers(t, dir, 5, func(t *testing.T, session *sdkmcp.ClientSession, ctx context.Context, _ string) {
		callToolText(t, session, ctx, "codograph", map[string]any{"action": "status"})
	})
}

// TestConcurrentContainers_HeavyScan reproduces LCS-BUG-50 under realistic
// load: N containers each do a full health scan + coupling hot_spots on a
// real repo (Oculus ~27K LOC). This matches the actual usage pattern that
// triggers the hang — long-running scans saturating CPU while new containers
// try to establish their stdio pipe.
//
// Run: go test -tags=e2e -run TestConcurrentContainers_HeavyScan -timeout 300s -v .
func TestConcurrentContainers_HeavyScan(t *testing.T) {
	requireDocker(t)
	requireImage(t)

	// Use Oculus as the large fixture — ~27K LOC, 22 packages
	repoPath := os.Getenv("HEAVY_FIXTURE")
	if repoPath == "" {
		home, _ := os.UserHomeDir()
		candidates := []string{
			filepath.Join(home, "Workspace", "oculus"),
			filepath.Join(home, "Workspace", "locus"),
		}
		for _, c := range candidates {
			if _, err := os.Stat(filepath.Join(c, "go.mod")); err == nil {
				repoPath = c
				break
			}
		}
	}
	if repoPath == "" {
		t.Skip("no large repo found (set HEAVY_FIXTURE or have ~/Workspace/oculus)")
	}
	t.Logf("fixture: %s", repoPath)

	runConcurrentContainers(t, repoPath, 4, func(t *testing.T, session *sdkmcp.ClientSession, ctx context.Context, dir string) {
		// 1. Full health scan — triggers tree-sitter, git analysis, nesting depth
		t.Log("  scan_local (health)...")
		callToolText(t, session, ctx, "codograph", map[string]any{
			"action": "scan_local",
			"path":   dir,
			"intent": "health",
		})

		// 2. Coupling hot spots — reads from cache, computes coupling
		t.Log("  coupling hot_spots...")
		callToolText(t, session, ctx, "analysis", map[string]any{
			"action": "coupling",
			"path":   dir,
			"view":   "hot_spots",
			"top_n":  10,
		})

		// 3. Cycles — graph analysis
		t.Log("  cycles...")
		callToolText(t, session, ctx, "analysis", map[string]any{
			"action": "cycles",
			"path":   dir,
		})
	})
}

// runConcurrentContainers starts N containers in parallel, connects each,
// runs workFn, and reports failures. Timeout per container: 90s.
func runConcurrentContainers(
	t *testing.T,
	dir string,
	n int,
	workFn func(t *testing.T, session *sdkmcp.ClientSession, ctx context.Context, dir string),
) {
	t.Helper()

	const perContainerTimeout = 90 * time.Second

	type result struct {
		index   int
		elapsed time.Duration
		err     error
	}

	results := make(chan result, n)
	var wg sync.WaitGroup

	for i := range n {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			start := time.Now()
			ctx, cancel := context.WithTimeout(context.Background(), perContainerTimeout)
			defer cancel()

			client := sdkmcp.NewClient(
				&sdkmcp.Implementation{Name: fmt.Sprintf("concurrency-test-%d", idx), Version: "1.0"},
				nil,
			)
			transport := &sdkmcp.CommandTransport{
				Command: exec.Command("docker", "run", "--rm", "-i",
					"-v", dir+":"+dir+":ro,z",
					containerImage,
					"serve", "--workspace", dir,
				),
			}

			session, err := client.Connect(ctx, transport, nil)
			if err != nil {
				results <- result{index: idx, elapsed: time.Since(start), err: fmt.Errorf("connect: %w", err)}
				return
			}
			defer session.Close()

			workFn(t, session, ctx, dir)
			results <- result{index: idx, elapsed: time.Since(start)}
		}(i)
	}

	wg.Wait()
	close(results)

	var failed int
	for r := range results {
		if r.err != nil {
			t.Errorf("container %d: FAILED after %v: %v", r.index, r.elapsed, r.err)
			failed++
		} else {
			t.Logf("container %d: OK in %v", r.index, r.elapsed)
		}
	}

	if failed > 0 {
		t.Fatalf("%d/%d containers failed (LCS-BUG-50)", failed, n)
	}
}
