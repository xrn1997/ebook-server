import { ref } from 'vue'
import * as ipc from '../electron'
import { forgetToken } from '../api'
import type { RuntimeInfo, ServiceError, ServiceStatus } from '../../../shared/types'

/** renderer 侧的展示状态 = 主进程的五种状态 + 「尚未取到过状态」的 unknown */
export type DisplayStatus = ServiceStatus | 'unknown'

export const serviceStatus = ref<DisplayStatus>('unknown')
export const serviceLogs = ref<string[]>([])

/** 主进程报告的最近一次失败（端口被占用、配置无效、自动登录失败等），就绪后清空 */
export const serviceError = ref<ServiceError | null>(null)

/** 概览页用的运行监控快照（端口、数据库大小、运行时长、是否需要重启） */
export const runtimeInfo = ref<RuntimeInfo | null>(null)

/** 状态的中文展示文案（侧栏、概览页、服务页共用） */
export function statusLabel(status: DisplayStatus): string {
  const map: Record<DisplayStatus, string> = {
    running: '运行中',
    stopped: '已停止',
    starting: '启动中',
    stopping: '停止中',
    error: '异常',
    unknown: '未知',
  }
  return map[status]
}

/** 拉取一次运行监控信息（服务未起来时主进程会给空端点，仍然可显示状态） */
export async function refreshRuntimeInfo(): Promise<void> {
  runtimeInfo.value = await ipc.getRuntimeInfo()
}

function applyStatus(status: ServiceStatus | 'unknown'): void {
  serviceStatus.value = status
  if (status === 'running') {
    serviceError.value = null
    void refreshRuntimeInfo()
  }
  if (status === 'stopped' || status === 'error') {
    // 进程没了/起不来，手上的令牌不再有意义，下次调用重新换取
    forgetToken()
  }
}

/** 初始化服务状态监听（在 App.vue onMounted 中调用） */
export function initServiceListener(): () => void {
  ipc.getServiceStatus().then(applyStatus)

  const unsubStatus = ipc.onStatusChange(applyStatus)

  const unsubLog = ipc.onLogLine((line) => {
    serviceLogs.value.push(line)
    if (serviceLogs.value.length > 500) {
      serviceLogs.value = serviceLogs.value.slice(-500)
    }
  })

  const unsubError = ipc.onServiceError((error) => {
    serviceError.value = error
  })

  return () => {
    unsubStatus?.()
    unsubLog?.()
    unsubError?.()
  }
}
