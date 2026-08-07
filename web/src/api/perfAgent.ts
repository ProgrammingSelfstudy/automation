// 本地采集 Agent 客户端：浏览器直连用户自己电脑上跑着的本地采集 Agent
// （client/cmd/main.go 编译出来的，默认监听 127.0.0.1:9527），走实时采集；
// 采集结束后的完整记录由页面转发给中心平台的 /api/perf/tasks 落 MySQL
// （见 docs/architecture-perf-rabbit-merge.md）。

export const PERF_AGENT_BASE_URL =
  import.meta.env.VITE_PERF_AGENT_BASE_URL || 'http://127.0.0.1:9527'

// perfAgentStopBeaconURL 供 PerfTestPage 在 beforeunload 时用
// navigator.sendBeacon 发一个尽力而为的停止请求——标签页被刷新/关掉时
// 没法等 fetch 的响应，sendBeacon 是浏览器唯一保证"页面卸载后还能把这个
// 请求发出去"的方式。这只解开 Agent 那边的设备+App 占用锁，不会把这次
// 采集记录同步到中心平台（那需要拿到最终数据再转发，卸载期间做不到）。
export function perfAgentStopBeaconURL(taskId: string): string {
  return `${PERF_AGENT_BASE_URL}/api/collect/perf/${encodeURIComponent(taskId)}/stop`
}

const PROBE_TIMEOUT_MS = 1500

// MIN_COMPATIBLE_AGENT_VERSION 是这版页面能正常对话的最低 Agent 版本号。
// 每次改动浏览器<->Agent 的接口协议时，这里和
// client/common/version.go 的 AgentVersion 要一起改。
export const MIN_COMPATIBLE_AGENT_VERSION = '1.0.0'

function compareVersions(a: string, b: string): number {
  const partsA = a.split('.').map((part) => Number(part) || 0)
  const partsB = b.split('.').map((part) => Number(part) || 0)
  for (let i = 0; i < Math.max(partsA.length, partsB.length); i++) {
    const diff = (partsA[i] ?? 0) - (partsB[i] ?? 0)
    if (diff !== 0) return diff
  }
  return 0
}

export type PerfDevice = {
  device_id: string
  device_name: string
  platform: string
  version: string
  model: string
  status: 'available' | 'busy' | 'offline' | string
}

export type PerfDeviceApp = {
  app_name?: string
  package_name: string
  executable?: string
}

export type PerfMonitoringSample = {
  collected_at: string
  total_cpu: number
  app_cpu: number
  memory_pss: number
  java_heap: number
  native_heap: number
  fps: number
  jank: number
  big_jank: number
}

export type PerfMonitoringTask = {
  task_id: string
  status: 'collecting' | 'stopped' | 'interrupted' | 'error'
  device_id: string
  package_name: string
  process_name: string
  platform: string
  device_model?: string
  start_time: string
  sample_interval_ms: number
  last_error: string
  samples: PerfMonitoringSample[]
}

type ApiEnvelope<T> = {
  code: number
  data: T
  msg: string
}

async function readEnvelope<T>(response: Response, fallbackMessage: string): Promise<T> {
  let result: ApiEnvelope<T> | null = null
  try {
    result = (await response.json()) as ApiEnvelope<T>
  } catch {
    // 非 JSON 响应用 HTTP 状态兜底
  }
  if (!response.ok) {
    throw new Error(result?.msg || `${fallbackMessage}（HTTP ${response.status}）`)
  }
  if (!result || result.code !== 0) {
    throw new Error(result?.msg || fallbackMessage)
  }
  return result.data
}

export type PerfAgentProbeResult =
  | { reachable: false }
  | { reachable: true; version: string; compatible: boolean }

// probePerfAgent 检测本地 Agent 是否在跑，并顺带拿到它上报的版本号跟
// MIN_COMPATIBLE_AGENT_VERSION 比较。探测不到就返回 reachable: false，由
// 页面展示"请先下载并启动本地采集工具"的提示；探测到但版本太旧则
// compatible: false，页面据此展示"请升级"而不是"未检测到"，不抛错、
// 不崩页面。
export async function probePerfAgent(): Promise<PerfAgentProbeResult> {
  try {
    const response = await fetch(`${PERF_AGENT_BASE_URL}/api/agent/info`, {
      method: 'GET',
      headers: { Accept: 'application/json' },
      signal: AbortSignal.timeout(PROBE_TIMEOUT_MS),
    })
    if (!response.ok) {
      return { reachable: false }
    }
    const payload = await readEnvelope<{ version: string }>(response, '探测本地采集工具失败')
    const version = payload.version || '0.0.0'
    return {
      reachable: true,
      version,
      compatible: compareVersions(version, MIN_COMPATIBLE_AGENT_VERSION) >= 0,
    }
  } catch {
    return { reachable: false }
  }
}

export async function listPerfDevices(signal?: AbortSignal): Promise<PerfDevice[]> {
  const response = await fetch(`${PERF_AGENT_BASE_URL}/api/device/list`, {
    method: 'GET',
    headers: { Accept: 'application/json' },
    signal,
  })
  const payload = await readEnvelope<{
    devices: Array<{
      device_id: string
      device_name?: string
      platform: string
      version: string
      brand?: string
      model?: string
      status: string
    }>
  }>(response, '查询设备失败')

  return (payload.devices ?? []).map((device) => ({
    device_id: device.device_id,
    device_name: device.device_name || device.model || device.brand || device.device_id,
    platform: device.platform || 'android',
    version: device.version || '',
    model: device.model || device.brand || '未知型号',
    status: device.status?.toLowerCase() === 'online' ? 'available' : 'offline',
  }))
}

export async function listPerfDeviceApps(deviceId: string, signal?: AbortSignal): Promise<PerfDeviceApp[]> {
  const response = await fetch(`${PERF_AGENT_BASE_URL}/api/devices/${encodeURIComponent(deviceId)}/apps`, {
    method: 'GET',
    headers: { Accept: 'application/json' },
    signal,
  })
  const payload = await readEnvelope<{ items: PerfDeviceApp[] }>(response, '查询设备应用失败')
  return payload.items ?? []
}

// PerfAgentBusyError：Agent 返回"这个设备+App 已经有任务在跑"（错误码
// 10011，见 client/internal/perf/start/start_collect.go）。常见于标签页
// 被刷新/关掉时没走"停止采集"，Agent 进程内存里的任务没人通知，前端也
// 就再也不知道它的存在——不能只报错让用户卡死，taskId 让调用方可以直接
// 接管显示这个任务（见 PerfTestPage.tsx 的 handleStart）。
export class PerfAgentBusyError extends Error {
  taskId: string
  constructor(taskId: string, message: string) {
    super(message)
    this.name = 'PerfAgentBusyError'
    this.taskId = taskId
  }
}

const ALREADY_COLLECTING_CODE = 10011

export async function startPerfMonitoring(params: {
  deviceId: string
  packageName: string
  processName?: string
  deviceModel?: string
}): Promise<{ task_id: string; start_time: string; sample_interval_ms: number }> {
  const response = await fetch(`${PERF_AGENT_BASE_URL}/api/collect/perf/start`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
    body: JSON.stringify({
      device_id: params.deviceId,
      package_name: params.packageName,
      process_name: params.processName || params.packageName,
      device_model: params.deviceModel || '',
    }),
  })

  let result: ApiEnvelope<{ task_id: string; start_time: string; sample_interval_ms: number }> | null = null
  try {
    result = await response.json()
  } catch {
    // 非 JSON 响应用 HTTP 状态兜底
  }

  if (result?.code === ALREADY_COLLECTING_CODE) {
    const taskId = (result.data as { task_id?: string } | null)?.task_id ?? ''
    throw new PerfAgentBusyError(taskId, result.msg || '该应用正在采集')
  }
  if (!response.ok) {
    throw new Error(result?.msg || `启动性能采集失败（HTTP ${response.status}）`)
  }
  if (!result || result.code !== 0) {
    throw new Error(result?.msg || '启动性能采集失败')
  }
  return result.data
}

function numberValue(value: unknown): number {
  const result = Number(value)
  return Number.isFinite(result) ? result : 0
}

type RawSample = {
  collect_time: string
  data: {
    cpu?: { app_cpu?: number; total_cpu?: number }
    memory?: { java_heap?: number; native_heap?: number; total_pss?: number }
    fps?: { fps?: number; core_animation_frames_per_second?: number }
    jank?: { jank?: number; big_jank?: number }
  }
}

function normalizeRawSample(sample: RawSample): PerfMonitoringSample {
  return {
    collected_at: sample.collect_time,
    total_cpu: numberValue(sample.data?.cpu?.total_cpu),
    app_cpu: numberValue(sample.data?.cpu?.app_cpu),
    memory_pss: numberValue(sample.data?.memory?.total_pss),
    java_heap: numberValue(sample.data?.memory?.java_heap),
    native_heap: numberValue(sample.data?.memory?.native_heap),
    fps: numberValue(sample.data?.fps?.fps ?? sample.data?.fps?.core_animation_frames_per_second),
    jank: numberValue(sample.data?.jank?.jank),
    big_jank: numberValue(sample.data?.jank?.big_jank),
  }
}

// getPerfMonitoringTask 是一次性拉取任务当前完整状态的轮询接口。页面日常
// 展示改走 connectPerfMonitoringWS 推送；这个函数保留给"停止采集后再确认
// 一次最终数据"这种场景兜底用——不依赖 WS 消息和 HTTP 响应谁先到。
export async function getPerfMonitoringTask(taskId: string, signal?: AbortSignal): Promise<PerfMonitoringTask> {
  const response = await fetch(`${PERF_AGENT_BASE_URL}/api/collect/perf/${encodeURIComponent(taskId)}`, {
    method: 'GET',
    headers: { Accept: 'application/json' },
    signal,
  })
  const payload = await readEnvelope<{
    task_id: string
    device_id: string
    package_name: string
    process_name?: string
    platform?: string
    device_model?: string
    status: 'collecting' | 'stopped' | 'interrupted'
    start_time: string
    sample_interval_ms: number
    last_error: string
    samples: RawSample[]
  }>(response, '查询性能数据失败')

  return {
    task_id: payload.task_id,
    status: payload.status,
    device_id: payload.device_id,
    package_name: payload.package_name,
    process_name: payload.process_name || payload.package_name,
    platform: payload.platform || 'android',
    device_model: payload.device_model,
    start_time: payload.start_time,
    sample_interval_ms: payload.sample_interval_ms,
    last_error: payload.last_error || '',
    samples: (payload.samples ?? []).map(normalizeRawSample),
  }
}

export async function stopPerfMonitoring(taskId: string): Promise<{ stop_time: string }> {
  const response = await fetch(`${PERF_AGENT_BASE_URL}/api/collect/perf/${encodeURIComponent(taskId)}/stop`, {
    method: 'POST',
    headers: { Accept: 'application/json' },
  })
  return readEnvelope(response, '停止性能采集失败')
}

export type PerfMonitoringUpdate = {
  status: 'collecting' | 'stopped' | 'interrupted' | 'error'
  lastError: string
  newSamples: PerfMonitoringSample[]
}

// connectPerfMonitoringWS 订阅本地 Agent 的增量推送，替代原来"每秒轮询一次
// GET /api/collect/perf/:taskId"的方式：有新样本或任务状态变化 Agent 才推，
// 不用客户端定时器空转。返回一个取消订阅函数（关闭 WebSocket）。
export function connectPerfMonitoringWS(
  taskId: string,
  handlers: {
    onUpdate: (update: PerfMonitoringUpdate) => void
    onError?: (message: string) => void
  },
): () => void {
  const wsURL = `${PERF_AGENT_BASE_URL.replace(/^http/, 'ws')}/ws/collect/perf/${encodeURIComponent(taskId)}`
  const socket = new WebSocket(wsURL)

  socket.onmessage = (event) => {
    try {
      const payload = JSON.parse(event.data as string) as {
        status: 'collecting' | 'stopped' | 'interrupted' | 'error'
        last_error: string
        samples: RawSample[]
      }
      handlers.onUpdate({
        status: payload.status,
        lastError: payload.last_error || '',
        newSamples: (payload.samples ?? []).map(normalizeRawSample),
      })
    } catch {
      handlers.onError?.('解析采集推送数据失败')
    }
  }
  socket.onerror = () => handlers.onError?.('本地采集工具的 WebSocket 连接出错')

  return () => socket.close()
}
