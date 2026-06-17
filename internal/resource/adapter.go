// Package resource monitors and controls Locus process resources.
package resource

import (
	"github.com/dpopsuev/oculus/v3/lsp"
)

// PoolAdapter wraps lsp.RealPool to satisfy PoolInspector.
type PoolAdapter struct {
	Pool *lsp.RealPool
}

// PIDs returns child process IDs from the pool.
func (a *PoolAdapter) PIDs() []int { return a.Pool.PIDs() }

// ReapIdle evicts idle LSP servers.
func (a *PoolAdapter) ReapIdle() int { return a.Pool.ReapIdle() }

// Status returns the pool's current state.
func (a *PoolAdapter) Status() PoolStatusView {
	s := a.Pool.Status()
	byLang := make(map[string]int, len(s.ByLang))
	for l, n := range s.ByLang {
		byLang[string(l)] = n
	}
	return PoolStatusView{Active: s.Active, ByLang: byLang}
}
