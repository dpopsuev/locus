package cache

import (
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/dpopsuev/locus/internal/arch"
)

// ScanCache stores and retrieves architecture scan results on the filesystem,
// keyed by (repo path, git SHA). Multiple SHAs can coexist per repo,
// enabling branch-aware cache hits (scan A, scan B, then re-read A).
type ScanCache struct {
	root string // e.g. ~/.locus/cache
}

func New(root string) *ScanCache {
	return &ScanCache{root: root}
}

func (c *ScanCache) Root() string { return c.root }

// DefaultCacheDir returns ~/.locus/cache.
func DefaultCacheDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".locus", "cache")
}

// Get returns a cached report for the given repo at the given SHA.
func (c *ScanCache) Get(repoPath, sha string) (*arch.ContextReport, bool, error) {
	if sha == "" {
		return nil, false, nil
	}
	p := c.entryPath(repoPath, sha)
	f, err := os.Open(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, false, nil
	}
	defer gz.Close()

	var report arch.ContextReport
	if err := json.NewDecoder(gz).Decode(&report); err != nil {
		return nil, false, nil
	}
	return &report, true, nil
}

// GetCurrent resolves HEAD for the repo and returns the cached report if present.
// Returns the resolved SHA alongside the report.
func (c *ScanCache) GetCurrent(repoPath string) (*arch.ContextReport, string, bool, error) {
	sha := ResolveHEAD(repoPath)
	if sha == "" {
		return nil, "", false, nil
	}
	report, hit, err := c.Get(repoPath, sha)
	return report, sha, hit, err
}

// Put stores a report keyed by (repo, sha). Writes are atomic (temp + rename).
func (c *ScanCache) Put(repoPath, sha string, report *arch.ContextReport) error {
	if sha == "" {
		return fmt.Errorf("empty SHA")
	}
	p := c.entryPath(repoPath, sha)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(filepath.Dir(p), ".tmp-*")
	if err != nil {
		return err
	}
	defer func() {
		tmp.Close()
		os.Remove(tmp.Name())
	}()

	gz := gzip.NewWriter(tmp)
	if err := json.NewEncoder(gz).Encode(report); err != nil {
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), p)
}

// Invalidate removes all cached entries for a repo.
func (c *ScanCache) Invalidate(repoPath string) error {
	dir := filepath.Join(c.root, RepoHash(repoPath))
	return os.RemoveAll(dir)
}

func (c *ScanCache) entryPath(repoPath, sha string) string {
	return filepath.Join(c.root, RepoHash(repoPath), sha+".json.gz")
}

// RepoHash returns a deterministic hash for a repository path.
func RepoHash(repoPath string) string {
	abs, err := filepath.Abs(repoPath)
	if err != nil {
		abs = repoPath
	}
	h := sha256.Sum256([]byte(abs))
	return fmt.Sprintf("%x", h[:8])
}

// ResolveHEAD returns the current HEAD SHA for a git repo, or "" if not a repo.
func ResolveHEAD(repoPath string) string {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// ResolveBranch returns the SHA for a named branch/ref in a git repo.
func ResolveBranch(repoPath, ref string) (string, error) {
	cmd := exec.Command("git", "rev-parse", ref)
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("resolve %q: %w", ref, err)
	}
	return strings.TrimSpace(string(out)), nil
}

