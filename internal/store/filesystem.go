package store

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dpopsuev/oculus/arch"
	"github.com/dpopsuev/oculus/cache"
	"github.com/dpopsuev/oculus/history"
)

// FilesystemStore implements Store by delegating to the existing
// cache.ScanCache (gzipped JSON files) and history package (JSONL files).
// This is the v1 adapter — zero behavior change from the pre-hexagonal code.
type FilesystemStore struct {
	sc      *cache.ScanCache
	histDir string
	mu      sync.Mutex // guards projects.json
}

// NewFilesystem creates a FilesystemStore wrapping existing cache and history.
func NewFilesystem(sc *cache.ScanCache, histDir string) *FilesystemStore {
	return &FilesystemStore{sc: sc, histDir: histDir}
}

func (f *FilesystemStore) GetReport(_ context.Context, project, sha string) (report *arch.ContextReport, found bool, err error) {
	return f.sc.Get(project, sha)
}

func (f *FilesystemStore) PutReport(_ context.Context, project, sha string, report *arch.ContextReport) error {
	if err := f.sc.Put(project, sha, report); err != nil {
		return err
	}
	f.autoRegisterProject(project, sha, report)
	return nil
}

func (f *FilesystemStore) Invalidate(_ context.Context, project string) error {
	return f.sc.Invalidate(project)
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

func (f *FilesystemStore) ListProjects(_ context.Context) ([]ProjectInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.loadProjects()
}

func (f *FilesystemStore) UpsertProject(_ context.Context, info ProjectInfo) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	projects, _ := f.loadProjects()
	found := false
	for i := range projects {
		if projects[i].Path == info.Path {
			projects[i] = info
			found = true
			break
		}
	}
	if !found {
		projects = append(projects, info)
	}
	return f.saveProjects(projects)
}

func (f *FilesystemStore) projectsPath() string {
	return filepath.Join(f.sc.Root(), "projects.json")
}

func (f *FilesystemStore) loadProjects() ([]ProjectInfo, error) {
	data, err := os.ReadFile(f.projectsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var projects []ProjectInfo
	if err := json.Unmarshal(data, &projects); err != nil {
		return nil, err
	}
	return projects, nil
}

func (f *FilesystemStore) saveProjects(projects []ProjectInfo) error {
	p := f.projectsPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(projects, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o600)
}

func (f *FilesystemStore) PutComponentMeta(_ context.Context, project, sha string, meta []ComponentMeta) error {
	p := f.metaPath(project, sha)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o600)
}

func (f *FilesystemStore) ListComponentMeta(_ context.Context, project, sha string) ([]ComponentMeta, error) {
	data, err := os.ReadFile(f.metaPath(project, sha))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var meta []ComponentMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	return meta, nil
}

func (f *FilesystemStore) SearchComponents(_ context.Context, project, sha, query string) ([]ComponentMeta, error) {
	meta, err := f.ListComponentMeta(context.Background(), project, sha)
	if err != nil || len(meta) == 0 {
		return nil, err
	}
	tokens := strings.Fields(strings.ToLower(query))
	type scored struct {
		meta  ComponentMeta
		score int
	}
	var results []scored
	for _, m := range meta {
		s := 0
		kwSet := make(map[string]bool)
		for _, kw := range m.Keywords {
			kwSet[strings.ToLower(kw)] = true
		}
		nameLower := strings.ToLower(m.Name)
		for _, t := range tokens {
			if kwSet[t] {
				s += 2
			}
			if strings.Contains(nameLower, t) {
				s++
			}
		}
		if s > 0 {
			results = append(results, scored{meta: m, score: s})
		}
	}
	sort.Slice(results, func(i, j int) bool { return results[i].score > results[j].score })
	out := make([]ComponentMeta, len(results))
	for i, r := range results {
		out[i] = r.meta
	}
	return out, nil
}

func (f *FilesystemStore) GetDesiredState(_ context.Context, project string) (*DesiredState, error) {
	p := f.desiredStatePath(project)
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var ds DesiredState
	if err := json.Unmarshal(data, &ds); err != nil {
		return nil, err
	}
	return &ds, nil
}

func (f *FilesystemStore) PutDesiredState(_ context.Context, project string, state *DesiredState) error {
	p := f.desiredStatePath(project)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o600)
}

func (f *FilesystemStore) desiredStatePath(project string) string {
	return filepath.Join(f.sc.Root(), cache.RepoHash(project), "desired-state.json")
}

func (f *FilesystemStore) metaPath(project, sha string) string {
	return filepath.Join(f.sc.Root(), cache.RepoHash(project), sha+"-meta.json")
}

// Auto-register project on PutReport.
func (f *FilesystemStore) autoRegisterProject(project, sha string, report *arch.ContextReport) {
	info := ProjectInfo{
		Path:       project,
		Name:       report.ModulePath,
		Language:   report.Scanner,
		LastSHA:    sha,
		LastScan:   time.Now(),
		Components: len(report.Architecture.Services),
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	projects, _ := f.loadProjects()
	found := false
	for i := range projects {
		if projects[i].Path == info.Path {
			projects[i] = info
			found = true
			break
		}
	}
	if !found {
		projects = append(projects, info)
	}
	_ = f.saveProjects(projects)
}

func (f *FilesystemStore) Close() error {
	return nil
}
