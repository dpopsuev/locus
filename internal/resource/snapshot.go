package resource

import "time"

// Snapshot is a point-in-time view of all resource metrics.
type Snapshot struct {
	Timestamp   time.Time        `json:"timestamp"`
	SelfRSSMB   float64          `json:"self_rss_mb"`
	ChildProcs  []ChildProcess   `json:"child_processes"`
	TotalRSSMB  float64          `json:"total_rss_mb"`
	Goroutines  int              `json:"goroutines"`
	LRU         LRUState         `json:"lru"`
	LSPPool     LSPPoolState     `json:"lsp_pool"`
	SGCache     SGCacheState     `json:"sg_cache"`
	PressureLog []PressureEvent  `json:"pressure_log,omitempty"`
}

// ChildProcess holds RSS for a single child (e.g. gopls).
type ChildProcess struct {
	PID     int     `json:"pid"`
	Command string  `json:"command"`
	RSSMB   float64 `json:"rss_mb"`
}

// LRUState reports in-memory cache occupancy.
type LRUState struct {
	Capacity int `json:"capacity"`
	Size     int `json:"size"`
}

// LSPPoolState reports LSP server pool occupancy.
type LSPPoolState struct {
	Active    int            `json:"active"`
	MaxActive int            `json:"max_active"`
	TTL       string         `json:"ttl"`
	ByLang    map[string]int `json:"by_language,omitempty"`
}

// SGCacheState reports SymbolGraph cache occupancy.
type SGCacheState struct {
	Entries int    `json:"entries"`
	TTL     string `json:"ttl"`
}

// PressureEvent records a memory pressure action taken by the monitor.
type PressureEvent struct {
	Timestamp time.Time `json:"timestamp"`
	Action    string    `json:"action"`
	Reason    string    `json:"reason"`
}
