package types

// Snapshot is the full aggregated state pushed to clients over WS / REST.
type Snapshot struct {
	TS         int64                     `json:"ts"`
	System     *SystemStats              `json:"system,omitempty"`
	Services   []ServiceStatus           `json:"services"`
	Containers map[string]ContainerState `json:"containers,omitempty"`
}

type SystemStats struct {
	Hostname string      `json:"hostname"`
	Uptime   uint64      `json:"uptime"`
	CPU      CPUStats    `json:"cpu"`
	Memory   MemoryStats `json:"memory"`
	Load     LoadStats   `json:"load"`
	Disks    []DiskStats `json:"disks"`
	Net      []NetStats  `json:"net"`
}

type CPUStats struct {
	Percent float64   `json:"percent"`
	Cores   []float64 `json:"cores"`
	Count   int       `json:"count"`
}

type MemoryStats struct {
	Total     uint64  `json:"total"`
	Used      uint64  `json:"used"`
	Available uint64  `json:"available"`
	Percent   float64 `json:"percent"`
}

type LoadStats struct {
	Load1  float64 `json:"load1"`
	Load5  float64 `json:"load5"`
	Load15 float64 `json:"load15"`
}

type DiskStats struct {
	Mount   string  `json:"mount"`
	Device  string  `json:"device"`
	FSType  string  `json:"fstype"`
	Total   uint64  `json:"total"`
	Used    uint64  `json:"used"`
	Free    uint64  `json:"free"`
	Percent float64 `json:"percent"`
}

type NetStats struct {
	Name      string  `json:"name"`
	BytesRecv uint64  `json:"bytes_recv"`
	BytesSent uint64  `json:"bytes_sent"`
	RecvRate  float64 `json:"recv_rate"` // bytes/s
	SentRate  float64 `json:"sent_rate"` // bytes/s
	ErrIn     uint64  `json:"err_in"`
	ErrOut    uint64  `json:"err_out"`
}

type ServiceStatus struct {
	Name      string         `json:"name"`
	Type      string         `json:"type"`
	Status    string         `json:"status"` // up | down | error
	LatencyMS float64        `json:"latency_ms"`
	LastError string         `json:"last_error,omitempty"`
	Extracted map[string]any `json:"extracted,omitempty"`
	Container *ContainerState `json:"container,omitempty"`
	TS        int64          `json:"ts"`
}

type ContainerState struct {
	Name         string  `json:"name"`
	ID           string  `json:"id"`
	Image        string  `json:"image"`
	State        string  `json:"state"`
	RestartCount int     `json:"restart_count"`
	CPUPerc      float64 `json:"cpu_perc"`
	MemUsage     uint64  `json:"mem_usage"`
	MemLimit     uint64  `json:"mem_limit"`
	MemPerc      float64 `json:"mem_perc"`
	NetRx        uint64  `json:"net_rx"`
	NetTx        uint64  `json:"net_tx"`
	RxRate       float64 `json:"rx_rate"`
	TxRate       float64 `json:"tx_rate"`
	Error        string  `json:"error,omitempty"`
}