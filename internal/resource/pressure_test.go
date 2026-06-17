package resource

import (
	"testing"
	"time"
)

func TestPressure_NoLimit(t *testing.T) {
	eng := &stubEngine{sgLen: 5}
	cfg := Config{MemLimitMB: 0, MonitorInterval: 30 * time.Second}
	mon := New(cfg, nil, nil, eng)

	snap := &Snapshot{TotalRSSMB: 999}
	mon.applyPressure(snap)

	if eng.sgFlushed != 0 {
		t.Error("should not flush when MemLimitMB=0")
	}
}

func TestPressure_UnderLimit(t *testing.T) {
	eng := &stubEngine{sgLen: 5}
	cfg := Config{MemLimitMB: 1000, MonitorInterval: 30 * time.Second}
	mon := New(cfg, nil, nil, eng)

	snap := &Snapshot{TotalRSSMB: 500}
	mon.applyPressure(snap)

	if eng.sgFlushed != 0 {
		t.Error("should not flush when under limit")
	}
}

func TestPressure_Phase1_SGFlush(t *testing.T) {
	eng := &stubEngine{sgLen: 3}
	cfg := Config{MemLimitMB: 500, MonitorInterval: 30 * time.Second}
	mon := New(cfg, nil, nil, eng)

	snap := &Snapshot{TotalRSSMB: 600}
	mon.applyPressure(snap)

	if eng.sgFlushed != 3 {
		t.Errorf("sgFlushed = %d, want 3", eng.sgFlushed)
	}
	if !mon.cooldown {
		t.Error("cooldown should be set after pressure action")
	}

	mon.mu.Lock()
	if len(mon.pressLog) != 1 {
		t.Errorf("pressLog len = %d, want 1", len(mon.pressLog))
	}
	if mon.pressLog[0].Action != "sg_flush" {
		t.Errorf("action = %q, want sg_flush", mon.pressLog[0].Action)
	}
	mon.mu.Unlock()
}

func TestPressure_Phase2_LRUEvict(t *testing.T) {
	eng := &stubEngine{sgLen: 0}
	lru := &stubLRU{len: 4, cap: 8}
	cfg := Config{MemLimitMB: 500, MonitorInterval: 30 * time.Second}
	mon := New(cfg, lru, nil, eng)

	snap := &Snapshot{TotalRSSMB: 600}
	mon.applyPressure(snap)

	if lru.evicted != 2 {
		t.Errorf("evicted = %d, want 2", lru.evicted)
	}
	if !mon.cooldown {
		t.Error("cooldown should be set")
	}
}

func TestPressure_Phase3_LSPReap(t *testing.T) {
	eng := &stubEngine{sgLen: 0}
	lru := &stubLRU{len: 0, cap: 4}
	pool := &stubPool{reaped: 2}
	cfg := Config{MemLimitMB: 500, MonitorInterval: 30 * time.Second}
	mon := New(cfg, lru, pool, eng)

	snap := &Snapshot{TotalRSSMB: 600}
	mon.applyPressure(snap)

	if !mon.cooldown {
		t.Error("cooldown should be set after LSP reap")
	}

	mon.mu.Lock()
	if len(mon.pressLog) != 1 || mon.pressLog[0].Action != "lsp_reap" {
		t.Errorf("expected lsp_reap in pressure log, got %v", mon.pressLog)
	}
	mon.mu.Unlock()
}

func TestPressure_Escalation(t *testing.T) {
	eng := &stubEngine{sgLen: 0}
	lru := &stubLRU{len: 0, cap: 4}
	pool := &stubPool{reaped: 0}
	cfg := Config{MemLimitMB: 500, MonitorInterval: 30 * time.Second}
	mon := New(cfg, lru, pool, eng)

	snap := &Snapshot{TotalRSSMB: 600}
	mon.applyPressure(snap)

	if mon.cooldown {
		t.Error("no action taken, cooldown should not be set")
	}
}
