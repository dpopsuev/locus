// Package query provides a composable, batch-capable Go API for Locus.
// It is the public SDK for external consumers such as Djinn.
//
// All queries in a batch share a single scan, amortizing the filesystem
// walk across multiple analysis actions. Individual query failures are isolated
// and do not abort the batch.
package query

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/dpopsuev/locus/internal/protocol"
	"github.com/dpopsuev/oculus/lsp"
)

// Sentinel errors.
var ErrPathRequired = errors.New("path is required")

// Request describes a batch of queries to execute against a repository.
type Request struct {
	// Path is the repository root (required).
	Path string `json:"path"`

	// CacheKey is an optional scan cache key (from a previous scan_local).
	// If empty, a scan is triggered automatically.
	CacheKey string `json:"cache_key,omitempty"`

	// Intent controls scan depth when no CacheKey is provided.
	// Values: "architecture", "coupling", "health" (default), "full".
	Intent string `json:"intent,omitempty"`

	// Actions is the list of analysis actions to execute.
	// Each action name maps to a Protocol method (e.g. "hot_spots", "cycles").
	Actions []Action `json:"actions"`
}

// Action describes a single query within a batch.
type Action struct {
	// Name is the action identifier (e.g. "hot_spots", "cycles", "violations").
	Name string `json:"name"`

	// Params holds action-specific parameters.
	Params map[string]any `json:"params,omitempty"`
}

// Response holds results for a batch query.
type Response struct {
	// CacheKey is the scan cache key used/generated for this batch.
	CacheKey string `json:"cache_key"`

	// Results contains one result per input action, in the same order.
	Results []Result `json:"results"`
}

// Result is a single action's outcome within a batch.
type Result struct {
	// Action is the action name that produced this result.
	Action string `json:"action"`

	// OK is true if the action succeeded.
	OK bool `json:"ok"`

	// Err is the error message if the action failed.
	Err string `json:"error,omitempty"`

	// Data is the structured JSON result of the action.
	Data json.RawMessage `json:"data,omitempty"`
}

// Client is the public entry point for Locus queries.
type Client struct {
	proto *protocol.Protocol
}

// New creates a Client backed by the given store and workspace roots.
// Pass an optional LSP connection pool for warm-server mode.
func New(s protocol.ProtocolStore, workspaces []string, pool ...lsp.Pool) *Client {
	return &Client{proto: protocol.New(s, workspaces, pool...)}
}

// Query executes a batch of actions, sharing a single scan/cache.
func (c *Client) Query(ctx context.Context, req Request) (*Response, error) {
	if req.Path == "" {
		return nil, ErrPathRequired
	}

	// Ensure a scan exists (one scan for all actions).
	cacheKey := req.CacheKey
	if cacheKey == "" {
		intent := req.Intent
		if intent == "" {
			intent = "health"
		}
		sr, err := c.proto.ScanProject(ctx, req.Path, protocol.ScanOpts{
			Intent: intent,
		})
		if err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		cacheKey = sr.CacheKey
	}

	results := make([]Result, len(req.Actions))
	for i, a := range req.Actions {
		results[i] = dispatch(ctx, c.proto, req.Path, cacheKey, a)
	}

	return &Response{CacheKey: cacheKey, Results: results}, nil
}
