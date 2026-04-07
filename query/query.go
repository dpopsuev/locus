// Package query provides backward-compatible access to the batch query API.
// The implementation lives in internal/protocol. This package re-exports the
// types and wraps Protocol.BatchQuery for external consumers.
package query

import (
	"context"
	"encoding/json"

	"github.com/dpopsuev/locus/internal/protocol"
	"github.com/dpopsuev/oculus/lsp"
)

// Type aliases for backward compatibility.
type (
	Request  = protocol.BatchRequest
	Action   = protocol.BatchAction
	Response = protocol.BatchResponse
	Result   = protocol.BatchResult
)

// Re-export sentinel errors.
var (
	ErrPathRequired     = protocol.ErrBatchPathRequired
	ErrUnknownAction    = protocol.ErrUnknownAction
	ErrUnsupportedBatch = protocol.ErrUnsupportedBatch
)

// Ensure json is used (referenced by Result.Data type alias).
var _ json.RawMessage

// Client wraps Protocol for backward-compatible batch queries.
type Client struct {
	proto *protocol.Protocol
}

// New creates a Client backed by the given store and workspace roots.
func New(s protocol.ProtocolStore, workspaces []string, pool ...lsp.Pool) *Client {
	return &Client{proto: protocol.New(s, workspaces, pool...)}
}

// Query executes a batch of actions, sharing a single scan/cache.
func (c *Client) Query(ctx context.Context, req Request) (*Response, error) {
	return c.proto.BatchQuery(ctx, req)
}
