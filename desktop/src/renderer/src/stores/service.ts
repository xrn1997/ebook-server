import { ref } from 'vue'
import * as ipc from '../electron'

export type ServiceStatus = 'stopped' | 'starting' | 'running' | 'stopping' | 'error' | 'unknown'

export const serviceStatus = ref<ServiceStatus>('unknown')
export const serviceLogs = ref<string[]>([])

/** 初始化服务状态监听（在 App.vue onMounted 中调用） */
export function initServiceListener(): () => void {
  ipc.getServiceStatus().then((status) => {
    serviceStatus.value = status as ServiceStatus
  })

  const unsubStatus = ipc.onStatusChange((status) => {
    serviceStatus.value = status as ServiceStatus
  })

  const unsubLog = ipc.onLogLine((line) => {
    serviceLogs.value.push(line)
    if (serviceLogs.value.length > 500) {
      serviceLogs.value = serviceLogs.value.slice(-500)
    }
  })

  return () => {
    unsubStatus?.()
    unsubLog?.()
  }
}
