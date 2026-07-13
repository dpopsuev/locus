package mcp

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/dpopsuev/oculus/v3/engine"
	"github.com/dpopsuev/oculus/v3/lang"
	"github.com/fsnotify/fsnotify"
)

// WorkspaceWatcher watches roots for source-file changes, debounces, and
// marks the engine dirty + flushes SG — without auto-scanning.
type WorkspaceWatcher struct {
	eng      *engine.Engine
	debounce time.Duration
	warmIdle time.Duration

	mu       sync.Mutex
	timers   map[string]*time.Timer
	warmers  map[string]*time.Timer
	watcher  *fsnotify.Watcher
	stop     chan struct{}
	stopped  sync.Once
	onDirty  func(path string) // test spy
}

// StartWorkspaceWatcher begins watching roots. Returns nil watcher if eng is nil
// or no roots exist. Caller must Close.
func StartWorkspaceWatcher(eng *engine.Engine, roots []string) *WorkspaceWatcher {
	if eng == nil || len(roots) == 0 {
		return nil
	}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		slog.LogAttrs(context.TODO(), slog.LevelWarn, "workspace watcher unavailable", slog.Any(logKeyError, err))
		return nil
	}
	ww := &WorkspaceWatcher{
		eng:      eng,
		debounce: 500 * time.Millisecond,
		warmIdle: 2 * time.Second,
		timers:   map[string]*time.Timer{},
		warmers:  map[string]*time.Timer{},
		watcher:  w,
		stop:     make(chan struct{}),
	}
	for _, root := range roots {
		_ = ww.addTree(root)
	}
	go ww.loop()
	return ww
}

func (ww *WorkspaceWatcher) addTree(root string) error {
	abs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	return filepath.WalkDir(abs, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if path != abs && lang.ShouldSkipDir(name) {
				return filepath.SkipDir
			}
			_ = ww.watcher.Add(path)
		}
		return nil
	})
}

func (ww *WorkspaceWatcher) loop() {
	for {
		select {
		case <-ww.stop:
			return
		case ev, ok := <-ww.watcher.Events:
			if !ok {
				return
			}
			if ev.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) == 0 {
				continue
			}
			if shouldIgnoreWatchPath(ev.Name) {
				continue
			}
			root := ww.matchRoot(ev.Name)
			if root == "" {
				continue
			}
			ww.schedule(root)
		case err, ok := <-ww.watcher.Errors:
			if !ok {
				return
			}
			slog.LogAttrs(context.TODO(), slog.LevelDebug, "watch error", slog.Any(logKeyError, err))
		}
	}
}

func shouldIgnoreWatchPath(name string) bool {
	base := filepath.Base(name)
	if strings.HasPrefix(base, ".") {
		return true
	}
	ext := strings.ToLower(filepath.Ext(base))
	switch ext {
	case ".go", ".ts", ".tsx", ".js", ".jsx", ".py", ".rs", ".java",
		".mod", ".sum", ".toml", ".json":
		return false
	}
	return true
}

func (ww *WorkspaceWatcher) matchRoot(file string) string {
	abs, _ := filepath.Abs(file)
	best := ""
	for _, w := range ww.eng.Workspaces() {
		wa, _ := filepath.Abs(w)
		if strings.HasPrefix(abs, wa+string(os.PathSeparator)) || abs == wa {
			if len(wa) > len(best) {
				best = wa
			}
		}
	}
	return best
}

func (ww *WorkspaceWatcher) schedule(root string) {
	ww.mu.Lock()
	defer ww.mu.Unlock()
	if t, ok := ww.timers[root]; ok {
		t.Stop()
	}
	ww.timers[root] = time.AfterFunc(ww.debounce, func() {
		ww.eng.MarkDirty(root)
		if ww.onDirty != nil {
			ww.onDirty(root)
		}
		slog.LogAttrs(context.TODO(), slog.LevelDebug, "workspace dirty", slog.String(logKeyPath, root))
		ww.scheduleWarm(root)
	})
}

func (ww *WorkspaceWatcher) scheduleWarm(root string) {
	ww.mu.Lock()
	defer ww.mu.Unlock()
	if t, ok := ww.warmers[root]; ok {
		t.Stop()
	}
	ww.warmers[root] = time.AfterFunc(ww.warmIdle, func() {
		if ww.eng.Pool() == nil {
			return
		}
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			_ = ww.eng.WarmLSP(ctx, root)
		}()
	})
}

// InjectDirty is for tests — simulates a debounced dirty event.
func (ww *WorkspaceWatcher) InjectDirty(root string) {
	if ww == nil {
		return
	}
	ww.schedule(root)
}

// Close stops the watcher.
func (ww *WorkspaceWatcher) Close() {
	if ww == nil {
		return
	}
	ww.stopped.Do(func() {
		if ww.stop != nil {
			close(ww.stop)
		}
		if ww.watcher != nil {
			_ = ww.watcher.Close()
		}
	})
}
