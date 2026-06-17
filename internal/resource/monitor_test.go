package resource

import (
	"testing"
	"time"
)

type stubLRU struct {
	len, cap int
	evicted  int
}

func (s *stubLRU) Len() int      { return s.len }
func (s *stubLRU) Capacity() int { return s.cap }
func (s *stubLRU) EvictOldest(n int) int {
	evict := min(n, s.len)
	s.len -= evict
	s.evicted += evict
	return evict
}

type stubPool struct {
	pids   []int
	active int
	reaped int
}

func (s *stubPool) PIDs() []int { return s.pids }
func (s *stubPool) ReapIdle() int {
	n := s.reaped
	s.reaped = 0
	return n
}
func (s *stubPool) Status() PoolStatusView {
	return PoolStatusView{Active: s.active, ByLang: map[string]int{"Go": s.active}}
}

type stubEngine struct {
	sgLen     int
	sgFlushed int
}

func (s *stubEngine) SGCacheLen() int { return s.sgLen }
func (s *stubEngine) SGCacheFlush() int {
	n := s.sgLen
	s.sgLen = 0
	s.sgFlushed += n
	return n
}

func TestMonitor_Collect(t *testing.T) {
	lru := &stubLRU{len: 3, cap: 4}
	pool := &stubPool{active: 2}
	eng := &stubEngine{sgLen: 5}

	cfg := Config{
		LRUCapacity:     4,
		LSPMaxActive:    3,
		LSPTTL:          5 * time.Minute,
		SGCacheTTL:      10 * time.Minute,
		MonitorInterval: 30 * time.Second,
	}

	mon := New(cfg, lru, pool, eng)
	snap := mon.Collect()

	if snap.LRU.Size != 3 {
		t.Errorf("LRU.Size = %d, want 3", snap.LRU.Size)
	}
	if snap.LRU.Capacity != 4 {
		t.Errorf("LRU.Capacity = %d, want 4", snap.LRU.Capacity)
	}
	if snap.LSPPool.Active != 2 {
		t.Errorf("LSPPool.Active = %d, want 2", snap.LSPPool.Active)
	}
	if snap.LSPPool.MaxActive != 3 {
		t.Errorf("LSPPool.MaxActive = %d, want 3", snap.LSPPool.MaxActive)
	}
	if snap.SGCache.Entries != 5 {
		t.Errorf("SGCache.Entries = %d, want 5", snap.SGCache.Entries)
	}
	if snap.Goroutines == 0 {
		t.Error("Goroutines should be > 0")
	}
}

func TestMonitor_NilDeps(t *testing.T) {
	cfg := Config{MonitorInterval: 30 * time.Second}
	mon := New(cfg, nil, nil, nil)
	snap := mon.Collect()

	if snap.Goroutines == 0 {
		t.Error("Goroutines should be > 0")
	}
	if snap.LRU.Capacity != 0 {
		t.Errorf("LRU.Capacity = %d, want 0", snap.LRU.Capacity)
	}
}
