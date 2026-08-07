import type {
  ApiResponse,
  CollectPerfSamplePayload,
  CollectPerfTaskPayload,
  Device,
  DeviceApp,
  DeviceAppsPayload,
  DeviceListPayload,
  MonitoringMetrics,
  MonitoringSample,
  MonitoringStartResult,
  MonitoringStopResult,
  PerfHistoryItem,
} from './types'

const API_BASE_URL = import.meta.env.VITE_API_BASE_URL ?? ''

async function readApiResponse<T>(response: Response, fallbackMessage: string): Promise<T> {
  let result: ApiResponse<T> | null = null

  try {
    result = (await response.json()) as ApiResponse<T>
  } catch {
    // 非 JSON 响应使用 HTTP 状态兜底。
  }

  if (!response.ok) {
    throw new Error(result?.msg || `${fallbackMessage}（HTTP ${response.status}）`)
  }

  if (!result || result.code !== 0) {
    throw new Error(result?.msg || fallbackMessage)
  }

  return result.data
}

export async function getDevices(signal?: AbortSignal): Promise<Device[]> {
  const response = await fetch(`${API_BASE_URL}/api/device/list`, {
    method: 'GET',
    headers: { Accept: 'application/json' },
    signal,
  })
  const payload = await readApiResponse<DeviceListPayload>(response, '查询设备失败')

  if (!Array.isArray(payload.devices)) throw new Error('设备服务返回了无法识别的数据')

  return payload.devices.map((device) => ({
    device_id: device.device_id,
    device_name: device.device_name || device.model || device.brand || device.device_id,
    platform: device.platform || 'android',
    version: formatDeviceVersion(device.platform, device.version),
    model: device.model || device.brand || '未知型号',
    product_type: device.product_type,
    status: device.status.toLowerCase() === 'online' ? 'available' : 'offline',
    error_message: '',
  }))
}

function formatDeviceVersion(platform: string, version: string): string {
  const devicePlatform = (platform || 'android').toLowerCase()
  const cleanVersion = (version || '').replace(/^(Android|iOS)\s*/i, '')
  const systemName = devicePlatform === 'ios' ? 'iOS' : 'Android'

  return cleanVersion ? `${systemName} ${cleanVersion}` : systemName
}

export function getDeviceName(device: Device): string {
  return device.device_name || device.model || '未命名设备'
}

export async function getDeviceApps(deviceId: string, signal?: AbortSignal): Promise<DeviceApp[]> {
  const response = await fetch(`${API_BASE_URL}/api/devices/${encodeURIComponent(deviceId)}/apps`, {
    method: 'GET',
    headers: { Accept: 'application/json' },
    signal,
  })
  const payload = await readApiResponse<DeviceAppsPayload>(response, '查询设备应用失败')

  if (!Array.isArray(payload.items)) throw new Error('应用服务返回了无法识别的数据')
  return payload.items
}

function numberValue(value: unknown): number {
  const result = Number(value)
  return Number.isFinite(result) ? result : 0
}

function hasOwnValue(value: object, key: string): boolean {
  return Object.prototype.hasOwnProperty.call(value, key)
}

function collectAvailableMetrics(samples: CollectPerfSamplePayload[]): Record<string, boolean> {
  const available: Record<string, boolean> = {}

  samples.forEach((sample) => {
    const cpu = sample.data?.cpu ?? {}
    const memory = sample.data?.memory ?? {}
    const fps = sample.data?.fps ?? {}
    const gpu = sample.data?.gpu ?? {}
    const jank = sample.data?.jank ?? {}

    if (hasOwnValue(cpu, 'app_cpu')) available.app_cpu = true
    if (hasOwnValue(cpu, 'total_cpu')) available.total_cpu = true
    if (hasOwnValue(memory, 'total_pss')) available.memory_pss = true
    if (hasOwnValue(memory, 'java_heap')) available.java_heap = true
    if (hasOwnValue(memory, 'native_heap')) available.native_heap = true
    if (hasOwnValue(memory, 'stack')) available.stack = true
    if (hasOwnValue(memory, 'graphics')) available.graphics = true
    if (hasOwnValue(fps, 'fps') || hasOwnValue(fps, 'core_animation_frames_per_second')) available.fps = true
    if (hasOwnValue(fps, 'frames')) available.frames = true
    if (hasOwnValue(fps, 'refresh_rate')) available.refresh_rate = true
    if (hasOwnValue(gpu, 'device_utilization')) available.gpu_device_utilization = true
    if (hasOwnValue(jank, 'small_jank')) available.small_jank = true
    if (hasOwnValue(jank, 'jank')) available.jank = true
    if (hasOwnValue(jank, 'big_jank')) available.big_jank = true
    if (hasOwnValue(jank, 'total_small_jank')) available.total_small_jank = true
    if (hasOwnValue(jank, 'total_jank')) available.total_jank = true
    if (hasOwnValue(jank, 'total_big_jank')) available.total_big_jank = true
  })

  return available
}

function normalizeSample(sample: CollectPerfSamplePayload): MonitoringSample {
  const cpu = sample.data?.cpu ?? {}
  const memory = sample.data?.memory ?? {}
  const fps = sample.data?.fps ?? {}
  const gpu = sample.data?.gpu ?? {}
  const jank = sample.data?.jank ?? {}

  return {
    collected_at: sample.collect_time,
    app_cpu: numberValue(cpu.app_cpu),
    total_cpu: numberValue(cpu.total_cpu),
    memory_pss: numberValue(memory.total_pss),
    java_heap: numberValue(memory.java_heap),
    native_heap: numberValue(memory.native_heap),
    stack: numberValue(memory.stack),
    graphics: numberValue(memory.graphics),
    fps: numberValue(fps.fps ?? fps.core_animation_frames_per_second),
    gpu_device_utilization: numberValue(gpu.device_utilization),
    frames: numberValue(fps.frames),
    refresh_rate: numberValue(fps.refresh_rate),
    small_jank: numberValue(jank.small_jank),
    jank: numberValue(jank.jank),
    big_jank: numberValue(jank.big_jank),
    total_small_jank: numberValue(jank.total_small_jank),
    total_jank: numberValue(jank.total_jank),
    total_big_jank: numberValue(jank.total_big_jank),
  }
}

function normalizeTask(payload: CollectPerfTaskPayload): MonitoringMetrics {
  const normalizedStatus = payload.status === 'collecting'
    ? 'running'
    : payload.status === 'interrupted'
      ? 'interrupted'
      : 'stopped'
  const rawSamples = Array.isArray(payload.samples) ? payload.samples : []

  return {
    task_id: payload.task_id,
    device_id: payload.device_id,
    package_name: payload.package_name,
    process_name: payload.process_name || payload.package_name,
    platform: payload.platform || 'android',
    device_model: payload.device_model,
    available_metrics: collectAvailableMetrics(rawSamples),
    status: normalizedStatus,
    start_time: payload.start_time,
    sample_interval_ms: payload.sample_interval_ms,
    last_error: payload.last_error || '',
    samples: rawSamples.map(normalizeSample),
  }
}

export async function startMonitoring(deviceId: string, packageName: string, processName?: string, deviceModel?: string): Promise<MonitoringStartResult> {
  const response = await fetch(`${API_BASE_URL}/api/collect/perf/start`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
    body: JSON.stringify({
      device_id: deviceId,
      package_name: packageName,
      process_name: processName || packageName,
      device_model: deviceModel || '',
    }),
  })

  return readApiResponse<MonitoringStartResult>(response, '启动性能采集失败')
}

export async function getMonitoringMetrics(taskId: string, signal?: AbortSignal): Promise<MonitoringMetrics> {
  const response = await fetch(`${API_BASE_URL}/api/collect/perf/${encodeURIComponent(taskId)}`, {
    method: 'GET',
    headers: { Accept: 'application/json' },
    signal,
  })
  const payload = await readApiResponse<CollectPerfTaskPayload>(response, '查询性能数据失败')
  return normalizeTask(payload)
}

export async function stopMonitoring(taskId: string): Promise<MonitoringStopResult> {
  const response = await fetch(`${API_BASE_URL}/api/collect/perf/${encodeURIComponent(taskId)}/stop`, {
    method: 'POST',
    headers: { Accept: 'application/json' },
  })

  return readApiResponse<MonitoringStopResult>(response, '停止性能采集失败')
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function stringValue(value: unknown): string {
  return typeof value === 'string' ? value : value == null ? '' : String(value)
}

function normalizeHistoryItem(value: unknown): PerfHistoryItem {
  const item = isRecord(value) ? value : {}
  const samples = Array.isArray(item.samples) ? item.samples : []
  const totalSamples = numberValue(item.total_samples ?? item.sample_count ?? samples.length)

  return {
    task_id: stringValue(item.task_id ?? item.taskId ?? item.id),
    device_id: stringValue(item.device_id ?? item.deviceId),
    package_name: stringValue(item.package_name ?? item.packageName),
    process_name: stringValue(item.process_name ?? item.processName),
    platform: stringValue(item.platform),
    status: stringValue(item.status),
    start_time: stringValue(item.start_time ?? item.startTime),
    stop_time: stringValue(item.stop_time ?? item.stopTime),
    end_time: stringValue(item.end_time ?? item.endTime),
    sample_interval_ms: numberValue(item.sample_interval_ms ?? item.sampleIntervalMS ?? item.sampleInterval),
    sample_count: totalSamples,
    total_samples: totalSamples,
    duration_seconds: numberValue(item.duration_seconds ?? item.durationSeconds),
    last_error: stringValue(item.last_error ?? item.lastError),
  }
}

function normalizeHistoryList(payload: unknown): PerfHistoryItem[] {
  if (Array.isArray(payload)) return payload.map(normalizeHistoryItem).filter((item) => item.task_id)
  if (!isRecord(payload)) return []

  const candidates = [payload.items, payload.histories, payload.history, payload.tasks, payload.records, payload.list]
  const list = candidates.find(Array.isArray)
  return Array.isArray(list) ? list.map(normalizeHistoryItem).filter((item) => item.task_id) : []
}

export async function getPerfHistory(signal?: AbortSignal): Promise<PerfHistoryItem[]> {
  const response = await fetch(`${API_BASE_URL}/api/collect/perf-history`, {
    method: 'GET',
    headers: { Accept: 'application/json' },
    signal,
  })
  const payload = await readApiResponse<unknown>(response, '查询历史记录失败')
  return normalizeHistoryList(payload)
}

export async function getPerfHistoryTask(taskId: string, signal?: AbortSignal): Promise<MonitoringMetrics> {
  const response = await fetch(`${API_BASE_URL}/api/collect/perf-history/${encodeURIComponent(taskId)}`, {
    method: 'GET',
    headers: { Accept: 'application/json' },
    signal,
  })
  const payload = await readApiResponse<unknown>(response, '查询历史详情失败')
  const taskPayload = isRecord(payload) && isRecord(payload.task) ? payload.task : payload
  return normalizeTask(taskPayload as CollectPerfTaskPayload)
}

function getCsvFilename(taskId: string, contentDisposition: string | null): string {
  const encodedFilename = contentDisposition?.match(/filename\*=UTF-8''([^;]+)/i)?.[1]
  if (encodedFilename) {
    try {
      return decodeURIComponent(encodedFilename)
    } catch {
      return encodedFilename
    }
  }

  const quotedFilename = contentDisposition?.match(/filename="?([^";]+)"?/i)?.[1]
  return quotedFilename || `perf-history-${taskId}.csv`
}

export async function downloadPerfHistoryCsv(taskId: string): Promise<void> {
  const response = await fetch(`${API_BASE_URL}/api/collect/perf-history/${encodeURIComponent(taskId)}/csv`, {
    method: 'GET',
    headers: { Accept: 'text/csv,application/octet-stream,*/*' },
  })

  if (!response.ok) {
    let message = `导出 CSV 失败（HTTP ${response.status}）`

    try {
      const result = (await response.clone().json()) as Partial<ApiResponse<unknown>>
      message = result.msg || message
    } catch {
      const text = await response.text().catch(() => '')
      if (text) message = text
    }

    throw new Error(message)
  }

  const blob = await response.blob()
  const objectUrl = window.URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = objectUrl
  link.download = getCsvFilename(taskId, response.headers.get('content-disposition'))
  document.body.appendChild(link)
  link.click()
  link.remove()
  window.URL.revokeObjectURL(objectUrl)
}

export async function deletePerfHistory(taskId: string): Promise<void> {
  const response = await fetch(`${API_BASE_URL}/api/collect/perf-history/${encodeURIComponent(taskId)}`, {
    method: 'DELETE',
    headers: { Accept: 'application/json' },
  })

  await readApiResponse<unknown>(response, '删除历史记录失败')
}
