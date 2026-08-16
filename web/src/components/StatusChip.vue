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
  padding: 3px 10px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.3px;
}
.dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: currentColor;
}
.up { background: var(--success-dim); color: var(--success); }
.down { background: var(--error-dim); color: var(--error); }
.error { background: var(--warn-dim); color: var(--warn); }
</style>
