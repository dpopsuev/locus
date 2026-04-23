//go:build e2e

package locus_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestSoak_MemorySLA runs parallel probes across multiple repos and monitors
// container RSS. Fails if memory exceeds the SLA budget (4GB).
//
// This reproduces the OOM crash from OCL-BUG-10: parallel multi-repo probes
// spawning unbounded gopls instances that exhaust host memory.
//
// Run: go test -tags=e2e -run TestSoak_MemorySLA -timeout 300s -v .
func TestSoak_MemorySLA(t *testing.T) {
	requireDocker(t)
	requireImage(t)

	const (
		memoryBudgetMB = 4096
		pollInterval   = 2 * time.Second
		soakDuration   = 60 * time.Second
	)

	repos := discoverRepos(t)
	if len(repos) < 3 {
		t.Skipf("need at least 3 repos for soak test, found %d", len(repos))
	}
	if len(repos) > 6 {
		repos = repos[:6]
	}
	t.Logf("soak repos (%d): %v", len(repos), repos)

	// Start container with memory visibility
	containerName := "locus-soak-test"
	cleanup := startSoakContainer(t, containerName, repos)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), soakDuration+30*time.Second)
	defer cancel()

	// Connect MCP client
	client := sdkmcp.NewClient(
		&sdkmcp.Implementation{Name: "soak-test", Version: "1.0"},
		nil,
	)
	transport := &sdkmcp.CommandTransport{
		Command: exec.Command("docker", "run", "--rm", "-i",
			"-v", "/home/dpopsuev:/home/dpopsuev:ro,z",
			containerImage,
			"serve", "--workspace", "/"),
	}
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer session.Close()

	// Memory monitor goroutine
	var peakRSS int64
	var memViolation string
	monitorDone := make(chan struct{})
	go func() {
		defer close(monitorDone)
		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				rss := getContainerRSS(t, containerName)
				if rss > peakRSS {
					peakRSS = rss
				}
				rssMB := rss / (1024 * 1024)
				t.Logf("RSS: %dMB (peak: %dMB, budget: %dMB)", rssMB, peakRSS/(1024*1024), memoryBudgetMB)
				if rssMB > memoryBudgetMB && memViolation == "" {
					memViolation = fmt.Sprintf("RSS %dMB exceeded budget %dMB", rssMB, memoryBudgetMB)
				}
			}
		}
	}()

	// Parallel scans + analysis across all repos
	var wg sync.WaitGroup
	for _, repo := range repos {
		wg.Add(1)
		go func(r string) {
			defer wg.Done()

			// Scan
			t.Logf("  scanning %s...", r)
			_, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
				Name:      "codograph",
				Arguments: map[string]any{"action": "scan_local", "path": r, "intent": "health"},
			})
			if err != nil {
				t.Logf("  scan %s: %v", r, err)
				return
			}

			// Coupling
			t.Logf("  coupling %s...", r)
			_, err = session.CallTool(ctx, &sdkmcp.CallToolParams{
				Name: "analysis",
				Arguments: map[string]any{
					"action":    "coupling",
					"path":      r,
					"view":      "hot_spots",
					"top_n":     5,
					"churn_days": 30,
				},
			})
			if err != nil {
				t.Logf("  coupling %s: %v", r, err)
			}

			// Cycles
			t.Logf("  cycles %s...", r)
			_, _ = session.CallTool(ctx, &sdkmcp.CallToolParams{
				Name:      "analysis",
				Arguments: map[string]any{"action": "cycles", "path": r},
			})
		}(repo)
	}

	wg.Wait()
	cancel()
	<-monitorDone

	t.Logf("peak RSS: %dMB", peakRSS/(1024*1024))
	if memViolation != "" {
		t.Fatalf("MEMORY SLA VIOLATED: %s", memViolation)
	}
}

func discoverRepos(t *testing.T) []string {
	t.Helper()
	workspace := "/home/dpopsuev/Workspace"
	entries, err := os.ReadDir(workspace)
	if err != nil {
		t.Skipf("cannot read %s: %v", workspace, err)
	}

	var repos []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := workspace + "/" + e.Name()
		if _, err := os.Stat(dir + "/go.mod"); err == nil {
			repos = append(repos, dir)
		} else if _, err := os.Stat(dir + "/Cargo.toml"); err == nil {
			repos = append(repos, dir)
		} else if _, err := os.Stat(dir + "/package.json"); err == nil {
			repos = append(repos, dir)
		}
	}
	return repos
}

func startSoakContainer(t *testing.T, name string, repos []string) func() {
	t.Helper()
	_ = exec.Command("docker", "stop", name).Run()
	_ = exec.Command("docker", "rm", name).Run()

	args := []string{"run", "-d", "--name", name,
		"-p", "18081:8081",
		"-v", "/home/dpopsuev:/home/dpopsuev:ro,z",
		containerImage,
		"serve", "--transport", "http", "--addr", ":8081",
	}
	out, err := exec.Command("docker", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("start soak container: %v\n%s", err, out)
	}
	time.Sleep(2 * time.Second)

	return func() {
		_ = exec.Command("docker", "stop", name).Run()
		_ = exec.Command("docker", "rm", name).Run()
	}
}

func getContainerRSS(t *testing.T, name string) int64 {
	t.Helper()
	// Read cgroup memory.current for total container memory
	out, err := exec.Command("docker", "exec", name, "cat", "/sys/fs/cgroup/memory.current").CombinedOutput()
	if err != nil {
		// Fallback to /proc/1/status
		out, err = exec.Command("docker", "exec", name, "sh", "-c",
			"awk '/VmRSS/{print $2}' /proc/1/status").CombinedOutput()
		if err != nil {
			return 0
		}
		kb, _ := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
		return kb * 1024
	}
	bytes, _ := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	return bytes
}
