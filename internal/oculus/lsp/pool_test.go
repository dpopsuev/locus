package lsp_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/dpopsuev/locus/internal/oculus/lang"
	"github.com/dpopsuev/locus/internal/oculus/lsp"
)

// --- StubPool contract tests ---

func TestStubPool_GetReturnsError(t *testing.T) {
	p := &lsp.StubPool{}
	client, err := p.Get(lang.Go, "/tmp/test")
	if !errors.Is(err, lsp.ErrNoPool) {
		t.Fatalf("expected ErrNoPool, got %v", err)
	}
	if client != nil {
		t.Fatal("expected nil client")
	}
}

func TestStubPool_ShutdownIdempotent(t *testing.T) {
	p := &lsp.StubPool{}
	if err := p.Shutdown(context.Background()); err != nil {
		t.Fatalf("first shutdown: %v", err)
	}
	if err := p.Shutdown(context.Background()); err != nil {
		t.Fatalf("second shutdown: %v", err)
	}
}

func TestStubPool_StatusEmpty(t *testing.T) {
	p := &lsp.StubPool{}
	s := p.Status()
	if s.Active != 0 {
		t.Fatalf("expected 0 active, got %d", s.Active)
	}
	if s.Idle != 0 {
		t.Fatalf("expected 0 idle, got %d", s.Idle)
	}
	if len(s.ByLang) != 0 {
		t.Fatalf("expected empty ByLang, got %v", s.ByLang)
	}
}

// --- RealPool contract tests ---

func requireGopls(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not available")
	}
}

func makeGoRoot(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\ngo 1.21\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestRealPool_GetReturnsClient(t *testing.T) {
	requireGopls(t)
	dir := makeGoRoot(t)
	pool := lsp.NewPool()
	defer pool.Shutdown(context.Background()) //nolint:errcheck // best-effort cleanup

	client, err := pool.Get(lang.Go, dir)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
}

func TestRealPool_GetReusesConnection(t *testing.T) {
	requireGopls(t)
	dir := makeGoRoot(t)
	pool := lsp.NewPool()
	defer pool.Shutdown(context.Background()) //nolint:errcheck // best-effort cleanup

	c1, err := pool.Get(lang.Go, dir)
	if err != nil {
		t.Fatalf("first Get: %v", err)
	}
	c2, err := pool.Get(lang.Go, dir)
	if err != nil {
		t.Fatalf("second Get: %v", err)
	}
	if c1 != c2 {
		t.Fatal("expected same client from two Gets")
	}
}

func TestRealPool_ShutdownCleansUp(t *testing.T) {
	requireGopls(t)
	dir := makeGoRoot(t)
	pool := lsp.NewPool()

	_, err := pool.Get(lang.Go, dir)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	s := pool.Status()
	if s.Active != 1 {
		t.Fatalf("expected 1 active before shutdown, got %d", s.Active)
	}

	if err := pool.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	s = pool.Status()
	if s.Active != 0 {
		t.Fatalf("expected 0 active after shutdown, got %d", s.Active)
	}
}

func TestRealPool_StatusReportsActive(t *testing.T) {
	requireGopls(t)
	dir := makeGoRoot(t)
	pool := lsp.NewPool()
	defer pool.Shutdown(context.Background()) //nolint:errcheck // best-effort cleanup

	s := pool.Status()
	if s.Active != 0 {
		t.Fatalf("expected 0 active initially, got %d", s.Active)
	}

	_, err := pool.Get(lang.Go, dir)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	s = pool.Status()
	if s.Active != 1 {
		t.Fatalf("expected 1 active, got %d", s.Active)
	}
	if s.ByLang[lang.Go] != 1 {
		t.Fatalf("expected 1 Go connection, got %d", s.ByLang[lang.Go])
	}
}
