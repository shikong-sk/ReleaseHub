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
  method: 'POST' | 'PATCH' | 'DELETE'
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

async function readError(response: Response): Promise<string> {
  try {
    const payload = (await response.json()) as { error?: string }
    return payload.error ?? ''
  } catch {
    return ''
  }
}
