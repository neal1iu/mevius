const API_BASE_URL = '/api'

const TOKEN_KEY = 'mevius_token'

export class ApiError extends Error {
  readonly status: number

  constructor(status: number, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

export function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY)
}

export function setToken(token: string): void {
  localStorage.setItem(TOKEN_KEY, token)
}

export function clearToken(): void {
  localStorage.removeItem(TOKEN_KEY)
}

type UnauthorizedHandler = () => void

let unauthorizedHandler: UnauthorizedHandler | null = null

export function setUnauthorizedHandler(handler: UnauthorizedHandler | null): void {
  unauthorizedHandler = handler
}

async function readErrorMessage(res: Response): Promise<string> {
  let body: unknown
  try {
    body = await res.json()
  } catch {
    return res.statusText
  }
  if (body && typeof body === 'object') {
    const record = body as Record<string, unknown>
    if (typeof record.error === 'string') return record.error
    if (typeof record.message === 'string') return record.message
  }
  return res.statusText
}

export async function apiFetch<T>(path: string, init?: RequestInit): Promise<T> {
  const token = getToken()

  const headers = new Headers(init?.headers)
  headers.set('Accept', 'application/json')
  if (init?.body != null && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }
  if (token) {
    headers.set('Authorization', `Bearer ${token}`)
  }

  const res = await fetch(`${API_BASE_URL}${path}`, { ...init, headers })

  if (res.status === 401) {
    clearToken()
    unauthorizedHandler?.()
    throw new ApiError(401, 'Unauthorized')
  }

  if (!res.ok) {
    const message = await readErrorMessage(res)
    throw new ApiError(res.status, message)
  }

  if (res.status === 204) {
    return undefined as T
  }

  const contentType = res.headers.get('content-type') ?? ''
  if (contentType.includes('application/json')) {
    return (await res.json()) as T
  }

  return (await res.text()) as unknown as T
}

if (import.meta.env.DEV) {
  const w = window as unknown as {
    meviusApi?: {
      apiFetch: typeof apiFetch
      getToken: typeof getToken
      setToken: typeof setToken
      clearToken: typeof clearToken
    }
  }
  w.meviusApi = { apiFetch, getToken, setToken, clearToken }
}
