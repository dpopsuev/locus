package analysis

import (
	"os/exec"
	"strings"

	"github.com/dpopsuev/locus/internal/survey"
)

// DeepFallbackAnalyzer chains LSP -> TreeSitter -> Regex for DeepAnalyzer
// methods. Each method tries the highest-fidelity analyzer first and falls
// through on error or empty results. The Layer field on results indicates
// which analyzer produced the data.
type DeepFallbackAnalyzer struct {
	lsp   DeepAnalyzer
	ts    DeepAnalyzer
	regex DeepAnalyzer
}

// NewDeepFallback creates a DeepFallbackAnalyzer. It checks whether
// gopls is available; if not, the LSP layer is skipped.
func NewDeepFallback(root string) *DeepFallbackAnalyzer {
	f := &DeepFallbackAnalyzer{
		regex: &RegexDeepAnalyzer{},
	}
	// Tree-sitter deep analyzer uses ParsedProject
	if ts, err := NewTreeSitterDeep(root); err == nil {
		f.ts = ts
	}
	// LSP deep analyzer checks for gopls
	lang := survey.DetectLanguage(root)
	cmd := survey.DefaultLSPServer(lang)
	if cmd != "" {
		bin := strings.Fields(cmd)[0]
		if _, err := exec.LookPath(bin); err == nil {
			f.lsp = NewLSPDeep(root)
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
	if f.ts != nil {
		if r, err := f.ts.DetectStateMachines(root); err == nil && len(r) > 0 {
			return r, nil
		}
	}
	return f.regex.DetectStateMachines(root)
}
