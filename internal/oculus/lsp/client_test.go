package lsp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestClientWriteMessage(t *testing.T) {
	var buf bytes.Buffer
	c := NewClient(strings.NewReader(""), &buf)

	err := c.writeMessage(JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
	})
	if err != nil {
		t.Fatalf("writeMessage: %v", err)
	}

	out := buf.String()
	if !strings.HasPrefix(out, "Content-Length: ") {
		t.Errorf("output missing Content-Length header: %q", out)
	}
	if !strings.Contains(out, "\r\n\r\n") {
		t.Errorf("output missing header terminator: %q", out)
	}

	parts := strings.SplitN(out, "\r\n\r\n", 2)
	if len(parts) != 2 {
		t.Fatalf("unexpected format: %q", out)
	}
	var req JSONRPCRequest
	if err := json.Unmarshal([]byte(parts[1]), &req); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if req.Method != "initialize" {
		t.Errorf("method = %q, want initialize", req.Method)
	}
}

func TestClientReadMessage(t *testing.T) {
	body := `{"jsonrpc":"2.0","id":1,"result":{"capabilities":{}}}`
	msg := fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body)

	c := NewClient(strings.NewReader(msg), nil)
	resp, err := c.readMessage()
	if err != nil {
		t.Fatalf("readMessage: %v", err)
	}
	if resp.ID == nil || *resp.ID != 1 {
		t.Errorf("id = %v, want 1", resp.ID)
	}
	if resp.Error != nil {
		t.Errorf("unexpected error: %v", resp.Error)
	}
	if resp.Result == nil {
		t.Error("result is nil")
	}
}

func TestClientRoundTrip(t *testing.T) {
	respBody := `{"jsonrpc":"2.0","id":1,"result":{"serverInfo":{"name":"test"}}}`
	respMsg := fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(respBody), respBody)

	var reqBuf bytes.Buffer
	c := NewClient(strings.NewReader(respMsg), &reqBuf)

	result, err := c.Request("initialize", map[string]any{"processId": 1})
	if err != nil {
		t.Fatalf("Request: %v", err)
	}

	if !strings.Contains(string(result), "test") {
		t.Errorf("result = %s, want to contain 'test'", result)
	}

	sent := reqBuf.String()
	if !strings.Contains(sent, "initialize") {
		t.Errorf("sent request missing method: %s", sent)
	}
}

func TestClientNotify(t *testing.T) {
	var buf bytes.Buffer
	c := NewClient(strings.NewReader(""), &buf)

	err := c.Notify("initialized", struct{}{})
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "initialized") {
		t.Errorf("notification missing method: %s", out)
	}
	if strings.Contains(out, `"id"`) {
		t.Errorf("notification should not have an id: %s", out)
	}
}

func TestClientReadError(t *testing.T) {
	body := `{"jsonrpc":"2.0","id":1,"error":{"code":-32600,"message":"invalid request"}}`
	msg := fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body)

	c := NewClient(strings.NewReader(msg), nil)
	resp, err := c.readMessage()
	if err != nil {
		t.Fatalf("readMessage: %v", err)
	}
	if resp.Error == nil {
		t.Fatal("expected error in response")
	}
	if resp.Error.Code != -32600 {
		t.Errorf("error code = %d, want -32600", resp.Error.Code)
	}
}

func TestClientMissingContentLength(t *testing.T) {
	msg := "Content-Type: application/json\r\n\r\n{}"
	c := NewClient(strings.NewReader(msg), nil)
	_, err := c.readMessage()
	if err == nil {
		t.Fatal("expected error for missing Content-Length")
	}
}
