package mcp

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Dogfood corpus: synthetic multi-file Go tree exercising the agent LSP
// surface (resolve → definition/references/show/rename dry-run) without
// touching real local repos. Pool is stubbed in MCP handler tests; we assert
// the tool wiring and locator path, not live gopls quality (see docs/LSP_GOTCHAS.md).
func TestLSPDogfood_SyntheticCorpus(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.com/dogfood\n\ngo 1.22\n")
	write("pkg/greet/greet.go", "package greet\n\nfunc Hello() string { return \"hi\" }\n")
	write("cmd/app/main.go", "package main\n\nimport \"example.com/dogfood/pkg/greet\"\n\nfunc main() { _ = greet.Hello() }\n")
	gitInitDogfood(t, dir)

	h := newHandlerWithWorkspace(t, dir)
	ctx := context.Background()
	if _, _, err := h.handleScanProject(ctx, nil, &codographActionInput{Intent: "architecture"}); err != nil {
		t.Fatalf("scan: %v", err)
	}

	cases := []struct {
		action string
		symbol string
		extra  map[string]any
		want   string
	}{
		{ActionResolve, "pkg/greet/greet.go:Hello", nil, "Hello"},
		{ActionDefinition, "pkg/greet/greet.go:Hello", nil, "definition"},
		{ActionReferences, "Hello", nil, "reference"},
		{ActionShow, "pkg/greet/greet.go:Hello", nil, "show"},
		{ActionRename, "pkg/greet/greet.go:Hello", map[string]any{"new_name": "Howdy"}, "Howdy"},
	}
	for _, tc := range cases {
		in := analysisInput{Action: tc.action, Path: dir, Symbol: tc.symbol}
		if tc.extra != nil {
			if v, ok := tc.extra["new_name"].(string); ok {
				in.NewName = v
			}
		}
		raw, _ := json.Marshal(in)
		var op AnalysisOp
		switch tc.action {
		case ActionResolve:
			op = opResolve
		case ActionDefinition:
			op = opDefinition
		case ActionReferences:
			op = opReferences
		case ActionShow:
			op = opShow
		case ActionRename:
			op = opRename
		}
		res, err := op.Run(ctx, h, raw)
		if err != nil {
			t.Fatalf("%s: %v", tc.action, err)
		}
		if !strings.Contains(strings.ToLower(res.Text), strings.ToLower(tc.want)) &&
			!strings.Contains(res.Text, "unavailable") &&
			!strings.Contains(res.Text, "escalat") {
			t.Fatalf("%s: expected %q or degrade, got %s", tc.action, tc.want, res.Text)
		}
		t.Logf("%s: %s", tc.action, truncate(res.Text, 200))
	}
}

func gitInitDogfood(t *testing.T, dir string) {
	t.Helper()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init")
	run("add", ".")
	run("commit", "-m", "init")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
