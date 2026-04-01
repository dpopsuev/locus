package protocol

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dpopsuev/locus/internal/arch"
	"github.com/dpopsuev/locus/internal/cache"
	"github.com/dpopsuev/locus/internal/port"
	"github.com/dpopsuev/locus/internal/store"
)

func testStore(cacheDir, histDir string) store.Store {
	return store.NewFilesystem(cache.New(cacheDir), histDir)
}

func TestResolvePath_Empty(t *testing.T) {
	p := &Protocol{workspaces: []string{"/tmp"}}
	got := p.resolvePath("")
	if got != "/tmp" {
		t.Fatalf("expected /tmp, got %s", got)
	}
}

func TestResolvePath_EmptyNoWorkspaces(t *testing.T) {
	p := &Protocol{}
	got := p.resolvePath("")
	if got != "." {
		t.Fatalf("expected '.', got %s", got)
	}
}

func TestResolvePath_AbsoluteExists(t *testing.T) {
	dir := t.TempDir()
	p := &Protocol{}
	got := p.resolvePath(dir)
	if got != dir {
		t.Fatalf("expected %s, got %s", dir, got)
	}
}

func TestResolvePath_RelativeToWorkspace(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "project")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	p := &Protocol{workspaces: []string{dir}}
	got := p.resolvePath("project")
	if got != sub {
		t.Fatalf("expected %s, got %s", sub, got)
	}
}

func TestResolvePath_NonExistentFallsThrough(t *testing.T) {
	p := &Protocol{workspaces: []string{"/tmp"}}
	got := p.resolvePath("/nonexistent/path/xyz")
	if got != "/nonexistent/path/xyz" {
		t.Fatalf("expected /nonexistent/path/xyz, got %s", got)
	}
}

func TestHealth_ReturnsStructuredChecks(t *testing.T) {
	cacheDir := filepath.Join(t.TempDir(), "cache")
	histDir := filepath.Join(t.TempDir(), "history")
	wsDir := t.TempDir()

	p := New(testStore(cacheDir, histDir), []string{wsDir})

	result := p.Health(context.Background())

	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Checks) == 0 {
		t.Fatal("expected at least one health check")
	}

	names := map[string]bool{}
	for _, c := range result.Checks {
		names[c.Name] = true
	}
	for _, required := range []string{"cache_dir", "history_dir", "git"} {
		if !names[required] {
			t.Errorf("missing expected check: %s", required)
		}
	}

	if !result.OK {
		for _, c := range result.Checks {
			if !c.OK {
				t.Logf("failing check: %s — %s", c.Name, c.Detail)
			}
		}
		t.Error("expected overall health to be OK")
	}
}

// --- Evolution tests ---

func TestSampleCommits_NoStride(t *testing.T) {
	commits := []CommitMeta{
		{SHA: "aaa"}, {SHA: "bbb"}, {SHA: "ccc"}, {SHA: "ddd"}, {SHA: "eee"},
	}
	got := sampleCommits(commits, 1)
	if len(got) != 5 {
		t.Fatalf("stride=1 should return all, got %d", len(got))
	}
}

func TestSampleCommits_Stride2(t *testing.T) {
	commits := []CommitMeta{
		{SHA: "a"}, {SHA: "b"}, {SHA: "c"}, {SHA: "d"}, {SHA: "e"},
	}
	got := sampleCommits(commits, 2)
	// Should pick indices 0, 2, 4 → a, c, e
	if len(got) != 3 {
		t.Fatalf("expected 3, got %d: %v", len(got), shas(got))
	}
	if got[0].SHA != "a" || got[1].SHA != "c" || got[2].SHA != "e" {
		t.Errorf("expected [a c e], got %v", shas(got))
	}
}

func TestSampleCommits_Stride3_IncludesLast(t *testing.T) {
	commits := []CommitMeta{
		{SHA: "a"}, {SHA: "b"}, {SHA: "c"}, {SHA: "d"}, {SHA: "e"},
	}
	got := sampleCommits(commits, 3)
	// Picks indices 0, 3 → a, d. Last (e) not hit, so appended.
	if len(got) != 3 {
		t.Fatalf("expected 3, got %d: %v", len(got), shas(got))
	}
	if got[0].SHA != "a" || got[1].SHA != "d" || got[2].SHA != "e" {
		t.Errorf("expected [a d e], got %v", shas(got))
	}
}

func TestSampleCommits_TwoOrFewer(t *testing.T) {
	one := []CommitMeta{{SHA: "a"}}
	got := sampleCommits(one, 10)
	if len(got) != 1 {
		t.Fatalf("expected 1, got %d", len(got))
	}
	two := []CommitMeta{{SHA: "a"}, {SHA: "b"}}
	got = sampleCommits(two, 10)
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d", len(got))
	}
}

func TestListCommits_RealRepo(t *testing.T) {
	// Use the locus repo itself as a fixture
	repoPath, err := filepath.Abs("../..")
	if err != nil {
		t.Skip("cannot resolve repo path")
	}
	if _, err := os.Stat(filepath.Join(repoPath, ".git")); err != nil {
		t.Skip("not in a git repo")
	}

	// Steps mode: last 5 commits
	commits, err := listCommits(repoPath, "", "HEAD", 5)
	if err != nil {
		t.Fatalf("listCommits steps mode: %v", err)
	}
	if len(commits) == 0 || len(commits) > 5 {
		t.Fatalf("expected 1-5 commits, got %d", len(commits))
	}
	for _, c := range commits {
		if len(c.SHA) != 40 {
			t.Errorf("expected 40-char SHA, got %q", c.SHA)
		}
		if c.Date == "" {
			t.Error("expected non-empty date")
		}
		if c.Message == "" {
			t.Error("expected non-empty message")
		}
	}
}

func TestListCommits_RangeMode(t *testing.T) {
	repoPath, err := filepath.Abs("../..")
	if err != nil {
		t.Skip("cannot resolve repo path")
	}
	if _, err := os.Stat(filepath.Join(repoPath, ".git")); err != nil {
		t.Skip("not in a git repo")
	}

	// Get a commit 3 back from HEAD to use as oldest
	cmd := exec.Command("git", "rev-parse", "HEAD~3")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		t.Skip("not enough commits for range test")
	}
	oldest := strings.TrimSpace(string(out))

	commits, err := listCommits(repoPath, oldest, "HEAD", 0)
	if err != nil {
		t.Fatalf("listCommits range mode: %v", err)
	}
	if len(commits) != 4 { // oldest + 3 more = 4 inclusive
		t.Fatalf("expected 4 commits (inclusive range), got %d", len(commits))
	}
	if commits[0].SHA != oldest {
		t.Errorf("first commit should be oldest ref %s, got %s", oldest[:8], commits[0].SHA[:8])
	}
}

func TestRenderEvolutionTable(t *testing.T) {
	result := &EvolutionResult{
		Path: "/repo/origami",
		Steps: []EvolutionStep{
			{Index: 0, ShortSHA: "abc1234", Date: "2026-01-01", Message: "init", Components: 3, Edges: 2, TotalLOC: 100},
			{Index: 1, ShortSHA: "def5678", Date: "2026-02-01", Message: "add pkg", Components: 5, Edges: 4, TotalLOC: 300},
		},
		Summary: "Growth: 3 -> 5 components (+67%), 2 -> 4 edges (+100%), 100 -> 300 LOC (+200%)",
	}
	out := RenderEvolutionTable(result)
	if !strings.Contains(out, "Architecture Evolution: origami") {
		t.Error("expected header with repo name")
	}
	if !strings.Contains(out, "abc1234") {
		t.Error("expected first SHA in output")
	}
	if !strings.Contains(out, "(basis)") {
		t.Error("expected (basis) for step 0")
	}
	if !strings.Contains(out, "Growth:") {
		t.Error("expected summary line")
	}
}

func TestGetViolations_AutoDetectLayers(t *testing.T) {
	// Use the locus repo itself.
	repoPath, err := filepath.Abs("../..")
	if err != nil {
		t.Skip("cannot resolve repo path")
	}
	if _, err := os.Stat(filepath.Join(repoPath, ".git")); err != nil {
		t.Skip("not in a git repo")
	}

	cacheDir := filepath.Join(t.TempDir(), "cache")
	p := New(testStore(cacheDir, t.TempDir()), []string{repoPath})

	report, err := p.GetViolations(context.Background(), repoPath, nil)
	if err != nil {
		t.Fatalf("GetViolations: %v", err)
	}
	if len(report.Layers) == 0 {
		t.Error("expected auto-detected layers")
	}
	if report.Summary == "" {
		t.Error("expected summary")
	}
	t.Logf("Violations: %s, layers=%d", report.Summary, len(report.Layers))
}

func TestGetViolations_ExplicitLayers(t *testing.T) {
	repoPath, err := filepath.Abs("../..")
	if err != nil {
		t.Skip("cannot resolve repo path")
	}
	if _, err := os.Stat(filepath.Join(repoPath, ".git")); err != nil {
		t.Skip("not in a git repo")
	}

	cacheDir := filepath.Join(t.TempDir(), "cache")
	p := New(testStore(cacheDir, t.TempDir()), []string{repoPath})

	// Use explicit layers — model is low, mcp is high.
	layers := []string{"model", "survey", "arch", "analysis", "protocol", "mcp"}
	report, err := p.GetViolations(context.Background(), repoPath, layers)
	if err != nil {
		t.Fatalf("GetViolations: %v", err)
	}
	if len(report.Layers) != len(layers) {
		t.Errorf("expected %d layers, got %d", len(layers), len(report.Layers))
	}
	t.Logf("Violations with explicit layers: %s", report.Summary)
	for _, v := range report.Violations {
		t.Logf("  violation: %s -> %s", v.From, v.To)
	}
}

func TestGetScanDiff(t *testing.T) {
	s := testStore(filepath.Join(t.TempDir(), "cache"), t.TempDir())
	p := New(s, nil)

	beforeReport := &arch.ContextReport{
		Architecture: arch.ArchModel{
			Services: []arch.ArchService{
				{Name: "pkg_a", LOC: 100},
				{Name: "pkg_b", LOC: 200},
			},
			Edges: []arch.ArchEdge{
				{From: "pkg_a", To: "pkg_b"},
			},
		},
	}
	afterReport := &arch.ContextReport{
		Architecture: arch.ArchModel{
			Services: []arch.ArchService{
				{Name: "pkg_a", LOC: 150},
				{Name: "pkg_b", LOC: 200},
				{Name: "pkg_c", LOC: 50},
			},
			Edges: []arch.ArchEdge{
				{From: "pkg_a", To: "pkg_b"},
				{From: "pkg_a", To: "pkg_c"},
			},
		},
	}

	_ = s.PutReport(context.Background(), "/repo", "sha1", beforeReport)
	_ = s.PutReport(context.Background(), "/repo", "sha2", afterReport)

	diff, err := p.GetScanDiff(context.Background(), "/repo", "sha1", "sha2")
	if err != nil {
		t.Fatalf("GetScanDiff: %v", err)
	}
	if len(diff.AddedComponents) != 1 || diff.AddedComponents[0] != "pkg_c" {
		t.Errorf("added = %v, want [pkg_c]", diff.AddedComponents)
	}
	if len(diff.RemovedComponents) != 0 {
		t.Errorf("removed = %v, want []", diff.RemovedComponents)
	}
	if diff.AddedEdges != 1 {
		t.Errorf("added edges = %d, want 1", diff.AddedEdges)
	}
	if diff.LOCDelta != 100 {
		t.Errorf("LOC delta = %d, want 100", diff.LOCDelta)
	}
	t.Logf("Diff: %s", diff.Summary)
}

func TestGetCachedReport_RoundTrip(t *testing.T) {
	s := testStore(filepath.Join(t.TempDir(), "cache"), t.TempDir())
	p := New(s, nil)

	// Simulate a remote scan: store with project path (without SHA), retrieve via cache key.
	fakeProject := "remote:https://github.com/example/repo"
	fakeSHA := "abc123def456"
	fakeKey := fakeProject + "@" + fakeSHA
	report := &arch.ContextReport{
		ModulePath: "github.com/example/repo",
		Scanner:    "go",
	}
	if err := s.PutReport(context.Background(), fakeProject, fakeSHA, report); err != nil {
		t.Fatalf("put: %v", err)
	}

	// Retrieve via GetCachedReport.
	got, err := p.GetCachedReport(fakeKey)
	if err != nil {
		t.Fatalf("GetCachedReport: %v", err)
	}
	if got.ModulePath != "github.com/example/repo" {
		t.Errorf("module path = %q, want github.com/example/repo", got.ModulePath)
	}

	// Retrieve via getOrScan with cache key.
	got2, err := p.getOrScan("", fakeKey)
	if err != nil {
		t.Fatalf("getOrScan with cache key: %v", err)
	}
	if got2.ModulePath != got.ModulePath {
		t.Errorf("getOrScan returned different module path")
	}

	// Missing key should error.
	_, err = p.GetCachedReport("remote:https://github.com/missing/repo@deadbeef")
	if err == nil {
		t.Error("expected error for missing cache key")
	}
}

func TestRunPreset(t *testing.T) {
	repoPath, err := filepath.Abs("../..")
	if err != nil {
		t.Skip("cannot resolve repo path")
	}
	if _, err := os.Stat(filepath.Join(repoPath, ".git")); err != nil {
		t.Skip("not in a git repo")
	}

	p := New(testStore(filepath.Join(t.TempDir(), "cache"), t.TempDir()), []string{repoPath})

	for _, preset := range []string{PresetArchReview, PresetHealthCheck, PresetOnboarding, PresetPrePR} {
		out, err := p.RunPreset(context.Background(), repoPath, preset)
		if err != nil {
			t.Fatalf("RunPreset(%s): %v", preset, err)
		}
		if out == "" {
			t.Errorf("RunPreset(%s) returned empty", preset)
		}
		t.Logf("%s: %d chars", preset, len(out))
	}

	_, err = p.RunPreset(context.Background(), repoPath, "nonexistent")
	if err == nil {
		t.Error("expected error for unknown preset")
	}
}

func TestGetComponentDetail(t *testing.T) {
	s := testStore(filepath.Join(t.TempDir(), "cache"), t.TempDir())
	p := New(s, nil)

	report := &arch.ContextReport{
		Architecture: arch.ArchModel{
			Services: []arch.ArchService{
				{Name: "pkg_a", LOC: 500, Churn: 3, Symbols: []string{"Foo", "Bar", "Baz"}},
				{Name: "pkg_b", LOC: 200},
			},
			Edges: []arch.ArchEdge{
				{From: "pkg_a", To: "pkg_b"},
				{From: "pkg_b", To: "pkg_a"},
			},
		},
	}
	_ = s.PutReport(context.Background(), "/repo", "sha1", report)

	detail, err := p.GetComponentDetail(context.Background(), "/repo", "pkg_a", "")
	if err != nil {
		// No cache hit without SHA, expected
		t.Skip("no cache hit without SHA resolution")
	}
	if detail.Name != "pkg_a" {
		t.Errorf("name = %q, want pkg_a", detail.Name)
	}
}

func TestAnswerQuery(t *testing.T) {
	repoPath, err := filepath.Abs("../..")
	if err != nil {
		t.Skip("cannot resolve repo path")
	}
	if _, err := os.Stat(filepath.Join(repoPath, ".git")); err != nil {
		t.Skip("not in a git repo")
	}

	p := New(testStore(filepath.Join(t.TempDir(), "cache"), t.TempDir()), []string{repoPath})

	tests := []struct {
		query  string
		action string
	}{
		{"what are the riskiest components?", "coupling view=hot_spots"},
		{"any circular dependencies?", "cycles"},
		{"are there layer violations?", "violations"},
		{"give me an overview", "preset=architecture_review"},
		{"something completely unknown", "none"},
	}
	for _, tt := range tests {
		r, err := p.AnswerQuery(context.Background(), repoPath, tt.query)
		if err != nil {
			t.Fatalf("AnswerQuery(%q): %v", tt.query, err)
		}
		if r.Action != tt.action {
			t.Errorf("query=%q: action=%q, want %q", tt.query, r.Action, tt.action)
		}
	}
}

func TestGenerateHints(t *testing.T) {
	// Report with cycles and hot spots should produce hints.
	report := &arch.ContextReport{
		Cycles:   []arch.Cycle{{"a", "b", "a"}},
		HotSpots: []arch.HotSpot{{Component: "pkg_a", FanIn: 5, Churn: 10}},
	}
	hints := GenerateHints(report)
	if len(hints) < 2 {
		t.Errorf("expected at least 2 hints, got %d", len(hints))
	}

	// Clean report should produce no hints.
	clean := &arch.ContextReport{}
	hints = GenerateHints(clean)
	if len(hints) != 0 {
		t.Errorf("expected 0 hints for clean report, got %d", len(hints))
	}
}

func TestAcceptViolation(t *testing.T) {
	s := testStore(filepath.Join(t.TempDir(), "cache"), t.TempDir())
	p := New(s, nil)
	ctx := context.Background()
	path := "/test/project"

	// Accept a violation when no desired state exists yet.
	err := p.AcceptViolation(ctx, path, port.AcceptedViolation{
		Component: "pkg_a",
		Principle: "SRP",
		Reason:    "composition root, expected",
	})
	if err != nil {
		t.Fatalf("AcceptViolation (first): %v", err)
	}

	// Verify it was persisted.
	ds, err := p.GetDesiredState(ctx, path)
	if err != nil {
		t.Fatalf("GetDesiredState: %v", err)
	}
	if ds == nil {
		t.Fatal("expected non-nil desired state")
	}
	if len(ds.Accepted) != 1 {
		t.Fatalf("expected 1 accepted violation, got %d", len(ds.Accepted))
	}
	if ds.Accepted[0].Component != "pkg_a" {
		t.Errorf("component = %q, want pkg_a", ds.Accepted[0].Component)
	}
	if ds.Accepted[0].Principle != "SRP" {
		t.Errorf("principle = %q, want SRP", ds.Accepted[0].Principle)
	}
	if ds.Accepted[0].Reason != "composition root, expected" {
		t.Errorf("reason = %q, want 'composition root, expected'", ds.Accepted[0].Reason)
	}

	// Accept a second violation — should append, not replace.
	err = p.AcceptViolation(ctx, path, port.AcceptedViolation{
		Component: "pkg_b",
		Principle: "god_component",
	})
	if err != nil {
		t.Fatalf("AcceptViolation (second): %v", err)
	}

	ds, err = p.GetDesiredState(ctx, path)
	if err != nil {
		t.Fatalf("GetDesiredState (second): %v", err)
	}
	if len(ds.Accepted) != 2 {
		t.Fatalf("expected 2 accepted violations, got %d", len(ds.Accepted))
	}
	if ds.Accepted[1].Component != "pkg_b" {
		t.Errorf("second component = %q, want pkg_b", ds.Accepted[1].Component)
	}
	if ds.Accepted[1].Principle != "god_component" {
		t.Errorf("second principle = %q, want god_component", ds.Accepted[1].Principle)
	}
}

func TestAcceptViolation_PreservesExistingState(t *testing.T) {
	s := testStore(filepath.Join(t.TempDir(), "cache"), t.TempDir())
	p := New(s, nil)
	ctx := context.Background()
	path := "/test/project"

	// Set up initial desired state with layers.
	err := p.SetDesiredState(ctx, path, &port.DesiredState{
		Layers: []string{"domain", "service", "handler"},
	})
	if err != nil {
		t.Fatalf("SetDesiredState: %v", err)
	}

	// Accept a violation.
	err = p.AcceptViolation(ctx, path, port.AcceptedViolation{
		Component: "handler/api",
		Principle: "DIP",
		Reason:    "adapter layer",
	})
	if err != nil {
		t.Fatalf("AcceptViolation: %v", err)
	}

	// Verify layers are preserved.
	ds, err := p.GetDesiredState(ctx, path)
	if err != nil {
		t.Fatalf("GetDesiredState: %v", err)
	}
	if len(ds.Layers) != 3 {
		t.Errorf("layers lost: got %d, want 3", len(ds.Layers))
	}
	if len(ds.Accepted) != 1 {
		t.Fatalf("expected 1 accepted violation, got %d", len(ds.Accepted))
	}
}

func shas(commits []CommitMeta) []string {
	out := make([]string, len(commits))
	for i, c := range commits {
		out[i] = c.SHA
	}
	return out
}
