export const API_BASE_URL = (
  import.meta.env.VITE_API_BASE_URL || 'http://localhost:8080'
).replace(/\/$/, '')

export type TaskStatus = 'pending' | 'running' | 'success' | 'failed' | 'canceled'

export type ScenarioStep = {
  name: string
  method: 'GET' | 'POST' | 'PUT' | 'DELETE' | 'PATCH'
  url: string
  body_tpl: string
  headers: Record<string, string>
  extract: Record<string, string>
}

export type Scenario = {
  steps: ScenarioStep[]
  formula: string
}

export type ScenarioResponse = {
  id: string
  name: string
  definition: Scenario
  created_at: string
}

export type InterfaceResponse = {
  id: string
  name: string
  step: ScenarioStep
  created_at: string
}

export type EnvironmentResponse = {
  id: string
  name: string
  variables: Record<string, string>
  default_headers: Record<string, string>
  created_at: string
}

export type TrySendInterfaceResponse = {
  request: {
    method: string
    url: string
    headers: Record<string, string>
    body: string
  }
  response: {
    status_code: number
    body: string
    cost_ms: number
    truncated: boolean
  }
  error: string
}

export type AuthUser = {
  id: string
  username: string
  totp_enabled: boolean
  created_at: string
}

export type AuthUserResponse = {
  user: AuthUser
}

export type LoadTestConfig = {
  scenario: Scenario
  per_account_count: number
}

export type TaskResponse = {
  id: string
  module_type: string
  name: string
  status: TaskStatus
  config: unknown
  concurrency: number
  total_count: number
  success_count: number
  fail_count: number
  err_msg?: string
  created_by: string
  created_at: string
  started_at?: string
  finished_at?: string
}

export type AccountResponse = {
  id: string
  group_id: string
  username: string
  extra?: unknown
  enabled: boolean
  created_at: string
}

export type CreateAccountRequest = {
  group_id: string
  username: string
  password: string
  extra?: unknown
  enabled?: boolean
}

export type CreateTaskRequest = {
  module_type: string
  name: string
  config: LoadTestConfig
  account_group_id: string
  concurrency: number
}

export type ResultRowResponse = {
  id: number
  task_id: string
  account_id: string
  account_name: string
  seq_no: number
  steps: unknown
  formula_result: number
  success: boolean
  err_msg?: string
  cost_ms: number
  created_at: string
}

export type AccountResultsResponse = {
  account_id: string
  account_name: string
  rows: ResultRowResponse[]
}

export type ProgressEvent = {
  task_id: string
  account_id: string
  seq_no: number
  step_name: string
  success: boolean
  err_msg?: string
  cost_ms: number
  timestamp: string
}

export class ApiError extends Error {
  status: number

  constructor(message: string, status: number) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

type QueryValue = string | number | boolean | undefined | null

function buildQuery(params: Record<string, QueryValue>): string {
  const search = new URLSearchParams()
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined && value !== null && value !== '') {
      search.set(key, String(value))
    }
  }
  const encoded = search.toString()
  return encoded ? `?${encoded}` : ''
}

export async function extractErrorMessage(response: Response): Promise<string> {
  const fallback = `请求失败（状态码 ${response.status}）`
  const contentType = response.headers.get('content-type') || ''
  if (contentType.includes('application/json')) {
    try {
      const body = (await response.json()) as { error?: unknown }
      if (typeof body.error === 'string' && body.error.trim() !== '') {
        return body.error
      }
    } catch {
      return fallback
    }
  }

  try {
    const text = await response.text()
    return text.trim() || fallback
  } catch {
    return fallback
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers)
  if (init?.body && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }

  const response = await fetch(`${API_BASE_URL}${path}`, {
    ...init,
    credentials: 'include',
    headers,
  })
  if (!response.ok) {
    throw new ApiError(await extractErrorMessage(response), response.status)
  }
  if (response.status === 204) {
    return undefined as T
  }
  return (await response.json()) as T
}

export function listTasks(params: { status?: string; limit?: number } = {}) {
  return request<TaskResponse[]>(`/api/tasks${buildQuery(params)}`)
}

export function getTask(taskID: string) {
  return request<TaskResponse>(`/api/tasks/${encodeURIComponent(taskID)}`)
}

export function createTask(payload: CreateTaskRequest) {
  return request<TaskResponse>('/api/tasks', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export function cancelTask(taskID: string) {
  return request<{ canceled: boolean }>(`/api/tasks/${encodeURIComponent(taskID)}/cancel`, {
    method: 'POST',
  })
}

export function getTaskResults(taskID: string, accountID?: string) {
  return request<AccountResultsResponse[]>(
    `/api/tasks/${encodeURIComponent(taskID)}/results${buildQuery({
      account_id: accountID,
    })}`,
  )
}

export function exportTaskURL(taskID: string) {
  return `${API_BASE_URL}/api/tasks/${encodeURIComponent(taskID)}/export`
}

export function listAccounts(groupID: string) {
  return request<AccountResponse[]>(
    `/api/accounts${buildQuery({
      group_id: groupID,
    })}`,
  )
}

export function createAccount(payload: CreateAccountRequest) {
  return request<AccountResponse>('/api/accounts', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export function listInterfaces() {
  return request<InterfaceResponse[]>('/api/interfaces')
}

export function createInterface(payload: { name: string; step: ScenarioStep }) {
  return request<InterfaceResponse>('/api/interfaces', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export function updateInterface(id: string, payload: { name: string; step: ScenarioStep }) {
  return request<InterfaceResponse>(`/api/interfaces/${encodeURIComponent(id)}`, {
    method: 'PUT',
    body: JSON.stringify(payload),
  })
}

export function listEnvironments() {
  return request<EnvironmentResponse[]>('/api/environments')
}

export function createEnvironment(payload: {
  name: string
  variables?: Record<string, string>
  default_headers?: Record<string, string>
}) {
  return request<EnvironmentResponse>('/api/environments', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export function updateEnvironment(
  id: string,
  payload: { name: string; variables: Record<string, string>; default_headers: Record<string, string> },
) {
  return request<EnvironmentResponse>(`/api/environments/${encodeURIComponent(id)}`, {
    method: 'PUT',
    body: JSON.stringify(payload),
  })
}

export function trySendInterface(payload: { step: ScenarioStep; environment_id?: string }) {
  return request<TrySendInterfaceResponse>('/api/interfaces/try-send', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export function listScenarios() {
  return request<ScenarioResponse[]>('/api/scenarios')
}

export function createScenario(payload: { name: string; definition: Scenario }) {
  return request<ScenarioResponse>('/api/scenarios', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export function login(payload: {
  username: string
  password: string
  code?: string
  backup_code?: string
}) {
  return request<AuthUserResponse>('/api/auth/login', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export function logout() {
  return request<{ logged_out: boolean }>('/api/auth/logout', {
    method: 'POST',
  })
}

export function me() {
  return request<AuthUserResponse>('/api/auth/me')
}

export function createUser(payload: { username: string; password: string }) {
  return request<AuthUserResponse>('/api/auth/users', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export function regenerateBackupCodes(payload: { password: string }) {
  return request<{ backup_codes: string[] }>('/api/auth/backup-codes/regenerate', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export function taskProgressURL(taskID: string) {
  const base = new URL(API_BASE_URL, window.location.origin)
  base.protocol = base.protocol === 'https:' ? 'wss:' : 'ws:'
  base.pathname = `/ws/tasks/${encodeURIComponent(taskID)}/progress`
  base.search = ''
  base.hash = ''
  return base.toString()
}
