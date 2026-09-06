<script setup lang="ts">
defineProps<{
  page: number
  pageCount: number
  total: number
  unit: string
  hasPrev: boolean
  hasNext: boolean
}>()

defineEmits<{ prev: []; next: [] }>()
</script>

<template>
  <div style="display: flex; gap: 12px; align-items: center; margin-top: 12px;">
    <button class="btn" :disabled="!hasPrev" @click="$emit('prev')">上一页</button>
    <span class="sub">第 {{ page }} / {{ pageCount }} 页，共 {{ total }} {{ unit }}</span>
    <button class="btn" :disabled="!hasNext" @click="$emit('next')">下一页</button>
  </div>
</template>
