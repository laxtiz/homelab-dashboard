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

	mu       sync.RWMutex
	sys      *system.Collector
	ctr      *container.Client
	ctrCfg   config.ContainerConfig
	probes   []*serviceProbe
	interval time.Duration
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
	c := &Collector{hub: hub}
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
	if err := c.apply(cfg, false); err != nil {
		log.Printf("collector: reload apply failed: %v", err)
	}
}

func (c *Collector) Run(ctx context.Context) {
	tick := time.NewTicker(c.interval)
	defer tick.Stop()

	// immediate first snapshot
	c.cycle(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			c.mu.RLock()
			iv := c.interval
			c.mu.RUnlock()
			tick.Reset(iv)
			c.cycle(ctx)
		}
	}
}

func (c *Collector) cycle(ctx context.Context) {
	c.mu.RLock()
	sys := c.sys
	ctr := c.ctr
	ctrCfg := c.ctrCfg
	probes := c.probes
	interval := c.interval
	c.mu.RUnlock()

	snap := &types.Snapshot{TS: time.Now().UnixMilli(), Services: []types.ServiceStatus{}}

	if sys != nil {
		snap.System = sys.Collect(ctx)
	}

	var containers map[string]types.ContainerState
	if ctr != nil {
		containers = ctr.Collect(ctx, interval)
		snap.Containers = containers
	}

	for _, sp := range probes {
		snap.Services = append(snap.Services, sp.poll(ctx, interval))
	}

	// merge container state into services
	if len(containers) > 0 && ctrCfg.Enabled {
		for i := range snap.Services {
			ref := probes[i].ref
			if ref == nil || !ref.IsEnabled() {
				continue
			}
			cs, ok := containers[ref.Name]
			if !ok {
				cs = types.ContainerState{Name: ref.Name, State: "unknown", Error: "container not found"}
			}
			cc := cs
			snap.Services[i].Container = &cc
		}
	}

	c.hub.Broadcast(snap)
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
