package scribe_test

import (
	"testing"

	oculus "github.com/dpopsuev/oculus/v3"
	bridge "github.com/dpopsuev/locus/bridges/scribe"
)

const compAuth = "auth"

func TestTranslateScan_Components(t *testing.T) {
	report := &oculus.ContextReport{}
	report.Architecture.Services = []oculus.ArchService{
		{Name: compAuth, Package: "pkg/auth", LOC: 500, Churn: 12},
		{Name: "db", Package: "pkg/db", LOC: 300, Churn: 5},
	}
	report.Architecture.Edges = []oculus.ArchEdge{
		{From: compAuth, To: "db", Weight: 3},
	}

	result := bridge.TranslateScan(report, "myapp")

	if len(result.Records) != 2 {
		t.Fatalf("records = %d; want 2", len(result.Records))
	}

	auth := result.Records[0]
	if auth.ID != "myapp/auth" {
		t.Errorf("id = %q; want myapp/auth", auth.ID)
	}
	if auth.Kind != "knowledge.source" {
		t.Errorf("kind = %q; want knowledge.source", auth.Kind)
	}
	if auth.Extra["loc"] != 500 {
		t.Errorf("loc = %v; want 500", auth.Extra["loc"])
	}
	if auth.Extra["churn"] != 12 {
		t.Errorf("churn = %v; want 12", auth.Extra["churn"])
	}

	hasLabel := false
	for _, l := range auth.Labels {
		if l == "source:locus" {
			hasLabel = true
		}
	}
	if !hasLabel {
		t.Error("missing source:locus label")
	}
}

func TestTranslateScan_Edges(t *testing.T) {
	report := &oculus.ContextReport{}
	report.Architecture.Services = []oculus.ArchService{
		{Name: compAuth, Package: "pkg/auth"},
		{Name: "db", Package: "pkg/db"},
	}
	report.Architecture.Edges = []oculus.ArchEdge{
		{From: compAuth, To: "db"},
	}

	result := bridge.TranslateScan(report, "myapp")

	if len(result.Edges) != 1 {
		t.Fatalf("edges = %d; want 1", len(result.Edges))
	}

	edge := result.Edges[0]
	if edge.From != "myapp/auth" {
		t.Errorf("from = %q; want myapp/auth", edge.From)
	}
	if edge.To != "myapp/db" {
		t.Errorf("to = %q; want myapp/db", edge.To)
	}
	if edge.Relation != "depends_on" {
		t.Errorf("relation = %q; want depends_on", edge.Relation)
	}
}

func TestTranslateScan_EmptyReport(t *testing.T) {
	report := &oculus.ContextReport{}
	result := bridge.TranslateScan(report, "empty")

	if len(result.Records) != 0 {
		t.Errorf("records = %d; want 0", len(result.Records))
	}
	if len(result.Edges) != 0 {
		t.Errorf("edges = %d; want 0", len(result.Edges))
	}
}
