import { getJson, requestJson } from './http'
import type { ProxyItem, ProxyListResponse, ProxyPayload } from '@/types/proxy'

export function listProxies(): Promise<ProxyListResponse> {
  return getJson<ProxyListResponse>('/api/proxies')
}

export function createProxy(payload: ProxyPayload): Promise<ProxyItem> {
  return requestJson<ProxyItem>('/api/proxies', {
    method: 'POST',
    body: payload
  })
}

export function getProxy(id: number): Promise<ProxyItem> {
  return getJson<ProxyItem>(`/api/proxies/${id}`)
}

export function updateProxy(id: number, payload: ProxyPayload): Promise<ProxyItem> {
  return requestJson<ProxyItem>(`/api/proxies/${id}`, {
    method: 'PATCH',
    body: payload
  })
}

export async function deleteProxy(id: number): Promise<void> {
  await requestJson<void>(`/api/proxies/${id}`, {
    method: 'DELETE'
  })
}

export function testProxyConnection(id: number): Promise<{ status: string; message: string }> {
  return requestJson<{ status: string; message: string }>(`/api/proxies/${id}/test`, {
    method: 'POST'
  })
}
