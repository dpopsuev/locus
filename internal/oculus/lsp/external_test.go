package lsp_test

import (
	"testing"

	"github.com/dpopsuev/locus/internal/oculus/lsp"
)

func TestStubPool_External(t *testing.T) {
	pool := &lsp.StubPool{}
	status := pool.Status()
	_ = status
}
