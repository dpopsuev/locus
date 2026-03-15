package store

import (
	"container/list"
	"context"
	"sync"

	"github.com/dpopsuev/locus/internal/arch"
)

const DefaultLRUCapacity = 16

// LRUStore decorates any Store with an in-memory LRU cache for GetReport.
// All writes go through to the inner Store. Reads check the LRU first.
type LRUStore struct {
	inner    Store
	mu       sync.Mutex
	items    map[string]*list.Element
	order    *list.List
	capacity int
}

type lruEntry struct {
	key    string
	report *arch.ContextReport
}

// NewLRU wraps a Store with an in-memory LRU cache.
func NewLRU(inner Store, capacity int) *LRUStore {
	if capacity <= 0 {
		capacity = DefaultLRUCapacity
	}
	return &LRUStore{
		inner:    inner,
		items:    make(map[string]*list.Element),
		order:    list.New(),
		capacity: capacity,
	}
}

func lruKey(project, sha string) string {
	return project + "\x00" + sha
}

func (s *LRUStore) GetReport(ctx context.Context, project, sha string) (*arch.ContextReport, bool, error) {
	key := lruKey(project, sha)

	s.mu.Lock()
	if el, ok := s.items[key]; ok {
		s.order.MoveToFront(el)
		report := el.Value.(*lruEntry).report
		s.mu.Unlock()
		return report, true, nil
	}
	s.mu.Unlock()

	report, hit, err := s.inner.GetReport(ctx, project, sha)
	if err != nil || !hit {
		return report, hit, err
	}

	s.mu.Lock()
	s.addLocked(key, report)
	s.mu.Unlock()

	return report, true, nil
}

func (s *LRUStore) PutReport(ctx context.Context, project, sha string, report *arch.ContextReport) error {
	err := s.inner.PutReport(ctx, project, sha, report)
	if err != nil {
		return err
	}

	key := lruKey(project, sha)
	s.mu.Lock()
	s.addLocked(key, report)
	s.mu.Unlock()

	return nil
}

func (s *LRUStore) addLocked(key string, report *arch.ContextReport) {
	if el, ok := s.items[key]; ok {
		s.order.MoveToFront(el)
		el.Value.(*lruEntry).report = report
		return
	}
	el := s.order.PushFront(&lruEntry{key: key, report: report})
	s.items[key] = el
	if s.order.Len() > s.capacity {
		s.evictLocked()
	}
}

func (s *LRUStore) evictLocked() {
	back := s.order.Back()
	if back == nil {
		return
	}
	s.order.Remove(back)
	delete(s.items, back.Value.(*lruEntry).key)
}

// All other methods delegate to inner.

func (s *LRUStore) ListProjects(ctx context.Context) ([]ProjectInfo, error) {
	return s.inner.ListProjects(ctx)
}

func (s *LRUStore) UpsertProject(ctx context.Context, info ProjectInfo) error {
	return s.inner.UpsertProject(ctx, info)
}

func (s *LRUStore) PutComponentMeta(ctx context.Context, project, sha string, meta []ComponentMeta) error {
	return s.inner.PutComponentMeta(ctx, project, sha, meta)
}

func (s *LRUStore) ListComponentMeta(ctx context.Context, project, sha string) ([]ComponentMeta, error) {
	return s.inner.ListComponentMeta(ctx, project, sha)
}

func (s *LRUStore) SearchComponents(ctx context.Context, project, sha, query string) ([]ComponentMeta, error) {
	return s.inner.SearchComponents(ctx, project, sha, query)
}

func (s *LRUStore) RecordScan(ctx context.Context, source, repoPath, sha string, report *arch.ContextReport) error {
	return s.inner.RecordScan(ctx, source, repoPath, sha, report)
}

func (s *LRUStore) ListHistory(ctx context.Context, repoPath string, limit int) ([]HistoryEntry, error) {
	return s.inner.ListHistory(ctx, repoPath, limit)
}

func (s *LRUStore) GetHistoryReport(ctx context.Context, repoPath string, index int) (*arch.ContextReport, error) {
	return s.inner.GetHistoryReport(ctx, repoPath, index)
}

func (s *LRUStore) ResolveHEAD(repoPath string) string {
	return s.inner.ResolveHEAD(repoPath)
}

func (s *LRUStore) ResolveBranch(repoPath, ref string) (string, error) {
	return s.inner.ResolveBranch(repoPath, ref)
}

func (s *LRUStore) Close() error {
	return s.inner.Close()
}
