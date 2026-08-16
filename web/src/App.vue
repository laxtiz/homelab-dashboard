<script setup lang="ts">
import { onMounted, computed, ref, watch } from 'vue'
import { state, connect } from './ws'
import { fmtBytes, fmtRate, fmtUptime, fmtTime } from './types'
import Spark from './components/Spark.vue'
import StatusChip from './components/StatusChip.vue'

const MAX = 120
const cpuHist = ref<number[]>([])
const memHist = ref<number[]>([])
const latHist = ref<Record<string, number[]>>({})

onMounted(() => connect())

const system = computed(() => state.snapshot?.system)
const services = computed(() => state.snapshot?.services ?? [])
const containers = computed(() => state.snapshot?.containers ?? {})

let lastCpu = 0
let lastMem = 0
function push(arr: number[], v: number) {
  arr.push(v)
  if (arr.length > MAX) arr.shift()
}

const cpuPct = computed(() => state.snapshot?.system?.cpu.percent ?? 0)
const memPct = computed(() => state.snapshot?.system?.memory.percent ?? 0)

// update histories reactively from snapshots
watch(
  () => state.snapshot,
  (snap) => {
    if (!snap?.system) return
    push(cpuHist.value, snap.system.cpu.percent)
    push(memHist.value, snap.system.memory.percent)
    for (const s of snap.services) {
      const h = (latHist.value[s.name] ??= [])
      push(h, s.latency_ms)
    }
  },
)

function extractedEntries(ext?: Record<string, string | number>) {
  return ext ? Object.entries(ext) : []
}

function netStat() {
  return system.value?.net.find((n) => n.name !== 'lo') ?? system.value?.net[0]
}
</script>

<template>
  <div class="app">
    <header>
      <div class="brand">
        <span class="logo">◉</span>
        <span>Homelab Dashboard</span>
      </div>
      <div class="meta">
        <span v-if="system" class="meta-item">🖥 {{ system.hostname }}</span>
        <span v-if="system" class="meta-item">⏱ {{ fmtUptime(system.uptime) }}</span>
        <span class="meta-item">
          <span class="conn" :class="state.connected ? 'ok' : 'err'"></span>
          {{ state.connected ? '已连接' : '连接中…' }}
        </span>
        <span v-if="state.lastUpdate" class="meta-item muted">更新 {{ fmtTime(state.lastUpdate) }}</span>
      </div>
    </header>

    <div v-if="state.reload" class="reload" :class="state.reload.ok ? 'ok' : 'err'">
      <template v-if="state.reload.ok">
        配置已热重载 (v{{ state.reload.version }})
      </template>
      <template v-else>
        配置重载失败: {{ state.reload.error }}
      </template>
    </div>

    <main>
      <section v-if="system">
        <h2>系统</h2>
        <div class="grid">
          <div class="card">
            <div class="card-title">CPU</div>
            <div class="value">{{ cpuPct.toFixed(1) }}%</div>
            <div class="sub">{{ system.cpu.count }} 核心 · load {{ system.load.load1.toFixed(2) }}</div>
            <Spark :data="cpuHist" color="#58a6ff" height="44px" />
          </div>

          <div class="card">
            <div class="card-title">内存</div>
            <div class="value">{{ memPct.toFixed(1) }}%</div>
            <div class="sub">{{ fmtBytes(system.memory.used) }} / {{ fmtBytes(system.memory.total) }}</div>
            <Spark :data="memHist" color="#3fb950" height="44px" />
          </div>

          <div class="card">
            <div class="card-title">磁盘</div>
            <div class="disk-row" v-for="d in system.disks" :key="d.mount">
              <div class="disk-line">
                <span>{{ d.mount }}</span>
                <span>{{ d.percent.toFixed(1) }}%</span>
              </div>
              <div class="bar"><div class="bar-fill" :style="{ width: d.percent + '%' }"></div></div>
              <div class="sub">{{ fmtBytes(d.used) }} / {{ fmtBytes(d.total) }} · {{ d.fstype }}</div>
            </div>
          </div>

          <div class="card" v-if="netStat()">
            <div class="card-title">网络 · {{ netStat()!.name }}</div>
            <div class="value-sm">↓ {{ fmtRate(netStat()!.recv_rate) }}</div>
            <div class="value-sm">↑ {{ fmtRate(netStat()!.sent_rate) }}</div>
            <div class="sub">收 {{ fmtBytes(netStat()!.bytes_recv) }} · 发 {{ fmtBytes(netStat()!.bytes_sent) }}</div>
          </div>
        </div>
      </section>

      <section v-if="services.length">
        <h2>服务 <span class="muted small">({{ services.length }})</span></h2>
        <div class="grid">
          <div v-for="s in services" :key="s.name" class="card service">
            <div class="service-head">
              <span class="svc-name">{{ s.name }}</span>
              <StatusChip :status="s.status" />
            </div>
            <div class="sub">
              {{ s.type.toUpperCase() }} · {{ s.latency_ms.toFixed(1) }}ms
            </div>

            <div v-if="s.last_error" class="error-msg">{{ s.last_error }}</div>

            <div v-if="extractedEntries(s.extracted).length" class="metrics">
              <div v-for="[k, v] in extractedEntries(s.extracted)" :key="k" class="metric">
                <span class="metric-k">{{ k }}</span>
                <span class="metric-v">{{ v }}</span>
              </div>
            </div>

            <div v-if="latHist[s.name]?.length" class="spark-wrap">
              <Spark :data="latHist[s.name]" color="#d29922" height="30px" />
            </div>

            <div v-if="s.container" class="container-block">
              <div class="cb-title">🐳 {{ s.container.name }}</div>
              <div class="cb-grid">
                <span :class="['cstate', s.container.state]">{{ s.container.state }}</span>
                <span>{{ s.container.cpu_perc.toFixed(1) }}% CPU</span>
                <span>{{ s.container.mem_perc.toFixed(1) }}% MEM</span>
                <span class="muted">↓{{ fmtRate(s.container.rx_rate) }}</span>
                <span class="muted">↑{{ fmtRate(s.container.tx_rate) }}</span>
                <span class="muted" v-if="s.container.error">⚠ {{ s.container.error }}</span>
              </div>
            </div>
          </div>
        </div>
      </section>

      <section v-if="Object.keys(containers).length">
        <h2>容器运行时</h2>
        <div class="grid">
          <div v-for="c in Object.values(containers)" :key="c.name" class="card container-card">
            <div class="container-head">
              <span class="svc-name">{{ c.name }}</span>
              <span :class="['cstate', c.state]">{{ c.state }}</span>
            </div>
            <div class="sub">{{ c.image }}</div>
            <div class="cb-grid">
              <span>{{ c.cpu_perc.toFixed(1) }}% CPU</span>
              <span>{{ c.mem_perc.toFixed(1) }}% MEM</span>
              <span>{{ fmtBytes(c.mem_usage) }}</span>
            </div>
            <div v-if="c.error" class="error-msg">{{ c.error }}</div>
          </div>
        </div>
      </section>
    </main>

    <footer class="muted small">
      Dashboard · WebSocket 实时推送 · 配置热重载 · 无数据库
    </footer>
  </div>
</template>

<style scoped>
.app { max-width: 1180px; margin: 0 auto; padding: 16px 20px 40px; }
header { display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 8px; padding: 8px 0 16px; border-bottom: 1px solid #21262d; }
.brand { display: flex; align-items: center; gap: 10px; font-size: 18px; font-weight: 700; }
.logo { color: #58a6ff; }
.meta { display: flex; gap: 14px; align-items: center; flex-wrap: wrap; font-size: 13px; }
.meta-item { display: inline-flex; align-items: center; gap: 5px; }
.muted { color: #8b949e; }
.small { font-size: 12px; }
.conn { width: 9px; height: 9px; border-radius: 50%; display: inline-block; }
.conn.ok { background: #3fb950; box-shadow: 0 0 6px #3fb950; }
.conn.err { background: #f85149; }
.reload { margin: 10px 0; padding: 8px 12px; border-radius: 8px; font-size: 13px; }
.reload.ok { background: rgba(63,185,80,0.12); color: #3fb950; }
.reload.err { background: rgba(248,81,73,0.12); color: #f85149; }
h2 { font-size: 14px; text-transform: uppercase; letter-spacing: 1px; color: #8b949e; margin: 26px 0 12px; }
.grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(260px, 1fr)); gap: 14px; }
.card { background: #161b22; border: 1px solid #21262d; border-radius: 12px; padding: 14px 16px; }
.card-title { font-size: 12px; color: #8b949e; text-transform: uppercase; letter-spacing: 0.5px; margin-bottom: 6px; }
.value { font-size: 26px; font-weight: 700; }
.value-sm { font-size: 17px; font-weight: 600; margin: 2px 0; }
.sub { color: #8b949e; font-size: 12px; margin-top: 4px; }
.disk-row { margin-bottom: 10px; }
.disk-line { display: flex; justify-content: space-between; font-size: 13px; margin-bottom: 4px; }
.bar { height: 6px; background: #21262d; border-radius: 3px; overflow: hidden; }
.bar-fill { height: 100%; background: #3fb950; border-radius: 3px; }
.service-head, .container-head { display: flex; justify-content: space-between; align-items: center; margin-bottom: 6px; }
.svc-name { font-weight: 600; font-size: 15px; }
.error-msg { color: #f85149; font-size: 12px; margin-top: 6px; word-break: break-all; }
.metrics { display: grid; grid-template-columns: repeat(auto-fill, minmax(110px, 1fr)); gap: 8px; margin-top: 10px; }
.metric { background: #0d1117; border-radius: 8px; padding: 6px 8px; }
.metric-k { display: block; font-size: 11px; color: #8b949e; }
.metric-v { font-size: 14px; font-weight: 600; }
.spark-wrap { margin-top: 10px; }
.container-block { margin-top: 12px; border-top: 1px dashed #30363d; padding-top: 10px; }
.cb-title { font-size: 13px; font-weight: 600; margin-bottom: 6px; }
.cb-grid { display: flex; flex-wrap: wrap; gap: 10px; font-size: 12px; }
.cstate { padding: 1px 8px; border-radius: 999px; font-size: 11px; font-weight: 600; }
.cstate.running { background: rgba(63,185,80,0.15); color: #3fb950; }
.cstate.exited { background: rgba(139,148,158,0.15); color: #8b949e; }
.cstate.error { background: rgba(248,81,73,0.15); color: #f85149; }
.container-card { display: flex; flex-direction: column; gap: 4px; }
footer { margin-top: 40px; text-align: center; }
</style>