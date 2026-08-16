package config

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// ReloadEvent describes the outcome of a config hot-reload.
type ReloadEvent struct {
	TS      int64  `json:"ts"`
	OK      bool   `json:"ok"`
	Error   string `json:"error,omitempty"`
	Version int    `json:"version"`
}

// Manager loads a YAML config, watches it with fsnotify and calls the
// onReload callback whenever the file changes (validating before applying).
type Manager struct {
	path    string
	mu      sync.RWMutex
	cfg     *Config
	onLoad  func(*Config)
	onError func(ReloadEvent)
	version int
}

func NewManager(path string, onLoad func(*Config), onError func(ReloadEvent)) (*Manager, error) {
	m := &Manager{path: path, onLoad: onLoad, onError: onError}
	cfg, err := Load(path)
	if err != nil {
		return nil, err
	}
	m.cfg = cfg
	m.version = 1
	onLoad(cfg)
	return m, nil
}

func (m *Manager) Get() *Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cfg
}

func (m *Manager) Version() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.version
}

func (m *Manager) Start(ctx context.Context) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer w.Close()

	if err := w.Add(m.path); err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-w.Events:
			if !ok {
				return nil
			}
			if ev.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) == 0 {
				continue
			}
			// editors often rename/recreate the file; re-add in case the inode changed.
			_ = w.Add(m.path)
			m.reload()
		case err, ok := <-w.Errors:
			if !ok {
				return nil
			}
			log.Printf("config watcher error: %v", err)
		}
	}
}

func (m *Manager) reload() {
	cfg, err := Load(m.path)
	ev := ReloadEvent{TS: timeNow(), Version: m.version}
	if err != nil {
		ev.OK = false
		ev.Error = err.Error()
		if m.onError != nil {
			m.onError(ev)
		}
		log.Printf("config reload FAILED: %v", err)
		return
	}
	m.mu.Lock()
	m.cfg = cfg
	m.version++
	ev.OK = true
	ev.Version = m.version
	m.mu.Unlock()
	if m.onLoad != nil {
		m.onLoad(cfg)
	}
	if m.onError != nil {
		m.onError(ev)
	}
	log.Printf("config reloaded (version %d)", m.version)
}

func timeNow() int64 { return time.Now().UnixMilli() }