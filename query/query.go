// Package query provides backward-compatible access to the batch query API.
// The implementation lives in internal/protocol. This package re-exports the
// types and wraps Protocol.BatchQuery for external consumers.
package query

import (
	"context"
	"encoding/json"

	"github.com/dpopsuev/oculus/engine"
	"github.com/dpopsuev/oculus/lsp"
)

// Type aliases for backward compatibility.
type (
	Request  = engine.BatchRequest
	Action   = engine.BatchAction
	Response = engine.BatchResponse
	Result   = engine.BatchResult
)

// Re-export sentinel errors.
var (
	ErrPathRequired     = engine.ErrBatchPathRequired
	ErrUnknownAction    = engine.ErrUnknownAction
	ErrUnsupportedBatch = engine.ErrUnsupportedBatch
)

// Ensure json is used (referenced by Result.Data type alias).
var _ json.RawMessage

// Client wraps Protocol for backward-compatible batch queries.
type Client struct {
	proto *engine.Engine
}

// New creates a Client backed by the given store and workspace roots.
func New(s engine.Store, workspaces []string, pool ...lsp.Pool) *Client {
	return &Client{proto: engine.New(s, workspaces, pool...)}
}

// Query executes a batch of actions, sharing a single scan/cache.
func (c *Client) Query(ctx context.Context, req Request) (*Response, error) {
	return c.proto.BatchQuery(ctx, req)
}
