package diagram

import (
	"strings"
	"testing"
)

func TestClassifyHealth(t *testing.T) {
	tests := []struct {
		fanIn, churn int
		want         Health
	}{
		{0, 0, Healthy},
		{2, 7, Healthy},
		{3, 7, Healthy},
		{2, 8, Healthy},
		{3, 8, Sick},
		{4, 10, Sick},
		{5, 14, Sick},
		{4, 15, Sick},
		{5, 15, Fatal},
		{10, 20, Fatal},
	}
	for _, tt := range tests {
		got := ClassifyHealth(tt.fanIn, tt.churn)
		if got != tt.want {
			t.Errorf("ClassifyHealth(%d, %d) = %d, want %d", tt.fanIn, tt.churn, got, tt.want)
		}
	}
}

func TestResolve_AllModes(t *testing.T) {
	theme := DefaultTheme()
	for _, mode := range []string{"dark", "light", "natural"} {
		rt := theme.Resolve(mode)
		if rt.Mode != mode {
			t.Errorf("mode = %q, want %q", rt.Mode, mode)
		}
		for name, hex := range rt.ResolvedColors {
			if !strings.HasPrefix(hex, "#") {
				t.Errorf("color %q in mode %q = %q, want hex starting with #", name, mode, hex)
			}
		}
	}
}

func TestResolve_DefaultMode(t *testing.T) {
	theme := DefaultTheme()
	rt := theme.Resolve("")
	if rt.Mode != "natural" {
		t.Errorf("empty mode resolved to %q, want natural", rt.Mode)
	}
}

func TestClassDefs(t *testing.T) {
	theme := DefaultTheme()
	rt := theme.Resolve("dark")
	defs := rt.ClassDefs()

	for _, want := range []string{
		"classDef healthy",
		"classDef sick",
		"classDef fatal",
		"classDef component",
		"classDef entry",
		"fill:#68D391",
		"fill:#F6E05E",
		"fill:#FC8181",
	} {
		if !strings.Contains(defs, want) {
			t.Errorf("ClassDefs() missing %q", want)
		}
	}
}

func TestInitDirective(t *testing.T) {
	theme := DefaultTheme()
	rt := theme.Resolve("dark")
	dir := rt.InitDirective()

	if !strings.HasPrefix(dir, "%%{init:") {
		t.Errorf("InitDirective should start with %%%%{init:, got %q", dir[:20])
	}
	if !strings.Contains(dir, "'primaryColor': '#2D3748'") {
		t.Error("InitDirective missing dark surface color")
	}
}

func TestHealthClass(t *testing.T) {
	theme := DefaultTheme()
	rt := theme.Resolve("natural")
	if rt.HealthClass(Healthy) != "healthy" {
		t.Error("HealthClass(Healthy) != healthy")
	}
	if rt.HealthClass(Sick) != "sick" {
		t.Error("HealthClass(Sick) != sick")
	}
	if rt.HealthClass(Fatal) != "fatal" {
		t.Error("HealthClass(Fatal) != fatal")
	}
}

func TestNodeSuffix(t *testing.T) {
	theme := DefaultTheme()
	rt := theme.Resolve("natural")
	if rt.NodeSuffix(Fatal) != ":::fatal" {
		t.Errorf("NodeSuffix(Fatal) = %q, want :::fatal", rt.NodeSuffix(Fatal))
	}
}
