package lsp

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/dpopsuev/locus/internal/oculus/lang"
)

// poolKey uniquely identifies an LSP server connection by language and workspace root.
type poolKey struct {
	lang lang.Language
	root string
}

// poolEntry holds a live LSP server connection.
type poolEntry struct {
	client *Client
	cmd    *exec.Cmd
	stdin  io.WriteCloser
}

// RealPool manages reusable LSP server connections keyed by (language, root).
// Thread-safe via sync.Mutex.
type RealPool struct {
	mu      sync.Mutex
	conns   map[poolKey]*poolEntry
	stopped bool
}

// NewPool creates a new connection pool for long-running (serve) mode.
func NewPool() *RealPool {
	return &RealPool{
		conns: make(map[poolKey]*poolEntry),
	}
}

// Get returns a warm LSP client for the given language and workspace root.
// If no connection exists, one is lazily spawned.
func (p *RealPool) Get(language lang.Language, root string) (*Client, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		absRoot = root
	}
	key := poolKey{lang: language, root: absRoot}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.stopped {
		return nil, ErrPoolShutDown
	}

	if entry, ok := p.conns[key]; ok {
		return entry.client, nil
	}

	entry, err := spawnServer(language, absRoot)
	if err != nil {
		return nil, err
	}
	p.conns[key] = entry
	return entry.client, nil
}

// Release signals that the caller is done with the connection.
// The pool keeps it alive for future callers.
func (p *RealPool) Release(lang.Language, string) {
	// no-op: connection stays alive in pool
}

// Shutdown gracefully stops all managed LSP servers.
func (p *RealPool) Shutdown(_ context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.stopped = true
	for key, entry := range p.conns {
		shutdownEntry(entry)
		delete(p.conns, key)
	}
	return nil
}

// Status returns the current pool state for health reporting.
func (p *RealPool) Status() PoolStatus {
	p.mu.Lock()
	defer p.mu.Unlock()

	byLang := make(map[lang.Language]int)
	for key := range p.conns {
		byLang[key.lang]++
	}
	return PoolStatus{
		Active: len(p.conns),
		Idle:   0,
		ByLang: byLang,
	}
}

// spawnServer starts a new LSP server process and performs the initialize handshake.
func spawnServer(language lang.Language, absRoot string) (*poolEntry, error) {
	cmdStr := lang.DefaultLSPServer(language)
	if cmdStr == "" {
		return nil, fmt.Errorf("%w: %v", ErrNoLSPServer, language)
	}

	parts := strings.Fields(cmdStr)
	bin, err := exec.LookPath(parts[0])
	if err != nil {
		return nil, fmt.Errorf("lsp pool: server %s not found: %w", parts[0], err)
	}

	cmd := exec.Command(bin, parts[1:]...)
	cmd.Dir = absRoot
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("lsp pool: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("lsp pool: stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("lsp pool: start %s: %w", parts[0], err)
	}

	client := NewClient(stdout, stdin)

	if err := initialize(client, absRoot); err != nil {
		stdin.Close()
		_ = cmd.Wait()
		return nil, fmt.Errorf("lsp pool: initialize: %w", err)
	}

	return &poolEntry{
		client: client,
		cmd:    cmd,
		stdin:  stdin,
	}, nil
}

// initialize performs the LSP initialize/initialized handshake.
func initialize(client *Client, root string) error {
	rootURI := "file://" + root
	params := map[string]any{
		"processId": os.Getpid(),
		"rootUri":   rootURI,
		"capabilities": map[string]any{
			"textDocument": map[string]any{
				"documentSymbol": map[string]any{"hierarchicalDocumentSymbolSupport": true},
				"typeHierarchy":  map[string]any{},
				"callHierarchy":  map[string]any{},
				"implementation": map[string]any{},
			},
		},
	}
	if _, err := client.Request("initialize", params); err != nil {
		return err
	}
	return client.Notify("initialized", struct{}{})
}

// shutdownEntry sends LSP shutdown+exit and cleans up process resources.
func shutdownEntry(entry *poolEntry) {
	_, _ = entry.client.Request("shutdown", nil)
	_ = entry.client.Notify("exit", nil)
	entry.stdin.Close()
	_ = entry.cmd.Wait()
}
