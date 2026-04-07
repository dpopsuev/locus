package lint_test

import (
	"testing"

	"github.com/dpopsuev/locus/internal/arch"
	"github.com/dpopsuev/locus/internal/lint"
)

func TestRun_External(t *testing.T) {
	report := &arch.ContextReport{Architecture: arch.ArchModel{}}
	result := lint.Run(report, lint.RunOpts{})
	if result == nil {
		t.Fatal("nil result")
	}
}
