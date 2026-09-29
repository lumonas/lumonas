const API_BASE = '/api/v1'

let csrfToken: string | null = null

export function setCsrfToken(token: string) {
  csrfToken = token
}

export function clearCsrfToken() {
  csrfToken = null
}

export class ApiError extends Error {
  status: number
  body?: unknown

  constructor(status: number, method: string, path: string, body?: unknown) {
    super(`Request failed: ${method} ${path} → ${status}`)
    this.status = status
    this.body = body
  }
}

// Server rejects a mutation whose CSRF token is unknown/expired/not bound to
// this session; the retry logic keys off that message.
function isCsrfRejection(body: unknown): boolean {
  return (
    typeof body === 'object' &&
    body !== null &&
    'error' in body &&
    typeof (body as { error?: unknown }).error === 'string' &&
    (body as { error: string }).error.toLowerCase().includes('csrf')
  )
}

async function fetchCsrfToken(): Promise<string | null> {
  const response = await fetch(`${API_BASE}/auth/csrf`)
  if (!response.ok) return null
  const payload = (await response.json()) as { csrfToken?: string }
  if (!payload.csrfToken) return null
  csrfToken = payload.csrfToken
  return csrfToken
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {

  const headers = new Headers(init?.headers)
  if (!headers.has('Content-Type') && !(typeof FormData !== 'undefined' && init?.body instanceof FormData)) {
    headers.set('Content-Type', 'application/json')
  }
  const method = init?.method ?? 'GET'
  if (csrfToken && method !== 'GET' && method !== 'HEAD') {
    headers.set('X-CSRF-Token', csrfToken)
  }
  const response = await fetch(`${API_BASE}${path}`, {
    ...init,
    headers,
  })
  if (!response.ok) {
    let body: unknown
    try {
      body = await response.json()
    } catch {
      body = undefined
    }
    // The daemon may have restarted (in-memory CSRF tokens are lost while
    // the session survives in SQLite) or the token was issued for an older
    // session. Reissue once and retry a single time.
    if (response.status === 403 && isCsrfRejection(body) && method !== 'GET' && method !== 'HEAD') {
      clearCsrfToken()
      const fresh = await fetchCsrfToken()
      if (fresh) {
        return request<T>(path, init)
      }
    }
    throw new ApiError(response.status, init?.method ?? 'GET', path, body)
  }
  if (response.status === 204) return undefined as T
  return (await response.json()) as T
}

export function apiGet<T>(path: string): Promise<T> {
  return request<T>(path)
}

export function apiPost<T>(path: string, body?: unknown): Promise<T> {
  return request<T>(path, { method: 'POST', body: body ? JSON.stringify(body) : undefined })
}

export function apiMultipart<T>(path: string, body: FormData): Promise<T> {
  return request<T>(path, { method: 'POST', body })
}

export function apiPutChunk(path: string, body: Blob, offset: number): Promise<void> {
  return request<void>(path, { method: 'PUT', body, headers: { 'Content-Type': 'application/offset+octet-stream', 'Upload-Offset': String(offset) } })
}

export async function apiDownload(path: string): Promise<Blob> {
  const response = await fetch(`${API_BASE}${path}`)
  if (!response.ok) {
    let body: unknown
    try {
      body = await response.json()
    } catch {
      body = undefined
    }
    throw new ApiError(response.status, 'GET', path, body)
  }
  return response.blob()
}

export function apiPatch<T>(path: string, body?: unknown): Promise<T> {
  return request<T>(path, { method: 'PATCH', body: body ? JSON.stringify(body) : undefined })
}

export function apiPut<T>(path: string, body?: unknown): Promise<T> {
	return request<T>(path, { method: 'PUT', body: body ? JSON.stringify(body) : undefined })
}

export function apiDelete<T = unknown>(path: string, body?: unknown): Promise<T> {
  return request<T>(path, { method: 'DELETE', body: body ? JSON.stringify(body) : undefined })
}
