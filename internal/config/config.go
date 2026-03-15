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

// NewStore creates a Store based on environment configuration.
// Default: filesystem backend wrapped in LRU.
func NewStore() store.Store {
	backend := envOr(EnvStore, BackendFilesystem)

	switch backend {
	case BackendFilesystem:
		sc := cache.New(envOr(EnvCacheDir, cache.DefaultCacheDir()))
		histDir := envOr(EnvHistoryDir, history.DefaultHistoryDir())
		fs := store.NewFilesystem(sc, histDir)
		return store.NewLRU(fs, store.DefaultLRUCapacity)
	default:
		// Unknown backend falls back to filesystem.
		sc := cache.New(envOr(EnvCacheDir, cache.DefaultCacheDir()))
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
