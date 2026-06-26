export async function getJson<T>(path: string): Promise<T> {
  const response = await fetch(path, {
    headers: jsonHeaders()
  })

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
  const token = localStorage.getItem('releasehub_token')
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
