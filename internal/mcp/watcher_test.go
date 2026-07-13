package mcp

import (
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	oculuscache "github.com/dpopsuev/oculus/v3/cache"
	"github.com/dpopsuev/oculus/v3/engine"
	"github.com/dpopsuev/locus/internal/store"
)

func TestWorkspaceWatcher_InjectDirty(t *testing.T) {
	dir := t.TempDir()
	sc := oculuscache.New(t.TempDir())
	db := store.NewFilesystem(sc, t.TempDir())
	eng := engine.New(db, []string{dir})

	ww := &WorkspaceWatcher{
		eng:      eng,
		debounce: 20 * time.Millisecond,
		warmIdle: time.Hour,
		timers:   map[string]*time.Timer{},
		warmers:  map[string]*time.Timer{},
		stop:     make(chan struct{}),
	}
	var called atomic.Int32
	ww.onDirty = func(path string) {
		called.Add(1)
		if filepath.Clean(path) != filepath.Clean(dir) {
			t.Errorf("dirty path=%q", path)
		}
	}
	ww.InjectDirty(dir)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if called.Load() >= 1 && eng.IsDirty(dir) {
			ww.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("dirty not signaled")
}

func TestShouldIgnoreWatchPath(t *testing.T) {
	if shouldIgnoreWatchPath("foo.go") {
		t.Fatal(".go should watch")
	}
	if !shouldIgnoreWatchPath("foo.txt") {
		t.Fatal(".txt should ignore")
	}
}
