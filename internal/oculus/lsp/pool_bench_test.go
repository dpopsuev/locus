package lsp_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/dpopsuev/locus/internal/oculus/lang"
	"github.com/dpopsuev/locus/internal/oculus/lsp"
)

// BenchmarkPoolGet_Warm measures the cost of retrieving an already-cached
// LSP connection from the pool (the fast path in serve mode).
func BenchmarkPoolGet_Warm(b *testing.B) {
	if _, err := exec.LookPath("gopls"); err != nil {
		b.Skip("gopls not available")
	}

	dir := b.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\ngo 1.21\n"), 0o644); err != nil {
		b.Fatal(err)
	}

	pool := lsp.NewPool()
	defer pool.Shutdown(context.Background()) //nolint:errcheck // best-effort cleanup

	// Warm up: create the connection once.
	if _, err := pool.Get(lang.Go, dir); err != nil {
		b.Fatalf("warm-up Get: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := pool.Get(lang.Go, dir)
		if err != nil {
			b.Fatalf("Get: %v", err)
		}
		pool.Release(lang.Go, dir)
	}
}
