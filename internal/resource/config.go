package resource

import (
	"os"
	"strconv"
	"time"
)

// Resource manager defaults — tuned for lower memory than oculus's built-in values.
const (
	DefaultLRUCapacity     = 4
	DefaultLSPMaxActive    = 3
	DefaultLSPTTL          = 5 * time.Minute
	DefaultSGCacheTTL      = 10 * time.Minute
	DefaultMonitorInterval = 30 * time.Second
)

// Config holds operator-tunable resource knobs parsed from LOCUS_* env vars.
type Config struct {
	LRUCapacity     int
	LSPMaxActive    int
	LSPTTL          time.Duration
	SGCacheTTL      time.Duration
	MemLimitMB      int64
	MonitorInterval time.Duration
}

// LoadConfig reads resource configuration from LOCUS_* environment variables.
func LoadConfig() Config {
	return Config{
		LRUCapacity:     envInt("LOCUS_LRU_CAPACITY", DefaultLRUCapacity),
		LSPMaxActive:    envInt("LOCUS_LSP_MAX_ACTIVE", DefaultLSPMaxActive),
		LSPTTL:          envDuration("LOCUS_LSP_TTL", DefaultLSPTTL),
		SGCacheTTL:      envDuration("LOCUS_SG_CACHE_TTL", DefaultSGCacheTTL),
		MemLimitMB:      int64(envInt("LOCUS_MEM_LIMIT_MB", 0)),
		MonitorInterval: envDuration("LOCUS_MONITOR_INTERVAL", DefaultMonitorInterval),
	}
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return fallback
}
