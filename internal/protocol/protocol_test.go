package protocol

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dpopsuev/locus/internal/cache"
)

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

	sc := cache.New(cacheDir)
	p := New(sc, histDir, []string{wsDir})

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

func shas(commits []CommitMeta) []string {
	out := make([]string, len(commits))
	for i, c := range commits {
		out[i] = c.SHA
	}
	return out
}
