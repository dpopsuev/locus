package protocol

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolvePath_Empty(t *testing.T) {
	p := &Protocol{workspaces: []string{"/tmp"}}
	got := p.resolvePath("")
	if got != "/tmp" {
		t.Fatalf("expected /tmp, got %s", got)
	}
}

func TestResolvePath_EmptyNoWorkspaces(t *testing.T) {
	p := &Protocol{}
	got := p.resolvePath("")
	if got != "." {
		t.Fatalf("expected '.', got %s", got)
	}
}

func TestResolvePath_AbsoluteExists(t *testing.T) {
	dir := t.TempDir()
	p := &Protocol{}
	got := p.resolvePath(dir)
	if got != dir {
		t.Fatalf("expected %s, got %s", dir, got)
	}
}

func TestResolvePath_RelativeToWorkspace(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "project")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	p := &Protocol{workspaces: []string{dir}}
	got := p.resolvePath("project")
	if got != sub {
		t.Fatalf("expected %s, got %s", sub, got)
	}
}

func TestResolvePath_NonExistentFallsThrough(t *testing.T) {
	p := &Protocol{workspaces: []string{"/tmp"}}
	got := p.resolvePath("/nonexistent/path/xyz")
	if got != "/nonexistent/path/xyz" {
		t.Fatalf("expected /nonexistent/path/xyz, got %s", got)
	}
}
