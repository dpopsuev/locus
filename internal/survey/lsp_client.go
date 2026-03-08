package survey

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
)

// lspClient implements the JSON-RPC 2.0 transport for LSP communication
// over a stdin/stdout pipe pair.
type lspClient struct {
	w      io.Writer
	r      *bufio.Reader
	mu     sync.Mutex
	nextID int
}

func newLSPClient(r io.Reader, w io.Writer) *lspClient {
	return &lspClient{
		w:      w,
		r:      bufio.NewReader(r),
		nextID: 1,
	}
}

type jsonRPCRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int            `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *jsonRPCError) Error() string {
	return fmt.Sprintf("LSP error %d: %s", e.Code, e.Message)
}

// Request sends a JSON-RPC request and reads the response, skipping
// any interleaved notifications from the server.
func (c *lspClient) Request(method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	id := c.nextID
	c.nextID++
	c.mu.Unlock()

	req := jsonRPCRequest{
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
func (c *lspClient) Notify(method string, params any) error {
	req := jsonRPCRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
	return c.writeMessage(req)
}

func (c *lspClient) writeMessage(msg any) error {
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

func (c *lspClient) readMessage() (*jsonRPCResponse, error) {
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
		return nil, fmt.Errorf("missing Content-Length header")
	}

	body := make([]byte, contentLen)
	if _, err := io.ReadFull(c.r, body); err != nil {
		return nil, fmt.Errorf("reading body: %w", err)
	}

	var resp jsonRPCResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &resp, nil
}
