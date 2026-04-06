package analysis

import (
	"os/exec"
	"strings"
	"sync"

	"github.com/dpopsuev/locus/internal/oculus/lsp"
)

var deepCache sync.Map // key: "path@sha" → *DeepFallbackAnalyzer

// CachedDeepFallback returns a cached DeepFallbackAnalyzer for the given path.
// The cache key is (path, HEAD SHA) — a new commit invalidates the cache.
// Language-agnostic: works for any repo type.
func CachedDeepFallback(path string, pool ...lsp.Pool) *DeepFallbackAnalyzer {
	sha := resolveHead(path)
	key := path + "@" + sha

	if cached, ok := deepCache.Load(key); ok {
		return cached.(*DeepFallbackAnalyzer)
	}

	var p lsp.Pool
	if len(pool) > 0 {
		p = pool[0]
	}
	da := NewDeepFallback(path, p)
	deepCache.Store(key, da)
	return da
}

func resolveHead(path string) string {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = path
	out, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}
