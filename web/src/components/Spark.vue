<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount, watch } from 'vue'
import * as echarts from 'echarts'

const props = defineProps<{
  data: number[]
  color?: string
  height?: string
}>()

const el = ref<HTMLDivElement>()
let chart: echarts.ECharts | null = null

function hexToRgba(hex: string, alpha: number): string {
  const h = hex.replace('#', '')
  const r = parseInt(h.substring(0, 2), 16)
  const g = parseInt(h.substring(2, 4), 16)
  const b = parseInt(h.substring(4, 6), 16)
  return `rgba(${r},${g},${b},${alpha})`
}

function toHex(c: string): string {
  if (c.startsWith('#')) return c
  if (c.startsWith('var(')) return '#58a6ff'
  return c
}

function init() {
  if (!el.value) return
  chart = echarts.init(el.value)
  render()
}

function render() {
  if (!chart) return
  const lineColor = toHex(props.color ?? '#58a6ff')
  chart.setOption({
    grid: { left: 0, right: 0, top: 2, bottom: 0 },
    xAxis: { type: 'category', show: false, data: props.data.map((_, i) => i) },
    yAxis: { type: 'value', show: false, min: 0, max: (v: { max: number }) => Math.max(v.max, 1) },
    series: [
      {
        type: 'line',
        data: props.data,
        showSymbol: false,
        smooth: true,
        lineStyle: { width: 2.5, color: lineColor },
        areaStyle: {
          color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
            { offset: 0, color: hexToRgba(lineColor, 0.5) },
            { offset: 1, color: hexToRgba(lineColor, 0.15) },
          ]),
        },
      },
    ],
    animation: false,
  })
}

function resize() {
  chart?.resize()
}

watch(
  () => props.data,
  () => render(),
  { deep: true },
)

onMounted(() => {
  init()
  window.addEventListener('resize', resize)
})
onBeforeUnmount(() => {
  window.removeEventListener('resize', resize)
  chart?.dispose()
})
</script>

<template>
  <div ref="el" :style="{ width: '100%', height: props.height ?? '40px' }"></div>
</template>
