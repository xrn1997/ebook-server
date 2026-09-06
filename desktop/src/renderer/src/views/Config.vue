<script setup lang="ts">
import { ref, onMounted } from 'vue'
import * as ipc from '../electron'
import type { AppConfig } from '../../../shared/types'

const activeTab = ref('smtp')
const config = ref<AppConfig | null>(null)
const loading = ref(true)
const saving = ref(false)
const loadError = ref('')
const saveErrors = ref<string[]>([])
/** 主进程确认已落盘：后端只在启动时读配置，所以必须重启才生效 */
const saved = ref(false)

const tabs = [
  { key: 'smtp', label: '邮件服务' },
  { key: 'security', label: '安全密钥' },
  { key: 'admin', label: '管理员' },
  { key: 'server', label: '服务设置' },
]

onMounted(async () => {
  // 一次调用同时拿配置与失败原因：此前分两个 IPC 各读一遍磁盘
  const { config: loaded, error } = await ipc.loadConfig()
  if (loaded) {
    config.value = loaded
  } else {
    loadError.value = error ?? '读取配置失败'
  }
  loading.value = false
})

async function save() {
  if (!config.value) return
  saving.value = true
  saveErrors.value = []
  saved.value = false
  const result = await ipc.saveConfig(config.value)
  saving.value = false
  if ('errors' in result) {
    saveErrors.value = result.errors
  } else {
    saved.value = result.needRestart
  }
}

async function restartNow() {
  await ipc.restartService()
  saved.value = false
}
</script>

<template>
  <div>
    <h2>配置</h2>
    <p class="sub">修改配置后需重启服务生效</p>

    <div v-if="loading" class="sub">加载中...</div>

    <div v-else-if="!config" class="alert alert-error">{{ loadError }}</div>

    <div v-else>
      <div class="tabs">
        <button v-for="t in tabs" :key="t.key" class="tab" :class="{ active: activeTab === t.key }" @click="activeTab = t.key">
          {{ t.label }}
        </button>
      </div>

      <div v-if="saveErrors.length" class="alert alert-error">
        <div v-for="e in saveErrors" :key="e">{{ e }}</div>
      </div>
      <div v-if="saved" class="alert alert-success">
        配置已保存。
        <a href="#" @click.prevent="restartNow" style="text-decoration: underline;">立即重启服务</a>
      </div>

      <!-- 邮件服务 Tab -->
      <div v-if="activeTab === 'smtp'" class="card">
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
          <input v-model="config.smtp.password" type="password" placeholder="留空保持现值" />
        </div>
        <p class="sub">界面不回显已保存的值：留空保存 = 保持现值；输入新值 = 覆盖。敏感键写入用户数据目录的 .env（SMTP_PASSWORD），其余密钥写入 config.yaml，改完需重启服务生效。</p>
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
          <input v-model="config.jwt.secret" placeholder="留空保持现值" />
        </div>
        <div class="form-group">
          <label>JWT 过期时间（分钟）</label>
          <input v-model.number="config.jwt.expire_min" type="number" />
        </div>
        <div class="form-group">
          <label>Admin JWT Secret</label>
          <input v-model="config.admin.jwt_secret" placeholder="留空保持现值" />
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
          <input v-model="config.admin.password" type="password" placeholder="留空保持现值" />
        </div>
        <div class="form-group">
          <label>管理端令牌有效期（分钟）</label>
          <input v-model.number="config.admin.expire_min" type="number" min="1" />
          <p class="sub">管理端登录态的有效时长，改完需重启服务</p>
        </div>
        <p class="sub">
          桌面应用用这份凭据自动登录本机后台接口（用户/评论/日志视图）。
        </p>
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
        <p class="sub">
          后台靠监听地址与公网隔离（ADR-0010）：保持 127.0.0.1，需要局域网访问时填具体内网 IP；
          0.0.0.0 这类通配地址会被拒绝保存。远程管理请走 SSH 隧道/VPN。
        </p>
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
