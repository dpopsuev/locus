package scribe_test

import (
	"testing"

	"github.com/dpopsuev/battery/translate"
	oculus "github.com/dpopsuev/oculus/v3"
	"github.com/dpopsuev/oculus/v3/model"
	bridge "github.com/dpopsuev/locus/bridges/scribe"
)

// --- TranslateScan (component-level) ---

func TestTranslateScan_Components(t *testing.T) {
	report := &oculus.ContextReport{}
	report.Architecture.Services = []oculus.ArchService{
		{Name: "auth", Package: "pkg/auth", LOC: 500, Churn: 12},
		{Name: "db", Package: "pkg/db", LOC: 300, Churn: 5},
	}
	report.Architecture.Edges = []oculus.ArchEdge{
		{From: "auth", To: "db", Weight: 3},
	}

	result := bridge.TranslateScan(report, "myapp")

	assertRecordCount(t, result.Records, 2)
	assertRecord(t, result.Records[0], "myapp/auth", "code.file", "auth")
	assertExtra(t, result.Records[0], "loc", 500)
	assertExtra(t, result.Records[0], "churn", 12)
	assertLabel(t, result.Records[0], "source:locus")
	assertLabel(t, result.Records[0], "project:myapp")
}

func TestTranslateScan_ComponentEdges(t *testing.T) {
	report := &oculus.ContextReport{}
	report.Architecture.Services = []oculus.ArchService{
		{Name: "auth", Package: "pkg/auth"},
		{Name: "db", Package: "pkg/db"},
	}
	report.Architecture.Edges = []oculus.ArchEdge{
		{From: "auth", To: "db"},
	}

	result := bridge.TranslateScan(report, "myapp")

	assertEdgeCount(t, result.Edges, 1)
	assertEdge(t, result.Edges[0], "myapp/auth", "depends_on", "myapp/db")
}

func TestTranslateScan_TrustZoneLabel(t *testing.T) {
	report := &oculus.ContextReport{}
	report.Architecture.Services = []oculus.ArchService{
		{Name: "api", Package: "pkg/api", TrustZone: "entrypoint"},
	}

	result := bridge.TranslateScan(report, "myapp")
	assertLabel(t, result.Records[0], "zone:entrypoint")
}

func TestTranslateScan_EmptyReport(t *testing.T) {
	report := &oculus.ContextReport{}
	result := bridge.TranslateScan(report, "empty")
	assertRecordCount(t, result.Records, 0)
	assertEdgeCount(t, result.Edges, 0)
}

// --- TranslateScanWithSymbols: nil SymbolGraph (fallback path) ---

func TestTranslateScanWithSymbols_NilSG_KindMapping(t *testing.T) {
	tests := []struct {
		name     string
		symKind  model.SymbolKind
		wantKind string
	}{
		{"interface", model.SymbolInterface, "code.interface"},
		{"struct", model.SymbolStruct, "code.struct"},
		{"class", model.SymbolClass, "code.struct"},
		{"function", model.SymbolFunction, "code.function"},
		{"method", model.SymbolMethod, "code.method"},
		{"constant", model.SymbolConstant, "code.function"},
		{"variable", model.SymbolVariable, "code.function"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := &oculus.ContextReport{}
			report.Architecture.Services = []oculus.ArchService{
				{Name: "pkg", Package: "pkg", Symbols: []model.Symbol{
					{Name: "Sym", Kind: tt.symKind, Exported: true},
				}},
			}
			result := bridge.TranslateScanWithSymbols(report, nil, "p")
			sym := findRecord(result.Records, "p/pkg:sym")
			if sym == nil {
				t.Fatal("missing symbol record")
			}
			if sym.Kind != tt.wantKind {
				t.Errorf("kind = %q; want %q", sym.Kind, tt.wantKind)
			}
		})
	}
}

func TestTranslateScanWithSymbols_NilSG_Visibility(t *testing.T) {
	report := &oculus.ContextReport{}
	report.Architecture.Services = []oculus.ArchService{
		{Name: "pkg", Package: "pkg", Symbols: []model.Symbol{
			{Name: "Public", Kind: model.SymbolFunction, Exported: true},
			{Name: "private", Kind: model.SymbolFunction, Exported: false},
		}},
	}
	result := bridge.TranslateScanWithSymbols(report, nil, "p")

	pub := findRecord(result.Records, "p/pkg:public")
	assertLabel(t, *pub, "visibility:public")

	priv := findRecord(result.Records, "p/pkg:private")
	assertLabel(t, *priv, "visibility:private")
}

func TestTranslateScanWithSymbols_NilSG_Signature(t *testing.T) {
	report := &oculus.ContextReport{}
	report.Architecture.Services = []oculus.ArchService{
		{Name: "pkg", Package: "pkg", Symbols: []model.Symbol{
			{Name: "Fn", Kind: model.SymbolFunction, Exported: true, Signature: "func Fn() error"},
			{Name: "NoSig", Kind: model.SymbolFunction, Exported: true},
		}},
	}
	result := bridge.TranslateScanWithSymbols(report, nil, "p")

	fn := findRecord(result.Records, "p/pkg:fn")
	assertSection(t, *fn, "signature", "func Fn() error")

	noSig := findRecord(result.Records, "p/pkg:nosig")
	assertNoSection(t, *noSig, "signature")
}

func TestTranslateScanWithSymbols_NilSG_ContainsEdges(t *testing.T) {
	report := &oculus.ContextReport{}
	report.Architecture.Services = []oculus.ArchService{
		{Name: "svc", Package: "svc", Symbols: []model.Symbol{
			{Name: "A", Kind: model.SymbolFunction, Exported: true},
			{Name: "B", Kind: model.SymbolFunction, Exported: true},
		}},
	}
	result := bridge.TranslateScanWithSymbols(report, nil, "p")

	contains := filterEdgesByRelation(result.Edges, "contains")
	assertEdgeCount(t, contains, 2)
	assertEdge(t, contains[0], "p/svc", "contains", "p/svc:a")
	assertEdge(t, contains[1], "p/svc", "contains", "p/svc:b")
}

// --- TranslateScanWithSymbols: full SymbolGraph ---

func TestTranslateScanWithSymbols_SG_KindMapping(t *testing.T) {
	tests := []struct {
		name     string
		sgKind   string
		wantKind string
	}{
		{"interface", "interface", "code.interface"},
		{"struct", "struct", "code.struct"},
		{"class", "class", "code.struct"},
		{"function", "function", "code.function"},
		{"method", "method", "code.method"},
		{"unknown_kind", "widget", "code.function"},
		{"empty_kind", "", "code.function"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := &oculus.ContextReport{}
			sg := &oculus.SymbolGraph{
				Nodes: []oculus.Symbol{
					{Name: "Sym", Package: "pkg", Kind: tt.sgKind, Exported: true},
				},
			}
			result := bridge.TranslateScanWithSymbols(report, sg, "p")
			symbols := filterSymbols(result.Records)
			if len(symbols) != 1 {
				t.Fatalf("symbols = %d; want 1", len(symbols))
			}
			if symbols[0].Kind != tt.wantKind {
				t.Errorf("kind = %q; want %q", symbols[0].Kind, tt.wantKind)
			}
		})
	}
}

func TestTranslateScanWithSymbols_SG_EdgeKindMapping(t *testing.T) {
	tests := []struct {
		name    string
		sgKind  string
		wantRel string
	}{
		{"call", "call", "calls"},
		{"implements", "implements", "implements"},
		{"extends", "extends", "implements"},
		{"embeds", "embeds", "embeds"},
		{"field_ref", "field_ref", "field_ref"},
		{"goroutine", "goroutine", "calls"},
		{"channel_send", "channel_send", "calls"},
		{"channel_recv", "channel_recv", "calls"},
		{"await_call", "await_call", "calls"},
		{"promise_chain", "promise_chain", "calls"},
		{"task_spawn", "task_spawn", "calls"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := &oculus.ContextReport{}
			sg := &oculus.SymbolGraph{
				Nodes: []oculus.Symbol{
					{Name: "A", Package: "pkg", Kind: "function"},
					{Name: "B", Package: "pkg", Kind: "function"},
				},
				Edges: []oculus.SymbolEdge{
					{SourceFQN: "pkg.A", TargetFQN: "pkg.B", Kind: tt.sgKind},
				},
			}
			result := bridge.TranslateScanWithSymbols(report, sg, "p")
			edges := filterEdgesByRelation(result.Edges, tt.wantRel)
			if len(edges) != 1 {
				t.Errorf("%s edges = %d; want 1 (all edges: %v)", tt.wantRel, len(edges), result.Edges)
			}
		})
	}
}

func TestTranslateScanWithSymbols_SG_UnknownEdgeKindDropped(t *testing.T) {
	report := &oculus.ContextReport{}
	sg := &oculus.SymbolGraph{
		Nodes: []oculus.Symbol{
			{Name: "A", Package: "pkg", Kind: "function"},
			{Name: "B", Package: "pkg", Kind: "function"},
		},
		Edges: []oculus.SymbolEdge{
			{SourceFQN: "pkg.A", TargetFQN: "pkg.B", Kind: "unknown_kind"},
		},
	}
	result := bridge.TranslateScanWithSymbols(report, sg, "p")

	nonContains := filterNotRelation(result.Edges, "contains")
	assertEdgeCount(t, nonContains, 0)
}

func TestTranslateScanWithSymbols_SG_PrivateSymbols(t *testing.T) {
	report := &oculus.ContextReport{}
	sg := &oculus.SymbolGraph{
		Nodes: []oculus.Symbol{
			{Name: "Public", Package: "pkg", Kind: "function", Exported: true},
			{Name: "private", Package: "pkg", Kind: "function", Exported: false},
		},
	}
	result := bridge.TranslateScanWithSymbols(report, sg, "p")

	symbols := filterSymbols(result.Records)
	if len(symbols) != 2 {
		t.Fatalf("symbols = %d; want 2 (private should be included)", len(symbols))
	}

	priv := findRecordByTitle(symbols, "private")
	if priv == nil {
		t.Fatal("private symbol missing from SymbolGraph output")
	}
	assertLabel(t, *priv, "visibility:private")
	if priv.Extra["exported"] != false {
		t.Error("private symbol should have exported=false")
	}
}

func TestTranslateScanWithSymbols_SG_ParamReturnTypes(t *testing.T) {
	report := &oculus.ContextReport{}
	sg := &oculus.SymbolGraph{
		Nodes: []oculus.Symbol{
			{
				Name: "Process", Package: "pkg", Kind: "function",
				ParamTypes:  []string{"context.Context", "string"},
				ReturnTypes: []string{"*Result", "error"},
				Signature:   "func Process(ctx context.Context, id string) (*Result, error)",
			},
			{
				Name: "NoTypes", Package: "pkg", Kind: "function",
			},
		},
	}
	result := bridge.TranslateScanWithSymbols(report, sg, "p")

	proc := findRecordByTitle(result.Records, "Process")
	assertSection(t, *proc, "param_types", "context.Context, string")
	assertSection(t, *proc, "return_types", "*Result, error")
	assertSection(t, *proc, "signature", "func Process(ctx context.Context, id string) (*Result, error)")

	noTypes := findRecordByTitle(result.Records, "NoTypes")
	assertNoSection(t, *noTypes, "param_types")
	assertNoSection(t, *noTypes, "return_types")
}

func TestTranslateScanWithSymbols_SG_FileLocation(t *testing.T) {
	report := &oculus.ContextReport{}
	sg := &oculus.SymbolGraph{
		Nodes: []oculus.Symbol{
			{Name: "Fn", Package: "pkg", Kind: "function", File: "pkg/handler.go", Line: 42, EndLine: 50},
			{Name: "NoFile", Package: "pkg", Kind: "function"},
		},
	}
	result := bridge.TranslateScanWithSymbols(report, sg, "p")

	fn := findRecordByTitle(result.Records, "Fn")
	assertExtra(t, *fn, "file", "pkg/handler.go")
	assertExtra(t, *fn, "line", 42)
	assertExtra(t, *fn, "end_line", 50)

	noFile := findRecordByTitle(result.Records, "NoFile")
	if _, ok := noFile.Extra["file"]; ok {
		t.Error("NoFile should not have file extra")
	}
}

func TestTranslateScanWithSymbols_SG_ReceiverType(t *testing.T) {
	report := &oculus.ContextReport{}
	sg := &oculus.SymbolGraph{
		Nodes: []oculus.Symbol{
			{Name: "Repo.Get", Package: "pkg", Kind: "method", ReceiverType: "*Repo"},
			{Name: "standalone", Package: "pkg", Kind: "function"},
		},
	}
	result := bridge.TranslateScanWithSymbols(report, sg, "p")

	method := findRecordByTitle(result.Records, "Repo.Get")
	assertExtra(t, *method, "receiver_type", "*Repo")

	fn := findRecordByTitle(result.Records, "standalone")
	if _, ok := fn.Extra["receiver_type"]; ok {
		t.Error("standalone function should not have receiver_type")
	}
}

func TestTranslateScanWithSymbols_SG_ComponentContainsEdges(t *testing.T) {
	report := &oculus.ContextReport{}
	report.Architecture.Services = []oculus.ArchService{
		{Name: "svc", Package: "pkg/svc"},
	}
	sg := &oculus.SymbolGraph{
		Nodes: []oculus.Symbol{
			{Name: "A", Package: "pkg/svc", Kind: "function"},
			{Name: "B", Package: "pkg/svc", Kind: "function"},
			{Name: "C", Package: "pkg/other", Kind: "function"},
		},
	}
	result := bridge.TranslateScanWithSymbols(report, sg, "p")

	contains := filterEdgesByRelation(result.Edges, "contains")
	if len(contains) != 2 {
		t.Errorf("contains edges = %d; want 2 (only A and B match svc package)", len(contains))
	}
}

func TestTranslateScanWithSymbols_SG_EmptySymbolGraph(t *testing.T) {
	report := &oculus.ContextReport{}
	report.Architecture.Services = []oculus.ArchService{
		{Name: "svc", Package: "pkg"},
	}
	sg := &oculus.SymbolGraph{}

	result := bridge.TranslateScanWithSymbols(report, sg, "p")

	components := filterByKind(result.Records, "code.file")
	symbols := filterSymbols(result.Records)
	assertRecordCount(t, components, 1)
	assertRecordCount(t, symbols, 0)
}

func TestTranslateScanWithSymbols_SG_FullHexagonalPattern(t *testing.T) {
	report := &oculus.ContextReport{}
	report.Architecture.Services = []oculus.ArchService{
		{Name: "domain", Package: "domain"},
		{Name: "adapter", Package: "adapter"},
	}

	sg := &oculus.SymbolGraph{
		Nodes: []oculus.Symbol{
			{Name: "Repository", Package: "domain", Kind: "interface", Exported: true},
			{Name: "Service", Package: "domain", Kind: "struct", Exported: true},
			{Name: "Service.Create", Package: "domain", Kind: "method", Exported: true},
			{Name: "PostgresRepo", Package: "adapter", Kind: "struct", Exported: true},
			{Name: "PostgresRepo.Save", Package: "adapter", Kind: "method", Exported: true},
		},
		Edges: []oculus.SymbolEdge{
			{SourceFQN: "adapter.PostgresRepo", TargetFQN: "domain.Repository", Kind: "implements"},
			{SourceFQN: "domain.Service", TargetFQN: "domain.Repository", Kind: "field_ref"},
			{SourceFQN: "domain.Service.Create", TargetFQN: "adapter.PostgresRepo.Save", Kind: "call"},
		},
	}

	result := bridge.TranslateScanWithSymbols(report, sg, "app")

	symbols := filterSymbols(result.Records)
	if len(symbols) != 5 {
		t.Fatalf("symbols = %d; want 5", len(symbols))
	}

	assertRecordKind(t, symbols, "Repository", "code.interface")
	assertRecordKind(t, symbols, "Service", "code.struct")
	assertRecordKind(t, symbols, "Service.Create", "code.method")
	assertRecordKind(t, symbols, "PostgresRepo", "code.struct")
	assertRecordKind(t, symbols, "PostgresRepo.Save", "code.method")

	implEdges := filterEdgesByRelation(result.Edges, "implements")
	if len(implEdges) != 1 {
		t.Fatalf("implements edges = %d; want 1", len(implEdges))
	}

	fieldRefEdges := filterEdgesByRelation(result.Edges, "field_ref")
	if len(fieldRefEdges) != 1 {
		t.Fatalf("field_ref edges = %d; want 1", len(fieldRefEdges))
	}

	callEdges := filterEdgesByRelation(result.Edges, "calls")
	if len(callEdges) != 1 {
		t.Fatalf("calls edges = %d; want 1", len(callEdges))
	}

	domainContains := filterEdgesFrom(filterEdgesByRelation(result.Edges, "contains"), "app/domain")
	adapterContains := filterEdgesFrom(filterEdgesByRelation(result.Edges, "contains"), "app/adapter")
	if len(domainContains) != 3 {
		t.Errorf("domain contains = %d; want 3 (Repository, Service, Service.Create)", len(domainContains))
	}
	if len(adapterContains) != 2 {
		t.Errorf("adapter contains = %d; want 2 (PostgresRepo, PostgresRepo.Save)", len(adapterContains))
	}
}

// --- assertion helpers ---

func assertRecordCount(t *testing.T, records []translate.Record, want int) {
	t.Helper()
	if len(records) != want {
		t.Fatalf("records = %d; want %d", len(records), want)
	}
}

func assertEdgeCount(t *testing.T, edges []translate.Edge, want int) {
	t.Helper()
	if len(edges) != want {
		t.Fatalf("edges = %d; want %d", len(edges), want)
	}
}

func assertRecord(t *testing.T, r translate.Record, wantID, wantKind, wantTitle string) {
	t.Helper()
	if r.ID != wantID {
		t.Errorf("id = %q; want %q", r.ID, wantID)
	}
	if r.Kind != wantKind {
		t.Errorf("kind = %q; want %q", r.Kind, wantKind)
	}
	if r.Title != wantTitle {
		t.Errorf("title = %q; want %q", r.Title, wantTitle)
	}
}

func assertEdge(t *testing.T, e translate.Edge, wantFrom, wantRel, wantTo string) {
	t.Helper()
	if e.From != wantFrom {
		t.Errorf("from = %q; want %q", e.From, wantFrom)
	}
	if e.Relation != wantRel {
		t.Errorf("relation = %q; want %q", e.Relation, wantRel)
	}
	if e.To != wantTo {
		t.Errorf("to = %q; want %q", e.To, wantTo)
	}
}

func assertExtra(t *testing.T, r translate.Record, key string, want any) {
	t.Helper()
	got, ok := r.Extra[key]
	if !ok {
		t.Errorf("extra[%q] missing", key)
		return
	}
	if got != want {
		t.Errorf("extra[%q] = %v (%T); want %v (%T)", key, got, got, want, want)
	}
}

func assertLabel(t *testing.T, r translate.Record, want string) {
	t.Helper()
	for _, l := range r.Labels {
		if l == want {
			return
		}
	}
	t.Errorf("missing label %q in %v", want, r.Labels)
}

func assertSection(t *testing.T, r translate.Record, name, wantText string) {
	t.Helper()
	for _, s := range r.Sections {
		if s.Name == name {
			if s.Text != wantText {
				t.Errorf("section %q text = %q; want %q", name, s.Text, wantText)
			}
			return
		}
	}
	t.Errorf("missing section %q", name)
}

func assertNoSection(t *testing.T, r translate.Record, name string) {
	t.Helper()
	for _, s := range r.Sections {
		if s.Name == name {
			t.Errorf("unexpected section %q present", name)
			return
		}
	}
}

func assertRecordKind(t *testing.T, records []translate.Record, title, wantKind string) {
	t.Helper()
	r := findRecordByTitle(records, title)
	if r == nil {
		t.Errorf("missing record with title %q", title)
		return
	}
	if r.Kind != wantKind {
		t.Errorf("%s kind = %q; want %q", title, r.Kind, wantKind)
	}
}

// --- filter helpers ---

func filterByKind(records []translate.Record, kind string) []translate.Record {
	var out []translate.Record
	for _, r := range records {
		if r.Kind == kind {
			out = append(out, r)
		}
	}
	return out
}

func filterSymbols(records []translate.Record) []translate.Record {
	var out []translate.Record
	for _, r := range records {
		if r.Kind != "code.file" {
			out = append(out, r)
		}
	}
	return out
}

func findRecord(records []translate.Record, id string) *translate.Record {
	for i, r := range records {
		if r.ID == id {
			return &records[i]
		}
	}
	return nil
}

func findRecordByTitle(records []translate.Record, title string) *translate.Record {
	for i, r := range records {
		if r.Title == title {
			return &records[i]
		}
	}
	return nil
}

func filterEdgesByRelation(edges []translate.Edge, rel string) []translate.Edge {
	var out []translate.Edge
	for _, e := range edges {
		if e.Relation == rel {
			out = append(out, e)
		}
	}
	return out
}

func filterNotRelation(edges []translate.Edge, rel string) []translate.Edge {
	var out []translate.Edge
	for _, e := range edges {
		if e.Relation != rel {
			out = append(out, e)
		}
	}
	return out
}

func filterEdgesFrom(edges []translate.Edge, from string) []translate.Edge {
	var out []translate.Edge
	for _, e := range edges {
		if e.From == from {
			out = append(out, e)
		}
	}
	return out
}

func TestTranslateScanWithSymbols_TraitAndInherits(t *testing.T) {
	report := &oculus.ContextReport{}
	report.Architecture.Services = []oculus.ArchService{
		{Name: "app", Package: "app"},
	}

	sg := &oculus.SymbolGraph{
		Nodes: []oculus.Symbol{
			{Name: "Drawable", Package: "app", Kind: "trait", Exported: true},
			{Name: "Clickable", Package: "app", Kind: "trait", Exported: true},
			{Name: "Button", Package: "app", Kind: "struct", Exported: true},
		},
		Edges: []oculus.SymbolEdge{
			{SourceFQN: "app.Button", TargetFQN: "app.Drawable", Kind: "implements"},
			{SourceFQN: "app.Clickable", TargetFQN: "app.Drawable", Kind: "inherits"},
		},
	}

	result := bridge.TranslateScanWithSymbols(report, sg, "myapp")

	symbols := filterSymbols(result.Records)
	assertRecordKind(t, symbols, "Drawable", "code.interface")
	assertRecordKind(t, symbols, "Clickable", "code.interface")
	assertRecordKind(t, symbols, "Button", "code.struct")

	implEdges := filterEdgesByRelation(result.Edges, "implements")
	if len(implEdges) != 2 {
		t.Fatalf("implements edges = %d; want 2 (one implements + one inherits mapped to implements)", len(implEdges))
	}
}
