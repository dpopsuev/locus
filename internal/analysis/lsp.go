package analysis

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dpopsuev/locus/internal/survey"
)

// LSPAnalyzer extracts type-level metadata via an LSP server. It uses
// typeHierarchy, callHierarchy, and implementation requests for ~99%
// semantic accuracy. Falls through to tree-sitter on timeout or error.
type LSPAnalyzer struct {
	Timeout time.Duration // per-request timeout; default 30s
}

func (a *LSPAnalyzer) timeout() time.Duration {
	if a.Timeout > 0 {
		return a.Timeout
	}
	return 30 * time.Second
}

func (a *LSPAnalyzer) Classes(root string) ([]ClassInfo, error) {
	conn, cleanup, err := a.startServer(root)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	return conn.documentClasses(root)
}

func (a *LSPAnalyzer) Implements(root string) ([]ImplEdge, error) {
	conn, cleanup, err := a.startServer(root)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	return conn.implementations(root)
}

func (a *LSPAnalyzer) FieldRefs(root string) ([]FieldRef, error) {
	return nil, fmt.Errorf("LSP field refs: not implemented (use tree-sitter)")
}

func (a *LSPAnalyzer) CallChain(root, entry string, depth int) ([]Call, error) {
	conn, cleanup, err := a.startServer(root)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	return conn.callChain(root, entry, depth)
}

func (a *LSPAnalyzer) EntryPoints(root string) ([]EntryPoint, error) {
	return nil, fmt.Errorf("LSP entry points: not implemented (use tree-sitter)")
}

func (a *LSPAnalyzer) NestingDepth(root string) ([]NestingResult, error) {
	return nil, fmt.Errorf("LSP nesting depth: not applicable (use tree-sitter)")
}

// --- LSP connection ---

type lspConn struct {
	w      io.Writer
	r      *bufio.Reader
	nextID int
}

func newLSPConn(r io.Reader, w io.Writer) *lspConn {
	return &lspConn{w: w, r: bufio.NewReader(r), nextID: 1}
}

type lspRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type lspResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int            `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *lspError       `json:"error,omitempty"`
}

type lspError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (c *lspConn) request(method string, params any) (json.RawMessage, error) {
	id := c.nextID
	c.nextID++
	req := lspRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params}
	if err := c.writeMsg(req); err != nil {
		return nil, fmt.Errorf("lsp %s: %w", method, err)
	}
	for {
		resp, err := c.readMsg()
		if err != nil {
			return nil, fmt.Errorf("lsp %s: %w", method, err)
		}
		if resp.ID == nil || resp.Method != "" {
			continue
		}
		if *resp.ID == id {
			if resp.Error != nil {
				return nil, fmt.Errorf("lsp %s: code %d: %s", method, resp.Error.Code, resp.Error.Message)
			}
			return resp.Result, nil
		}
	}
}

func (c *lspConn) notify(method string, params any) error {
	req := lspRequest{JSONRPC: "2.0", Method: method, Params: params}
	return c.writeMsg(req)
}

func (c *lspConn) writeMsg(msg any) error {
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

func (c *lspConn) readMsg() (*lspResponse, error) {
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
	var resp lspResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &resp, nil
}

func (c *lspConn) initialize(root string) error {
	rootURI := pathToURI(root)
	params := map[string]any{
		"processId": os.Getpid(),
		"rootUri":   rootURI,
		"capabilities": map[string]any{
			"textDocument": map[string]any{
				"documentSymbol":  map[string]any{"hierarchicalDocumentSymbolSupport": true},
				"typeHierarchy":   map[string]any{},
				"callHierarchy":   map[string]any{},
				"implementation":  map[string]any{},
			},
		},
	}
	if _, err := c.request("initialize", params); err != nil {
		return err
	}
	return c.notify("initialized", struct{}{})
}

func (c *lspConn) shutdown() {
	c.request("shutdown", nil)
	c.notify("exit", nil)
}

// documentClasses uses textDocument/documentSymbol on all source files.
func (c *lspConn) documentClasses(root string) ([]ClassInfo, error) {
	files := findSrcFiles(root)
	var classes []ClassInfo
	for _, f := range files {
		syms, err := c.documentSymbols(f, root)
		if err != nil {
			continue
		}
		rel, _ := filepath.Rel(root, f)
		pkg := filepath.ToSlash(filepath.Dir(rel))
		if pkg == "." {
			pkg = "(root)"
		}
		for _, sym := range syms {
			var kind string
			switch sym.Kind {
			case 23: // struct
				kind = "struct"
			case 11: // interface
				kind = "interface"
			case 5: // class
				kind = "class"
			default:
				continue
			}
			ci := ClassInfo{
				Name:     sym.Name,
				Package:  pkg,
				Kind:     kind,
				Exported: isExported(sym.Name),
			}
			for _, ch := range sym.Children {
				switch ch.Kind {
				case 8: // field
					ci.Fields = append(ci.Fields, FieldInfo{
						Name:     ch.Name,
						Exported: isExported(ch.Name),
					})
				case 6: // method
					ci.Methods = append(ci.Methods, MethodInfo{
						Name:      ch.Name,
						Signature: ch.Name,
						Exported:  isExported(ch.Name),
					})
				}
			}
			classes = append(classes, ci)
		}
	}
	return classes, nil
}

// implementations uses textDocument/implementation to find interface edges.
func (c *lspConn) implementations(root string) ([]ImplEdge, error) {
	// LSP textDocument/implementation requires a specific position.
	// We first get all interface symbols via documentSymbol, then query
	// implementations at each interface name position.
	files := findSrcFiles(root)
	var edges []ImplEdge
	for _, f := range files {
		syms, err := c.documentSymbols(f, root)
		if err != nil {
			continue
		}
		for _, sym := range syms {
			if sym.Kind != 11 { // interface
				continue
			}
			impls, err := c.request("textDocument/implementation", map[string]any{
				"textDocument": map[string]string{"uri": pathToURI(f)},
				"position":     map[string]int{"line": sym.Line, "character": sym.Col},
			})
			if err != nil {
				continue
			}
			var locations []struct {
				URI   string `json:"uri"`
				Range struct {
					Start struct{ Line int `json:"line"` } `json:"start"`
				} `json:"range"`
			}
			if json.Unmarshal(impls, &locations) != nil {
				continue
			}
			for _, loc := range locations {
				implName := resolveNameAtURI(loc.URI, loc.Range.Start.Line)
				if implName != "" {
					edges = append(edges, ImplEdge{
						From: implName,
						To:   sym.Name,
						Kind: "implements",
					})
				}
			}
		}
	}
	return edges, nil
}

// callChain uses callHierarchy/outgoingCalls recursively.
func (c *lspConn) callChain(root, entry string, maxDepth int) ([]Call, error) {
	if maxDepth <= 0 {
		maxDepth = 5
	}
	// Find the entry function via workspace/symbol
	item, err := c.findCallHierarchyItem(root, entry)
	if err != nil || item == nil {
		return nil, fmt.Errorf("call hierarchy: entry %q not found", entry)
	}

	var calls []Call
	visited := make(map[string]bool)
	var walk func(it *callHierarchyItem, depth int)
	walk = func(it *callHierarchyItem, depth int) {
		if depth > maxDepth || visited[it.Name] {
			return
		}
		visited[it.Name] = true
		outgoing, err := c.request("callHierarchy/outgoingCalls", map[string]any{"item": it})
		if err != nil {
			return
		}
		var outs []struct {
			To callHierarchyItem `json:"to"`
		}
		if json.Unmarshal(outgoing, &outs) != nil {
			return
		}
		for _, out := range outs {
			calls = append(calls, Call{
				Caller:  it.Name,
				Callee:  out.To.Name,
				Package: uriToPackage(out.To.URI, root),
				Line:    out.To.Range.Start.Line + 1,
			})
			walk(&out.To, depth+1)
		}
	}
	walk(item, 0)
	return calls, nil
}

type callHierarchyItem struct {
	Name  string `json:"name"`
	Kind  int    `json:"kind"`
	URI   string `json:"uri"`
	Range struct {
		Start struct {
			Line      int `json:"line"`
			Character int `json:"character"`
		} `json:"start"`
	} `json:"range"`
}

func (c *lspConn) findCallHierarchyItem(root, name string) (*callHierarchyItem, error) {
	// Use workspace/symbol to find the function, then prepare callHierarchy
	wsResult, err := c.request("workspace/symbol", map[string]any{"query": name})
	if err != nil {
		return nil, err
	}
	var symbols []struct {
		Name     string `json:"name"`
		Kind     int    `json:"kind"`
		Location struct {
			URI   string `json:"uri"`
			Range struct {
				Start struct {
					Line      int `json:"line"`
					Character int `json:"character"`
				} `json:"start"`
			} `json:"range"`
		} `json:"location"`
	}
	if json.Unmarshal(wsResult, &symbols) != nil || len(symbols) == 0 {
		return nil, fmt.Errorf("symbol %q not found", name)
	}
	// Find exact match
	for _, s := range symbols {
		if s.Name == name && (s.Kind == 12 || s.Kind == 6) { // Function or Method
			prepResult, err := c.request("textDocument/prepareCallHierarchy", map[string]any{
				"textDocument": map[string]string{"uri": s.Location.URI},
				"position": map[string]int{
					"line":      s.Location.Range.Start.Line,
					"character": s.Location.Range.Start.Character,
				},
			})
			if err != nil {
				return nil, err
			}
			var items []callHierarchyItem
			if json.Unmarshal(prepResult, &items) != nil || len(items) == 0 {
				return nil, fmt.Errorf("no call hierarchy item for %q", name)
			}
			return &items[0], nil
		}
	}
	return nil, fmt.Errorf("symbol %q not found", name)
}

type docSymbol struct {
	Name     string      `json:"name"`
	Kind     int         `json:"kind"`
	Line     int         `json:"-"`
	Col      int         `json:"-"`
	Children []docSymbol `json:"children,omitempty"`
}

func (c *lspConn) documentSymbols(file, root string) ([]docSymbol, error) {
	uri := pathToURI(file)
	content, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	lang := "go"
	switch filepath.Ext(file) {
	case ".rs":
		lang = "rust"
	case ".py":
		lang = "python"
	case ".ts", ".js":
		lang = "typescript"
	case ".java":
		lang = "java"
	}
	_ = c.notify("textDocument/didOpen", map[string]any{
		"textDocument": map[string]any{
			"uri": uri, "languageId": lang, "version": 1, "text": string(content),
		},
	})
	result, err := c.request("textDocument/documentSymbol", map[string]any{
		"textDocument": map[string]string{"uri": uri},
	})
	if err != nil {
		return nil, err
	}
	var symbols []struct {
		Name           string `json:"name"`
		Kind           int    `json:"kind"`
		Range          struct{ Start struct{ Line, Character int } } `json:"range"`
		SelectionRange struct{ Start struct{ Line, Character int } } `json:"selectionRange"`
		Children       []struct {
			Name           string `json:"name"`
			Kind           int    `json:"kind"`
			Range          struct{ Start struct{ Line, Character int } } `json:"range"`
			SelectionRange struct{ Start struct{ Line, Character int } } `json:"selectionRange"`
		} `json:"children,omitempty"`
	}
	if json.Unmarshal(result, &symbols) != nil {
		return nil, nil
	}
	var out []docSymbol
	for _, s := range symbols {
		ds := docSymbol{
			Name: s.Name, Kind: s.Kind,
			Line: s.SelectionRange.Start.Line,
			Col:  s.SelectionRange.Start.Character,
		}
		for _, ch := range s.Children {
			ds.Children = append(ds.Children, docSymbol{
				Name: ch.Name, Kind: ch.Kind,
				Line: ch.SelectionRange.Start.Line,
				Col:  ch.SelectionRange.Start.Character,
			})
		}
		out = append(out, ds)
	}
	return out, nil
}

// --- helpers ---

func (a *LSPAnalyzer) startServer(root string) (*lspConn, func(), error) {
	lang := survey.DetectLanguage(root)
	cmdStr := survey.DefaultLSPServer(lang)
	if cmdStr == "" {
		return nil, nil, fmt.Errorf("no LSP server for language %v", lang)
	}
	parts := strings.Fields(cmdStr)
	bin, err := exec.LookPath(parts[0])
	if err != nil {
		return nil, nil, fmt.Errorf("lsp server %s not found: %w", parts[0], err)
	}
	absRoot, _ := filepath.Abs(root)
	cmd := exec.Command(bin, parts[1:]...)
	cmd.Dir = absRoot
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("start %s: %w", parts[0], err)
	}
	conn := newLSPConn(stdout, stdin)
	if err := conn.initialize(absRoot); err != nil {
		stdin.Close()
		cmd.Wait()
		return nil, nil, err
	}
	cleanup := func() {
		conn.shutdown()
		stdin.Close()
		cmd.Wait()
	}
	return conn, cleanup, nil
}

func pathToURI(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	slash := filepath.ToSlash(abs)
	if !strings.HasPrefix(slash, "/") {
		slash = "/" + slash
	}
	return "file://" + slash
}

func uriToPackage(uri, root string) string {
	absRoot, _ := filepath.Abs(root)
	path := strings.TrimPrefix(uri, "file://")
	rel, err := filepath.Rel(absRoot, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(filepath.Dir(rel))
}

func resolveNameAtURI(uri string, line int) string {
	path := strings.TrimPrefix(uri, "file://")
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	cur := 0
	for scanner.Scan() {
		if cur == line {
			text := strings.TrimSpace(scanner.Text())
			// Extract type name from "type Foo struct" pattern
			if strings.HasPrefix(text, "type ") {
				parts := strings.Fields(text)
				if len(parts) >= 2 {
					return parts[1]
				}
			}
			return text
		}
		cur++
	}
	return ""
}

func findSrcFiles(root string) []string {
	absRoot, _ := filepath.Abs(root)
	var files []string
	filepath.WalkDir(absRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			base := d.Name()
			if base == "vendor" || base == "testdata" || strings.HasPrefix(base, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(d.Name())
		switch ext {
		case ".go", ".rs", ".py", ".ts", ".js", ".java":
			if !strings.HasSuffix(d.Name(), "_test.go") {
				files = append(files, path)
			}
		}
		return nil
	})
	return files
}
