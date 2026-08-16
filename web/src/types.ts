export interface Snapshot {
  ts: number
  system?: SystemStats
  services: ServiceStatus[]
  containers?: Record<string, ContainerState>
}

export interface SystemStats {
  hostname: string
  uptime: number
  cpu: { percent: number; cores: number[]; count: number }
  memory: { total: number; used: number; available: number; percent: number }
  load: { load1: number; load5: number; load15: number }
  disks: { mount: string; device: string; fstype: string; total: number; used: number; free: number; percent: number }[]
  net: { name: string; bytes_recv: number; bytes_sent: number; recv_rate: number; sent_rate: number; err_in: number; err_out: number }[]
}

export interface ServiceStatus {
  name: string
  type: string
  status: 'up' | 'down' | 'error'
  latency_ms: number
  last_error?: string
  extracted?: Record<string, string | number>
  container?: ContainerState
  ts: number
}

export interface ContainerState {
  name: string
  id: string
  image: string
  state: string
  restart_count: number
  cpu_perc: number
  mem_usage: number
  mem_limit: number
  mem_perc: number
  net_rx: number
  net_tx: number
  rx_rate: number
  tx_rate: number
  error?: string
}

export interface ReloadEvent {
  ts: number
  ok: boolean
  error?: string
  version: number
}

export interface WsMessage<T = unknown> {
  type: 'snapshot' | 'reload' | 'ping'
  data: T
}

export function fmtBytes(n: number): string {
  if (!n && n !== 0) return '-'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let i = 0
  let v = n
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(v >= 100 || i === 0 ? 0 : 1)} ${units[i]}`
}

export function fmtRate(n: number): string {
  return `${fmtBytes(n)}/s`
}

export function fmtUptime(sec: number): string {
  const d = Math.floor(sec / 86400)
  const h = Math.floor((sec % 86400) / 3600)
  const m = Math.floor((sec % 3600) / 60)
  return d > 0 ? `${d}d ${h}h ${m}m` : h > 0 ? `${h}h ${m}m` : `${m}m`
}

export function fmtTime(ms: number): string {
  return new Date(ms).toLocaleTimeString('zh-CN', { hour12: false })
}