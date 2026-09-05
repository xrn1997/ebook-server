<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import * as ipc from '../electron'

const activeTab = ref('smtp')
const config = ref<any>(null)
const loading = ref(true)
const saving = ref(false)
const saveResult = ref<{ ok?: boolean; errors?: string[]; needRestart?: boolean } | null>(null)

const tabs = [
  { key: 'smtp', label: '邮件服务' },
  { key: 'security', label: '安全密钥' },
  { key: 'admin', label: '管理员' },
  { key: 'server', label: '服务设置' },
]

onMounted(async () => {
  config.value = await ipc.getConfig()
  loading.value = false
})

const smtpWarning = computed(() => {
  if (!config.value) return ''
  return !config.value.smtp?.password ? 'SMTP 密码未设置，邮件功能将不可用' : ''
})

async function save() {
  saving.value = true
  saveResult.value = null
  const result = await ipc.saveConfig(config.value)
  saving.value = false
  if (result.errors?.length) {
    saveResult.value = { errors: result.errors }
  } else {
    saveResult.value = { ok: true, needRestart: true }
  }
}

async function restartNow() {
  await ipc.restartService()
  saveResult.value = null
}
</script>

<template>
  <div>
    <h2>配置</h2>
    <p class="sub">修改配置后需重启服务生效</p>

    <div v-if="loading" class="sub">加载中...</div>

    <div v-else-if="config">
      <div class="tabs">
        <button v-for="t in tabs" :key="t.key" class="tab" :class="{ active: activeTab === t.key }" @click="activeTab = t.key">
          {{ t.label }}
        </button>
      </div>

      <div v-if="saveResult?.errors?.length" class="alert alert-error">
        <div v-for="e in saveResult.errors" :key="e">{{ e }}</div>
      </div>
      <div v-if="saveResult?.ok" class="alert alert-success">
        配置已保存。
        <a href="#" @click.prevent="restartNow" style="text-decoration: underline;">立即重启服务</a>
      </div>

      <!-- 邮件服务 Tab -->
      <div v-if="activeTab === 'smtp'" class="card">
        <div v-if="smtpWarning" class="alert alert-warning">{{ smtpWarning }}</div>
        <div class="form-group">
          <label>SMTP 主机</label>
          <input v-model="config.smtp.host" placeholder="smtp.qq.com" />
        </div>
        <div class="form-group">
          <label>端口</label>
          <input v-model.number="config.smtp.port" type="number" placeholder="465" />
        </div>
        <div class="form-group">
          <label>发信账号</label>
          <input v-model="config.smtp.username" placeholder="no-reply@example.com" />
        </div>
        <div class="form-group">
          <label>授权码/密码</label>
          <input v-model="config.smtp.password" type="password" placeholder="SMTP 授权码" />
        </div>
        <div class="form-group">
          <label>发件人</label>
          <input v-model="config.smtp.from" placeholder="no-reply@example.com" />
        </div>
        <div class="form-group">
          <label>
            <input type="checkbox" v-model="config.smtp.insecure" /> 关闭 TLS 校验（只开发环境）
          </label>
        </div>
      </div>

      <!-- 安全密钥 Tab -->
      <div v-if="activeTab === 'security'" class="card">
        <div class="form-group">
          <label>JWT Secret</label>
          <input v-model="config.jwt.secret" placeholder="随机字符串" />
        </div>
        <div class="form-group">
          <label>JWT 过期时间（分钟）</label>
          <input v-model.number="config.jwt.expire_min" type="number" />
        </div>
        <div class="form-group">
          <label>Admin JWT Secret</label>
          <input v-model="config.admin.jwt_secret" placeholder="管理端随机字符串" />
        </div>
      </div>

      <!-- 管理员 Tab -->
      <div v-if="activeTab === 'admin'" class="card">
        <div class="form-group">
          <label>管理员用户名</label>
          <input v-model="config.admin.username" />
        </div>
        <div class="form-group">
          <label>管理员密码</label>
          <input v-model="config.admin.password" type="password" />
        </div>
      </div>

      <!-- 服务设置 Tab -->
      <div v-if="activeTab === 'server'" class="card">
        <div class="form-group">
          <label>公开 API 端口</label>
          <input v-model.number="config.server.port" type="number" />
        </div>
        <div class="form-group">
          <label>运行模式</label>
          <select v-model="config.server.mode">
            <option value="debug">debug</option>
            <option value="release">release</option>
          </select>
        </div>
        <div class="form-group">
          <label>数据库路径</label>
          <input v-model="config.database.path" placeholder="ebook.db" />
        </div>
        <div class="form-group">
          <label>上传目录</label>
          <input v-model="config.upload.dir" placeholder="uploads" />
        </div>
        <div class="form-group">
          <label>管理后台监听地址</label>
          <input v-model="config.admin.listen_addr" placeholder="127.0.0.1" />
        </div>
        <div class="form-group">
          <label>管理后台端口</label>
          <input v-model.number="config.admin.listen_port" type="number" />
        </div>
        <div class="form-group">
          <label>
            <input type="checkbox" v-model="config.api_docs.enabled" /> 公开 API 端口提供 Swagger 文档
          </label>
        </div>
      </div>

      <div style="margin-top: 16px; display: flex; gap: 8px;">
        <button class="btn btn-primary" @click="save" :disabled="saving">
          {{ saving ? '保存中...' : '保存配置' }}
        </button>
      </div>
    </div>
  </div>
</template>
