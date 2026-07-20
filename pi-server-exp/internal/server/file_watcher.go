package server

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
)

// These directories are dependency, VCS, cache, or generated-output trees.
// Watching them is expensive and does not produce useful source-change signals.
var ignoredWatchDirs = map[string]struct{}{
	".git": {}, ".next": {}, ".turbo": {}, ".cache": {}, ".idea": {},
	"node_modules": {}, "dist": {}, "build": {}, "coverage": {}, "vendor": {},
}

func shouldWatchDir(path string) bool {
	_, ignored := ignoredWatchDirs[strings.ToLower(filepath.Base(path))]
	return !ignored
}

func (s *Server) ensureFileWatcher(p *PiProcess) {
	s.watchMu.Lock()
	if _, ok := s.watchers[p.id]; ok {
		s.watchMu.Unlock()
		return
	}
	s.watchMu.Unlock()

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		s.logger.Warn("file watcher failed", "session", p.id, "error", err)
		return
	}
	cwd := p.CWD()
	maxWatches := s.cfg.MaxWatches
	if maxWatches <= 0 {
		maxWatches = 2048
	}
	watchedPaths := make(map[string]bool)
	capped := false
	walkErr := filepath.WalkDir(cwd, func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return err
		}
		if path != cwd && !shouldWatchDir(path) {
			return filepath.SkipDir
		}
		if len(watchedPaths) >= maxWatches {
			capped = true
			return filepath.SkipDir
		}
		if err := watcher.Add(path); err != nil {
			s.logger.Debug("cannot watch directory", "session", p.id, "path", path, "error", err)
			return nil
		}
		watchedPaths[path] = true
		return nil
	})
	if walkErr != nil {
		s.logger.Warn("file watcher walk failed", "session", p.id, "error", walkErr)
	}
	if capped {
		s.logger.Warn("file watcher directory cap reached", "session", p.id, "maxWatches", maxWatches)
	}

	stop := make(chan struct{})
	s.watchMu.Lock()
	if _, exists := s.watchers[p.id]; exists {
		s.watchMu.Unlock()
		_ = watcher.Close()
		return
	}
	s.watchers[p.id] = func() { close(stop); _ = watcher.Close() }
	s.watchMu.Unlock()
	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Op&fsnotify.Create != 0 && len(watchedPaths) < maxWatches && shouldWatchDir(event.Name) {
					if d, err := filepath.Abs(event.Name); err == nil {
						if info, err := os.Stat(d); err == nil && info.IsDir() {
							if err := watcher.Add(d); err == nil {
								watchedPaths[d] = true
							}
						}
					}
				}
				if event.Op&fsnotify.Remove != 0 {
					if d, err := filepath.Abs(event.Name); err == nil {
						delete(watchedPaths, d)
					}
				}
				rel, err := filepath.Rel(cwd, event.Name)
				if err != nil {
					rel = event.Name
				}
				p.Emit(RPCEvent{"type": "file_change", "path": filepath.ToSlash(rel), "change": fileChange(event.Op)})
			case err, ok := <-watcher.Errors:
				if ok {
					s.logger.Warn("file watcher error", "session", p.id, "error", err)
				}
			case <-stop:
				return
			}
		}
	}()
}

func (s *Server) stopWatcher(sessionID string) {
	s.watchMu.Lock()
	stop := s.watchers[sessionID]
	delete(s.watchers, sessionID)
	s.watchMu.Unlock()
	if stop != nil {
		stop()
	}
}

func (s *Server) stopWatchers() {
	s.watchMu.Lock()
	stops := make([]func(), 0, len(s.watchers))
	for _, stop := range s.watchers {
		stops = append(stops, stop)
	}
	s.watchers = map[string]func(){}
	s.watchMu.Unlock()
	for _, stop := range stops {
		stop()
	}
}

func fileChange(op fsnotify.Op) string {
	if op&fsnotify.Create != 0 {
		return "created"
	}
	if op&fsnotify.Remove != 0 {
		return "deleted"
	}
	if op&fsnotify.Rename != 0 {
		return "renamed"
	}
	if op&fsnotify.Write != 0 {
		return "modified"
	}
	return strings.ToLower(op.String())
}
