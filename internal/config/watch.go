package config

import (
	"context"
	"log"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// 编辑器 (如 vim) 原子保存时会先 rename 原文件再创建新文件, 文件级 watch 会
// 因 inode 变化而失效; 这里改用目录 watch, 并用 debounce + 重试吸收保存瞬间
// 文件短暂缺失的窗口。
const (
	reloadDebounce = 200 * time.Millisecond
	reloadRetries  = 3
	reloadBackoff  = 100 * time.Millisecond
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

	// 监听配置文件所在目录而非文件本身: 编辑器 (如 vim) 保存时先 rename 再
	// 重建文件, 文件级 watch 会随 inode 变化而失效, 目录 watch 则持续存活。
	if err := w.Add(filepath.Dir(m.path)); err != nil {
		return err
	}

	var debounce <-chan time.Time
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-w.Events:
			if !ok {
				return nil
			}
			if ev.Name != m.path {
				continue
			}
			if ev.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) == 0 {
				continue
			}
			// 一次保存会触发 rename/create/write 多个事件, 合并后只重载一次。
			if debounce == nil {
				debounce = time.After(reloadDebounce)
			}
		case <-debounce:
			debounce = nil
			m.reload()
		case err, ok := <-w.Errors:
			if !ok {
				return nil
			}
			log.Printf("config watcher error: %v", err)
		}
	}
}

// loadWithRetry 读取配置; 编辑器原子保存时文件会短暂缺失, 重试几次再放弃。
func (m *Manager) loadWithRetry() (*Config, error) {
	cfg, err := Load(m.path)
	if err == nil {
		return cfg, nil
	}
	for i := 0; i < reloadRetries; i++ {
		time.Sleep(reloadBackoff << i)
		cfg, err = Load(m.path)
		if err == nil {
			return cfg, nil
		}
	}
	return nil, err
}

func (m *Manager) reload() {
	cfg, err := m.loadWithRetry()
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
