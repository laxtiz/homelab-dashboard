import { reactive } from 'vue'
import type { Snapshot, WsMessage, ReloadEvent } from './types'

export const state = reactive<{
  connected: boolean
  lastUpdate: number | null
  snapshot: Snapshot | null
  reload: ReloadEvent | null
}>({
  connected: false,
  lastUpdate: null,
  snapshot: null,
  reload: null,
})

let ws: WebSocket | null = null
let retry = 0

function url(): string {
  const proto = location.protocol === 'https:' ? 'wss' : 'ws'
  return `${proto}://${location.host}/ws`
}

export function connect() {
  try {
    ws = new WebSocket(url())
  } catch {
    scheduleReconnect()
    return
  }

  ws.onopen = () => {
    state.connected = true
    retry = 0
  }

  ws.onmessage = (ev) => {
    const msg = JSON.parse(ev.data) as WsMessage
    if (msg.type === 'snapshot') {
      state.snapshot = msg.data as Snapshot
      state.lastUpdate = Date.now()
    } else if (msg.type === 'reload') {
      state.reload = msg.data as ReloadEvent
    }
  }

  ws.onclose = () => {
    state.connected = false
    scheduleReconnect()
  }

  ws.onerror = () => ws?.close()
}

function scheduleReconnect() {
  const delay = Math.min(15000, 500 * 2 ** retry)
  retry++
  setTimeout(connect, delay)
}