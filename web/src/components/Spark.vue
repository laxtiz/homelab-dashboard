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

function init() {
  if (!el.value) return
  chart = echarts.init(el.value)
  render()
}

function render() {
  if (!chart) return
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
        lineStyle: { width: 1.5, color: props.color ?? '#58a6ff' },
        areaStyle: { color: 'rgba(88,166,255,0.15)' },
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
  { deep: false },
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