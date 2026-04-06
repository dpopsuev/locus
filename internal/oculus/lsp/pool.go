package lsp

import (
	"context"
	"errors"

	"github.com/dpopsuev/locus/internal/oculus/lang"
)

// ErrNoPool is returned when no pool is available (CLI mode).
var ErrNoPool = errors.New("lsp: no connection pool available")

// ErrPoolShutDown is returned when Get is called on a shut-down pool.
var ErrPoolShutDown = errors.New("lsp pool: shut down")

// ErrNoLSPServer is returned when no LSP server is configured for a language.
var ErrNoLSPServer = errors.New("lsp pool: no server for language")

// Pool manages reusable LSP server connections. In long-running mode
// (locus serve), connections are kept alive across requests. In CLI mode,
// pool is nil and analyzers fall back to cold-start per request.
type Pool interface {
	// Get returns a warm LSP client for the given language and workspace root.
	// If no connection exists, one is lazily started. Returns ErrNoPool from
	// StubPool, or a spawn error if the LSP server can't be started.
	Get(language lang.Language, root string) (*Client, error)

	// Release signals that the caller is done with the connection. The pool
	// keeps it alive for future callers. Does not close the connection.
	Release(language lang.Language, root string)

	// Shutdown gracefully stops all managed LSP servers. Sends LSP shutdown
	// and exit notifications, then kills processes.
	Shutdown(ctx context.Context) error

	// Status returns the current pool state for health reporting.
	Status() PoolStatus
}

// PoolStatus reports the current state of the connection pool.
type PoolStatus struct {
	Active int                   `json:"active"`
	Idle   int                   `json:"idle"`
	ByLang map[lang.Language]int `json:"by_language"`
}
