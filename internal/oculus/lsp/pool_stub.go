package lsp

import (
	"context"

	"github.com/dpopsuev/locus/internal/oculus/lang"
)

// StubPool is a no-op pool for CLI mode. Get always returns ErrNoPool,
// forcing analyzers to fall back to cold-start per request.
type StubPool struct{}

func (s *StubPool) Get(lang.Language, string) (*Client, error) { return nil, ErrNoPool }
func (s *StubPool) Release(lang.Language, string)              {}
func (s *StubPool) Shutdown(context.Context) error             { return nil }
func (s *StubPool) Status() PoolStatus                         { return PoolStatus{} }
