package collector

import (
	"context"
	"log"
	"sync"
	"time"

	"dashboard/internal/config"
	"dashboard/internal/container"
	"dashboard/internal/probe"
	"dashboard/internal/system"
	"dashboard/internal/types"
	"dashboard/internal/ws"
)

type Collector struct {
	cfgMgr *config.Manager
	hub    *ws.Hub

	mu         sync.RWMutex
	sys        *system.Collector
	ctr        *container.Client
	ctrCfg     config.ContainerConfig
	probes     []*serviceProbe
	interval   time.Duration
	baseCtx    context.Context
	taskCancel context.CancelFunc
	pool       *pool
}

// pool 是采集结果缓存池: 调度器异步采集后写入, 推送器组帧时读取。
// 推送永不等待采集 —— 读到的总是最近一轮已完成的结果。
type pool struct {
	mu         sync.RWMutex
	system     *types.SystemStats
	containers map[string]types.ContainerState
	services   map[string]types.ServiceStatus
}

func newPool() *pool {
	return &pool{
		containers: map[string]types.ContainerState{},
		services:   map[string]types.ServiceStatus{},
	}
}

func (p *pool) setSystem(s *types.SystemStats) {
	p.mu.Lock()
	p.system = s
	p.mu.Unlock()
}

func (p *pool) setContainers(m map[string]types.ContainerState) {
	p.mu.Lock()
	p.containers = m
	p.mu.Unlock()
}

func (p *pool) setService(name string, s types.ServiceStatus) {
	p.mu.Lock()
	p.services[name] = s
	p.mu.Unlock()
}

// snapshot 返回读快照 (map 做拷贝), 供组帧使用, 避免与外层写并发。
func (p *pool) snapshot() (system *types.SystemStats, containers map[string]types.ContainerState, services map[string]types.ServiceStatus) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	system = p.system
	containers = make(map[string]types.ContainerState, len(p.containers))
	for k, v := range p.containers {
		containers[k] = v
	}
	services = make(map[string]types.ServiceStatus, len(p.services))
	for k, v := range p.services {
		services[k] = v
	}
	return
}

type serviceProbe struct {
	cfg   config.ServiceConfig
	probe probe.Probe
	ref   *config.ContainerRef

	mu         sync.Mutex
	lastRun    time.Time
	lastStatus types.ServiceStatus
	firstRun   bool
}

func New(initial *config.Config, hub *ws.Hub) (*Collector, error) {
	c := &Collector{hub: hub, pool: newPool()}
	if err := c.apply(initial, true); err != nil {
		return nil, err
	}
	return c, nil
}

// apply (re)builds collectors and probes from cfg. On reload the same
// instance is reused so probe history survives.
func (c *Collector) apply(cfg *config.Config, initial bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.interval = cfg.Server.Interval.Std()

	if cfg.System.Enabled {
		c.sys = system.New(cfg.System.Disks, cfg.System.NetInterfaces)
	} else {
		c.sys = nil
	}

	if cfg.Container.Enabled {
		if c.ctr == nil || c.ctr.DaemonHost() != cfg.Container.Endpoint {
			cli, err := container.New(cfg.Container.Endpoint, cfg.Container.Containers)
			if err != nil {
				if initial {
					return err
				}
				log.Printf("collector: container runtime unavailable: %v", err)
			} else {
				c.ctr = cli
			}
		} else {
			_ = c.ctr.Reconfigure(cfg.Container.Endpoint, cfg.Container.Containers)
		}
	} else {
		c.ctr = nil
	}
	c.ctrCfg = cfg.Container

	// rebuild probes
	newProbes := make([]*serviceProbe, 0, len(cfg.Services))
	for _, sc := range cfg.Services {
		sc.Defaults(cfg.Server)
		if err := sc.Validate(); err != nil {
			log.Printf("collector: skipping %v", err)
			continue
		}
		p, err := probe.Compile(sc)
		if err != nil {
			log.Printf("collector: skipping service %q: %v", sc.Name, err)
			continue
		}
		newProbes = append(newProbes, &serviceProbe{
			cfg:   sc,
			probe: p,
			ref:   sc.Container,
		})
	}
	c.probes = newProbes
	return nil
}

func (c *Collector) Reload(cfg *config.Config) {
	c.mu.RLock()
	baseCtx := c.baseCtx
	c.mu.RUnlock()

	if err := c.apply(cfg, false); err != nil {
		log.Printf("collector: reload apply failed: %v", err)
		return
	}

	if baseCtx == nil {
		return
	}
	// 立即用新配置补一轮缓存, 避免新探针在首个 tick 前显示 pending
	go c.round(baseCtx)
	c.restartTasks(baseCtx)
}

// Run 启动采集调度器与推送器。推送节奏严格跟随 server.interval,
// 采集异步进行, 慢探针只会让对应缓存项陈旧而不会阻塞广播。
func (c *Collector) Run(ctx context.Context) {
	c.mu.Lock()
	c.baseCtx = ctx
	c.mu.Unlock()

	// 首轮同步采集, 保证首帧不空
	c.round(ctx)

	c.restartTasks(ctx)

	c.mu.RLock()
	iv := c.interval
	c.mu.RUnlock()
	push := time.NewTicker(iv)
	defer push.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-push.C:
			c.mu.RLock()
			pushIv := c.interval
			c.mu.RUnlock()
			push.Reset(pushIv)
			c.hub.Broadcast(c.buildSnapshot())
		}
	}
}

// restartTasks 取消上一批采集任务, 并按当前配置重新派发。
func (c *Collector) restartTasks(ctx context.Context) {
	c.mu.Lock()
	if c.taskCancel != nil {
		c.taskCancel()
	}
	taskCtx, cancel := context.WithCancel(ctx)
	c.taskCancel = cancel
	c.mu.Unlock()

	c.startTasks(taskCtx)
}

// round 同步采集一轮写入缓存池。
func (c *Collector) round(ctx context.Context) {
	c.mu.RLock()
	sys := c.sys
	ctr := c.ctr
	interval := c.interval
	probes := c.probes
	c.mu.RUnlock()

	if sys != nil {
		c.pool.setSystem(sys.Collect(ctx))
	}
	if ctr != nil {
		c.pool.setContainers(ctr.Collect(ctx, interval))
	}
	for _, sp := range probes {
		c.pool.setService(sp.cfg.Name, sp.poll(ctx, interval))
	}
}

// startTasks 按各自的采集周期派发异步任务; 完成后写入缓存池, 推送器不等待。
func (c *Collector) startTasks(ctx context.Context) {
	c.mu.RLock()
	sys := c.sys
	ctr := c.ctr
	interval := c.interval
	probes := c.probes
	c.mu.RUnlock()

	if sys != nil {
		go c.systemTask(ctx, sys, interval)
	}
	if ctr != nil {
		go c.containerTask(ctx, ctr, interval)
	}
	for _, sp := range probes {
		iv := sp.cfg.Interval.Std()
		if iv <= 0 {
			iv = interval
		}
		go c.serviceTask(ctx, sp, interval, iv)
	}
}

func (c *Collector) systemTask(ctx context.Context, sys *system.Collector, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			c.pool.setSystem(sys.Collect(ctx))
		}
	}
}

func (c *Collector) containerTask(ctx context.Context, ctr *container.Client, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			c.pool.setContainers(ctr.Collect(ctx, interval))
		}
	}
}

func (c *Collector) serviceTask(ctx context.Context, sp *serviceProbe, interval, iv time.Duration) {
	t := time.NewTicker(iv)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			c.pool.setService(sp.cfg.Name, sp.poll(ctx, interval))
		}
	}
}

// buildSnapshot 从缓存池组帧: 按配置顺序组装服务, TS 代表推送时刻。
func (c *Collector) buildSnapshot() *types.Snapshot {
	c.mu.RLock()
	probes := c.probes
	ctrCfg := c.ctrCfg
	c.mu.RUnlock()

	system, containers, services := c.pool.snapshot()

	snap := &types.Snapshot{
		TS:         time.Now().UnixMilli(),
		System:     system,
		Containers: containers,
		Services:   make([]types.ServiceStatus, 0, len(probes)),
	}
	for i, sp := range probes {
		st, ok := services[sp.cfg.Name]
		if !ok {
			st = types.ServiceStatus{Name: sp.cfg.Name, Type: sp.cfg.Type, Status: "pending"}
		}

		ref := probes[i].ref
		if len(containers) > 0 && ctrCfg.Enabled && ref != nil && ref.IsEnabled() {
			cs, ok := containers[ref.Name]
			if !ok {
				cs = types.ContainerState{Name: ref.Name, State: "unknown", Error: "container not found"}
			}
			cc := cs
			st.Container = &cc
		}
		snap.Services = append(snap.Services, st)
	}
	return snap
}

func (sp *serviceProbe) poll(ctx context.Context, defaultInterval time.Duration) types.ServiceStatus {
	sp.mu.Lock()
	defer sp.mu.Unlock()

	iv := sp.cfg.Interval.Std()
	if iv <= 0 {
		iv = defaultInterval
	}
	now := time.Now()
	due := sp.firstRun || now.Sub(sp.lastRun) >= iv

	if !due {
		return sp.lastStatus
	}
	sp.firstRun = false

	res := sp.probe.Run(ctx)
	st := types.ServiceStatus{
		Name:      sp.cfg.Name,
		Type:      sp.cfg.Type,
		Status:    res.Status,
		LatencyMS: res.LatencyMS,
		Extracted: res.Extracted,
		TS:        now.UnixMilli(),
	}
	if res.Err != nil {
		st.Status = "error"
		st.LastError = res.Err.Error()
	}
	sp.lastRun = now
	sp.lastStatus = st
	return st
}
