const TOKEN_KEY = 'releasehub_token'

// 处理 401 响应：清除 token 并跳转到登录页
function handleUnauthorized(): void {
  if (localStorage.getItem(TOKEN_KEY)) {
    localStorage.removeItem(TOKEN_KEY)
    // 避免在登录页重复跳转
    if (window.location.pathname !== '/login') {
      window.location.href = '/login'
    }
  }
}

export async function getJson<T>(path: string): Promise<T> {
  const response = await fetch(path, {
    headers: jsonHeaders()
  })

  if (response.status === 401) {
    handleUnauthorized()
    throw new Error('登录已过期，请重新登录')
  }

  if (!response.ok) {
    throw new Error(`请求失败: ${response.status}`)
  }

  return response.json() as Promise<T>
}

interface RequestJsonOptions {
  method: 'POST' | 'PUT' | 'PATCH' | 'DELETE'
  body?: unknown
}

export async function requestJson<T>(path: string, options: RequestJsonOptions): Promise<T> {
  const response = await fetch(path, {
    method: options.method,
    headers: {
      ...jsonHeaders(),
      ...(options.body === undefined ? {} : { 'Content-Type': 'application/json' })
    },
    body: options.body === undefined ? undefined : JSON.stringify(options.body)
  })

  if (response.status === 401) {
    handleUnauthorized()
    throw new Error('登录已过期，请重新登录')
  }

  if (!response.ok) {
    const errorText = await readError(response)
    throw new Error(errorText || `请求失败: ${response.status}`)
  }

  if (response.status === 204) {
    return undefined as T
  }

  return response.json() as Promise<T>
}

function jsonHeaders(): HeadersInit {
  const token = localStorage.getItem(TOKEN_KEY)
  return {
    Accept: 'application/json',
    ...(token ? { Authorization: `Bearer ${token}` } : {})
  }
}

// authHeaders 返回带认证信息的请求头（用于非 JSON 请求，如文件下载）
function authHeaders(): HeadersInit {
  const token = localStorage.getItem(TOKEN_KEY)
  return token ? { Authorization: `Bearer ${token}` } : {}
}

// downloadFile 通过 fetch 带 Authorization 头获取文件流，触发浏览器下载
// 解决 <a href> 直接导航不带认证头导致 401 的问题
export async function downloadFile(url: string, filename?: string): Promise<void> {
  const response = await fetch(url, { headers: authHeaders() })

  if (response.status === 401) {
    handleUnauthorized()
    throw new Error('登录已过期，请重新登录')
  }

  if (!response.ok) {
    const errorText = await readError(response)
    throw new Error(errorText || `下载失败: ${response.status}`)
  }

  // 从 Content-Disposition 提取文件名，或用传入的 filename，或回退到 URL 末段
  const disposition = response.headers.get('Content-Disposition') || ''
  let name = filename
  if (!name) {
    const match = disposition.match(/filename\*?=(?:UTF-8'')?["']?([^"';\n]+)["']?/i)
    if (match) {
      name = decodeURIComponent(match[1])
    }
  }
  if (!name) {
    name = url.split('/').pop() || 'download'
  }

  const blob = await response.blob()
  const objectUrl = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = objectUrl
  a.download = name
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  // 释放 object URL，避免内存泄漏
  setTimeout(() => URL.revokeObjectURL(objectUrl), 100)
}

async function readError(response: Response): Promise<string> {
  try {
    const payload = (await response.json()) as { error?: string }
    return payload.error ?? ''
  } catch {
    return ''
  }
}
