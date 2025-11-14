package events

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// StartFSWatcher watches the workspaces root for external file changes (not going through API/MCP)
// and publishes normalized WorkspaceEvents to the hub.
// Best-effort recursive watching: we watch each workspace root directory under root, and dynamically
// add watchers for newly created top-level workspace directories. For nested subdirectories, we
// attempt to detect creation and add a watcher lazily; however, this is not guaranteed on all OSes.
// This is an MVP that covers most typical workflows for small workspaces.
func StartFSWatcher(root string, hub *Hub) (func(), error) {
	if hub == nil {
		return func() {}, nil
	}

	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	// Track watched directories to avoid duplicate Add calls
	var mu sync.Mutex
	watched := map[string]struct{}{}

	addWatch := func(dir string) {
		mu.Lock()
		defer mu.Unlock()
		if _, ok := watched[dir]; ok {
			return
		}
		if err := w.Add(dir); err != nil {
			slog.Debug("fswatch: failed to add watcher", "dir", dir, "error", err)
			return
		}
		watched[dir] = struct{}{}
		slog.Debug("fswatch: watching dir", "dir", dir)
	}

	// Seed: watch the workspaces root and each top-level workspace directory
	addWatch(root)
	entries, _ := os.ReadDir(root)
	for _, e := range entries {
		if e.IsDir() {
			addWatch(filepath.Join(root, e.Name()))
		}
	}

	// Debounce burst events (per workspace/path key)
	type key struct {
		wsID string
		path string
		typ  string
	}
	var debMu sync.Mutex
	debounced := map[key]time.Time{}
	const debounceWindow = 200 * time.Millisecond

	flush := func(wsID, relPath, evtType string, isDir bool) {
		if wsID == "" || relPath == "" {
			return
		}
		// Ignore protected names
		if isProtectedName(filepath.Base(relPath)) {
			return
		}
		hub.Publish(wsID, WorkspaceEvent{
			Type:  evtType,
			Path:  relPath,
			IsDir: isDir,
			Actor: &Actor{Kind: "fswatch"},
		})
	}

	coalescer := time.NewTicker(100 * time.Millisecond)
	stop := make(chan struct{})

	go func() {
		defer coalescer.Stop()
		for {
			select {
			case <-coalescer.C:
				now := time.Now()
				var toSend []key
				debMu.Lock()
				for k, t := range debounced {
					if now.Sub(t) >= debounceWindow {
						toSend = append(toSend, k)
						delete(debounced, k)
					}
				}
				debMu.Unlock()
				for _, k := range toSend {
					flush(k.wsID, k.path, k.typ, strings.HasSuffix(k.typ, ".created") || strings.HasSuffix(k.typ, ".deleted") && strings.HasSuffix(strings.ToLower(k.path), "/"))
				}
			case <-stop:
				return
			}
		}
	}()

	// Helper to map an absolute path to (workspaceID, relPath)
	splitPath := func(abs string) (string, string) {
		// abs should be within root
		if !strings.HasPrefix(abs, root) {
			return "", ""
		}
		relToRoot, err := filepath.Rel(root, abs)
		if err != nil || relToRoot == "." {
			return "", ""
		}
		parts := strings.Split(relToRoot, string(os.PathSeparator))
		if len(parts) == 0 {
			return "", ""
		}
		wsID := parts[0]
		rel := strings.Join(parts[1:], string(os.PathSeparator))
		return wsID, rel
	}

	go func() {
		for {
			select {
			case ev, ok := <-w.Events:
				if !ok {
					return
				}
				// Dynamically add watchers for newly created directories (best-effort)
				if ev.Op&fsnotify.Create == fsnotify.Create {
					if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
						addWatch(ev.Name)
					}
				}

				wsID, rel := splitPath(ev.Name)
				if wsID == "" || rel == "" {
					// It may be a create of a new top-level workspace directory
					if ev.Op&fsnotify.Create == fsnotify.Create {
						// attempt to add watch
						if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
							addWatch(ev.Name)
						}
					}
					continue
				}

				// Classify event
				evtType := ""
				isDir := false
				// Try to stat to determine if dir (may fail for remove)
				if info, err := os.Stat(ev.Name); err == nil {
					isDir = info.IsDir()
				}
				switch {
				case ev.Op&fsnotify.Create == fsnotify.Create:
					if isDir {
						evtType = "dir.created"
					} else {
						evtType = "file.created"
					}
				case ev.Op&fsnotify.Remove == fsnotify.Remove:
					if isDir {
						evtType = "dir.deleted"
					} else {
						evtType = "file.deleted"
					}
				case ev.Op&fsnotify.Rename == fsnotify.Rename:
					// Without destination we treat as delete; actual move may be observed as create elsewhere
					if isDir {
						evtType = "dir.deleted"
					} else {
						evtType = "file.deleted"
					}
				case ev.Op&fsnotify.Write == fsnotify.Write || ev.Op&fsnotify.Chmod == fsnotify.Chmod:
					evtType = "file.updated"
				default:
					continue
				}

				// Debounce publishing bursts
				debMu.Lock()
				debounced[key{wsID: wsID, path: rel, typ: evtType}] = time.Now()
				debMu.Unlock()

			case err, ok := <-w.Errors:
				if !ok {
					return
				}
				slog.Debug("fswatch: watcher error", "error", err)
			}
		}
	}()

	stopFn := func() {
		close(stop)
		_ = w.Close()
	}
	return stopFn, nil
}

// local copy of protected name logic; keep in sync with mcpsdk/tools.go
func isProtectedName(name string) bool {
	return name == ".git" || name == ".gitkeep"
}
