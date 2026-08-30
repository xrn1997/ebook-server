<script setup>
// API 文档：内嵌后端提供的 Swagger UI（/api-docs/index.html，由 gin-swagger 提供）。
// 文档内容从各 handler 的 Swagger 注释经 swag init 生成，改动接口后重跑 swag init 即自动更新。
</script>

<template>
  <div class="doc-page">
    <div class="bar">
      <h2>API 文档</h2>
      <span class="sub">由后端 handler 注解自动生成（Swagger UI）。也可直接访问 <a href="/api-docs/" target="_blank" rel="noopener">/api-docs</a>。</span>
    </div>
    <iframe :src="'/api-docs/index.html'" title="API 文档" class="frame"></iframe>
  </div>
</template>

<style scoped>
.doc-page { display: flex; flex-direction: column; height: 100%; }
.bar { display: flex; align-items: baseline; gap: 12px; margin-bottom: 12px; }
.bar h2 { font-size: 20px; margin: 0; }
.sub { color: #64748b; font-size: 13px; }
.frame { flex: 1; width: 100%; border: 1px solid #e2e8f0; border-radius: 12px; background: #fff; min-height: 70vh; }
</style>