//go:build e2e

package testkit_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dpopsuev/locus/internal/testkit"
)

func TestAllFixtures(t *testing.T) {
	root := findRepoRoot(t)
	testkitDir := filepath.Join(root, "testdata", "testkit")

	entries, err := os.ReadDir(testkitDir)
	if err != nil {
		t.Fatalf("read testkit dir: %v", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		lang := entry.Name()
		manifestPath := filepath.Join(testkitDir, lang, "manifest.json")
		if _, statErr := os.Stat(manifestPath); os.IsNotExist(statErr) {
			continue
		}

		t.Run(lang, func(t *testing.T) {
			fixtureDir := filepath.Join(testkitDir, lang)
			testkit.RunFixture(t, fixtureDir)
		})
	}
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root (no go.mod)")
		}
		dir = parent
	}
}
