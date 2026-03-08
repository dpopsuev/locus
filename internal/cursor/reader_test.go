package cursor

import (
	"path/filepath"
	"runtime"
	"testing"
)

func testdataRoot(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "testdata")
}

func TestReadRules(t *testing.T) {
	root := testdataRoot(t)
	rules, err := ReadRules(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 2 {
		t.Fatalf("got %d rules, want 2", len(rules))
	}

	byPath := map[string]Rule{}
	for _, r := range rules {
		byPath[r.Path] = r
	}

	ps, ok := byPath[filepath.Join(".cursor", "rules", "domain", "project-standards.mdc")]
	if !ok {
		t.Fatal("project-standards.mdc not found")
	}
	if ps.Description != "Project standards for testing" {
		t.Fatalf("description = %q", ps.Description)
	}
	if !ps.AlwaysApply {
		t.Fatal("expected alwaysApply = true")
	}
	if len(ps.Globs) != 0 {
		t.Fatalf("expected no globs, got %v", ps.Globs)
	}
	if ps.Body == "" {
		t.Fatal("expected non-empty body")
	}

	df, ok := byPath[filepath.Join(".cursor", "rules", "universal", "deterministic-first.mdc")]
	if !ok {
		t.Fatal("deterministic-first.mdc not found")
	}
	if df.AlwaysApply {
		t.Fatal("expected alwaysApply = false")
	}
	if len(df.Globs) != 2 {
		t.Fatalf("got %d globs, want 2: %v", len(df.Globs), df.Globs)
	}
	if df.Globs[0] != "circuits/**/*.yaml" {
		t.Fatalf("glob[0] = %q", df.Globs[0])
	}
}

func TestReadSkills(t *testing.T) {
	root := testdataRoot(t)
	skills, err := ReadSkills(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 2 {
		t.Fatalf("got %d skills, want 2", len(skills))
	}

	byName := map[string]Skill{}
	for _, s := range skills {
		byName[s.Name] = s
	}

	ii, ok := byName["index-integrity"]
	if !ok {
		t.Fatal("index-integrity skill not found")
	}
	if ii.Description != "Scan and validate index.mdc compliance." {
		t.Fatalf("description = %q", ii.Description)
	}
	if ii.Body == "" {
		t.Fatal("expected non-empty body")
	}

	sv, ok := byName["survey"]
	if !ok {
		t.Fatal("survey skill not found")
	}
	if sv.Description != "Scan repo and spill key concepts into .cursor." {
		t.Fatalf("description = %q", sv.Description)
	}
}

func TestReadRulesMissingDir(t *testing.T) {
	rules, err := ReadRules("/nonexistent/path")
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 0 {
		t.Fatalf("expected empty slice, got %d rules", len(rules))
	}
}

func TestReadSkillsMissingDir(t *testing.T) {
	skills, err := ReadSkills("/nonexistent/path")
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 0 {
		t.Fatalf("expected empty slice, got %d skills", len(skills))
	}
}
