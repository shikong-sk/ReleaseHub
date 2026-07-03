import { requestJson } from './http'

// 重启后端服务，异步触发优雅退出，由外部进程管理器重新拉起
export function restartService(): Promise<{ message: string }> {
  return requestJson<{ message: string }>('/api/system/restart', { method: 'POST' })
}
