package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func setupGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test Author",
			"GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test Author",
			"GIT_COMMITTER_EMAIL=test@example.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %s: %v", args, out, err)
		}
	}

	run("init")
	run("checkout", "-b", "main")

	if err := os.MkdirAll(filepath.Join(dir, "pkg", "alpha"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pkg", "alpha", "a.go"), []byte("package alpha\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "add alpha")

	if err := os.MkdirAll(filepath.Join(dir, "pkg", "beta"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pkg", "beta", "b.go"), []byte("package beta\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "add beta")

	if err := os.WriteFile(filepath.Join(dir, "pkg", "alpha", "a.go"), []byte("package alpha\n// updated\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "update alpha")

	return dir
}

func TestRecentCommits(t *testing.T) {
	dir := setupGitRepo(t)
	commits := RecentCommits(dir, 30, "testmod")

	if len(commits) == 0 {
		t.Fatal("expected commits, got none")
	}

	foundAlpha := false
	foundBeta := false
	for _, c := range commits {
		if c.Package == "pkg/alpha" {
			foundAlpha = true
		}
		if c.Package == "pkg/beta" {
			foundBeta = true
		}
		if c.Author != "Test Author" {
			t.Errorf("unexpected author %q", c.Author)
		}
	}
	if !foundAlpha {
		t.Error("expected commits for pkg/alpha")
	}
	if !foundBeta {
		t.Error("expected commits for pkg/beta")
	}
}

func TestAuthorOwnership(t *testing.T) {
	dir := setupGitRepo(t)
	authors := AuthorOwnership(dir, "testmod")

	if len(authors) == 0 {
		t.Fatal("expected author data, got none")
	}

	alphaAuthors, ok := authors["pkg/alpha"]
	if !ok {
		t.Fatal("expected author data for pkg/alpha")
	}
	if alphaAuthors[0].Name != "Test Author" {
		t.Errorf("expected Test Author, got %q", alphaAuthors[0].Name)
	}
	if alphaAuthors[0].Commits < 2 {
		t.Errorf("expected at least 2 commits for alpha, got %d", alphaAuthors[0].Commits)
	}
}

func TestFileHotSpots(t *testing.T) {
	dir := setupGitRepo(t)
	files := FileHotSpots(dir, 30)

	if len(files) == 0 {
		t.Fatal("expected hot files, got none")
	}

	foundAlphaFile := false
	for _, f := range files {
		if f.Path == "pkg/alpha/a.go" {
			foundAlphaFile = true
			if f.Changes < 2 {
				t.Errorf("expected at least 2 changes for pkg/alpha/a.go, got %d", f.Changes)
			}
		}
	}
	if !foundAlphaFile {
		t.Error("expected pkg/alpha/a.go in hot files")
	}
}
