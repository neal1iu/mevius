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

export interface DeployResponse {
  id: string
  status: string
  in_progress: boolean
  created_at: string
  updated_at?: string
}

export interface LogChunkResult {
  lines: string
  truncated: boolean
  meta?: Record<string, unknown>
}

export function isDeployableSlotKind(kind: string): boolean {
  return kind === 'repo' || kind === 'static-site'
}

export async function triggerDeploy(bindingID: string): Promise<DeployResponse> {
  return apiFetch<DeployResponse>(`/bindings/${bindingID}/deploys`, { method: 'POST' })
}

export async function listDeploys(bindingID: string): Promise<DeployResponse[]> {
  return apiFetch<DeployResponse[]>(`/bindings/${bindingID}/deploys`)
}

export async function fetchDeployLogs(bindingID: string, deployID: string): Promise<LogChunkResult> {
  const token = getToken()
  const headers = new Headers()
  headers.set('Accept', 'text/plain')
  if (token) {
    headers.set('Authorization', `Bearer ${token}`)
  }

  const res = await fetch(`/api/bindings/${bindingID}/deploys/${deployID}/logs?tail=256`, { headers })

  if (res.status === 401) {
    clearToken()
    unauthorizedHandler?.()
    throw new ApiError(401, 'Unauthorized')
  }

  if (!res.ok) {
    const msg = await res.text()
    throw new ApiError(res.status, msg)
  }

  const truncated = res.headers.get('X-Truncated') === 'true'
  const text = await res.text()

  let meta: Record<string, unknown> | undefined
  const ct = res.headers.get('content-type') ?? ''
  if (ct.includes('application/json')) {
    try {
      const body = JSON.parse(text)
      if (body && typeof body === 'object') {
        const record = body as Record<string, unknown>
        if (record.lines && typeof record.lines === 'string') {
          return { lines: record.lines, truncated: record.truncated === true, meta: record.meta as Record<string, unknown> | undefined }
        }
      }
    } catch {
    }
  }

  return { lines: text, truncated, meta }
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

export interface Binding {
  id: string
  slot_id: string
  account_id: string
  external_id: string
  external_url?: string
  cached_meta?: Record<string, unknown>
  sync_status: 'ok' | 'error' | 'auth_error' | 'orphaned' | 'never'
  last_synced_at?: string
  created_at: string
}

export interface ExternalResource {
  external_id: string
  display_name: string
  meta?: Record<string, unknown>
}

export interface AccountResponse {
  id: string
  provider: string
  label: string
  meta?: Record<string, unknown>
  created_at: string
}

export interface SlotResponse {
  id: string
  project_id: string
  name: string
  kind: string
  config?: unknown
  created_at: string
}

export async function getSlot(id: string): Promise<SlotResponse> {
  return apiFetch<SlotResponse>(`/slots/${id}`)
}

export async function listAccounts(): Promise<AccountResponse[]> {
  return apiFetch<AccountResponse[]>('/accounts')
}

export async function listBindings(slotId: string): Promise<Binding[]> {
  return apiFetch<Binding[]>(`/slots/${slotId}/bindings`)
}

export async function discoverResources(accountId: string, kind: string): Promise<ExternalResource[]> {
  return apiFetch<ExternalResource[]>(`/accounts/${accountId}/discover?kind=${encodeURIComponent(kind)}`)
}

export async function bindResource(slotId: string, accountId: string, externalId: string): Promise<Binding> {
  return apiFetch<Binding>(`/slots/${slotId}/bindings`, {
    method: 'POST',
    body: JSON.stringify({ account_id: accountId, external_id: externalId }),
  })
}

export async function refreshBinding(bindingId: string): Promise<Binding> {
  return apiFetch<Binding>(`/bindings/${bindingId}/refresh`, { method: 'POST' })
}

export async function unbindBinding(bindingId: string): Promise<void> {
  return apiFetch<void>(`/bindings/${bindingId}`, { method: 'DELETE' })
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
