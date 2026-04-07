package analysis_test

import (
	"testing"

	"github.com/dpopsuev/locus/internal/analysis"
)

func TestNewFallback_External(t *testing.T) {
	fa := analysis.NewFallback(t.TempDir(), nil)
	if fa == nil {
		t.Fatal("nil fallback analyzer")
	}
}
