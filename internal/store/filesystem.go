package store

import (
	"context"

	"github.com/dpopsuev/locus/internal/arch"
	"github.com/dpopsuev/locus/internal/cache"
	"github.com/dpopsuev/locus/internal/history"
)

// FilesystemStore implements Store by delegating to the existing
// cache.ScanCache (gzipped JSON files) and history package (JSONL files).
// This is the v1 adapter — zero behavior change from the pre-hexagonal code.
type FilesystemStore struct {
	sc      *cache.ScanCache
	histDir string
}

// NewFilesystem creates a FilesystemStore wrapping existing cache and history.
func NewFilesystem(sc *cache.ScanCache, histDir string) *FilesystemStore {
	return &FilesystemStore{sc: sc, histDir: histDir}
}

func (f *FilesystemStore) GetReport(_ context.Context, project, sha string) (*arch.ContextReport, bool, error) {
	return f.sc.Get(project, sha)
}

func (f *FilesystemStore) PutReport(_ context.Context, project, sha string, report *arch.ContextReport) error {
	return f.sc.Put(project, sha, report)
}

func (f *FilesystemStore) RecordScan(_ context.Context, source, repoPath, sha string, report *arch.ContextReport) error {
	return history.Record(f.sc, f.histDir, history.Source(source), repoPath, sha, report)
}

func (f *FilesystemStore) ListHistory(_ context.Context, repoPath string, limit int) ([]HistoryEntry, error) {
	entries, err := history.List(f.histDir, repoPath, limit)
	if err != nil {
		return nil, err
	}
	result := make([]HistoryEntry, len(entries))
	for i, e := range entries {
		result[i] = HistoryEntry{
			Timestamp:  e.Timestamp,
			SHA:        e.HeadSHA,
			Source:     string(e.Source),
			RepoPath:   e.RepoPath,
			Components: e.Components,
			Edges:      e.Edges,
		}
	}
	return result, nil
}

func (f *FilesystemStore) GetHistoryReport(_ context.Context, repoPath string, index int) (*arch.ContextReport, error) {
	return history.GetReport(f.sc, f.histDir, repoPath, index)
}

func (f *FilesystemStore) ResolveHEAD(repoPath string) string {
	return cache.ResolveHEAD(repoPath)
}

func (f *FilesystemStore) ResolveBranch(repoPath, ref string) (string, error) {
	return cache.ResolveBranch(repoPath, ref)
}

// CacheRoot returns the filesystem cache root directory (for health checks).
func (f *FilesystemStore) CacheRoot() string {
	return f.sc.Root()
}

// HistoryDir returns the history directory (for health checks).
func (f *FilesystemStore) HistoryDir() string {
	return f.histDir
}

func (f *FilesystemStore) Close() error {
	return nil
}
