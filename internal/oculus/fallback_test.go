package oculus

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFallback_Classes(t *testing.T) {
	dir := setupTestRepo(t)
	fb := NewFallback(dir, nil)
	classes, err := fb.Classes(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(classes) < 3 {
		t.Fatalf("expected at least 3 types, got %d", len(classes))
	}
}

func TestFallback_NestingDepth(t *testing.T) {
	dir := setupTestRepo(t)
	fb := NewFallback(dir, nil)
	results, err := fb.NestingDepth(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("expected nesting results")
	}
}

func TestFallback_RegexFallback(t *testing.T) {
	dir := t.TempDir()
	// Rust project (no tree-sitter Rust implementation but regex handles it)
	os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte("[package]\nname = \"test\"\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "main.rs"), []byte(`
pub struct Foo {
    name: String,
}

pub trait Bar {
    fn do_thing(&self);
}

impl Bar for Foo {
    fn do_thing(&self) {}
}
`), 0o644)

	fb := NewFallback(dir, nil)
	classes, err := fb.Classes(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Regex should find at least the struct and trait
	if len(classes) < 2 {
		t.Fatalf("regex fallback: expected at least 2 types, got %d", len(classes))
	}

	edges, err := fb.Implements(dir)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range edges {
		if e.From == "Foo" && e.To == "Bar" {
			found = true
		}
	}
	if !found {
		t.Error("regex fallback: expected Foo implements Bar")
	}
}
