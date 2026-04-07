package constraint_test

import (
	"testing"

	"github.com/dpopsuev/locus/internal/arch"
	"github.com/dpopsuev/locus/internal/constraint"
	"github.com/dpopsuev/locus/internal/graph"
)

func TestComputeImportDirection_External(t *testing.T) {
	edges := []arch.ArchEdge{{From: "a", To: "b", Weight: 1}}
	depths := graph.ImportDepth(edges)
	report := constraint.ComputeImportDirection(edges, depths)
	if report == nil {
		t.Fatal("nil report")
	}
}
