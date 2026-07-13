package mcp

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/dpopsuev/locus/internal/store"
	"github.com/dpopsuev/oculus/v3/engine"
)

const defaultWarmRecentCap = 5

// CollectWarmPaths returns unique existing directories from workspaces then
// recent projects, capped at maxN (default 5).
func CollectWarmPaths(workspaces []string, projects []store.ProjectInfo, maxN int) []string {
	if maxN <= 0 {
		maxN = defaultWarmRecentCap
	}
	seen := make(map[string]bool)
	var out []string
	add := func(p string) {
		if p == "" || len(out) >= maxN {
			return
		}
		abs, err := filepath.Abs(p)
		if err != nil {
			abs = p
		}
		abs = filepath.Clean(abs)
		if seen[abs] {
			return
		}
		if st, err := os.Stat(abs); err != nil || !st.IsDir() {
			return
		}
		seen[abs] = true
		out = append(out, abs)
	}
	for _, w := range workspaces {
		add(w)
	}
	sorted := append([]store.ProjectInfo(nil), projects...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].LastScan.After(sorted[j].LastScan)
	})
	for _, p := range sorted {
		add(p.Path)
	}
	return out
}

// WarmRecentProjects background-warms LSP for each path. Errors are logged,
// never returned. No-ops when eng has no pool.
func WarmRecentProjects(eng *engine.Engine, paths []string, warmFn func(context.Context, string) error) {
	if eng == nil || eng.Pool() == nil || len(paths) == 0 {
		return
	}
	if warmFn == nil {
		warmFn = eng.WarmLSP
	}
	for _, path := range paths {
		path := path
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			if err := warmFn(ctx, path); err != nil {
				slog.LogAttrs(ctx, slog.LevelDebug, "serve warm skipped",
					slog.String(logKeyPath, path), slog.Any(logKeyError, err))
			}
		}()
	}
}
