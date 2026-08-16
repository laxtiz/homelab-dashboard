package container

import (
	"context"
	"encoding/json"
	"io"
	"sync"
	"time"

	"github.com/moby/moby/api/types/container"
	dockerclient "github.com/moby/moby/client"

	"dashboard/internal/config"
	dash "dashboard/internal/types"
)

// Client talks to any Docker-Engine-API compatible runtime socket
// (docker, podman --compat, etc). Endpoint resolution order:
//  1. config endpoint
//  2. $DOCKER_HOST
//  3. unix:///var/run/docker.sock
type Client struct {
	mu      sync.RWMutex
	cli     *dockerclient.Client
	filters []config.ContainerFilter
	prevs   map[string]*prevStats
}

type prevStats struct {
	cpuTotal uint64
	sysTotal uint64
	netRx    uint64
	netTx    uint64
}

func New(endpoint string, filters []config.ContainerFilter) (*Client, error) {
	cli, err := dial(endpoint)
	if err != nil {
		return nil, err
	}
	return &Client{cli: cli, filters: filters, prevs: map[string]*prevStats{}}, nil
}

func dial(endpoint string) (*dockerclient.Client, error) {
	opts := []dockerclient.Opt{dockerclient.FromEnv}
	if endpoint != "" {
		opts = append(opts, dockerclient.WithHost(endpoint))
	}
	cli, err := dockerclient.NewClientWithOpts(opts...)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := cli.Ping(ctx, dockerclient.PingOptions{}); err != nil {
		return nil, err
	}
	return cli, nil
}

func (c *Client) Reconfigure(endpoint string, filters []config.ContainerFilter) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if endpoint != c.cli.DaemonHost() {
		cli, err := dial(endpoint)
		if err != nil {
			return err
		}
		c.cli = cli
		c.prevs = map[string]*prevStats{}
	}
	c.filters = filters
	return nil
}

func (c *Client) DaemonHost() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.cli.DaemonHost()
}

// Collect returns a snapshot of all monitored containers. When no filters are
// configured every running container is monitored.
func (c *Client) Collect(ctx context.Context, elapsed time.Duration) map[string]dash.ContainerState {
	c.mu.RLock()
	cli := c.cli
	filters := c.filters
	c.mu.RUnlock()

	list, err := cli.ContainerList(ctx, dockerclient.ContainerListOptions{All: false})
	if err != nil {
		return map[string]dash.ContainerState{"_error": {
			Name:  "container-runtime",
			State: "error",
			Error: err.Error(),
		}}
	}

	cur := map[string]dash.ContainerState{}
	for _, ctr := range list.Items {
		if !c.matches(filters, ctr) {
			continue
		}
		st := dash.ContainerState{
			Name:  containerName(ctr),
			ID:    ctr.ID[:min(12, len(ctr.ID))],
			Image: ctr.Image,
			State: string(ctr.State),
		}
		if info, err := cli.ContainerInspect(ctx, ctr.ID, dockerclient.ContainerInspectOptions{}); err == nil {
			st.RestartCount = info.Container.RestartCount
		}
		c.applyStats(ctx, cli, ctr.ID, &st, elapsed)
		cur[st.Name] = st
	}

	c.mu.Lock()
	for name := range c.prevs {
		if _, ok := cur[name]; !ok {
			delete(c.prevs, name)
		}
	}
	c.mu.Unlock()
	return cur
}

func (c *Client) applyStats(ctx context.Context, cli *dockerclient.Client, id string, st *dash.ContainerState, elapsed time.Duration) {
	// 非流式 + 不含 previous sample: client 会附带 one-shot=true,
	// daemon 立即返回缓存样本 (~10ms)。否则 daemon 要采两个间隔 1s 的
	// 样本 (~2s/容器), 会拖垮整体采集节奏。
	resp, err := cli.ContainerStats(ctx, id, dockerclient.ContainerStatsOptions{
		Stream: false,
	})
	if err != nil {
		st.Error = err.Error()
		return
	}
	defer resp.Body.Close()

	var raw container.StatsResponse
	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(&raw); err != nil && err != io.EOF {
		st.Error = err.Error()
		return
	}

	st.MemUsage = raw.MemoryStats.Usage
	st.MemLimit = raw.MemoryStats.Limit
	if raw.MemoryStats.Limit > 0 {
		st.MemPerc = float64(raw.MemoryStats.Usage) / float64(raw.MemoryStats.Limit) * 100
	}

	var onlineCPU uint32
	if n := len(raw.CPUStats.CPUUsage.PercpuUsage); n > 0 {
		onlineCPU = uint32(n)
	} else {
		onlineCPU = raw.CPUStats.OnlineCPUs
	}

	// one-shot 返回时 precpu_stats 为空, 回退到本采集器上一轮缓存。
	prevTotal := raw.PreCPUStats.CPUUsage.TotalUsage
	prevSystem := raw.PreCPUStats.SystemUsage
	if prevTotal == 0 || prevSystem == 0 {
		// fall back to the previous cycle tracked by this collector
		c.mu.RLock()
		p := c.prevs[st.Name]
		c.mu.RUnlock()
		if p != nil {
			prevTotal = p.cpuTotal
			prevSystem = p.sysTotal
		}
	}
	cpuDelta := diff(raw.CPUStats.CPUUsage.TotalUsage, prevTotal)
	sysDelta := diff(raw.CPUStats.SystemUsage, prevSystem)
	if sysDelta > 0 && cpuDelta > 0 && onlineCPU > 0 {
		st.CPUPerc = float64(cpuDelta) / float64(sysDelta) * float64(onlineCPU) * 100
	}

	var rx, tx uint64
	for _, n := range raw.Networks {
		rx += n.RxBytes
		tx += n.TxBytes
	}
	st.NetRx = rx
	st.NetTx = tx

	secs := elapsed.Seconds()
	if secs <= 0 {
		secs = 1
	}
	c.mu.RLock()
	prevNet := c.prevs[st.Name]
	c.mu.RUnlock()
	if prevNet != nil {
		st.RxRate = float64(diff(rx, prevNet.netRx)) / secs
		st.TxRate = float64(diff(tx, prevNet.netTx)) / secs
	}

	c.mu.Lock()
	c.prevs[st.Name] = &prevStats{
		cpuTotal: raw.CPUStats.CPUUsage.TotalUsage,
		sysTotal: raw.CPUStats.SystemUsage,
		netRx:    rx,
		netTx:    tx,
	}
	c.mu.Unlock()
}

func (c *Client) matches(filters []config.ContainerFilter, ctr container.Summary) bool {
	if len(filters) == 0 {
		return true
	}
	name := containerName(ctr)
	for _, f := range filters {
		if f.Name != "" && f.Name == name {
			return true
		}
		if f.Label != "" {
			for k, v := range ctr.Labels {
				if k == f.Label || k+"="+v == f.Label {
					return true
				}
			}
		}
	}
	return false
}

func containerName(ctr container.Summary) string {
	if len(ctr.Names) > 0 {
		n := ctr.Names[0]
		if len(n) > 0 && n[0] == '/' {
			return n[1:]
		}
		return n
	}
	return ctr.ID
}

func diff(a, b uint64) uint64 {
	if a >= b {
		return a - b
	}
	return b - a
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
