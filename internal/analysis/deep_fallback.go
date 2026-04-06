package analysis

import (
	"os"
	"os/exec"
	"strings"

	"github.com/dpopsuev/locus/internal/oculus/lang"
	"github.com/dpopsuev/locus/internal/oculus/lsp"
)

// DeepFallbackAnalyzer chains LSP -> TreeSitter -> Regex for DeepAnalyzer
// methods. Each method tries the highest-fidelity analyzer first and falls
// through on error or empty results. The Layer field on results indicates
// which analyzer produced the data.
type DeepFallbackAnalyzer struct {
	lsp   DeepAnalyzer
	goast DeepAnalyzer
	ts    DeepAnalyzer
	regex DeepAnalyzer
}

// NewDeepFallback creates a DeepFallbackAnalyzer. It checks whether
// gopls is available; if not, the LSP layer is skipped.
// If pool is non-nil, the LSP analyzer always uses the pool (pool handles
// availability). Pass nil for CLI/test mode.
func NewDeepFallback(root string, pool lsp.Pool) *DeepFallbackAnalyzer {
	f := &DeepFallbackAnalyzer{
		regex: &RegexDeepAnalyzer{},
	}
	// Language-specific AST analyzers — auto-detected.
	// LOCUS_CALLGRAPH_BACKEND=gotools uses x/tools/go/callgraph (CHA) for
	// higher precision at the cost of speed (requires full type-checking).
	if os.Getenv("LOCUS_CALLGRAPH_BACKEND") == "gotools" {
		if gt := NewGoToolsDeep(root); gt != nil {
			f.goast = gt
		}
	}
	if f.goast == nil {
		if ga := NewGoASTDeep(root); ga != nil {
			f.goast = ga
		} else if pa := NewPythonDeep(root); pa != nil {
			f.goast = pa
		} else if ta := NewTypeScriptDeep(root); ta != nil {
			f.goast = ta
		}
	}
	// Tree-sitter deep analyzer uses ParsedProject
	if ts, err := NewTreeSitterDeep(root); err == nil {
		f.ts = ts
	}
	// LSP deep analyzer
	if pool != nil {
		f.lsp = NewLSPDeepWithPool(root, pool)
	} else {
		detected := lang.DetectLanguage(root)
		cmd := lang.DefaultLSPServer(detected)
		if cmd != "" {
			bin := strings.Fields(cmd)[0]
			if _, err := exec.LookPath(bin); err == nil {
				f.lsp = NewLSPDeep(root)
			}
		}
	}
	return f
}

func (f *DeepFallbackAnalyzer) CallGraph(root string, opts CallGraphOpts) (*CallGraph, error) {
	if f.lsp != nil {
		if r, err := f.lsp.CallGraph(root, opts); err == nil && len(r.Edges) > 0 {
			return r, nil
		}
	}
	if f.goast != nil {
		if r, err := f.goast.CallGraph(root, opts); err == nil && len(r.Edges) > 0 {
			return r, nil
		}
	}
	if f.ts != nil {
		if r, err := f.ts.CallGraph(root, opts); err == nil && len(r.Edges) > 0 {
			return r, nil
		}
	}
	return f.regex.CallGraph(root, opts)
}

func (f *DeepFallbackAnalyzer) DataFlowTrace(root, entry string, depth int) (*DataFlow, error) {
	if f.lsp != nil {
		if r, err := f.lsp.DataFlowTrace(root, entry, depth); err == nil && len(r.Edges) > 0 {
			return r, nil
		}
	}
	if f.goast != nil {
		if r, err := f.goast.DataFlowTrace(root, entry, depth); err == nil && len(r.Edges) > 0 {
			return r, nil
		}
	}
	if f.ts != nil {
		if r, err := f.ts.DataFlowTrace(root, entry, depth); err == nil && len(r.Edges) > 0 {
			return r, nil
		}
	}
	return f.regex.DataFlowTrace(root, entry, depth)
}

func (f *DeepFallbackAnalyzer) DetectStateMachines(root string) ([]StateMachine, error) {
	if f.lsp != nil {
		if r, err := f.lsp.DetectStateMachines(root); err == nil && len(r) > 0 {
			return r, nil
		}
	}
	if f.goast != nil {
		if r, err := f.goast.DetectStateMachines(root); err == nil && len(r) > 0 {
			return r, nil
		}
	}
	if f.ts != nil {
		if r, err := f.ts.DetectStateMachines(root); err == nil && len(r) > 0 {
			return r, nil
		}
	}
	return f.regex.DetectStateMachines(root)
}
