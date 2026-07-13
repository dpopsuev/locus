package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpResolve_EmptyRejected(t *testing.T) {
	h := newHandlerWithWorkspace(t, t.TempDir())
	raw, _ := json.Marshal(analysisInput{Action: ActionResolve})
	_, err := opResolve.Run(context.Background(), h, raw)
	if !errors.Is(err, ErrLocatorRequired) {
		t.Fatalf("got %v, want ErrLocatorRequired", err)
	}
}

func TestOpResolve_AfterScan_PathSymbol(t *testing.T) {
	dir := t.TempDir()
	mustWrite := func(rel, body string) {
		t.Helper()
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("go.mod", "module example.com/resolve\n\ngo 1.22\n")
	mustWrite("core/config.go", "package core\n\ntype Config struct{}\n")
	mustWrite("core/run.go", "package core\n\nfunc Run() {}\n")
	gitInit(t, dir)

	h := newHandlerWithWorkspace(t, dir)
	ctx := context.Background()
	if _, _, err := h.handleScanProject(ctx, nil, &codographActionInput{Intent: "architecture"}); err != nil {
		t.Fatalf("scan: %v", err)
	}

	raw, _ := json.Marshal(analysisInput{
		Action: ActionResolve, Path: dir, Symbol: "core/config.go:Config",
	})
	res, err := opResolve.Run(ctx, h, raw)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Text, "Config") {
		t.Fatalf("resolve path:Symbol: %s", res.Text)
	}
	if strings.Contains(res.Text, `"candidates"`) && !strings.Contains(res.Text, `"hit"`) {
		t.Fatalf("expected unique hit for path:Symbol, got %s", res.Text)
	}
}

func gitInit(t *testing.T, dir string) {
	t.Helper()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init")
	run("add", ".")
	run("commit", "-m", "init")
}
