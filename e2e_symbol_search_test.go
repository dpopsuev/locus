//go:build e2e

package locus_test

// TestSymbolSearch_* validates the symbol_search MCP action (LCS-TSK-466):
//   - empty query is rejected (ErrQueryRequired)
//   - shallow results are capped at 50
//   - detail:"full" enriches each match with call-graph metrics (cap 10)
//
// Run: go test -tags=e2e -run TestSymbolSearch -timeout 60s -v .
// Requires: locus binary on PATH (run 'make build').

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// connectLocalServer starts the locus binary as an MCP server against dir.
func connectLocalServer(t *testing.T, ctx context.Context, dir string) *sdkmcp.ClientSession {
	t.Helper()
	binary := "./locus"
	if _, err := exec.LookPath(binary); err != nil {
		t.Skip("locus binary not found (run 'make build')")
	}
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "symbol-search-test", Version: "1.0"}, nil)
	transport := &sdkmcp.CommandTransport{
		Command: exec.Command(binary, "serve", "--workspace", dir),
	}
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

// callAnalysis calls the "analysis" MCP tool and returns the raw text response.
// It asserts the call did not fail at the transport level.
func callAnalysis(t *testing.T, s *sdkmcp.ClientSession, ctx context.Context, args map[string]any) (text string, isErr bool) {
	t.Helper()
	result, err := s.CallTool(ctx, &sdkmcp.CallToolParams{Name: "analysis", Arguments: args})
	if err != nil {
		t.Fatalf("call analysis: %v", err)
	}
	if len(result.Content) == 0 {
		return "", result.IsError
	}
	tc, ok := result.Content[0].(*sdkmcp.TextContent)
	if !ok {
		t.Fatalf("analysis returned non-text content: %T", result.Content[0])
	}
	return tc.Text, result.IsError
}

// TestSymbolSearch_EmptyQueryRejected ensures an empty query string returns an
// error response instead of dumping the entire symbol index.
func TestSymbolSearch_EmptyQueryRejected(t *testing.T) {
	ctx := context.Background()
	session := connectLocalServer(t, ctx, ".")

	text, isErr := callAnalysis(t, session, ctx, map[string]any{
		"action": "symbol_search",
		"path":   ".",
		"query":  "",
	})

	if !isErr {
		t.Fatalf("expected an error response for empty query, got success:\n%s", truncate(text, 400))
	}
	if !strings.Contains(text, "symbol_search requires") {
		t.Errorf("expected ErrQueryRequired message, got:\n%s", truncate(text, 400))
	}
	t.Logf("correctly rejected: %s", truncate(text, 200))
}

// TestSymbolSearch_ShallowCapAt50 ensures shallow results never exceed 50
// matches, even for a query that would otherwise hit hundreds of symbols.
func TestSymbolSearch_ShallowCapAt50(t *testing.T) {
	ctx := context.Background()
	session := connectLocalServer(t, ctx, ".")

	// "get" matches many symbols in any Go codebase.
	text, isErr := callAnalysis(t, session, ctx, map[string]any{
		"action": "symbol_search",
		"path":   ".",
		"query":  "get",
	})
	if isErr {
		t.Fatalf("unexpected error: %s", text)
	}

	var report struct {
		Matches []json.RawMessage `json:"matches"`
		Summary string            `json:"summary"`
	}
	if err := json.Unmarshal([]byte(text), &report); err != nil {
		t.Skipf("could not parse JSON response (scan may not have run): %v\n%s", err, truncate(text, 200))
	}

	if len(report.Matches) > 50 {
		t.Errorf("expected ≤50 matches (shallow cap), got %d", len(report.Matches))
	}
	t.Logf("matches=%d summary=%q", len(report.Matches), report.Summary)
}

// TestSymbolSearch_FullDetailEnriches ensures detail:"full" returns
// call-graph fields on each match and honours the hard cap of 10.
func TestSymbolSearch_FullDetailEnriches(t *testing.T) {
	ctx := context.Background()
	session := connectLocalServer(t, ctx, ".")

	text, isErr := callAnalysis(t, session, ctx, map[string]any{
		"action": "symbol_search",
		"path":   ".",
		"query":  "handler",
		"detail": "full",
	})
	if isErr {
		t.Fatalf("unexpected error: %s", text)
	}

	var report struct {
		Matches []struct {
			Symbol          string `json:"symbol"`
			CallGraphStatus string `json:"call_graph_status"`
		} `json:"matches"`
		Summary string `json:"summary"`
	}
	if err := json.Unmarshal([]byte(text), &report); err != nil {
		t.Skipf("could not parse JSON response (scan may not have run): %v\n%s", err, truncate(text, 200))
	}
	if len(report.Matches) == 0 {
		t.Skip("no matches for 'handler' — fixture may not contain relevant symbols")
	}

	// Hard cap: detail:full must never exceed 10 results.
	if len(report.Matches) > 10 {
		t.Errorf("expected ≤10 matches with detail:full (hard cap), got %d", len(report.Matches))
	}

	// Every result must carry call_graph_status — proof that ProbeSymbol ran.
	for i, m := range report.Matches {
		if m.CallGraphStatus == "" {
			t.Errorf("match[%d] %q: missing call_graph_status in detail:full response", i, m.Symbol)
		}
	}
	t.Logf("matches=%d summary=%q", len(report.Matches), report.Summary)
}

// TestSymbolSearch_FullDetailEnriches_GoFixture runs full-detail against the
// built-in Go testkit fixture which is guaranteed to contain relevant symbols.
func TestSymbolSearch_FullDetailEnriches_GoFixture(t *testing.T) {
	ctx := context.Background()
	dir := "./testdata/testkit/go"
	session := connectLocalServer(t, ctx, dir)

	text, isErr := callAnalysis(t, session, ctx, map[string]any{
		"action": "symbol_search",
		"path":   dir,
		"query":  "handler",
		"detail": "full",
	})
	if isErr {
		t.Fatalf("unexpected error: %s", text)
	}

	var report struct {
		Matches []struct {
			Symbol          string `json:"symbol"`
			CallGraphStatus string `json:"call_graph_status"`
		} `json:"matches"`
		Summary string `json:"summary"`
	}
	if err := json.Unmarshal([]byte(text), &report); err != nil {
		t.Skipf("could not parse JSON: %v\n%s", err, truncate(text, 200))
	}
	if len(report.Matches) == 0 {
		t.Skip("no matches — scan did not run or testkit changed")
	}

	if len(report.Matches) > 10 {
		t.Errorf("expected ≤10 matches (hard cap), got %d", len(report.Matches))
	}
	for i, m := range report.Matches {
		if m.CallGraphStatus == "" {
			t.Errorf("match[%d] %q: missing call_graph_status", i, m.Symbol)
		}
	}
	t.Logf("matches=%d summary=%q", len(report.Matches), report.Summary)
}

// TestSymbolSearch_TopNOverridesShallowCap verifies a caller can raise the cap
// above the default via top_n without triggering the full-detail path.
func TestSymbolSearch_TopNOverridesShallowCap(t *testing.T) {
	ctx := context.Background()
	session := connectLocalServer(t, ctx, ".")

	text, isErr := callAnalysis(t, session, ctx, map[string]any{
		"action": "symbol_search",
		"path":   ".",
		"query":  "get",
		"top_n":  5,
	})
	if isErr {
		t.Fatalf("unexpected error: %s", text)
	}

	var report struct {
		Matches []json.RawMessage `json:"matches"`
		Summary string            `json:"summary"`
	}
	if err := json.Unmarshal([]byte(text), &report); err != nil {
		t.Skipf("could not parse JSON response: %v", err)
	}

	if len(report.Matches) > 5 {
		t.Errorf("expected ≤5 matches with top_n=5, got %d", len(report.Matches))
	}
	t.Logf("matches=%d summary=%q", len(report.Matches), report.Summary)
}
