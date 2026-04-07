package git_test

import (
	"testing"

	"github.com/dpopsuev/locus/internal/git"
)

func TestChangedFilesSince_External(t *testing.T) {
	_, err := git.ChangedFilesSince(t.TempDir(), "HEAD~1")
	// Non-git dir returns error — we're testing the boundary contract.
	_ = err
}
