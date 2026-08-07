export type DeviceStatus = 'available' | 'busy' | 'offline' | string

export interface Device {
  device_id: string
  device_name?: string
  platform: string
  version: string
  model: string
  product_type?: string
  status: DeviceStatus
  error_message: string
}

export interface DeviceApp {
  app_name?: string
  package_name: string
  executable?: string
}

export interface MonitoringSample {
  collected_at: string
  total_cpu: number
  app_cpu: number
  memory_pss: number
  java_heap: number
  native_heap: number
  stack: number
  graphics: number
  fps: number
  gpu_device_utilization: number
  frames: number
  refresh_rate: number
  small_jank: number
  jank: number
  big_jank: number
  total_small_jank: number
  total_jank: number
  total_big_jank: number
}

export interface MonitoringMetrics {
  task_id: string
  status: 'running' | 'stopped' | 'interrupted' | 'error'
  device_id: string
  package_name: string
  process_name?: string
  platform?: string
  device_model?: string
  available_metrics?: Record<string, boolean>
  start_time: string
  sample_interval_ms: number
  last_error: string
  samples: MonitoringSample[]
}

export interface MonitoringStartResult {
  task_id: string
  status: 'collecting'
  device_id: string
  package_name: string
  process_name?: string
  platform?: string
  device_model?: string
  start_time: string
  sample_interval_ms: number
}

export interface MonitoringStopResult {
  task_id: string
  status: 'stopped'
  device_id: string
  package_name: string
  process_name?: string
  platform?: string
  device_model?: string
  stop_time: string
}

export interface PerfHistoryItem {
  task_id: string
  device_id: string
  package_name: string
  process_name?: string
  platform?: string
  device_model?: string
  status?: string
  start_time?: string
  stop_time?: string
  end_time?: string
  sample_interval_ms?: number
  sample_count?: number
  total_samples?: number
  duration_seconds?: number
  last_error?: string
}

export interface MonitoringSummary {
  duration_seconds: number
  sample_count: number
  avg_app_cpu: number
  avg_fps: number
  min_fps: number
  max_memory_pss_mb: number
  total_small_jank: number
  total_jank: number
  total_big_jank: number
}

export interface ApiResponse<T> {
  code: number
  data: T
  msg: string
}

export interface DeviceListPayload {
  total: number
  devices: Array<{
    device_id: string
    device_name?: string
    platform: string
    version: string
    brand: string
    model: string
    product_type?: string
    connection_type?: string
    status: string
  }>
}

export interface DeviceAppsPayload {
  total: number
  items: DeviceApp[]
}

export interface CollectPerfSamplePayload {
  collect_time: string
  data: {
    cpu?: { app_cpu?: number; total_cpu?: number }
    memory?: { java_heap?: number; native_heap?: number; stack?: number; graphics?: number; total_pss?: number }
    fps?: { fps?: number; core_animation_frames_per_second?: number; frames?: number; refresh_rate?: number }
    gpu?: { device_utilization?: number }
    jank?: {
      small_jank?: number
      jank?: number
      big_jank?: number
      total_small_jank?: number
      total_jank?: number
      total_big_jank?: number
    }
  }
}

export interface CollectPerfTaskPayload {
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
  samples: CollectPerfSamplePayload[]
}
