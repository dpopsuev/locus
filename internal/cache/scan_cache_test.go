package cache

import (
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/dpopsuev/oculus/arch"
)

func tempCache(t *testing.T) *ScanCache {
	t.Helper()
	return New(filepath.Join(t.TempDir(), "cache"))
}

func initGitRepo(t *testing.T, dir string) string {
	t.Helper()
	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "test@test.com"},
		{"config", "user.name", "Test"},
		{"commit", "--allow-empty", "-m", "init"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s: %v", args, out, err)
		}
	}
	return ResolveHEAD(dir)
}

func TestCacheHitBySHA(t *testing.T) {
	c := tempCache(t)
	report := &arch.ContextReport{ScanCore: arch.ScanCore{Scanner: "test", ModulePath: "example.com/test"}}

	if err := c.Put("/repo/a", "abc123", report); err != nil {
		t.Fatal(err)
	}

	got, hit, err := c.Get("/repo/a", "abc123")
	if err != nil {
		t.Fatal(err)
	}
	if !hit {
		t.Fatal("expected cache hit")
	}
	if got.Scanner != "test" {
		t.Fatalf("scanner = %q, want %q", got.Scanner, "test")
	}
}

func TestCacheMissDifferentSHA(t *testing.T) {
	c := tempCache(t)
	report := &arch.ContextReport{ScanCore: arch.ScanCore{Scanner: "test"}}

	if err := c.Put("/repo/a", "sha-1", report); err != nil {
		t.Fatal(err)
	}

	_, hit, err := c.Get("/repo/a", "sha-2")
	if err != nil {
		t.Fatal(err)
	}
	if hit {
		t.Fatal("expected cache miss for different SHA")
	}
}

func TestMultipleSHAsPerRepo(t *testing.T) {
	c := tempCache(t)

	r1 := &arch.ContextReport{ScanCore: arch.ScanCore{Scanner: "branch-a"}}
	r2 := &arch.ContextReport{ScanCore: arch.ScanCore{Scanner: "branch-b"}}

	if err := c.Put("/repo", "sha-a", r1); err != nil {
		t.Fatal(err)
	}
	if err := c.Put("/repo", "sha-b", r2); err != nil {
		t.Fatal(err)
	}

	got1, hit1, _ := c.Get("/repo", "sha-a")
	got2, hit2, _ := c.Get("/repo", "sha-b")

	if !hit1 || got1.Scanner != "branch-a" {
		t.Errorf("sha-a: hit=%v scanner=%q", hit1, got1.Scanner)
	}
	if !hit2 || got2.Scanner != "branch-b" {
		t.Errorf("sha-b: hit=%v scanner=%q", hit2, got2.Scanner)
	}
}

func TestGetCurrent(t *testing.T) {
	c := tempCache(t)

	repoPath := t.TempDir()
	sha := initGitRepo(t, repoPath)

	report := &arch.ContextReport{ScanCore: arch.ScanCore{Scanner: "current"}}
	if err := c.Put(repoPath, sha, report); err != nil {
		t.Fatal(err)
	}

	got, gotSHA, hit, err := c.GetCurrent(repoPath)
	if err != nil {
		t.Fatal(err)
	}
	if !hit {
		t.Fatal("expected cache hit for current HEAD")
	}
	if gotSHA != sha {
		t.Errorf("sha = %q, want %q", gotSHA, sha)
	}
	if got.Scanner != "current" {
		t.Errorf("scanner = %q, want %q", got.Scanner, "current")
	}
}

func TestGetCurrentMissAfterNewCommit(t *testing.T) {
	c := tempCache(t)

	repoPath := t.TempDir()
	sha1 := initGitRepo(t, repoPath)

	if err := c.Put(repoPath, sha1, &arch.ContextReport{ScanCore: arch.ScanCore{Scanner: "old"}}); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("git", "commit", "--allow-empty", "-m", "second")
	cmd.Dir = repoPath
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %s: %v", out, err)
	}

	_, _, hit, err := c.GetCurrent(repoPath)
	if err != nil {
		t.Fatal(err)
	}
	if hit {
		t.Fatal("expected cache miss after new commit (SHA changed)")
	}

	// But old SHA still accessible
	got, oldHit, _ := c.Get(repoPath, sha1)
	if !oldHit || got.Scanner != "old" {
		t.Error("old SHA entry should still be accessible")
	}
}

func TestInvalidate(t *testing.T) {
	c := tempCache(t)

	if err := c.Put("/repo", "sha1", &arch.ContextReport{ScanCore: arch.ScanCore{Scanner: "test"}}); err != nil {
		t.Fatal(err)
	}
	if err := c.Put("/repo", "sha2", &arch.ContextReport{ScanCore: arch.ScanCore{Scanner: "test2"}}); err != nil {
		t.Fatal(err)
	}

	if err := c.Invalidate("/repo"); err != nil {
		t.Fatal(err)
	}

	_, hit1, _ := c.Get("/repo", "sha1")
	_, hit2, _ := c.Get("/repo", "sha2")
	if hit1 || hit2 {
		t.Fatal("expected cache miss after invalidation")
	}
}

func TestCacheMissEmpty(t *testing.T) {
	c := tempCache(t)
	_, hit, err := c.Get("/nonexistent/repo", "sha1")
	if err != nil {
		t.Fatal(err)
	}
	if hit {
		t.Fatal("expected cache miss for unknown repo")
	}
}

func TestCacheMissEmptySHA(t *testing.T) {
	c := tempCache(t)
	_, hit, err := c.Get("/repo", "")
	if err != nil {
		t.Fatal(err)
	}
	if hit {
		t.Fatal("expected cache miss for empty SHA")
	}
}

func TestResolveBranch(t *testing.T) {
	repoPath := t.TempDir()
	initGitRepo(t, repoPath)

	cmd := exec.Command("git", "checkout", "-b", "feature-x")
	cmd.Dir = repoPath
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git checkout: %s: %v", out, err)
	}

	cmd = exec.Command("git", "commit", "--allow-empty", "-m", "feature commit")
	cmd.Dir = repoPath
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %s: %v", out, err)
	}

	sha, err := ResolveBranch(repoPath, "feature-x")
	if err != nil {
		t.Fatal(err)
	}
	if sha == "" {
		t.Fatal("expected non-empty SHA for feature-x")
	}

	headSHA := ResolveHEAD(repoPath)
	if sha != headSHA {
		t.Errorf("feature-x SHA %q != HEAD %q", sha, headSHA)
	}
}
