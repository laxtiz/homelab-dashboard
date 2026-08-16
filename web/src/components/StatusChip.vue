<script setup lang="ts">
defineProps<{
  status: 'up' | 'down' | 'error'
}>()
</script>

<template>
  <span class="chip" :class="status">
    <span class="dot"></span>
    {{ status === 'up' ? '在线' : status === 'down' ? '离线' : '错误' }}
  </span>
</template>

<style scoped>
.chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 2px 10px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 600;
}
.dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: currentColor;
}
.up { background: rgba(63,185,80,0.15); color: #3fb950; }
.down { background: rgba(248,81,73,0.15); color: #f85149; }
.error { background: rgba(210,153,34,0.15); color: #d29922; }
</style>