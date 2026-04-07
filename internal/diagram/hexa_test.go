package diagram

import (
	"errors"
	"strings"
	"testing"

	"github.com/dpopsuev/locus/internal/arch"
	"github.com/dpopsuev/locus/internal/diagram/core"
	"github.com/dpopsuev/locus/internal/diagram/metrics"
)

func hexaTheme() *core.ResolvedTheme {
	return core.DefaultTheme().Resolve(core.ThemeNatural)
}

func hexaTestInput() core.Input {
	return core.Input{
		Report: &arch.ContextReport{
			Architecture: arch.ArchModel{
				Services: []arch.ArchService{
					{Name: "domain/model"},
					{Name: "domain/entity"},
					{Name: "port/store"},
					{Name: "adapter/http"},
					{Name: "adapter/postgres"},
				},
				Edges: []arch.ArchEdge{
					{From: "adapter/http", To: "port/store"},
					{From: "port/store", To: "domain/model"},
					{From: "adapter/postgres", To: "port/store"},
				},
			},
		},
		HexaRoles: map[string]string{
			"domain/model":     "domain",
			"domain/entity":    "domain",
			"port/store":       "port",
			"adapter/http":     "adapter",
			"adapter/postgres": "adapter",
		},
		ResolvedTheme: hexaTheme(),
	}
}

func TestRenderHexa_BasicStructure(t *testing.T) {
	in := hexaTestInput()
	out, err := Render(in, core.Options{Type: "hexa"})
	if err != nil {
		t.Fatal(err)
	}

	assertContains(t, out, "graph TD")
	assertContains(t, out, "Domain Core")
	assertContains(t, out, "Ports")
	assertContains(t, out, "Adapters")
	assertContains(t, out, "domain_model")
	assertContains(t, out, "domain_entity")
	assertContains(t, out, "port_store")
	assertContains(t, out, "adapter_http")
	assertContains(t, out, "adapter_postgres")
	assertContains(t, out, "subgraph")
}

func TestRenderHexa_ViolationEdge(t *testing.T) {
	in := core.Input{
		Report: &arch.ContextReport{
			Architecture: arch.ArchModel{
				Services: []arch.ArchService{
					{Name: "domain/model"},
					{Name: "adapter/http"},
				},
				Edges: []arch.ArchEdge{
					{From: "domain/model", To: "adapter/http"},
				},
			},
		},
		HexaRoles: map[string]string{
			"domain/model": "domain",
			"adapter/http": "adapter",
		},
		ResolvedTheme: hexaTheme(),
	}

	out, err := Render(in, core.Options{Type: "hexa"})
	if err != nil {
		t.Fatal(err)
	}

	assertContains(t, out, "-.->")
	assertContains(t, out, "violation")
}

func TestRenderHexa_EmptyRoles(t *testing.T) {
	in := core.Input{
		Report: &arch.ContextReport{
			Architecture: arch.ArchModel{},
		},
		HexaRoles: nil,
	}

	_, err := Render(in, core.Options{Type: "hexa"})
	if err == nil {
		t.Fatal("expected error for nil HexaRoles")
	}
	if !errors.Is(err, core.ErrHexaRolesRequired) {
		t.Errorf("expected ErrHexaRolesRequired, got: %v", err)
	}
}

func TestRenderHexa_NormalEdge(t *testing.T) {
	in := hexaTestInput()
	out, err := Render(in, core.Options{Type: "hexa"})
	if err != nil {
		t.Fatal(err)
	}

	assertContains(t, out, "adapter_http --> port_store")
}

func TestRenderHexa_ScopeFilter(t *testing.T) {
	in := hexaTestInput()
	out, err := Render(in, core.Options{Type: "hexa", Scope: "adapter"})
	if err != nil {
		t.Fatal(err)
	}

	assertContains(t, out, "adapter_http")
	assertContains(t, out, "adapter_postgres")

	if strings.Contains(out, "domain_model") {
		t.Error("expected domain/model to be excluded by scope filter")
	}
	if strings.Contains(out, "port_store") {
		t.Error("expected port/store to be excluded by scope filter")
	}
}

func TestRenderHexa_PortToAdapterViolation(t *testing.T) {
	in := core.Input{
		Report: &arch.ContextReport{
			Architecture: arch.ArchModel{
				Services: []arch.ArchService{
					{Name: "port/repo"},
					{Name: "adapter/pg"},
				},
				Edges: []arch.ArchEdge{
					{From: "port/repo", To: "adapter/pg"},
				},
			},
		},
		HexaRoles: map[string]string{
			"port/repo":  "port",
			"adapter/pg": "adapter",
		},
		ResolvedTheme: hexaTheme(),
	}

	out, err := Render(in, core.Options{Type: "hexa"})
	if err != nil {
		t.Fatal(err)
	}

	assertContains(t, out, "-.->")
	assertContains(t, out, "violation")
}

func TestIsHexaViolation(t *testing.T) {
	tests := []struct {
		from, to string
		want     bool
	}{
		{"domain", "adapter", true},
		{"domain", "infra", true},
		{"domain", "app", true},
		{"domain", "port", false},
		{"port", "adapter", true},
		{"port", "infra", true},
		{"port", "domain", false},
		{"adapter", "port", false},
		{"adapter", "infra", false},
		{"adapter", "domain", false},
		{"app", "domain", false},
		{"app", "adapter", false},
		{"entrypoint", "app", false},
	}
	for _, tt := range tests {
		t.Run(tt.from+"->"+tt.to, func(t *testing.T) {
			got := metrics.IsHexaViolation(tt.from, tt.to)
			if got != tt.want {
				t.Errorf("IsHexaViolation(%q, %q) = %v, want %v", tt.from, tt.to, got, tt.want)
			}
		})
	}
}
