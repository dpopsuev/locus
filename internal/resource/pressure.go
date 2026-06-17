package resource

import "fmt"

func (m *Monitor) applyPressure(snap *Snapshot) {
	if m.cfg.MemLimitMB <= 0 {
		return
	}
	limitMB := float64(m.cfg.MemLimitMB)
	if snap.TotalRSSMB <= limitMB {
		return
	}

	reason := fmt.Sprintf("total_rss=%.0fMB > limit=%dMB", snap.TotalRSSMB, m.cfg.MemLimitMB)

	// Phase 1: flush SG cache (cheapest to regenerate).
	if m.engine != nil {
		if n := m.engine.SGCacheFlush(); n > 0 {
			m.logPressure("sg_flush", fmt.Sprintf("%s; flushed %d entries", reason, n))
			m.cooldown = true
			return
		}
	}

	// Phase 2: evict oldest LRU entries (costlier, forces rescan).
	if m.lru != nil {
		if n := m.lru.EvictOldest(2); n > 0 {
			m.logPressure("lru_evict", fmt.Sprintf("%s; evicted %d entries", reason, n))
			m.cooldown = true
			return
		}
	}

	// Phase 3: reap idle LSP servers (nuclear — kills gopls).
	if m.pool != nil {
		if n := m.pool.ReapIdle(); n > 0 {
			m.logPressure("lsp_reap", fmt.Sprintf("%s; reaped %d servers", reason, n))
			m.cooldown = true
			return
		}
	}
}
