package resource

import (
	"context"
	"log/slog"
	"runtime"
	"sync"
	"time"
)

const maxPressureLog = 50

const logKeyTotalRSSMB = "total_rss_mb"

// LRUInspector is the subset of LRUStore needed by the monitor.
type LRUInspector interface {
	Len() int
	Capacity() int
	EvictOldest(n int) int
}

// PoolInspector is the subset of lsp.RealPool needed by the monitor.
type PoolInspector interface {
	PIDs() []int
	ReapIdle() int
	Status() PoolStatusView
}

// PoolStatusView is a transport-neutral view of pool status.
type PoolStatusView struct {
	Active int
	ByLang map[string]int
}

// EngineInspector is the subset of engine.Engine needed by the monitor.
type EngineInspector interface {
	SGCacheLen() int
	SGCacheFlush() int
}

// Monitor collects resource snapshots and applies memory pressure.
type Monitor struct {
	cfg    Config
	lru    LRUInspector
	pool   PoolInspector
	engine EngineInspector

	done chan struct{}
	mu   sync.Mutex

	latest   *Snapshot
	pressLog []PressureEvent
	cooldown bool
}

// New creates a resource monitor. Any dependency may be nil.
func New(cfg Config, lru LRUInspector, pool PoolInspector, eng EngineInspector) *Monitor {
	return &Monitor{
		cfg:    cfg,
		lru:    lru,
		pool:   pool,
		engine: eng,
		done:   make(chan struct{}),
	}
}

// Start begins the background monitoring goroutine.
func (m *Monitor) Start(ctx context.Context) {
	go m.loop(ctx)
}

// Stop terminates the background monitoring goroutine.
func (m *Monitor) Stop() {
	select {
	case <-m.done:
	default:
		close(m.done)
	}
}

func (m *Monitor) loop(ctx context.Context) {
	ticker := time.NewTicker(m.cfg.MonitorInterval)
	defer ticker.Stop()

	if m.cfg.MemLimitMB <= 0 {
		slog.WarnContext(ctx, "resource monitor: LOCUS_MEM_LIMIT_MB not set — memory pressure disabled; set it to enable automatic eviction")
	}

	for {
		select {
		case <-m.done:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			snap := m.Collect()
			m.mu.Lock()
			m.latest = snap
			m.mu.Unlock()

			level := m.snapshotLevel(snap)
			slog.LogAttrs(ctx, level, "resource snapshot",
				slog.Float64("self_rss_mb", snap.SelfRSSMB),
				slog.Float64("total_rss_mb", snap.TotalRSSMB),
				slog.Int("goroutines", snap.Goroutines),
				slog.Int("lru_size", snap.LRU.Size),
				slog.Int("lsp_active", snap.LSPPool.Active),
				slog.Int("sg_entries", snap.SGCache.Entries),
			)

			if m.cooldown {
				m.cooldown = false
				continue
			}
			m.applyPressure(snap)
		}
	}
}

// RSS tripwire thresholds (MB). Crossing these escalates snapshot log level.
const (
	rssWarnMB  = 4096  // 4 GB
	rssCritMB  = 16384 // 16 GB
)

func (m *Monitor) snapshotLevel(snap *Snapshot) slog.Level {
	switch {
	case snap.TotalRSSMB >= rssCritMB:
		return slog.LevelError
	case snap.TotalRSSMB >= rssWarnMB:
		return slog.LevelWarn
	default:
		return slog.LevelDebug
	}
}

// Collect gathers a fresh resource snapshot.
func (m *Monitor) Collect() *Snapshot {
	snap := &Snapshot{
		Timestamp:  time.Now(),
		Goroutines: runtime.NumGoroutine(),
		SelfRSSMB:  selfRSSMB(),
	}

	if m.lru != nil {
		snap.LRU = LRUState{
			Capacity: m.lru.Capacity(),
			Size:     m.lru.Len(),
		}
	}

	if m.pool != nil {
		pids := m.pool.PIDs()
		snap.ChildProcs = childRSSMB(pids)
		status := m.pool.Status()
		snap.LSPPool = LSPPoolState{
			Active:    status.Active,
			MaxActive: m.cfg.LSPMaxActive,
			TTL:       m.cfg.LSPTTL.String(),
			ByLang:    status.ByLang,
		}
	}

	if m.engine != nil {
		snap.SGCache = SGCacheState{
			Entries: m.engine.SGCacheLen(),
			TTL:     m.cfg.SGCacheTTL.String(),
		}
	}

	var childTotal float64
	for _, c := range snap.ChildProcs {
		childTotal += c.RSSMB
	}
	snap.TotalRSSMB = snap.SelfRSSMB + childTotal

	m.mu.Lock()
	snap.PressureLog = make([]PressureEvent, len(m.pressLog))
	copy(snap.PressureLog, m.pressLog)
	m.mu.Unlock()

	return snap
}

// Latest returns the most recent cached snapshot.
func (m *Monitor) Latest() *Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.latest
}

func (m *Monitor) logPressure(action, reason string) {
	ev := PressureEvent{
		Timestamp: time.Now(),
		Action:    action,
		Reason:    reason,
	}
	m.mu.Lock()
	m.pressLog = append(m.pressLog, ev)
	if len(m.pressLog) > maxPressureLog {
		m.pressLog = m.pressLog[len(m.pressLog)-maxPressureLog:]
	}
	m.mu.Unlock()

	slog.Warn("resource pressure",
		slog.String("action", action),
		slog.String("reason", reason),
	)
}
