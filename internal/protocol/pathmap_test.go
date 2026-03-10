package protocol

import (
	"reflect"
	"testing"
)

func TestNewPathMapper(t *testing.T) {
	pm := NewPathMapper("")
	if pm == nil || len(pm.mappings) != 0 {
		t.Errorf("empty spec should produce empty mapper, got %d mappings", len(pm.mappings))
	}

	pm = NewPathMapper("/home/user:/workspace")
	if len(pm.mappings) != 1 {
		t.Fatalf("expected 1 mapping, got %d", len(pm.mappings))
	}
	if pm.mappings[0].Host != "/home/user" || pm.mappings[0].Container != "/workspace" {
		t.Errorf("expected Host=/home/user Container=/workspace, got %q %q",
			pm.mappings[0].Host, pm.mappings[0].Container)
	}

	pm = NewPathMapper("/home/user:/workspace,/tmp/build:/build")
	if len(pm.mappings) != 2 {
		t.Fatalf("expected 2 mappings, got %d", len(pm.mappings))
	}
}

func TestPathMapper_ToContainer(t *testing.T) {
	pm := NewPathMapper("/home/user/proj:/workspace")
	tests := []struct {
		host      string
		container string
	}{
		{"/home/user/proj", "/workspace"},
		{"/home/user/proj/internal/arch", "/workspace/internal/arch"},
		{"/home/user/proj/", "/workspace/"},
		{"/other/path", "/other/path"},
		{".", "."},
	}
	for _, tt := range tests {
		got := pm.ToContainer(tt.host)
		if got != tt.container {
			t.Errorf("ToContainer(%q) = %q, want %q", tt.host, got, tt.container)
		}
	}
}

func TestPathMapper_ToHost(t *testing.T) {
	pm := NewPathMapper("/home/user/proj:/workspace")
	tests := []struct {
		container string
		host     string
	}{
		{"/workspace", "/home/user/proj"},
		{"/workspace/internal/arch", "/home/user/proj/internal/arch"},
		{"/other/path", "/other/path"},
	}
	for _, tt := range tests {
		got := pm.ToHost(tt.container)
		if got != tt.host {
			t.Errorf("ToHost(%q) = %q, want %q", tt.container, got, tt.host)
		}
	}
}

func TestPathMapper_ToContainer_FirstMatchWins(t *testing.T) {
	pm := NewPathMapper("/home/user:/w1,/home/user/proj:/w2")
	// First matching prefix wins
	got := pm.ToContainer("/home/user/proj")
	if got != "/w1/proj" {
		t.Errorf("first match should win: got %q", got)
	}
}

func TestPathMapper_ToContainer_WithSpaces(t *testing.T) {
	pm := NewPathMapper(" /home/user : /workspace ")
	if len(pm.mappings) != 1 {
		t.Fatalf("expected 1 mapping after trim, got %d", len(pm.mappings))
	}
	if !reflect.DeepEqual(pm.mappings[0], PathMapping{Host: "/home/user", Container: "/workspace"}) {
		t.Errorf("expected trimmed mapping, got %+v", pm.mappings[0])
	}
}
