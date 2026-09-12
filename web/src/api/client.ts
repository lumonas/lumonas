const API_BASE = '/api/v1'

export class ApiError extends Error {
  status: number
  body?: unknown

  constructor(status: number, method: string, path: string, body?: unknown) {
    super(`Request failed: ${method} ${path} → ${status}`)
    this.status = status
    this.body = body
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {

  const headers = new Headers(init?.headers)
  if (!headers.has('Content-Type') && !(typeof FormData !== 'undefined' && init?.body instanceof FormData)) {
    headers.set('Content-Type', 'application/json')
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
