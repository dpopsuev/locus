package resource

import (
	"testing"
	"time"
)

func TestLoadConfig_Defaults(t *testing.T) {
	cfg := LoadConfig()

	if cfg.LRUCapacity != DefaultLRUCapacity {
		t.Errorf("LRUCapacity = %d, want %d", cfg.LRUCapacity, DefaultLRUCapacity)
	}
	if cfg.LSPMaxActive != DefaultLSPMaxActive {
		t.Errorf("LSPMaxActive = %d, want %d", cfg.LSPMaxActive, DefaultLSPMaxActive)
	}
	if cfg.LSPTTL != DefaultLSPTTL {
		t.Errorf("LSPTTL = %v, want %v", cfg.LSPTTL, DefaultLSPTTL)
	}
	if cfg.SGCacheTTL != DefaultSGCacheTTL {
		t.Errorf("SGCacheTTL = %v, want %v", cfg.SGCacheTTL, DefaultSGCacheTTL)
	}
	if cfg.MemLimitMB != 0 {
		t.Errorf("MemLimitMB = %d, want 0", cfg.MemLimitMB)
	}
	if cfg.MonitorInterval != DefaultMonitorInterval {
		t.Errorf("MonitorInterval = %v, want %v", cfg.MonitorInterval, DefaultMonitorInterval)
	}
}

func TestLoadConfig_FromEnv(t *testing.T) {
	t.Setenv("LOCUS_LRU_CAPACITY", "8")
	t.Setenv("LOCUS_LSP_MAX_ACTIVE", "1")
	t.Setenv("LOCUS_LSP_TTL", "2m")
	t.Setenv("LOCUS_SG_CACHE_TTL", "5m")
	t.Setenv("LOCUS_MEM_LIMIT_MB", "2048")
	t.Setenv("LOCUS_MONITOR_INTERVAL", "15s")

	cfg := LoadConfig()

	if cfg.LRUCapacity != 8 {
		t.Errorf("LRUCapacity = %d, want 8", cfg.LRUCapacity)
	}
	if cfg.LSPMaxActive != 1 {
		t.Errorf("LSPMaxActive = %d, want 1", cfg.LSPMaxActive)
	}
	if cfg.LSPTTL != 2*time.Minute {
		t.Errorf("LSPTTL = %v, want 2m", cfg.LSPTTL)
	}
	if cfg.SGCacheTTL != 5*time.Minute {
		t.Errorf("SGCacheTTL = %v, want 5m", cfg.SGCacheTTL)
	}
	if cfg.MemLimitMB != 2048 {
		t.Errorf("MemLimitMB = %d, want 2048", cfg.MemLimitMB)
	}
	if cfg.MonitorInterval != 15*time.Second {
		t.Errorf("MonitorInterval = %v, want 15s", cfg.MonitorInterval)
	}
}

func TestLoadConfig_InvalidEnv(t *testing.T) {
	t.Setenv("LOCUS_LRU_CAPACITY", "invalid")
	t.Setenv("LOCUS_LSP_TTL", "not-a-duration")

	cfg := LoadConfig()

	if cfg.LRUCapacity != DefaultLRUCapacity {
		t.Errorf("LRUCapacity = %d, want default %d", cfg.LRUCapacity, DefaultLRUCapacity)
	}
	if cfg.LSPTTL != DefaultLSPTTL {
		t.Errorf("LSPTTL = %v, want default %v", cfg.LSPTTL, DefaultLSPTTL)
	}
}
