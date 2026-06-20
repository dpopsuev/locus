// Package testdata provides pre-built Locus scan fixtures for cross-service testing.
// Importable by Scribe and other consumers in test files only.
package testdata

import (
	oculus "github.com/dpopsuev/oculus/v3"
	"github.com/dpopsuev/oculus/v3/graph"
	"github.com/dpopsuev/oculus/v3/model"
)

// HexagonalProject returns a fixture with both a ContextReport and a
// SymbolGraph representing a minimal hexagonal architecture:
//
//	domain package: Repository (interface), Service (struct), Service.Create (method)
//	adapter package: PgRepo (struct, implements Repository)
//
// Edges: implements, field_ref, call, embeds.
func HexagonalProject() (*oculus.ContextReport, *oculus.SymbolGraph) {
	report := &oculus.ContextReport{}
	report.Architecture.Services = []oculus.ArchService{
		{Name: "domain", Package: "app/domain", Language: model.LangGo, LOC: 200},
		{Name: "adapter", Package: "app/adapter", Language: model.LangGo, LOC: 150},
	}
	report.Architecture.Edges = []oculus.ArchEdge{
		{From: "adapter", To: "domain", Weight: 3},
	}
	report.ImportDepth = graph.ImportDepth(report.Architecture.Edges)

	sg := &oculus.SymbolGraph{
		Nodes: []oculus.Symbol{
			{Name: "Repository", Package: "app/domain", Kind: "interface", Exported: true, File: "domain/repo.go", Line: 5},
			{Name: "Service", Package: "app/domain", Kind: "struct", Exported: true, File: "domain/svc.go", Line: 10},
			{Name: "Service.Create", Package: "app/domain", Kind: "method", Exported: true, File: "domain/svc.go", Line: 20,
				ParamTypes: []string{"context.Context", "string"}, ReturnTypes: []string{"*Entity", "error"},
				Signature: "func (s *Service) Create(ctx context.Context, name string) (*Entity, error)"},
			{Name: "PgRepo", Package: "app/adapter", Kind: "struct", Exported: true, File: "adapter/pg.go", Line: 8},
		},
		Edges: []oculus.SymbolEdge{
			{SourceFQN: "app/adapter.PgRepo", TargetFQN: "app/domain.Repository", Kind: "implements"},
			{SourceFQN: "app/domain.Service", TargetFQN: "app/domain.Repository", Kind: "field_ref"},
			{SourceFQN: "app/domain.Service.Create", TargetFQN: "app/adapter.PgRepo", Kind: "call"},
			{SourceFQN: "app/domain.Service", TargetFQN: "app/domain.Repository", Kind: "embeds"},
		},
	}

	return report, sg
}

// SmallProject returns a 3-component scan with 2 dependency edges.
// Represents a typical small Go project: api → service → db.
func SmallProject() *oculus.ContextReport {
	r := &oculus.ContextReport{}
	r.Architecture.Services = []oculus.ArchService{
		{Name: "api", Package: "internal/api", Language: model.LangGo, LOC: 250, Churn: 8, HasTests: true},
		{Name: "service", Package: "internal/service", Language: model.LangGo, LOC: 400, Churn: 15, HasTests: true},
		{Name: "db", Package: "internal/db", Language: model.LangGo, LOC: 180, Churn: 3, HasTests: false},
	}
	r.Architecture.Edges = []oculus.ArchEdge{
		{From: "api", To: "service", Weight: 5},
		{From: "service", To: "db", Weight: 3},
	}
	r.HotSpots = []oculus.HotSpot{
		{Component: "service", FanIn: 2, Churn: 15},
	}
	return r
}

// MonorepoProject returns a 6-component scan with cyclic dependencies.
// Represents a monorepo with tighter coupling and more complexity.
func MonorepoProject() *oculus.ContextReport {
	r := &oculus.ContextReport{}
	r.Architecture.Services = []oculus.ArchService{
		{Name: "auth", Package: "packages/auth", Language: model.LangTypeScript, LOC: 500, Churn: 22, HasTests: true},
		{Name: "users", Package: "packages/users", Language: model.LangTypeScript, LOC: 350, Churn: 10, HasTests: true},
		{Name: "billing", Package: "packages/billing", Language: model.LangTypeScript, LOC: 600, Churn: 18, HasTests: true},
		{Name: "notifications", Package: "packages/notifications", Language: model.LangTypeScript, LOC: 200, Churn: 5, HasTests: false},
		{Name: "shared", Package: "packages/shared", Language: model.LangTypeScript, LOC: 150, Churn: 2, HasTests: true},
		{Name: "config", Package: "packages/config", Language: model.LangTypeScript, LOC: 80, Churn: 1, HasTests: false},
	}
	r.Architecture.Edges = []oculus.ArchEdge{
		{From: "auth", To: "users", Weight: 4},
		{From: "auth", To: "shared", Weight: 2},
		{From: "users", To: "shared", Weight: 3},
		{From: "billing", To: "users", Weight: 5},
		{From: "billing", To: "notifications", Weight: 2},
		{From: "notifications", To: "shared", Weight: 1},
		{From: "notifications", To: "config", Weight: 1},
		{From: "shared", To: "config", Weight: 1},
	}
	r.HotSpots = []oculus.HotSpot{
		{Component: "auth", FanIn: 0, Churn: 22},
		{Component: "billing", FanIn: 0, Churn: 18},
		{Component: "shared", FanIn: 3, Churn: 2},
	}
	return r
}
