package analysis

import (
	"os/exec"
	"strings"

	"github.com/dpopsuev/locus/internal/oculus/lang"
	"github.com/dpopsuev/locus/internal/oculus/lsp"
)

// FallbackAnalyzer chains LSP -> tree-sitter -> regex. Each method tries
// the highest-fidelity analyzer first and falls through on error or empty results.
type FallbackAnalyzer struct {
	lsp   TypeAnalyzer
	ts    TypeAnalyzer
	regex TypeAnalyzer
}

// NewFallback creates a FallbackAnalyzer. It checks whether an LSP server
// is available for the detected language; if not, the LSP layer is skipped.
// If pool is non-nil, the LSP analyzer always uses the pool (pool handles
// availability). Pass nil for CLI/test mode.
func NewFallback(root string, pool lsp.Pool) *FallbackAnalyzer {
	f := &FallbackAnalyzer{
		ts:    &TreeSitterAnalyzer{},
		regex: &RegexAnalyzer{},
	}
	if pool != nil {
		f.lsp = &LSPAnalyzer{pool: pool}
	} else {
		detected := lang.DetectLanguage(root)
		cmd := lang.DefaultLSPServer(detected)
		if cmd != "" {
			bin := strings.Fields(cmd)[0]
			if _, err := exec.LookPath(bin); err == nil {
				f.lsp = &LSPAnalyzer{}
			}
		}
	}
	return f
}

func (f *FallbackAnalyzer) Classes(root string) ([]ClassInfo, error) {
	if f.lsp != nil {
		if r, err := f.lsp.Classes(root); err == nil && len(r) > 0 {
			return r, nil
		}
	}
	if r, err := f.ts.Classes(root); err == nil && len(r) > 0 {
		return r, nil
	}
	return f.regex.Classes(root)
}

func (f *FallbackAnalyzer) Implements(root string) ([]ImplEdge, error) {
	if f.lsp != nil {
		if r, err := f.lsp.Implements(root); err == nil && len(r) > 0 {
			return r, nil
		}
	}
	if r, err := f.ts.Implements(root); err == nil && len(r) > 0 {
		return r, nil
	}
	return f.regex.Implements(root)
}

func (f *FallbackAnalyzer) FieldRefs(root string) ([]FieldRef, error) {
	if r, err := f.ts.FieldRefs(root); err == nil && len(r) > 0 {
		return r, nil
	}
	return f.regex.FieldRefs(root)
}

func (f *FallbackAnalyzer) CallChain(root, entry string, depth int) ([]Call, error) {
	if f.lsp != nil {
		if r, err := f.lsp.CallChain(root, entry, depth); err == nil && len(r) > 0 {
			return r, nil
		}
	}
	if r, err := f.ts.CallChain(root, entry, depth); err == nil && len(r) > 0 {
		return r, nil
	}
	return f.regex.CallChain(root, entry, depth)
}

func (f *FallbackAnalyzer) EntryPoints(root string) ([]EntryPoint, error) {
	if r, err := f.ts.EntryPoints(root); err == nil && len(r) > 0 {
		return r, nil
	}
	return f.regex.EntryPoints(root)
}

func (f *FallbackAnalyzer) NestingDepth(root string) ([]NestingResult, error) {
	if r, err := f.ts.NestingDepth(root); err == nil && len(r) > 0 {
		return r, nil
	}
	return f.regex.NestingDepth(root)
}
