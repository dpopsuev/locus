//go:build e2e

package e2e_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	testImage     = "locus-e2e-test"
	testContainer = "locus-e2e-test"
	testAddr      = "http://localhost:18081"
	testPort      = "18081:8081"
)

// --- container lifecycle ---

func buildImage(t *testing.T) {
	t.Helper()
	root := repoRoot(t)
	start := time.Now()
	run(t, "podman", "build", "-t", testImage, "-f", filepath.Join(root, "Dockerfile"), root)
	t.Logf("[YELLOW] image built in %s", time.Since(start).Round(time.Millisecond))
}

func startContainer(t *testing.T, mounts ...string) {
	t.Helper()
	args := []string{"run", "-d", "--name", testContainer, "-p", testPort}
	for _, m := range mounts {
		args = append(args, "-v", m)
	}
	args = append(args, testImage)
	start := time.Now()
	run(t, "podman", args...)
	t.Logf("[YELLOW] container started in %s", time.Since(start).Round(time.Millisecond))
}

func stopContainer(t *testing.T) {
	t.Helper()
	exec.Command("podman", "rm", "-f", testContainer).Run()
}

func repoRoot(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("go", "env", "GOMOD").CombinedOutput()
	if err != nil {
		t.Fatalf("go env GOMOD: %v", err)
	}
	mod := strings.TrimSpace(string(out))
	if mod == "" {
		t.Fatal("not inside a Go module")
	}
	return filepath.Dir(mod)
}

func run(t *testing.T, name string, args ...string) {
	t.Helper()
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("[ORANGE] %s %v: %v\n%s", name, args, err, out)
	}
}

// --- MCP helpers ---

func waitHealthy(t *testing.T, timeout time.Duration) {
	t.Helper()
	start := time.Now()
	deadline := time.Now().Add(timeout)
	body := `{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"e2e","version":"0.1"}}}`
	attempts := 0
	for time.Now().Before(deadline) {
		attempts++
		resp, err := doMCP(body, "")
		if err == nil {
			resp.Body.Close()
			t.Logf("[YELLOW] healthy after %d attempts (%s)", attempts, time.Since(start).Round(time.Millisecond))
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("[ORANGE] not healthy after %d attempts (%s)", attempts, timeout)
}

func initSession(t *testing.T) string {
	t.Helper()
	resp, err := doMCP(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"e2e","version":"0.1"}}}`, "")
	if err != nil {
		t.Fatalf("[ORANGE] initialize: %v", err)
	}
	sid := resp.Header.Get("Mcp-Session-Id")
	resp.Body.Close()
	if sid == "" {
		t.Fatal("[ORANGE] no Mcp-Session-Id")
	}
	doMCP(`{"jsonrpc":"2.0","method":"notifications/initialized"}`, sid)
	t.Logf("[YELLOW] session: %s...", sid[:16])
	return sid
}

func mcpToolCall(t *testing.T, sid string, id int, tool string, args map[string]any) map[string]any {
	t.Helper()
	params := map[string]any{"name": tool}
	if args != nil {
		params["arguments"] = args
	}
	req := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  "tools/call",
		"params":  params,
	}
	body, _ := json.Marshal(req)
	start := time.Now()
	resp, err := doMCP(string(body), sid)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("[ORANGE] %s (id=%d): %v", tool, id, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	jsonPayload := extractSSEData(raw)
	var result map[string]any
	if err := json.Unmarshal(jsonPayload, &result); err != nil {
		t.Fatalf("[ORANGE] unmarshal %s: %v\nraw: %s", tool, err, truncate(string(raw), 500))
	}
	t.Logf("[YELLOW] %s (id=%d) in %s (%d bytes)", tool, id, elapsed.Round(time.Millisecond), len(raw))
	return result
}

func extractSSEData(raw []byte) []byte {
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data: ") {
			return []byte(strings.TrimPrefix(line, "data: "))
		}
	}
	return raw
}

func extractText(t *testing.T, result map[string]any) string {
	t.Helper()
	if errObj, ok := result["error"]; ok {
		t.Fatalf("[ORANGE] MCP error: %v", errObj)
	}
	r, ok := result["result"].(map[string]any)
	if !ok {
		t.Fatalf("[ORANGE] no result: %v", result)
	}
	content, ok := r["content"].([]any)
	if !ok || len(content) == 0 {
		t.Fatalf("[ORANGE] empty content: %v", r)
	}
	first := content[0].(map[string]any)
	text, _ := first["text"].(string)
	return text
}

func doMCP(body, sid string) (*http.Response, error) {
	req, _ := http.NewRequest("POST", testAddr+"/", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if sid != "" {
		req.Header.Set("Mcp-Session-Id", sid)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, raw)
	}
	return resp, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// --- E2E Tests ---
//
// These tests replicate the bugs from CON-2026-353:
//   - scratch image has no git → scan_project/get_hot_spots fail
//   - no workspace mount → get_rules/scan_project can't find repos
//
// They PASS with Dockerfile.test (Alpine + git + mounts).

func TestE2E(t *testing.T) {
	if _, err := exec.LookPath("podman"); err != nil {
		t.Skip("podman not found")
	}

	locusRepo := repoRoot(t)

	// Mount a workspace that has .cursor/rules — Origami is the primary
	// consumer and always has them. Fall back to parent dir if not available.
	origamiPath := filepath.Join(filepath.Dir(locusRepo), "origami")
	workspacePath := envOr("WORKSPACE_PATH", origamiPath)
	if _, err := os.Stat(filepath.Join(workspacePath, ".cursor", "rules")); err != nil {
		workspacePath = filepath.Dir(locusRepo)
		t.Logf("[YELLOW] origami not found, falling back to %s", workspacePath)
	}

	stopContainer(t)
	t.Cleanup(func() { stopContainer(t) })

	t.Log("[YELLOW] === Locus E2E: git + workspace mount ===")
	buildImage(t)
	startContainer(t,
		locusRepo+":/repo:ro,z",
		workspacePath+":/workspace:ro,z",
	)
	waitHealthy(t, 30*time.Second)
	sid := initSession(t)
	callID := 10
	nextID := func() int { callID++; return callID }

	// Bug 1: scan_project needs git (HEAD SHA, churn) and filesystem access
	t.Run("scan_project", func(t *testing.T) {
		text := extractText(t, mcpToolCall(t, sid, nextID(), "scan_project", map[string]any{
			"path": "/repo",
		}))
		if len(text) < 100 {
			t.Fatalf("[ORANGE] scan_project returned suspiciously small result (%d bytes): %s", len(text), text)
		}
		if !strings.Contains(text, "component") && !strings.Contains(text, "edge") {
			t.Fatalf("[ORANGE] scan_project result missing graph data:\n%s", truncate(text, 500))
		}

		// CON-2026-368: verify LOC metric is present and non-zero
		var scanResult struct {
			Components []struct {
				Name string `json:"name"`
				LOC  int    `json:"loc"`
			} `json:"components"`
		}
		if err := json.Unmarshal([]byte(text), &scanResult); err != nil {
			t.Fatalf("[ORANGE] failed to parse scan JSON for LOC check: %v", err)
		}
		hasLOC := false
		for _, c := range scanResult.Components {
			if c.LOC > 0 {
				hasLOC = true
				break
			}
		}
		if !hasLOC {
			t.Fatal("[ORANGE] no component has LOC > 0 — metric pipeline broken")
		}
		t.Logf("[YELLOW] scan_project: %d bytes, %d components with LOC", len(text), len(scanResult.Components))
	})

	// CON-2026-367: path resolution should return actionable error for bad paths
	t.Run("scan_project_bad_path", func(t *testing.T) {
		result := mcpToolCall(t, sid, nextID(), "scan_project", map[string]any{
			"path": "/nonexistent/repo/xyz",
		})
		if errObj, ok := result["error"]; ok {
			t.Logf("[YELLOW] got expected error: %v", errObj)
			return
		}
		r, ok := result["result"].(map[string]any)
		if ok {
			content, _ := r["content"].([]any)
			if len(content) > 0 {
				first := content[0].(map[string]any)
				text, _ := first["text"].(string)
				if strings.Contains(text, "error") || strings.Contains(text, "no such file") || strings.Contains(text, "does not exist") {
					t.Logf("[YELLOW] got actionable error in text: %s", truncate(text, 200))
					return
				}
			}
			isErr, _ := r["isError"].(bool)
			if isErr {
				t.Logf("[YELLOW] scan_project correctly returned isError for bad path")
				return
			}
		}
		t.Fatal("[ORANGE] scan_project with bad path did not return error — path resolution broken")
	})

	// Bug 1 continued: get_hot_spots depends on git log for churn
	t.Run("get_hot_spots", func(t *testing.T) {
		text := extractText(t, mcpToolCall(t, sid, nextID(), "get_hot_spots", map[string]any{
			"path": "/repo",
		}))
		t.Logf("[YELLOW] get_hot_spots: %d bytes — %s", len(text), truncate(text, 300))
	})

	// Bug 1 continued: coupling table requires scan data
	t.Run("get_coupling_table", func(t *testing.T) {
		text := extractText(t, mcpToolCall(t, sid, nextID(), "get_coupling_table", map[string]any{
			"path": "/repo",
		}))
		if len(text) < 20 {
			t.Fatalf("[ORANGE] coupling table empty: %s", text)
		}
		t.Logf("[YELLOW] coupling_table: %d bytes — %s", len(text), truncate(text, 300))
	})

	// Bug 2: get_rules needs the workspace path to exist inside container
	t.Run("get_rules", func(t *testing.T) {
		rulesDir := filepath.Join(workspacePath, ".cursor", "rules")
		if _, err := os.Stat(rulesDir); err != nil {
			t.Skipf("no .cursor/rules at %s", workspacePath)
		}
		text := extractText(t, mcpToolCall(t, sid, nextID(), "get_rules", map[string]any{
			"path": "/workspace",
		}))
		if text == "null" || text == "[]" || len(text) < 10 {
			t.Fatalf("[ORANGE] get_rules returned empty for mounted workspace: %s", text)
		}
		t.Logf("[YELLOW] get_rules: %d bytes — %s", len(text), truncate(text, 300))
	})

	// Bug 2 continued: get_skills needs workspace mount
	t.Run("get_skills", func(t *testing.T) {
		skillsDir := filepath.Join(workspacePath, ".cursor", "skills")
		if _, err := os.Stat(skillsDir); err != nil {
			t.Skipf("no .cursor/skills at %s", workspacePath)
		}
		text := extractText(t, mcpToolCall(t, sid, nextID(), "get_skills", map[string]any{
			"path": "/workspace",
		}))
		t.Logf("[YELLOW] get_skills: %d bytes — %s", len(text), truncate(text, 300))
	})

	// Edge list validates graph analysis end-to-end
	t.Run("get_edge_list", func(t *testing.T) {
		text := extractText(t, mcpToolCall(t, sid, nextID(), "get_edge_list", map[string]any{
			"path": "/repo",
		}))
		if len(text) < 20 {
			t.Fatalf("[ORANGE] edge list empty: %s", text)
		}
		t.Logf("[YELLOW] edge_list: %d bytes — %s", len(text), truncate(text, 300))
	})

	// suggest_depth exercises the depth analysis
	t.Run("suggest_depth", func(t *testing.T) {
		text := extractText(t, mcpToolCall(t, sid, nextID(), "suggest_depth", map[string]any{
			"path": "/repo",
		}))
		t.Logf("[YELLOW] suggest_depth: %s", truncate(text, 200))
	})
}

// --- LLM Round-Trip Test ---

type ollamaToolCall struct {
	Function struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	} `json:"function"`
}

type ollamaMessage struct {
	Role      string           `json:"role"`
	Content   string           `json:"content"`
	ToolCalls []ollamaToolCall `json:"tool_calls,omitempty"`
}

type ollamaResponse struct {
	Message ollamaMessage `json:"message"`
}

func ollamaReachable(host string) bool {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(host + "/api/tags")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 200
}

func ollamaChatWithTools(t *testing.T, host, model string, messages []map[string]any, tools []map[string]any) ollamaResponse {
	t.Helper()
	payload := map[string]any{
		"model":    model,
		"stream":   false,
		"messages": messages,
		"options":  map[string]any{"temperature": 0.0},
	}
	if len(tools) > 0 {
		payload["tools"] = tools
	}
	body, _ := json.Marshal(payload)
	t.Logf("ollama: model=%s, messages=%d, payload=%d bytes", model, len(messages), len(body))

	client := &http.Client{Timeout: 300 * time.Second}
	resp, err := client.Post(host+"/api/chat", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("ollama failed: %v", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("ollama HTTP %d: %s", resp.StatusCode, truncate(string(raw), 500))
	}

	var result ollamaResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("decode: %v\nraw: %s", err, truncate(string(raw), 500))
	}
	return result
}

func TestE2E_LLMRoundTrip(t *testing.T) {
	if _, err := exec.LookPath("podman"); err != nil {
		t.Skip("podman not found")
	}

	ollamaHost := envOr("OLLAMA_HOST", "http://localhost:11434")
	ollamaModel := envOr("OLLAMA_MODEL", "llama3.1:8b")

	t.Logf("=== LLM Round-Trip (model=%s) ===", ollamaModel)

	if !ollamaReachable(ollamaHost) {
		t.Skipf("Ollama not reachable at %s — skipping", ollamaHost)
	}

	locusRepo := repoRoot(t)

	stopContainer(t)
	t.Cleanup(func() { stopContainer(t) })

	buildImage(t)
	startContainer(t, locusRepo+":/repo:ro,z")
	waitHealthy(t, 30*time.Second)
	sid := initSession(t)

	locusTools := []map[string]any{
		{
			"type": "function",
			"function": map[string]any{
				"name":        "scan_project",
				"description": "Scan a repository and return its architecture: components, dependencies, churn, hot spots, and symbols. You MUST call this to analyze a codebase.",
				"parameters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path":   map[string]any{"type": "string", "description": "Path to the repository root"},
						"format": map[string]any{"type": "string", "description": "Output format: json or summary"},
					},
					"required": []string{"path"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]any{
				"name":        "get_hot_spots",
				"description": "Return the hottest components in a repository (high fan-in + high churn). Call this to find risky areas.",
				"parameters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path":  map[string]any{"type": "string", "description": "Path to the repository root"},
						"top_n": map[string]any{"type": "integer", "description": "Number of hot spots to return"},
					},
					"required": []string{"path"},
				},
			},
		},
	}

	messages := []map[string]any{
		{"role": "system", "content": "You are a software architect. Use the provided tools to analyze codebases. Always call tools when asked about code."},
		{"role": "user", "content": "Scan the repository at /repo using format 'summary' and tell me which components are hot spots."},
	}

	scanCalled := false
	toolCalled := false
	maxTurns := 6

	for turn := 1; turn <= maxTurns; turn++ {
		t.Logf("--- Turn %d/%d ---", turn, maxTurns)
		start := time.Now()
		resp := ollamaChatWithTools(t, ollamaHost, ollamaModel, messages, locusTools)
		t.Logf("responded in %s", time.Since(start).Round(time.Millisecond))

		if len(resp.Message.ToolCalls) > 0 {
			tc := resp.Message.ToolCalls[0]
			argsJSON, _ := json.Marshal(tc.Function.Arguments)
			t.Logf("tool call: %s(%s)", tc.Function.Name, string(argsJSON))
			toolCalled = true

			if tc.Function.Name == "scan_project" {
				scanCalled = true
			}

			toolResult := extractText(t, mcpToolCall(t, sid, 300+turn, tc.Function.Name, tc.Function.Arguments))
			// Truncate large results to keep LLM context manageable
			toolResult = truncate(toolResult, 4000)
			t.Logf("tool result: %d bytes (fed to LLM)", len(toolResult))

			messages = append(messages,
				map[string]any{"role": "assistant", "content": "", "tool_calls": resp.Message.ToolCalls},
				map[string]any{"role": "tool", "content": toolResult},
			)
			continue
		}

		answer := resp.Message.Content
		t.Logf("answer (%d chars): %s", len(answer), truncate(answer, 500))

		if !toolCalled {
			t.Fatal("[ORANGE] LLM answered WITHOUT calling any tool — agent loop broken")
		}
		if !scanCalled {
			t.Fatal("[ORANGE] LLM never called scan_project")
		}
		t.Logf("[YELLOW] PASS: scan_project called=%v", scanCalled)
		return
	}

	if !toolCalled {
		t.Fatal("[ORANGE] exhausted turns without tool call")
	}
	t.Fatal("[ORANGE] exhausted turns without final answer")
}
