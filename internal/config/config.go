package config

import (
	"os"

	"github.com/dpopsuev/locus/internal/cache"
	"github.com/dpopsuev/locus/internal/history"
	"github.com/dpopsuev/locus/internal/store"
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
	backend := envOr(EnvStore, BackendFilesystem)

	switch backend {
	case BackendFilesystem:
		sc := cache.NewVersioned(envOr(EnvCacheDir, cache.DefaultCacheDir()), Version)
		histDir := envOr(EnvHistoryDir, history.DefaultHistoryDir())
		fs := store.NewFilesystem(sc, histDir)
		return store.NewLRU(fs, store.DefaultLRUCapacity)
	default:
		// Unknown backend falls back to filesystem.
		sc := cache.NewVersioned(envOr(EnvCacheDir, cache.DefaultCacheDir()), Version)
		histDir := envOr(EnvHistoryDir, history.DefaultHistoryDir())
		fs := store.NewFilesystem(sc, histDir)
		return store.NewLRU(fs, store.DefaultLRUCapacity)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
