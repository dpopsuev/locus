package config

import (
	"os"

	"github.com/dpopsuev/locus/internal/resource"
	"github.com/dpopsuev/locus/internal/store"
	"github.com/dpopsuev/oculus/v3/cache"
	"github.com/dpopsuev/oculus/v3/history"
)

const (
	EnvStore      = "LOCUS_STORE"
	EnvCacheDir   = "LOCUS_CACHE_DIR"
	EnvHistoryDir = "LOCUS_HISTORY_DIR"

	BackendFilesystem = "filesystem"
)

// Version is set by the CLI at startup for cache busting (BUG-30).
var Version = "dev"

// NewStore creates a Store based on environment configuration.
// Default: filesystem backend wrapped in LRU.
func NewStore() store.Store {
	return NewStoreWithConfig(resource.LoadConfig())
}

// NewStoreWithConfig creates a Store using explicit resource configuration.
func NewStoreWithConfig(cfg resource.Config) store.Store {
	sc := cache.NewVersioned(envOr(EnvCacheDir, cache.DefaultCacheDir()), Version)
	histDir := envOr(EnvHistoryDir, history.DefaultHistoryDir())
	fs := store.NewFilesystem(sc, histDir)
	lruCap := cfg.LRUCapacity
	if lruCap <= 0 {
		lruCap = store.DefaultLRUCapacity
	}
	return store.NewLRU(fs, lruCap)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
