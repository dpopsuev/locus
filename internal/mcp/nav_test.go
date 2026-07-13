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

func TestOpDefinition_EmptyRejected(t *testing.T) {
	h := newHandlerWithWorkspace(t, t.TempDir())
	raw, _ := json.Marshal(analysisInput{Action: ActionDefinition})
	_, err := opDefinition.Run(context.Background(), h, raw)
	if !errors.Is(err, ErrDefinitionLocatorRequired) {
		t.Fatalf("got %v", err)
	}
}

func TestOpReferences_EmptyRejected(t *testing.T) {
	h := newHandlerWithWorkspace(t, t.TempDir())
	raw, _ := json.Marshal(analysisInput{Action: ActionReferences})
	_, err := opReferences.Run(context.Background(), h, raw)
	if !errors.Is(err, ErrReferencesLocatorRequired) {
		t.Fatalf("got %v", err)
	}
}

func TestOpShow_EmptyRejected(t *testing.T) {
	h := newHandlerWithWorkspace(t, t.TempDir())
	raw, _ := json.Marshal(analysisInput{Action: ActionShow})
	_, err := opShow.Run(context.Background(), h, raw)
	if !errors.Is(err, ErrShowLocatorRequired) {
		t.Fatalf("got %v", err)
	}
}

func TestOpDefinition_AfterScan_AmbiguousOrHit(t *testing.T) {
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
	mustWrite("go.mod", "module example.com/nav\n\ngo 1.22\n")
	mustWrite("pkg/hello.go", "package pkg\n\nfunc Hello() {}\n")
	gitInitNav(t, dir)

	h := newHandlerWithWorkspace(t, dir)
	ctx := context.Background()
	if _, _, err := h.handleScanProject(ctx, nil, &codographActionInput{Intent: "architecture"}); err != nil {
		t.Fatalf("scan: %v", err)
	}

	raw, _ := json.Marshal(analysisInput{Action: ActionDefinition, Path: dir, Symbol: "pkg/hello.go:Hello"})
	res, err := opDefinition.Run(ctx, h, raw)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Text, "Hello") && !strings.Contains(res.Text, "definition") && !strings.Contains(res.Text, "unavailable") {
		t.Fatalf("unexpected: %s", res.Text)
	}
	t.Logf("definition: %s", res.Text)

	raw, _ = json.Marshal(analysisInput{Action: ActionReferences, Path: dir, Symbol: "Hello"})
	res, err = opReferences.Run(ctx, h, raw)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("references: %s", res.Text)

	raw, _ = json.Marshal(analysisInput{Action: ActionShow, Path: dir, Symbol: "pkg/hello.go:Hello"})
	res, err = opShow.Run(ctx, h, raw)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Text, "Hello") && !strings.Contains(res.Text, "show") && !strings.Contains(res.Text, "unavailable") {
		t.Fatalf("unexpected show: %s", res.Text)
	}
	t.Logf("show: %s", res.Text)
}

func gitInitNav(t *testing.T, dir string) {
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
