package system

import (
	"context"
	"log"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"

	"dashboard/internal/types"
)

type Collector struct {
	disks     []string
	netIfaces []string
	prevNet   map[string]*net.IOCountersStat
	prevNetTs time.Time
}

func New(disks, netIfaces []string) *Collector {
	return &Collector{disks: disks, netIfaces: netIfaces, prevNet: map[string]*net.IOCountersStat{}}
}

func (c *Collector) Collect(ctx context.Context) *types.SystemStats {
	now := time.Now()
	s := &types.SystemStats{}

	if hi, err := host.InfoWithContext(ctx); err == nil {
		s.Hostname = hi.Hostname
		s.Uptime = hi.Uptime
	}

	if vm, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		s.Memory = types.MemoryStats{
			Total:     vm.Total,
			Used:      vm.Used,
			Available: vm.Available,
			Percent:   vm.UsedPercent,
		}
	}

	if la, err := load.AvgWithContext(ctx); err == nil {
		s.Load = types.LoadStats{Load1: la.Load1, Load5: la.Load5, Load15: la.Load15}
	}

	if percs, err := cpu.PercentWithContext(ctx, 0, true); err == nil {
		total := 0.0
		for _, p := range percs {
			total += p
		}
		s.CPU.Cores = percs
		s.CPU.Count = len(percs)
		if len(percs) > 0 {
			s.CPU.Percent = total / float64(len(percs))
		}
	} else if err != nil {
		log.Printf("system: cpu: %v", err)
	}

	if info, err := cpu.InfoWithContext(ctx); err == nil && s.CPU.Count == 0 {
		s.CPU.Count = len(info)
	}

	s.Disks = c.collectDisks(ctx)
	s.Net = c.collectNet(ctx, now)
	return s
}

func (c *Collector) collectDisks(ctx context.Context) []types.DiskStats {
	var out []types.DiskStats
	parts, err := disk.PartitionsWithContext(ctx, false)
	if err != nil {
		log.Printf("system: disk partitions: %v", err)
		return out
	}
	for _, p := range parts {
		if len(c.disks) > 0 && !contains(c.disks, p.Mountpoint) {
			continue
		}
		u, err := disk.UsageWithContext(ctx, p.Mountpoint)
		if err != nil {
			continue
		}
		out = append(out, types.DiskStats{
			Mount:   p.Mountpoint,
			Device:  p.Device,
			FSType:  p.Fstype,
			Total:   u.Total,
			Used:    u.Used,
			Free:    u.Free,
			Percent: u.UsedPercent,
		})
	}
	return out
}

func (c *Collector) collectNet(ctx context.Context, now time.Time) []types.NetStats {
	counters, err := net.IOCountersWithContext(ctx, true)
	if err != nil {
		log.Printf("system: net: %v", err)
		return nil
	}
	if c.prevNetTs.IsZero() {
		c.prevNetTs = now
	}
	elapsed := now.Sub(c.prevNetTs).Seconds()
	if elapsed <= 0 {
		elapsed = 1
	}

	var out []types.NetStats
	for _, n := range counters {
		if len(c.netIfaces) > 0 && !contains(c.netIfaces, n.Name) {
			continue
		}
		st := types.NetStats{
			Name:      n.Name,
			BytesRecv: n.BytesRecv,
			BytesSent: n.BytesSent,
			ErrIn:     n.Errin,
			ErrOut:    n.Errout,
		}
		if prev, ok := c.prevNet[n.Name]; ok {
			recvDelta := diffUint64(n.BytesRecv, prev.BytesRecv)
			sentDelta := diffUint64(n.BytesSent, prev.BytesSent)
			st.RecvRate = float64(recvDelta) / elapsed
			st.SentRate = float64(sentDelta) / elapsed
		}
		out = append(out, st)
		cp := n
		c.prevNet[n.Name] = &cp
	}
	c.prevNetTs = now
	return out
}

func diffUint64(a, b uint64) uint64 {
	if a >= b {
		return a - b
	}
	return b - a
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
