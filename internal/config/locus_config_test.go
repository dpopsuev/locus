package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dpopsuev/locus/internal/config"
)

func TestLoadLocusConfig_Missing(t *testing.T) {
	dir := t.TempDir()
	ds, err := config.LoadLocusConfig(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ds != nil {
		t.Fatal("expected nil DesiredState when file is missing")
	}
}

func TestLoadLocusConfig_ValidYAML(t *testing.T) {
	dir := t.TempDir()
	content := `
layers:
  - domain
  - application
  - infra
roles:
  internal/driver: port
constraints:
  - component: internal/arch
    max_fan_in: 10
boundaries:
  - from_pattern: "infra/*"
    to_pattern: "domain/*"
    allow: false
accepted:
  - component: internal/old
    principle: SRP
    reason: legacy
`
	if err := os.WriteFile(filepath.Join(dir, ".locus.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	ds, err := config.LoadLocusConfig(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ds == nil {
		t.Fatal("expected non-nil DesiredState")
	}
	if len(ds.Layers) != 3 {
		t.Errorf("expected 3 layers, got %d", len(ds.Layers))
	}
	if ds.Layers[0] != "domain" {
		t.Errorf("expected first layer 'domain', got %q", ds.Layers[0])
	}
	if len(ds.Constraints) != 1 {
		t.Errorf("expected 1 constraint, got %d", len(ds.Constraints))
	}
	if ds.Constraints[0].MaxFanIn != 10 {
		t.Errorf("expected MaxFanIn=10, got %d", ds.Constraints[0].MaxFanIn)
	}
	if len(ds.Boundaries) != 1 {
		t.Errorf("expected 1 boundary, got %d", len(ds.Boundaries))
	}
	if ds.Roles["internal/driver"] != "port" {
		t.Errorf("expected role 'port', got %q", ds.Roles["internal/driver"])
	}
	if len(ds.Accepted) != 1 {
		t.Errorf("expected 1 accepted, got %d", len(ds.Accepted))
	}
}

func TestLoadLocusConfig_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".locus.yaml"), []byte(":::invalid"), 0o644); err != nil {
		t.Fatal(err)
	}

	ds, err := config.LoadLocusConfig(dir)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
	if ds != nil {
		t.Fatal("expected nil DesiredState on error")
	}
}

func TestLoadLocusConfig_ShellInjection_Layer(t *testing.T) {
	dir := t.TempDir()
	content := `
layers:
  - "domain; rm -rf /"
`
	if err := os.WriteFile(filepath.Join(dir, ".locus.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := config.LoadLocusConfig(dir)
	if err == nil {
		t.Fatal("expected error for shell metachar in layer name")
	}
}

func TestLoadLocusConfig_ShellInjection_BoundaryPattern(t *testing.T) {
	dir := t.TempDir()
	content := `
boundaries:
  - from_pattern: "$(whoami)/*"
    to_pattern: "domain/*"
    allow: false
`
	if err := os.WriteFile(filepath.Join(dir, ".locus.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := config.LoadLocusConfig(dir)
	if err == nil {
		t.Fatal("expected error for shell metachar in boundary from_pattern")
	}
}

func TestLoadLocusConfig_ShellInjection_Backtick(t *testing.T) {
	dir := t.TempDir()
	content := "layers:\n  - \"`whoami`\"\n"
	if err := os.WriteFile(filepath.Join(dir, ".locus.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := config.LoadLocusConfig(dir)
	if err == nil {
		t.Fatal("expected error for backtick in layer name")
	}
}

func TestLoadLocusConfig_ShellInjection_Pipe(t *testing.T) {
	dir := t.TempDir()
	content := `
constraints:
  - component: "internal/arch | cat /etc/passwd"
    max_fan_in: 10
`
	if err := os.WriteFile(filepath.Join(dir, ".locus.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := config.LoadLocusConfig(dir)
	if err == nil {
		t.Fatal("expected error for pipe in constraint component")
	}
}

func TestLoadLocusConfig_ShellInjection_RoleKey(t *testing.T) {
	dir := t.TempDir()
	content := `
roles:
  "internal/driver&&evil": port
`
	if err := os.WriteFile(filepath.Join(dir, ".locus.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := config.LoadLocusConfig(dir)
	if err == nil {
		t.Fatal("expected error for && in role key")
	}
}

func TestLoadLocusConfig_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".locus.yaml"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	ds, err := config.LoadLocusConfig(dir)
	if err != nil {
		t.Fatalf("unexpected error for empty file: %v", err)
	}
	// Empty YAML unmarshals to zero-value struct, which is valid.
	if ds == nil {
		t.Fatal("expected non-nil DesiredState for empty file")
	}
}
