package remote

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/dpopsuev/locus/internal/arch"
)

type Opts struct {
	Ref       string
	Keep      bool
	Depth     int
	ChurnDays int
	Budget    int
}

type Result struct {
	Report   *arch.ContextReport
	CloneDir string
	RefSHA   string
}

func CloneDir(repoURL, ref string) string {
	home, _ := os.UserHomeDir()
	raw := repoURL + "\n" + ref
	h := sha256.Sum256([]byte(raw))
	return filepath.Join(home, ".locus", "clones", fmt.Sprintf("%x", h[:8]))
}

func Codograph(ctx context.Context, repoURL string, opts Opts) (*Result, error) {
	ref := opts.Ref
	if ref == "" {
		ref = "HEAD"
	}

	dir := CloneDir(repoURL, ref)

	if err := shallowClone(ctx, repoURL, ref, dir); err != nil {
		return nil, fmt.Errorf("shallow clone %s: %w", repoURL, err)
	}

	if !opts.Keep {
		defer os.RemoveAll(dir)
	}

	churnDays := opts.ChurnDays
	if churnDays == 0 {
		churnDays = 30
	}

	report, err := arch.ScanAndBuild(dir, arch.ScanOpts{
		ExcludeTests: true,
		Depth:        opts.Depth,
		ChurnDays:    churnDays,
		Budget:       opts.Budget,
	})
	if err != nil {
		return nil, fmt.Errorf("scan %s: %w", repoURL, err)
	}

	sha := resolveHEAD(dir)

	return &Result{
		Report:   report,
		CloneDir: dir,
		RefSHA:   sha,
	}, nil
}

func shallowClone(ctx context.Context, repoURL, ref, dir string) error {
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		fetchCmd := exec.CommandContext(ctx, "git", "fetch", "--depth", "1", "origin", ref)
		fetchCmd.Dir = dir
		if out, err := fetchCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git fetch: %s: %w", string(out), err)
		}
		resetCmd := exec.CommandContext(ctx, "git", "reset", "--hard", "FETCH_HEAD")
		resetCmd.Dir = dir
		if out, err := resetCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git reset: %s: %w", string(out), err)
		}
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return err
	}

	args := []string{"clone", "--depth", "1"}
	if ref != "HEAD" {
		args = append(args, "--branch", ref)
	}
	args = append(args, NormalizeURL(repoURL), dir)

	cmd := exec.CommandContext(ctx, "git", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git clone: %s: %w", string(out), err)
	}
	return nil
}

// NormalizeURL converts shorthand GitHub URLs to full HTTPS URLs.
// Handles: github.com/org/repo, https://github.com/org/repo, git@github.com:org/repo.git
func NormalizeURL(raw string) string {
	if strings.HasPrefix(raw, "git@") {
		raw = strings.TrimPrefix(raw, "git@")
		raw = strings.Replace(raw, ":", "/", 1)
		raw = strings.TrimSuffix(raw, ".git")
		return "https://" + raw
	}

	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}

	if u, err := url.Parse(raw); err == nil {
		u.Path = strings.TrimSuffix(u.Path, ".git")
		return u.String()
	}

	return raw
}

// CacheKey returns a deterministic key for caching remote codographs.
func CacheKey(repoURL, refSHA string) string {
	return "remote:" + repoURL + "@" + refSHA
}

func resolveHEAD(dir string) string {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
