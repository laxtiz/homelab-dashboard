<script setup lang="ts">
import { onMounted, computed, ref, watch } from 'vue'
import { state, connect } from './ws'
import { fmtBytes, fmtRate, fmtUptime, fmtTime } from './types'
import Spark from './components/Spark.vue'
import StatusChip from './components/StatusChip.vue'
import Modal from './components/Modal.vue'

const MAX = 120
const cpuHist = ref<number[]>([])
const memHist = ref<number[]>([])
const latHist = ref<Record<string, number[]>>({})

onMounted(() => connect())

const system = computed(() => state.snapshot?.system)
const services = computed(() => state.snapshot?.services ?? [])
const containers = computed(() => state.snapshot?.containers ?? {})

function push(arr: number[], v: number) {
  arr.push(v)
  if (arr.length > MAX) arr.shift()
}

const cpuPct = computed(() => state.snapshot?.system?.cpu.percent ?? 0)
const memPct = computed(() => state.snapshot?.system?.memory.percent ?? 0)

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

function cpuColor(pct: number) {
  if (pct > 80) return 'var(--error)'
  if (pct > 50) return 'var(--warn)'
  return 'var(--accent)'
}

function memColor(pct: number) {
  if (pct > 85) return 'var(--error)'
  if (pct > 60) return 'var(--warn)'
  return 'var(--success)'
}

function diskColor(pct: number) {
  if (pct > 90) return 'var(--error)'
  if (pct > 70) return 'var(--warn)'
  return 'var(--success)'
}

const modalOpen = ref(false)
const modalService = ref<any>(null)
const MAX_VISIBLE_METRICS = 3

function openModal(s: any) {
  modalService.value = s
  modalOpen.value = true
}
</script>

<template>
  <div class="app">
    <header>
      <div class="brand">
        <span class="logo">◉</span>
        <span class="brand-text">Homelab Dashboard</span>
      </div>
      <div class="meta">
        <span v-if="system" class="meta-item">
          <span class="meta-icon">🖥</span>{{ system.hostname }}
        </span>
        <span class="sep"></span>
        <span v-if="system" class="meta-item">
          <span class="meta-icon">⏱</span>{{ fmtUptime(system.uptime) }}
        </span>
        <span class="sep"></span>
        <span class="meta-item">
          <span class="conn" :class="state.connected ? 'ok' : 'err'"></span>
          {{ state.connected ? '已连接' : '连接中…' }}
        </span>
        <span v-if="state.lastUpdate" class="meta-item muted">
          更新于 {{ fmtTime(state.lastUpdate) }}
        </span>
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
      <!-- 系统概览 -->
      <section v-if="system" class="section">
        <h2 class="section-title">系统概览</h2>
        <div class="grid grid-4">
          <div class="card stat-card">
            <div class="stat-head">
              <span class="card-icon cpu">⚡</span>
              <span class="card-label">CPU</span>
              <span class="stat-val ml-auto" :style="{ color: cpuColor(cpuPct) }">{{ cpuPct.toFixed(1) }}%</span>
            </div>
            <div class="stat-sub">{{ system.cpu.count }} 核心 · load {{ system.load.load1.toFixed(2) }}</div>
            <Spark :data="cpuHist" :color="cpuColor(cpuPct)" height="36px" />
          </div>

          <div class="card stat-card">
            <div class="stat-head">
              <span class="card-icon mem">📊</span>
              <span class="card-label">内存</span>
              <span class="stat-val ml-auto" :style="{ color: memColor(memPct) }">{{ memPct.toFixed(1) }}%</span>
            </div>
            <div class="stat-sub">{{ fmtBytes(system.memory.used) }} / {{ fmtBytes(system.memory.total) }}</div>
            <Spark :data="memHist" :color="memColor(memPct)" height="36px" />
          </div>

          <div class="card stat-card">
            <div class="stat-head">
              <span class="card-icon disk">💾</span>
              <span class="card-label">磁盘</span>
            </div>
            <div class="disk-list">
              <div v-for="d in system.disks" :key="d.mount" class="disk-row">
                <span class="disk-mount">{{ d.mount }}</span>
                <span class="disk-pct" :style="{ color: diskColor(d.percent) }">{{ d.percent.toFixed(1) }}%</span>
                <div class="bar">
                  <div class="bar-fill" :style="{ width: d.percent + '%', background: diskColor(d.percent) }"></div>
                </div>
                <span class="stat-sub">{{ fmtBytes(d.used) }} / {{ fmtBytes(d.total) }}</span>
              </div>
            </div>
          </div>

          <div class="card stat-card" v-if="netStat()">
            <div class="stat-head">
              <span class="card-icon net">🌐</span>
              <span class="card-label">网络</span>
            </div>
            <div class="net-grid">
              <div class="net-item">
                <span class="net-arrow down">↓</span>
                <span class="net-val">{{ fmtRate(netStat()!.recv_rate) }}</span>
              </div>
              <div class="net-item">
                <span class="net-arrow up">↑</span>
                <span class="net-val">{{ fmtRate(netStat()!.sent_rate) }}</span>
              </div>
            </div>
            <div class="stat-sub">
              收 {{ fmtBytes(netStat()!.bytes_recv) }} · 发 {{ fmtBytes(netStat()!.bytes_sent) }}
            </div>
          </div>
        </div>
      </section>

      <!-- 服务 -->
      <section v-if="services.length" class="section">
        <div class="section-header">
          <h2 class="section-title">服务</h2>
          <span class="section-badge">{{ services.length }}</span>
        </div>
        <div class="grid">
          <div v-for="s in services" :key="s.name" class="card svc-card">
            <!-- 头部 -->
            <div class="svc-head">
              <span class="svc-name">{{ s.name }}</span>
              <StatusChip :status="s.status" />
            </div>
            <div class="svc-meta">
              <span>{{ s.type.toUpperCase() }}</span>
              <span class="dot-sep">·</span>
              <span>{{ s.latency_ms.toFixed(1) }}ms</span>
            </div>

            <!-- metrics -->
            <div class="svc-metrics" v-if="extractedEntries(s.extracted).length">
              <div
                v-for="[k, v] in extractedEntries(s.extracted).slice(0, s.container ? 2 : MAX_VISIBLE_METRICS)"
                :key="k"
                class="metric-row"
              >
                <span class="metric-k">{{ k }}</span>
                <span class="metric-v">{{ v }}</span>
              </div>
            </div>

            <!-- sparkline -->
            <div v-if="latHist[s.name]?.length" class="spark-wrap">
              <Spark :data="latHist[s.name]" color="#d29922" height="24px" />
            </div>

            <!-- 查看更多 -->
            <button
              v-if="extractedEntries(s.extracted).length > (s.container ? 2 : MAX_VISIBLE_METRICS)"
              class="svc-more"
              @click="openModal(s)"
            >
              查看更多 ({{ extractedEntries(s.extracted).length }})
            </button>

            <div v-if="s.last_error" class="error-msg">{{ s.last_error }}</div>

            <!-- 容器信息（固定底部，横向标签） -->
            <div v-if="s.container" class="svc-footer">
              <div class="ct-bar">
                <span class="ct-label">🐳 {{ s.container.name }}</span>
                <span :class="['state-pill', s.container.state]">{{ s.container.state }}</span>
              </div>
              <div class="ct-tags">
                <span class="ct-tag">{{ s.container.cpu_perc.toFixed(1) }}% CPU</span>
                <span class="ct-tag">{{ s.container.mem_perc.toFixed(1) }}% MEM</span>
                <span class="ct-tag">{{ fmtBytes(s.container.mem_usage) }}</span>
              </div>
            </div>
          </div>
        </div>
      </section>

      <!-- 容器运行时 -->
      <section v-if="Object.keys(containers).length" class="section">
        <div class="section-header">
          <h2 class="section-title">容器运行时</h2>
          <span class="section-badge">{{ Object.keys(containers).length }}</span>
        </div>
        <div class="grid">
          <div v-for="c in Object.values(containers)" :key="c.name" class="card ct-card">
            <div class="ct-card-head">
              <span class="ct-label">🐳 {{ c.name }}</span>
              <span :class="['state-pill', c.state]">{{ c.state }}</span>
            </div>
            <div class="svc-meta">{{ c.image }}</div>
            <div class="ct-tags">
              <span class="ct-tag">{{ c.cpu_perc.toFixed(1) }}% CPU</span>
              <span class="ct-tag">{{ c.mem_perc.toFixed(1) }}% MEM</span>
              <span class="ct-tag">{{ fmtBytes(c.mem_usage) }}</span>
            </div>
            <div v-if="c.error" class="error-msg">{{ c.error }}</div>
          </div>
        </div>
      </section>
    </main>

    <footer>
      <span class="muted">Dashboard · WebSocket 实时推送 · 配置热重载 · 无数据库</span>
    </footer>

    <!-- 弹窗 -->
    <Modal :show="modalOpen" :title="modalService?.name" @close="modalOpen = false">
      <div v-if="modalService" class="modal-metrics">
        <div v-for="[k, v] in extractedEntries(modalService.extracted)" :key="k" class="modal-metric">
          <span class="modal-mk">{{ k }}</span>
          <span class="modal-mv">{{ v }}</span>
        </div>
      </div>
      <div v-if="modalService?.container" class="modal-section">
        <div class="modal-section-title">容器信息</div>
        <div class="modal-metrics">
          <div class="modal-metric">
            <span class="modal-mk">名称</span>
            <span class="modal-mv">{{ modalService.container.name }}</span>
          </div>
          <div class="modal-metric">
            <span class="modal-mk">状态</span>
            <span class="modal-mv">{{ modalService.container.state }}</span>
          </div>
          <div class="modal-metric">
            <span class="modal-mk">CPU</span>
            <span class="modal-mv">{{ modalService.container.cpu_perc.toFixed(1) }}%</span>
          </div>
          <div class="modal-metric">
            <span class="modal-mk">内存</span>
            <span class="modal-mv">{{ modalService.container.mem_perc.toFixed(1) }}%</span>
          </div>
        </div>
      </div>
      <div v-if="modalService?.last_error" class="error-msg" style="margin-top:12px">
        {{ modalService.last_error }}
      </div>
    </Modal>
  </div>
</template>

<style scoped>
/* ===== Layout ===== */
.app { max-width: 1200px; margin: 0 auto; padding: 0 24px 48px; }

/* ===== Header ===== */
header { display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 12px; padding: 20px 0 16px; border-bottom: 1px solid var(--border); }
.brand { display: flex; align-items: center; gap: 10px; }
.logo { color: var(--accent); font-size: 22px; filter: drop-shadow(0 0 8px rgba(88,166,255,0.4)); }
.brand-text { font-size: 18px; font-weight: 700; letter-spacing: -0.3px; }
.meta { display: flex; gap: 14px; align-items: center; flex-wrap: wrap; font-size: 13px; }
.meta-item { display: inline-flex; align-items: center; gap: 5px; }
.meta-icon { font-size: 13px; opacity: 0.8; }
.sep { width: 1px; height: 14px; background: var(--border-light); }
.muted { color: var(--text-secondary); }
.conn { width: 8px; height: 8px; border-radius: 50%; display: inline-block; }
.conn.ok { background: var(--success); box-shadow: 0 0 8px var(--success); animation: pulse 2s ease-in-out infinite; }
.conn.err { background: var(--error); box-shadow: 0 0 8px var(--error); }
@keyframes pulse { 0%, 100% { opacity: 1; } 50% { opacity: 0.6; } }

/* ===== Reload ===== */
.reload { margin: 12px 0; padding: 10px 14px; border-radius: var(--radius-sm); font-size: 13px; font-weight: 500; }
.reload.ok { background: var(--success-dim); color: var(--success); border: 1px solid rgba(63,185,80,0.2); }
.reload.err { background: var(--error-dim); color: var(--error); border: 1px solid rgba(248,81,73,0.2); }

/* ===== Sections ===== */
.section { margin-top: 28px; }
.section-header { display: flex; align-items: center; gap: 10px; margin-bottom: 14px; }
.section-title { font-size: 12px; text-transform: uppercase; letter-spacing: 1.5px; color: var(--text-secondary); font-weight: 600; margin: 0; }
.section-badge { display: inline-flex; align-items: center; justify-content: center; min-width: 20px; height: 20px; padding: 0 6px; border-radius: 999px; background: var(--accent-dim); color: var(--accent); font-size: 11px; font-weight: 700; }
.section > .section-title:not(.section-header .section-title) { margin-bottom: 14px; }

/* ===== Grid ===== */
.grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap: 14px; }
.grid-4 { grid-template-columns: repeat(4, 1fr); }

/* ===== Card ===== */
.card { background: var(--bg-card); border: 1px solid var(--border); border-radius: var(--radius); padding: 14px 16px; box-shadow: var(--shadow-card); transition: border-color var(--transition), box-shadow var(--transition), transform var(--transition); }
.card:hover { border-color: var(--border-light); box-shadow: var(--shadow-card-hover); transform: translateY(-1px); }

/* ===== Stat Card ===== */
.stat-card { display: flex; flex-direction: column; gap: 6px; }
.stat-head { display: flex; align-items: center; gap: 8px; }
.card-icon { font-size: 15px; width: 24px; height: 24px; display: inline-flex; align-items: center; justify-content: center; border-radius: 6px; }
.card-icon.cpu { background: rgba(88,166,255,0.12); color: var(--accent); }
.card-icon.mem { background: rgba(63,185,80,0.12); color: var(--success); }
.card-icon.disk { background: rgba(210,153,34,0.12); color: var(--warn); }
.card-icon.net { background: rgba(139,148,158,0.12); color: var(--text-secondary); }
.card-label { font-size: 12px; color: var(--text-secondary); text-transform: uppercase; letter-spacing: 0.5px; font-weight: 600; }
.stat-val { font-size: 22px; font-weight: 700; line-height: 1; letter-spacing: -0.5px; }
.stat-sub { color: var(--text-secondary); font-size: 12px; }
.ml-auto { margin-left: auto; }

/* Disk */
.disk-list { display: flex; flex-direction: column; gap: 8px; }
.disk-row { display: grid; grid-template-columns: auto 1fr auto; grid-template-areas: "mount pct" "bar bar" "sub sub"; gap: 3px 8px; align-items: center; }
.disk-mount { grid-area: mount; font-size: 13px; font-weight: 500; }
.disk-pct { grid-area: pct; font-size: 13px; font-weight: 700; text-align: right; }
.bar { grid-area: bar; height: 4px; background: var(--border); border-radius: 2px; overflow: hidden; }
.bar-fill { height: 100%; border-radius: 2px; transition: width 0.6s ease; }
.disk-row .stat-sub { grid-area: sub; }

/* Network */
.net-grid { display: flex; gap: 20px; }
.net-item { display: flex; align-items: baseline; gap: 4px; }
.net-arrow { font-size: 14px; font-weight: 700; }
.net-arrow.down { color: var(--success); }
.net-arrow.up { color: var(--accent); }
.net-val { font-size: 16px; font-weight: 700; letter-spacing: -0.3px; }

/* ===== Service Card ===== */
.svc-card { display: flex; flex-direction: column; gap: 0; }
.svc-head { display: flex; justify-content: space-between; align-items: center; }
.svc-name { font-weight: 600; font-size: 15px; letter-spacing: -0.2px; }
.svc-meta { color: var(--text-secondary); font-size: 12px; display: flex; align-items: center; gap: 4px; margin-top: 2px; margin-bottom: 8px; }
.dot-sep { opacity: 0.4; }

/* Metrics 垂直列表 */
.svc-metrics { flex: 1; min-height: 0; overflow: hidden; display: flex; flex-direction: column; gap: 0; }
.metric-row { display: flex; justify-content: space-between; align-items: center; padding: 6px 0; border-bottom: 1px solid var(--border); flex-shrink: 0; }
.metric-row:last-child { border-bottom: none; }
.metric-k { font-size: 12px; color: var(--text-secondary); }
.metric-v { font-size: 13px; font-weight: 600; color: var(--text); }

.spark-wrap { flex-shrink: 0; margin-top: 4px; }

/* 查看更多按钮 */
.svc-more { flex-shrink: 0; margin-top: 6px; background: var(--accent-dim); color: var(--accent); border: none; border-radius: var(--radius-sm); padding: 6px 10px; font-size: 12px; font-weight: 600; cursor: pointer; transition: background var(--transition); width: 100%; text-align: center; }
.svc-more:hover { background: rgba(88,166,255,0.2); }

/* ===== 容器信息（通用样式） ===== */
.svc-footer { margin-top: auto; padding-top: 8px; border-top: 1px solid var(--border); }
.ct-bar { display: flex; align-items: center; justify-content: space-between; margin-bottom: 6px; }
.ct-label { font-size: 12px; font-weight: 600; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.ct-tags { display: flex; flex-wrap: wrap; gap: 6px; }
.ct-tag { display: inline-flex; align-items: center; justify-content: center; width: calc(33.333% - 4px); padding: 6px 0; border: 1px solid var(--border); border-radius: var(--radius-sm); font-size: 12px; font-weight: 600; color: var(--text); background: var(--bg-inset); text-align: center; letter-spacing: 0.2px; }

/* ===== 容器卡片（独立区域） ===== */
.ct-card { display: flex; flex-direction: column; gap: 6px; }
.ct-card-head { display: flex; justify-content: space-between; align-items: center; }

/* State Pill */
.state-pill { padding: 2px 8px; border-radius: 999px; font-size: 11px; font-weight: 600; letter-spacing: 0.3px; }
.state-pill.running { background: var(--success-dim); color: var(--success); }
.state-pill.exited { background: var(--muted-dim); color: var(--text-secondary); }
.state-pill.error { background: var(--error-dim); color: var(--error); }

.error-msg { color: var(--error); font-size: 12px; margin-top: 4px; word-break: break-all; padding: 6px 8px; background: var(--error-dim); border-radius: var(--radius-sm); }

/* ===== Footer ===== */
footer { margin-top: 48px; padding-top: 20px; border-top: 1px solid var(--border); text-align: center; font-size: 12px; }

/* ===== Modal ===== */
.modal-metrics { display: flex; flex-direction: column; gap: 0; }
.modal-metric { display: flex; justify-content: space-between; align-items: center; padding: 8px 0; border-bottom: 1px solid var(--border); }
.modal-metric:last-child { border-bottom: none; }
.modal-mk { font-size: 13px; color: var(--text-secondary); }
.modal-mv { font-size: 14px; font-weight: 600; }
.modal-section { margin-top: 16px; }
.modal-section-title { font-size: 12px; text-transform: uppercase; letter-spacing: 0.8px; color: var(--text-secondary); font-weight: 600; margin-bottom: 8px; }

/* ===== Responsive ===== */
@media (max-width: 900px) { .grid-4 { grid-template-columns: repeat(2, 1fr); } }
@media (max-width: 768px) {
  .app { padding: 0 16px 32px; }
  header { padding: 16px 0 12px; }
  .grid { grid-template-columns: 1fr; }
  .grid-4 { grid-template-columns: 1fr 1fr; }
  .stat-val { font-size: 20px; }
  .meta { gap: 8px; font-size: 12px; }
  .sep { display: none; }
}
@media (max-width: 480px) {
  .grid-4 { grid-template-columns: 1fr; }
  .brand-text { font-size: 16px; }
}
</style>
