// Package lsp provides a JSON-RPC 2.0 client for Language Server Protocol
// communication. Part of the Oculus symbol resolution library.
package lsp

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
)

// ErrMissingContentLength is returned when an LSP message lacks the header.
var ErrMissingContentLength = errors.New("missing Content-Length header")

// Client implements the JSON-RPC 2.0 transport for LSP communication
// over a stdin/stdout pipe pair.
type Client struct {
	w      io.Writer
	r      *bufio.Reader
	mu     sync.Mutex
	nextID int
}

// NewClient creates an LSP client from reader/writer pairs (typically
// connected to an LSP server's stdin/stdout).
func NewClient(r io.Reader, w io.Writer) *Client {
	return &Client{
		w:      w,
		r:      bufio.NewReader(r),
		nextID: 1,
	}
}

// JSONRPCRequest is a JSON-RPC 2.0 request message.
type JSONRPCRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// JSONRPCResponse is a JSON-RPC 2.0 response message.
type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int            `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

// JSONRPCError represents an error in a JSON-RPC response.
type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *JSONRPCError) Error() string {
	return fmt.Sprintf("LSP error %d: %s", e.Code, e.Message)
}

// Request sends a JSON-RPC request and reads the response, skipping
// any interleaved notifications from the server.
func (c *Client) Request(method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	id := c.nextID
	c.nextID++
	c.mu.Unlock()

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	if err := c.writeMessage(req); err != nil {
		return nil, fmt.Errorf("lsp request %s: %w", method, err)
	}

	for {
		resp, err := c.readMessage()
		if err != nil {
			return nil, fmt.Errorf("lsp response %s: %w", method, err)
		}

		// Skip server-initiated notifications and requests (they have no id
		// or have a method field indicating a server->client request).
		if resp.ID == nil || resp.Method != "" {
			continue
		}

		if *resp.ID == id {
			if resp.Error != nil {
				return nil, resp.Error
			}
			return resp.Result, nil
		}
	}
}

// Notify sends a JSON-RPC notification (no response expected).
func (c *Client) Notify(method string, params any) error {
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
	return c.writeMessage(req)
}

func (c *Client) writeMessage(msg any) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))
	if _, err := io.WriteString(c.w, header); err != nil {
		return err
	}
	_, err = c.w.Write(body)
	return err
}

func (c *Client) readMessage() (*JSONRPCResponse, error) {
	contentLen := -1
	for {
		line, err := c.r.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("reading header: %w", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if strings.HasPrefix(line, "Content-Length:") {
			val := strings.TrimSpace(strings.TrimPrefix(line, "Content-Length:"))
			contentLen, err = strconv.Atoi(val)
			if err != nil {
				return nil, fmt.Errorf("invalid Content-Length %q: %w", val, err)
			}
		}
	}

	if contentLen < 0 {
		return nil, ErrMissingContentLength
	}

	body := make([]byte, contentLen)
	if _, err := io.ReadFull(c.r, body); err != nil {
		return nil, fmt.Errorf("reading body: %w", err)
	}

	var resp JSONRPCResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &resp, nil
}
