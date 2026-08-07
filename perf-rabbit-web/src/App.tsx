import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { MouseEvent as ReactMouseEvent } from 'react'
import {
  ArrowLeft,
  AppWindow,
  ChartNoAxesCombined,
  Check,
  ChevronRight,
  CircleAlert,
  Clock3,
  Cpu,
  Download,
  Gauge,
  History,
  LayoutDashboard,
  MemoryStick,
  Menu,
  MonitorSmartphone,
  MoreHorizontal,
  Play,
  Rabbit,
  RefreshCw,
  Search,
  ShieldCheck,
  Square,
  Smartphone,
  Trash2,
  Wifi,
  Zap,
} from 'lucide-react'
import {
  deletePerfHistory,
  downloadPerfHistoryCsv,
  getDeviceApps,
  getDeviceName,
  getDevices,
  getMonitoringMetrics,
  getPerfHistory,
  getPerfHistoryTask,
  startMonitoring,
  stopMonitoring,
} from './api'
import type {
  Device,
  DeviceApp,
  DeviceStatus,
  MonitoringMetrics,
  MonitoringSample,
  MonitoringSummary,
  PerfHistoryItem,
} from './types'

type StatusFilter = 'all' | 'available' | 'busy'

const SERVICE_ADDRESS = import.meta.env.VITE_API_DISPLAY_URL || (import.meta.env.DEV ? '127.0.0.1:9527' : window.location.host)

const STATUS_META: Record<string, { label: string; className: string }> = {
  available: { label: '可用', className: 'available' },
  busy: { label: '占用中', className: 'busy' },
  offline: { label: '离线', className: 'offline' },
}

function statusMeta(status: DeviceStatus) {
  return STATUS_META[status] ?? { label: status || '未知', className: 'unknown' }
}

function PlatformIcon({ platform }: { platform: string }) {
  return platform.toLowerCase() === 'android' ? (
    <div className="platform-icon android" aria-label="Android">
      <Smartphone size={28} strokeWidth={1.7} />
    </div>
  ) : (
    <div className="platform-icon ios" aria-label="iOS">
      <Smartphone size={28} strokeWidth={1.7} />
    </div>
  )
}

function DeviceCard({ device, onOpen }: { device: Device; onOpen: () => void }) {
  const meta = statusMeta(device.status)
  const available = device.status === 'available'

  return (
    <article className="device-card" onClick={onOpen} tabIndex={0} onKeyDown={(event) => event.key === 'Enter' && onOpen()}>
      <div className="device-card-top">
        <PlatformIcon platform={device.platform} />
        <span className={`status-pill ${meta.className}`}>
          <span className="status-dot" />
          {meta.label}
        </span>
        <button className="icon-button card-menu" aria-label="更多操作" onClick={(event) => event.stopPropagation()}>
          <MoreHorizontal size={19} />
        </button>
      </div>

      <div className="device-heading">
        <h3>{getDeviceName(device)}</h3>
        <span>{device.model}</span>
      </div>

      <div className="device-specs">
        <div>
          <span>系统版本</span>
          <strong>{device.version}</strong>
        </div>
        <div>
          <span>设备 ID</span>
          <strong className="mono">{device.device_id}</strong>
        </div>
      </div>

      {device.error_message && (
        <div className="inline-error">
          <CircleAlert size={14} /> {device.error_message}
        </div>
      )}

      <button className="start-button" disabled={!available} onClick={(event) => { event.stopPropagation(); onOpen() }}>
        {available ? '选择设备' : '暂不可用'}
        {available && <ChevronRight size={16} />}
      </button>
    </article>
  )
}

type MonitoringPhase = 'idle' | 'starting' | 'running' | 'stopping' | 'stopped' | 'interrupted' | 'error'
type MetricKey =
  | 'app_cpu'
  | 'total_cpu'
  | 'memory_pss'
  | 'java_heap'
  | 'native_heap'
  | 'stack'
  | 'graphics'
  | 'fps'
  | 'gpu_device_utilization'
  | 'frames'
  | 'refresh_rate'
  | 'small_jank'
  | 'jank'
  | 'big_jank'
  | 'total_small_jank'
  | 'total_jank'
  | 'total_big_jank'

type MetricSeriesConfig = { metric: MetricKey; label: string; color: string }

const ANDROID_DEFAULT_METRICS = new Set<MetricKey>([
  'app_cpu',
  'total_cpu',
  'memory_pss',
  'java_heap',
  'native_heap',
  'stack',
  'graphics',
  'fps',
  'frames',
  'refresh_rate',
  'small_jank',
  'jank',
  'big_jank',
  'total_small_jank',
  'total_jank',
  'total_big_jank',
])

function isIOSPlatform(platform?: string) {
  return (platform || '').toLowerCase() === 'ios'
}

function isAppProcessExitError(message: string): boolean {
  const normalized = message.toLowerCase()
  return message.includes('应用进程不存在')
    || message.includes('应用已退出')
    || message.includes('进程已退出')
    || message.includes('未获取到应用 TOTAL PSS')
    || normalized.includes('no process found')
    || normalized.includes('process not found')
    || normalized.includes('app is not running')
}

function isDeviceDisconnectedError(message: string): boolean {
  const normalized = message.toLowerCase()
  return message.includes('设备已断开')
    || message.includes('设备断开')
    || message.includes('设备离线')
    || normalized.includes('device offline')
    || normalized.includes('device disconnected')
    || normalized.includes('device not found')
    || normalized.includes('no devices')
}

function createMonitoringSummary(metrics: MonitoringMetrics): MonitoringSummary {
  const { samples } = metrics
  const average = (values: number[]) => values.length
    ? values.reduce((total, value) => total + value, 0) / values.length
    : 0

  return {
    duration_seconds: samples.length > 1 ? Math.round(((samples.length - 1) * metrics.sample_interval_ms) / 1_000) : 0,
    sample_count: samples.length,
    avg_app_cpu: average(samples.map((sample) => sample.app_cpu)),
    avg_fps: average(samples.map((sample) => sample.fps)),
    min_fps: samples.length ? Math.min(...samples.map((sample) => sample.fps)) : 0,
    max_memory_pss_mb: samples.length ? Math.max(...samples.map((sample) => sample.memory_pss)) : 0,
    total_small_jank: samples.at(-1)?.total_small_jank ?? 0,
    total_jank: samples.at(-1)?.total_jank ?? 0,
    total_big_jank: samples.at(-1)?.total_big_jank ?? 0,
  }
}

function MetricSparkline({ samples, metric, color }: { samples: MonitoringSample[]; metric: MetricKey; color: string }) {
  const recentSamples = samples.slice(-30)
  const values = recentSamples.map((sample) => Number(sample[metric]) || 0)
  const maxValue = Math.max(...values, 1)
  const pointForValue = (value: number, index: number) => {
    const x = values.length <= 1 ? 100 : (index / (values.length - 1)) * 100
    const y = 31 - (value / maxValue) * 27
    return `${x},${y}`
  }
  const points = values.length === 1
    ? `0,${31 - (values[0] / maxValue) * 27} 100,${31 - (values[0] / maxValue) * 27}`
    : values.map(pointForValue).join(' ')

  return (
    <svg className="metric-sparkline" viewBox="0 0 100 34" preserveAspectRatio="none" aria-hidden="true">
      <polyline points={points} fill="none" stroke={color} strokeWidth="2" vectorEffect="non-scaling-stroke" />
    </svg>
  )
}

function formatSampleTime(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value || '—'
  return date.toLocaleTimeString('zh-CN', { hour12: false })
}

function formatChartValue(metric: MetricKey, value: number, unit: string) {
  if (metric === 'refresh_rate') return `${value.toFixed(0)} Hz`
  if (unit === '%') return `${value.toFixed(1)}%`
  if (unit === 'MB') return `${value.toFixed(1)} MB`
  if (unit === 'fps') return `${value.toFixed(1)} FPS`
  if (unit === 'count') return value.toFixed(0)
  return `${value.toFixed(1)} ${unit}`
}

function RealtimeCurveChart({
  title,
  samples,
  series,
  unit,
  fixedRange,
}: {
  title: string
  samples: MonitoringSample[]
  series: MetricSeriesConfig[]
  unit: string
  fixedRange?: [number, number]
}) {
  const [hoverIndex, setHoverIndex] = useState<number | null>(null)
  const recentSamples = samples.slice(-60)
  const allValues = recentSamples.flatMap((sample) => series.map(({ metric }) => Number(sample[metric]) || 0))
  const dataMin = allValues.length ? Math.min(...allValues) : 0
  const dataMax = allValues.length ? Math.max(...allValues) : 1
  const rangeMin = fixedRange?.[0] ?? (dataMin > 0 ? dataMin * .94 : 0)
  const rawRangeMax = fixedRange?.[1] ?? Math.max(dataMax * 1.06, rangeMin + 1)
  const rangeMax = rawRangeMax > rangeMin ? rawRangeMax : rangeMin + 1
  const chartLeft = 43
  const chartRight = 592
  const chartTop = 14
  const chartBottom = 160
  const chartWidth = chartRight - chartLeft
  const chartHeight = chartBottom - chartTop
  const valueToY = (value: number) => chartBottom - ((value - rangeMin) / (rangeMax - rangeMin)) * chartHeight
  const sampleToX = (index: number) => recentSamples.length <= 1
    ? chartRight
    : chartLeft + (index / (recentSamples.length - 1)) * chartWidth
  const formatAxisValue = (value: number) => unit === '%' ? value.toFixed(0) : value.toFixed(0)
  const firstTime = recentSamples[0]?.collected_at
  const lastTime = recentSamples.at(-1)?.collected_at
  const chartSeries = series.map((item, seriesIndex) => {
    const values = recentSamples.map((sample) => Number(sample[item.metric]) || 0)
    const allZero = values.length > 0 && values.every((value) => value === 0)
    const zeroLaneOffset = allZero ? Math.min(seriesIndex * 4, 10) : 0
    const pointForSample = (value: number, index: number) => `${sampleToX(index)},${valueToY(value) - zeroLaneOffset}`
    const points = values.length === 1
      ? `${chartLeft},${valueToY(values[0]) - zeroLaneOffset} ${chartRight},${valueToY(values[0]) - zeroLaneOffset}`
      : values.map(pointForSample).join(' ')

    return { ...item, values, allZero, zeroLaneOffset, points }
  })
  const activeHoverIndex = hoverIndex == null || !recentSamples[hoverIndex] ? null : hoverIndex
  const hoverSample = activeHoverIndex == null ? null : recentSamples[activeHoverIndex]
  const hoverX = activeHoverIndex == null ? null : sampleToX(activeHoverIndex)
  const hoverRows = hoverSample
    ? chartSeries.map((item) => {
      const value = Number(hoverSample[item.metric]) || 0
      return {
        ...item,
        value,
        y: valueToY(value) - item.zeroLaneOffset,
      }
    })
    : []
  const tooltipWidth = 154
  const tooltipHeight = 28 + hoverRows.length * 14
  const hoverMinY = hoverRows.length ? Math.min(...hoverRows.map((row) => row.y)) : chartTop
  const tooltipX = hoverX == null
    ? 0
    : Math.max(8, Math.min(610 - tooltipWidth - 4, hoverX + 12 > 610 - tooltipWidth ? hoverX - tooltipWidth - 12 : hoverX + 12))
  const tooltipY = Math.max(8, Math.min(190 - tooltipHeight - 8, hoverMinY - tooltipHeight / 2))
  const handleChartMouseMove = (event: ReactMouseEvent<SVGSVGElement>) => {
    if (!recentSamples.length) return
    const rect = event.currentTarget.getBoundingClientRect()
    if (!rect.width) return

    const viewBoxX = ((event.clientX - rect.left) / rect.width) * 610
    const ratio = (viewBoxX - chartLeft) / chartWidth
    const nextIndex = Math.max(0, Math.min(recentSamples.length - 1, Math.round(ratio * (recentSamples.length - 1))))
    setHoverIndex((currentIndex) => currentIndex === nextIndex ? currentIndex : nextIndex)
  }

  return (
    <section className="realtime-chart-card" aria-label={title}>
      <div className="realtime-chart-header">
        <strong>{title}</strong>
        <div>{series.map((item) => <span key={item.metric}><i style={{ background: item.color }} />{item.label}</span>)}</div>
      </div>
      <svg
        className="realtime-chart"
        viewBox="0 0 610 190"
        preserveAspectRatio="none"
        role="img"
        aria-label={`${title}，最近 ${recentSamples.length} 个采样点`}
        onMouseMove={handleChartMouseMove}
        onMouseLeave={() => setHoverIndex(null)}
      >
        {[0, .25, .5, .75, 1].map((position) => {
          const y = chartTop + position * chartHeight
          const value = rangeMax - position * (rangeMax - rangeMin)
          return (
            <g key={position}>
              <line x1={chartLeft} y1={y} x2={chartRight} y2={y} className="chart-grid-line" />
              <text x={chartLeft - 7} y={y + 3} textAnchor="end" className="chart-axis-text">{formatAxisValue(value)}</text>
            </g>
          )
        })}
        <text x="8" y="12" className="chart-axis-unit">{unit}</text>
        {chartSeries.map((item) => (
          <polyline key={item.metric} points={item.points} fill="none" stroke={item.color} strokeWidth="2.4" vectorEffect="non-scaling-stroke" className={`chart-series ${item.allZero ? 'zero-series' : ''}`} />
        ))}
        <rect x={chartLeft} y={chartTop} width={chartWidth} height={chartHeight} fill="transparent" className="chart-hover-zone" />
        {hoverSample && hoverX != null && (
          <g className="chart-hover-layer">
            <line x1={hoverX} y1={chartTop} x2={hoverX} y2={chartBottom} className="chart-hover-line" />
            {hoverRows.map((item) => (
              <circle key={item.metric} cx={hoverX} cy={item.y} r="3.2" fill={item.color} className="chart-hover-dot" />
            ))}
            <g className="chart-hover-tooltip" transform={`translate(${tooltipX}, ${tooltipY})`}>
              <rect width={tooltipWidth} height={tooltipHeight} rx="8" className="chart-tooltip-bg" />
              <text x="10" y="14" className="chart-tooltip-time">时间 {formatSampleTime(hoverSample.collected_at)}</text>
              {hoverRows.map((item, rowIndex) => (
                <g key={item.metric} transform={`translate(0, ${28 + rowIndex * 14})`}>
                  <circle cx="12" cy="-3" r="2.4" fill={item.color} />
                  <text x="20" y="0" className="chart-tooltip-label">{item.label}</text>
                  <text x={tooltipWidth - 10} y="0" textAnchor="end" className="chart-tooltip-value">{formatChartValue(item.metric, item.value, unit)}</text>
                </g>
              ))}
            </g>
          </g>
        )}
        {firstTime && <text x={chartLeft} y="184" className="chart-axis-text">{new Date(firstTime).toLocaleTimeString('zh-CN', { hour12: false })}</text>}
        {lastTime && <text x={chartRight} y="184" textAnchor="end" className="chart-axis-text">{new Date(lastTime).toLocaleTimeString('zh-CN', { hour12: false })}</text>}
      </svg>
      {!recentSamples.length && <div className="chart-empty">等待性能采样数据</div>}
    </section>
  )
}

function MonitoringDashboard({
  metrics,
  phase,
  summary,
  error,
  onStop,
}: {
  metrics: MonitoringMetrics | null
  phase: MonitoringPhase
  summary: MonitoringSummary | null
  error: string
  onStop: () => void
}) {
  const samples = metrics?.samples ?? []
  const latest = samples.at(-1)
  const isActive = phase === 'running' || phase === 'stopping'
  const statusLabel = isActive ? '采集中' : phase === 'error' ? '异常停止' : '已停止'

  return (
    <section className="monitoring-dashboard" aria-label="实时性能数据">
      <div className="monitoring-header">
        <div>
          <div className="monitoring-title"><ChartNoAxesCombined size={19} /><strong>实时性能数据</strong><span className={`monitoring-status ${isActive ? 'running' : phase === 'error' ? 'error' : 'stopped'}`}><i />{statusLabel}</span></div>
          <p className="mono">{metrics?.package_name || '—'} · taskId: {metrics?.task_id || '—'} · 每秒自动刷新</p>
        </div>
        {isActive && (
          <button className="stop-monitoring-button" onClick={onStop} disabled={phase === 'stopping'}>
            <Square size={13} fill="currentColor" />{phase === 'stopping' ? '正在停止' : '停止采集'}
          </button>
        )}
      </div>

      {error && <div className="monitoring-error"><CircleAlert size={16} />{error}</div>}

      <div className="metric-cards">
        <div className="metric-card cyan">
          <div><span>FPS</span><strong>{latest ? latest.fps.toFixed(1) : '—'}<small>fps</small></strong></div>
          <MetricSparkline samples={samples} metric="fps" color="#159db5" />
        </div>
        <div className="metric-card purple">
          <div><span>appCPU</span><strong>{latest ? latest.app_cpu.toFixed(2) : '—'}<small>%</small></strong></div>
          <MetricSparkline samples={samples} metric="app_cpu" color="#6d5be8" />
        </div>
        <div className="metric-card green">
          <div><span>totalCPU</span><strong>{latest ? latest.total_cpu.toFixed(2) : '—'}<small>%</small></strong></div>
          <MetricSparkline samples={samples} metric="total_cpu" color="#1a9c69" />
        </div>
        <div className="metric-card amber">
          <div><span>totalPSS</span><strong>{latest ? latest.memory_pss.toFixed(1) : '—'}<small>MB</small></strong></div>
          <MetricSparkline samples={samples} metric="memory_pss" color="#c28123" />
        </div>
      </div>

      <div className="metric-breakdown">
        <section className="breakdown-card" aria-label="Memory Metrics">
          <div className="breakdown-header"><MemoryStick size={16} /><strong>Memory Metrics</strong><span>MB</span></div>
          <div className="breakdown-grid memory">
            {([
              ['totalPSS', latest?.memory_pss],
              ['javaHeap', latest?.java_heap],
              ['nativeHeap', latest?.native_heap],
              ['stack', latest?.stack],
              ['graphics', latest?.graphics],
            ] as const).map(([label, value]) => (
              <div key={label}><span>{label}</span><strong>{value == null ? '—' : value.toFixed(1)}<small> MB</small></strong></div>
            ))}
          </div>
        </section>

        <section className="breakdown-card" aria-label="Jank Metrics">
          <div className="breakdown-header"><Gauge size={16} /><strong>Jank Metrics</strong><span>count</span></div>
          <div className="breakdown-grid jank">
            {([
              ['smallJank', latest?.small_jank],
              ['jank', latest?.jank],
              ['bigJank', latest?.big_jank],
              ['totalSmallJank', latest?.total_small_jank],
              ['totalJank', latest?.total_jank],
              ['totalBigJank', latest?.total_big_jank],
            ] as const).map(([label, value]) => (
              <div key={label}><span>{label}</span><strong>{value == null ? '—' : value}</strong></div>
            ))}
          </div>
        </section>
      </div>

      <div className="realtime-charts">
        <RealtimeCurveChart
          title="FPS Realtime"
          samples={samples}
          unit="fps"
          fixedRange={[0, Math.max(60, latest?.refresh_rate ?? 0)]}
          series={[
            { metric: 'fps', label: 'FPS', color: '#159db5' },
            { metric: 'refresh_rate', label: 'refreshRate', color: '#9b6be3' },
          ]}
        />
        <RealtimeCurveChart
          title="CPU Realtime"
          samples={samples}
          unit="%"
          fixedRange={[0, 100]}
          series={[
            { metric: 'app_cpu', label: 'appCPU', color: '#6d5be8' },
            { metric: 'total_cpu', label: 'totalCPU', color: '#1a9c69' },
          ]}
        />
        <RealtimeCurveChart
          title="Memory Heaps Realtime"
          samples={samples}
          unit="MB"
          series={[
            { metric: 'memory_pss', label: 'totalPSS', color: '#c28123' },
            { metric: 'java_heap', label: 'javaHeap', color: '#6d5be8' },
            { metric: 'native_heap', label: 'nativeHeap', color: '#1a9c69' },
          ]}
        />
        <RealtimeCurveChart
          title="Memory Details Realtime"
          samples={samples}
          unit="MB"
          series={[
            { metric: 'stack', label: 'stack', color: '#159db5' },
            { metric: 'graphics', label: 'graphics', color: '#d24f7c' },
          ]}
        />
        <RealtimeCurveChart
          title="Jank Sample Realtime"
          samples={samples}
          unit="count"
          series={[
            { metric: 'small_jank', label: 'smallJank', color: '#159db5' },
            { metric: 'jank', label: 'jank', color: '#d24f7c' },
            { metric: 'big_jank', label: 'bigJank', color: '#e48a2b' },
          ]}
        />
        <RealtimeCurveChart
          title="Jank Total Realtime"
          samples={samples}
          unit="count"
          series={[
            { metric: 'total_small_jank', label: 'totalSmallJank', color: '#159db5' },
            { metric: 'total_jank', label: 'totalJank', color: '#d24f7c' },
            { metric: 'total_big_jank', label: 'totalBigJank', color: '#e48a2b' },
          ]}
        />
      </div>

      {summary && (phase === 'stopped' || phase === 'error') && (
        <div className="monitoring-summary">
          <div><span>采集时长</span><strong>{summary.duration_seconds}s</strong></div>
          <div><span>样本数量</span><strong>{summary.sample_count}</strong></div>
          <div><span>平均 FPS</span><strong>{summary.avg_fps.toFixed(1)}</strong></div>
          <div><span>平均 appCPU</span><strong>{summary.avg_app_cpu.toFixed(2)}%</strong></div>
          <div><span>峰值 totalPSS</span><strong>{summary.max_memory_pss_mb.toFixed(1)} MB</strong></div>
          <div><span>totalSmallJank</span><strong>{summary.total_small_jank}</strong></div>
          <div><span>totalJank</span><strong>{summary.total_jank}</strong></div>
          <div><span>totalBigJank</span><strong>{summary.total_big_jank}</strong></div>
        </div>
      )}

      <h3 className="samples-section-title">Performance Samples</h3>
      <div className="samples-table-wrap">
        <table className="samples-table">
          <thead><tr><th>采集时间</th><th>FPS</th><th>appCPU</th><th>totalCPU</th></tr></thead>
          <tbody>
            {samples.length ? samples.slice(-8).reverse().map((sample) => (
              <tr key={sample.collected_at}>
                <td>{new Date(sample.collected_at).toLocaleTimeString('zh-CN', { hour12: false })}</td>
                <td>{sample.fps.toFixed(1)}</td>
                <td>{sample.app_cpu.toFixed(2)}%</td>
                <td>{sample.total_cpu.toFixed(2)}%</td>
              </tr>
            )) : <tr><td colSpan={4} className="no-samples">正在等待第一条性能数据…</td></tr>}
          </tbody>
        </table>
      </div>

      <h3 className="samples-section-title">Memory Samples</h3>
      <div className="samples-table-wrap">
        <table className="samples-table">
          <thead><tr><th>采集时间</th><th>totalPSS</th><th>javaHeap</th><th>nativeHeap</th><th>stack</th><th>graphics</th></tr></thead>
          <tbody>
            {samples.length ? samples.slice(-8).reverse().map((sample) => (
              <tr key={sample.collected_at}>
                <td>{new Date(sample.collected_at).toLocaleTimeString('zh-CN', { hour12: false })}</td>
                <td>{sample.memory_pss.toFixed(1)} MB</td>
                <td>{sample.java_heap.toFixed(1)} MB</td>
                <td>{sample.native_heap.toFixed(1)} MB</td>
                <td>{sample.stack.toFixed(1)} MB</td>
                <td>{sample.graphics.toFixed(1)} MB</td>
              </tr>
            )) : <tr><td colSpan={6} className="no-samples">正在等待第一条内存数据…</td></tr>}
          </tbody>
        </table>
      </div>

      <h3 className="samples-section-title">Jank Samples</h3>
      <div className="samples-table-wrap">
        <table className="samples-table">
          <thead><tr><th>采集时间</th><th>smallJank</th><th>jank</th><th>bigJank</th><th>totalSmallJank</th><th>totalJank</th><th>totalBigJank</th></tr></thead>
          <tbody>
            {samples.length ? samples.slice(-8).reverse().map((sample) => (
              <tr key={sample.collected_at}>
                <td>{new Date(sample.collected_at).toLocaleTimeString('zh-CN', { hour12: false })}</td>
                <td>{sample.small_jank}</td>
                <td>{sample.jank}</td>
                <td>{sample.big_jank}</td>
                <td>{sample.total_small_jank}</td>
                <td>{sample.total_jank}</td>
                <td>{sample.total_big_jank}</td>
              </tr>
            )) : <tr><td colSpan={7} className="no-samples">正在等待第一条 Jank 数据…</td></tr>}
          </tbody>
        </table>
      </div>
    </section>
  )
}

function formatDuration(totalSeconds: number) {
  const safeSeconds = Math.max(0, Math.floor(totalSeconds))
  const hours = Math.floor(safeSeconds / 3600)
  const minutes = Math.floor((safeSeconds % 3600) / 60)
  const seconds = safeSeconds % 60
  return [hours, minutes, seconds].map((value) => String(value).padStart(2, '0')).join(':')
}

function isFinishedMonitoringPhase(phase: MonitoringPhase) {
  return phase === 'stopped' || phase === 'interrupted' || phase === 'error'
}

function MonitoringWorkbench({
  device,
  packageName,
  metrics,
  phase,
  summary,
  error,
  lastMetricsSyncedAt,
  lastMetricsCostMs,
  exitLabel = '返回应用选择',
  showWindowControls = true,
  onStop,
  onExit,
  onExportCsv,
}: {
  device: Device
  packageName: string
  metrics: MonitoringMetrics | null
  phase: MonitoringPhase
  summary: MonitoringSummary | null
  error: string
  lastMetricsSyncedAt: Date | null
  lastMetricsCostMs: number | null
  exitLabel?: string
  showWindowControls?: boolean
  onStop: () => void
  onExit: () => void
  onExportCsv?: () => Promise<void> | void
}) {
  const [exportingCsv, setExportingCsv] = useState(false)
  const samples = metrics?.samples ?? []
  const latest = samples.at(-1)
  const isCollectingView = phase === 'starting' || phase === 'running' || phase === 'stopping'
  const canStop = phase === 'running' || phase === 'stopping'
  const statusLabel = phase === 'running'
    ? '监控中'
    : phase === 'starting'
      ? '启动中'
      : phase === 'stopping'
        ? '停止中'
        : phase === 'interrupted' || phase === 'error'
          ? '已中断'
          : '已停止'
  const activePackageName = metrics?.package_name || packageName
  const elapsedSeconds = summary?.duration_seconds
    ?? (samples.length > 1 ? Math.round(((samples.length - 1) * (metrics?.sample_interval_ms ?? 1_000)) / 1_000) : 0)
  const activePlatform = metrics?.platform || device.platform
  const filterByActualMetrics = isIOSPlatform(activePlatform)
  const hasMetric = (metric: MetricKey) => {
    if (metrics?.available_metrics?.[metric]) return true
    if (!filterByActualMetrics && ANDROID_DEFAULT_METRICS.has(metric)) return true
    return false
  }
  const metricValue = (metric: MetricKey) => latest ? Number(latest[metric]) || 0 : 0
  const memoryMetricConfigs: MetricSeriesConfig[] = [
    { metric: 'memory_pss', label: 'totalPSS', color: '#55a2ff' },
    { metric: 'java_heap', label: 'javaHeap', color: '#a177ff' },
    { metric: 'native_heap', label: 'nativeHeap', color: '#3dd598' },
    { metric: 'stack', label: 'stack', color: '#27c8df' },
    { metric: 'graphics', label: 'graphics', color: '#f091bd' },
  ]
  const memoryMetrics = memoryMetricConfigs.filter((item) => hasMetric(item.metric))
  const jankMetricConfigs: MetricSeriesConfig[] = [
    { metric: 'small_jank', label: 'smallJank', color: '#27c8df' },
    { metric: 'jank', label: 'jank', color: '#f091bd' },
    { metric: 'big_jank', label: 'bigJank', color: '#f5aa42' },
    { metric: 'total_small_jank', label: 'totalSmallJank', color: '#55a2ff' },
    { metric: 'total_jank', label: 'totalJank', color: '#a177ff' },
    { metric: 'total_big_jank', label: 'totalBigJank', color: '#ff6b78' },
  ]
  const jankMetrics = jankMetricConfigs.filter((item) => hasMetric(item.metric))
  const fpsMetricConfigs: MetricSeriesConfig[] = [
    { metric: 'fps', label: 'FPS', color: '#27d9ea' },
    { metric: 'refresh_rate', label: 'refreshRate', color: '#a177ff' },
  ]
  const fpsSeries = fpsMetricConfigs.filter((item) => {
    if (!hasMetric(item.metric)) return false
    if (item.metric !== 'refresh_rate') return true
    return filterByActualMetrics || Boolean(latest && latest.refresh_rate > 0)
  })
  const cpuMetricConfigs: MetricSeriesConfig[] = [
    { metric: 'app_cpu', label: 'appCPU', color: '#4d8dff' },
    { metric: 'total_cpu', label: 'totalCPU', color: '#3dd598' },
  ]
  const cpuSeries = cpuMetricConfigs.filter((item) => hasMetric(item.metric))
  const memoryHeapKeys: MetricKey[] = ['memory_pss', 'java_heap', 'native_heap']
  const memoryDetailKeys: MetricKey[] = ['stack', 'graphics']
  const jankSampleKeys: MetricKey[] = ['small_jank', 'jank', 'big_jank']
  const jankTotalKeys: MetricKey[] = ['total_small_jank', 'total_jank', 'total_big_jank']
  const memoryHeapSeries = memoryMetrics.filter((item) => memoryHeapKeys.includes(item.metric))
  const memoryDetailSeries = memoryMetrics.filter((item) => memoryDetailKeys.includes(item.metric))
  const jankSampleSeries = jankMetrics.filter((item) => jankSampleKeys.includes(item.metric))
  const jankTotalSeries = jankMetrics.filter((item) => jankTotalKeys.includes(item.metric))
  const gpuSeries: MetricSeriesConfig[] = hasMetric('gpu_device_utilization')
    ? [{ metric: 'gpu_device_utilization', label: 'gpuDeviceUtilization', color: '#f5aa42' }]
    : []
  const metricCards = [
    {
      metric: 'fps' as MetricKey,
      className: 'cyan',
      label: 'FPS',
      value: latest ? latest.fps.toFixed(1) : '—',
      suffix: '',
      hint: latest && latest.fps >= 50 ? '稳定' : latest ? '观察中' : '等待中',
      color: '#27d9ea',
    },
    {
      metric: 'app_cpu' as MetricKey,
      className: 'blue',
      label: 'appCPU',
      value: latest ? latest.app_cpu.toFixed(1) : '—',
      suffix: '%',
      hint: '应用',
      color: '#4d8dff',
    },
    {
      metric: 'total_cpu' as MetricKey,
      className: 'violet',
      label: 'totalCPU',
      value: latest ? latest.total_cpu.toFixed(1) : '—',
      suffix: '%',
      hint: '设备',
      color: '#a177ff',
    },
    {
      metric: 'memory_pss' as MetricKey,
      className: 'green',
      label: 'totalPSS',
      value: latest ? latest.memory_pss.toFixed(0) : '—',
      suffix: ' MB',
      hint: '内存',
      color: '#3dd598',
    },
    {
      metric: 'gpu_device_utilization' as MetricKey,
      className: 'amber',
      label: 'gpuDeviceUtilization',
      value: latest ? latest.gpu_device_utilization.toFixed(1) : '—',
      suffix: '%',
      hint: 'GPU',
      color: '#f5aa42',
    },
  ].filter((item) => hasMetric(item.metric))
  const chartGroups = [
    { title: 'FPS 实时曲线', unit: 'fps', fixedRange: [0, Math.max(60, latest?.refresh_rate ?? 0)] as [number, number], series: fpsSeries },
    { title: 'CPU 实时曲线', unit: '%', fixedRange: [0, 100] as [number, number], series: cpuSeries },
    { title: 'GPU 实时曲线', unit: '%', fixedRange: [0, 100] as [number, number], series: gpuSeries },
    { title: '内存实时曲线', unit: 'MB', series: memoryHeapSeries },
    { title: '内存详情实时曲线', unit: 'MB', series: memoryDetailSeries },
    { title: 'Jank 实时曲线', unit: 'count', series: jankSampleSeries },
    { title: 'Jank 累计实时曲线', unit: 'count', series: jankTotalSeries },
  ].filter((group) => group.series.length > 0)
  const canExportCsv = Boolean(onExportCsv && metrics?.task_id && isFinishedMonitoringPhase(phase))
  const handleExportCsv = async () => {
    if (!onExportCsv || exportingCsv) return

    setExportingCsv(true)
    try {
      await onExportCsv()
    } finally {
      setExportingCsv(false)
    }
  }

  return (
    <div className="monitor-workbench-overlay">
      <section className="monitor-workbench" aria-label="实时性能监控工作台">
        <header className={`workbench-windowbar ${showWindowControls ? '' : 'no-window-controls'}`}>
          {showWindowControls && <div className="window-lights" aria-hidden="true"><i /><i /><i /></div>}
          <strong>PerfRabbit · 移动性能监控台</strong>
          <span className={`workbench-live-state ${isCollectingView ? 'running' : phase === 'interrupted' || phase === 'error' ? 'error' : ''}`}><i />{statusLabel}</span>
        </header>

        <aside className="workbench-session-panel">
          <div className="workbench-section-title">
            <MonitorSmartphone size={16} />
            <strong>测试会话</strong>
            <span>{samples.length} 个采样</span>
          </div>

          <div className="session-device-card">
            <div className="session-device-icon"><Smartphone size={24} /></div>
            <div>
              <strong>{getDeviceName(device)}</strong>
              <span>{device.platform} {device.version}</span>
            </div>
            <i className={isCollectingView ? 'active' : ''} />
          </div>

          <div className="session-meta">
            <span>设备 ID</span>
            <strong className="mono">{device.device_id}</strong>
          </div>
          <div className="session-meta">
            <span>包名</span>
            <strong className="mono">{activePackageName || '—'}</strong>
          </div>
          <div className="session-meta">
            <span>任务 ID</span>
            <strong className="mono">{metrics?.task_id || (phase === 'starting' ? '初始化中…' : '—')}</strong>
          </div>

          <div className="session-metric-list">
            {cpuSeries.length > 0 && <div><Cpu size={13} /><span>CPU</span><strong>{cpuSeries.map((item) => item.label).join(' / ')}</strong></div>}
            {hasMetric('fps') && <div><Gauge size={13} /><span>FPS</span><strong>{latest ? latest.fps.toFixed(1) : '—'}</strong></div>}
            {hasMetric('memory_pss') && <div><MemoryStick size={13} /><span>内存</span><strong>{latest ? `${latest.memory_pss.toFixed(1)} MB` : '—'}</strong></div>}
            {hasMetric('gpu_device_utilization') && <div><Zap size={13} /><span>GPU</span><strong>{latest ? `${latest.gpu_device_utilization.toFixed(1)}%` : '—'}</strong></div>}
          </div>

          {error && <div className="workbench-side-error"><CircleAlert size={14} />{error}</div>}

          <div className="session-actions">
            {canStop ? (
              <button className="workbench-stop-button" onClick={onStop} disabled={phase === 'stopping'}>
                <Square size={13} fill="currentColor" />
                {phase === 'stopping' ? '正在停止…' : '停止采集'}
              </button>
            ) : phase === 'starting' ? (
              <button className="workbench-stop-button" disabled>
                <RefreshCw size={13} className="spin" />
                正在启动采集…
              </button>
            ) : (
              <>
                {canExportCsv && (
                  <button className="workbench-export-button" onClick={handleExportCsv} disabled={exportingCsv}>
                    {exportingCsv ? <RefreshCw size={14} className="spin" /> : <Download size={14} />}
                    {exportingCsv ? '正在导出…' : '导出 CSV'}
                  </button>
                )}
                <button className="workbench-exit-button" onClick={onExit}>
                  <ArrowLeft size={14} />{exitLabel}
                </button>
              </>
            )}
          </div>
        </aside>

        <main className="workbench-realtime-panel">
          <div className="realtime-panel-heading">
            <div>
              <span className="workbench-kicker"><ChartNoAxesCombined size={15} />实时指标</span>
              <h2>{getDeviceName(device)}</h2>
              <p className="mono">{activePackageName || '等待任务数据'}</p>
            </div>
            <div className="recording-block">
              <span>{phase === 'starting' ? '初始化中' : canStop ? '采集中' : statusLabel}</span>
              <strong className="mono">{formatDuration(elapsedSeconds)}</strong>
            </div>
          </div>

          <div className="workbench-metric-grid">
            {metricCards.length ? metricCards.map((item) => (
              <div className={`workbench-metric ${item.className}`} key={item.metric}>
                <span>{item.label}</span>
                <strong>{item.value}<em>{item.suffix}</em></strong>
                <small>{item.hint}</small>
                <MetricSparkline samples={samples} metric={item.metric} color={item.color} />
              </div>
            )) : (
              <div className="workbench-metric-empty">等待后端返回可展示指标</div>
            )}
          </div>

          <div className="workbench-chart-stack">
            {chartGroups.length ? chartGroups.map((group) => (
              <RealtimeCurveChart
                key={group.title}
                title={group.title}
                samples={samples}
                unit={group.unit}
                fixedRange={group.fixedRange}
                series={group.series}
              />
            )) : (
              <div className="workbench-chart-empty">等待后端返回可绘制指标</div>
            )}
          </div>
        </main>

        <aside className="workbench-insights-panel">
          <div className="workbench-section-title">
            <MemoryStick size={16} />
            <strong>{memoryMetrics.length || jankMetrics.length ? '内存与卡顿' : '指标详情'}</strong>
            <span>实时</span>
          </div>

          {memoryMetrics.length > 0 && (
            <>
              <section className="insight-hero">
                <div><MemoryStick size={23} /></div>
                <strong>MemoryInfo</strong>
                <span>{memoryMetrics.map((item) => item.label).join(' · ')}</span>
              </section>

              <div className="insight-list memory">
                {memoryMetrics.map((item) => (
                  <div key={item.metric}>
                    <span><i style={{ background: item.color }} />{item.label}</span>
                    <strong>{metricValue(item.metric).toFixed(1)}<small> MB</small></strong>
                  </div>
                ))}
              </div>
            </>
          )}

          {jankMetrics.length > 0 && (
            <>
              <div className="insight-subheading"><Gauge size={15} /><strong>Jank 指标</strong></div>
              <div className="insight-jank-grid">
                {jankMetrics.map((item) => (
                  <div key={item.metric}>
                    <i style={{ color: item.color }} />
                    <span>{item.label}</span>
                    <strong style={{ color: item.color }}>{metricValue(item.metric)}</strong>
                  </div>
                ))}
              </div>
            </>
          )}

          {memoryMetrics.length === 0 && jankMetrics.length === 0 && (
            <div className="insight-empty-block">当前平台暂未返回内存或 Jank 指标。</div>
          )}

          <section className="collection-accuracy">
            <strong>任务状态</strong>
            <div><span>采样数</span><b>{samples.length}</b></div>
            {metrics?.sample_interval_ms ? <div><span>采样间隔</span><b>{metrics.sample_interval_ms}ms</b></div> : null}
            {lastMetricsSyncedAt ? <div><span>最近更新</span><b>{lastMetricsSyncedAt.toLocaleTimeString('zh-CN', { hour12: false })}</b></div> : null}
            {lastMetricsCostMs != null ? <div><span>接口耗时</span><b>{lastMetricsCostMs}ms</b></div> : null}
            {latest && hasMetric('refresh_rate') && (filterByActualMetrics || latest.refresh_rate > 0) ? <div><span>refreshRate</span><b>{latest.refresh_rate.toFixed(0)}Hz</b></div> : null}
            {latest && hasMetric('frames') && (filterByActualMetrics || latest.frames > 0) ? <div><span>frames</span><b>{latest.frames}</b></div> : null}
          </section>
        </aside>

        <footer className="workbench-footer">
          <span><MonitorSmartphone size={14} />{device.platform}</span>
          <span><ShieldCheck size={14} />{statusLabel}</span>
          <span className="mono">{metrics?.task_id || '任务初始化中'}</span>
          <strong><Play size={13} fill="currentColor" />{latest && hasMetric('fps') ? `${latest.fps.toFixed(1)} FPS` : '等待中'}</strong>
        </footer>
      </section>
    </div>
  )
}

function DeviceDetailPage({ device, onBack }: { device: Device; onBack: () => void }) {
  const meta = statusMeta(device.status)
  const [apps, setApps] = useState<DeviceApp[]>([])
  const [appsLoading, setAppsLoading] = useState(true)
  const [appsError, setAppsError] = useState('')
  const [appsQuery, setAppsQuery] = useState('')
  const [selectedPackage, setSelectedPackage] = useState('')
  const [taskId, setTaskId] = useState('')
  const [monitoringPhase, setMonitoringPhase] = useState<MonitoringPhase>('idle')
  const [metrics, setMetrics] = useState<MonitoringMetrics | null>(null)
  const [monitoringSummary, setMonitoringSummary] = useState<MonitoringSummary | null>(null)
  const [monitoringError, setMonitoringError] = useState('')
  const [startChecking, setStartChecking] = useState(false)
  const [lastMetricsSyncedAt, setLastMetricsSyncedAt] = useState<Date | null>(null)
  const [lastMetricsCostMs, setLastMetricsCostMs] = useState<number | null>(null)
  const [appExitNotice, setAppExitNotice] = useState(false)
  const [deviceDisconnectNotice, setDeviceDisconnectNotice] = useState(false)
  const autoStoppingRef = useRef(false)
  const taskStorageKey = `perfRabbit:collectTask:${device.device_id}`
  const selectedApp = useMemo(
    () => apps.find((app) => app.package_name === selectedPackage),
    [apps, selectedPackage],
  )

  const visibleApps = useMemo(() => {
    const normalizedQuery = appsQuery.trim().toLowerCase()
    if (!normalizedQuery) return apps
    return apps.filter((app) => [app.app_name, app.package_name]
      .some((value) => (value ?? '').toLowerCase().includes(normalizedQuery)))
  }, [apps, appsQuery])

  const loadApps = useCallback(async (signal?: AbortSignal) => {
    setAppsLoading(true)
    setAppsError('')

    try {
      setApps(await getDeviceApps(device.device_id, signal))
    } catch (requestError) {
      if (requestError instanceof DOMException && requestError.name === 'AbortError') return
      setAppsError(requestError instanceof Error ? requestError.message : '加载应用列表失败')
    } finally {
      if (!signal?.aborted) setAppsLoading(false)
    }
  }, [device.device_id])

  useEffect(() => {
    const controller = new AbortController()
    loadApps(controller.signal)
    return () => controller.abort()
  }, [loadApps])

  const loadMetrics = useCallback(async (activeTaskId: string, signal?: AbortSignal) => {
    const startedAt = Date.now()
    try {
      const nextMetrics = await getMonitoringMetrics(activeTaskId, signal)
      if (!signal?.aborted) {
        setLastMetricsCostMs(Date.now() - startedAt)
        setLastMetricsSyncedAt(new Date())
      }
      if (nextMetrics.device_id === device.device_id) {
        setTaskId(nextMetrics.task_id)
        setSelectedPackage(nextMetrics.package_name)
        setMetrics(nextMetrics)

        if (nextMetrics.status === 'interrupted' || isDeviceDisconnectedError(nextMetrics.last_error)) {
          window.localStorage.removeItem(taskStorageKey)
          setMonitoringPhase('interrupted')
          setMonitoringSummary(createMonitoringSummary(nextMetrics))
          setMonitoringError(nextMetrics.last_error || '设备已经断开，采集已自动中断')
          setDeviceDisconnectNotice(true)
        } else if (isAppProcessExitError(nextMetrics.last_error)) {
          if (nextMetrics.status === 'running' && !autoStoppingRef.current) {
            autoStoppingRef.current = true
            try {
              await stopMonitoring(activeTaskId)
            } catch {
              // 任务可能已被服务端终止，仍然展示已采集的数据。
            }
          }
          window.localStorage.removeItem(taskStorageKey)
          setMonitoringPhase('error')
          setMonitoringSummary(createMonitoringSummary(nextMetrics))
          setMonitoringError(`应用已退出或被杀掉，性能采集已自动停止：${nextMetrics.last_error}`)
          setAppExitNotice(true)
        } else if (nextMetrics.status === 'stopped') {
          window.localStorage.removeItem(taskStorageKey)
          setMonitoringPhase('stopped')
          setMonitoringSummary(createMonitoringSummary(nextMetrics))
          setMonitoringError(nextMetrics.last_error || '')
        } else {
          setMonitoringPhase('running')
          setMonitoringError(nextMetrics.last_error ? `最近一次采集失败：${nextMetrics.last_error}` : '')
        }
      }
      return nextMetrics
    } catch (requestError) {
      if (requestError instanceof DOMException && requestError.name === 'AbortError') return null
      setMonitoringError(requestError instanceof Error ? requestError.message : '查询性能数据失败')
      return null
    }
  }, [device.device_id, taskStorageKey])

  useEffect(() => {
    const restoredTaskId = window.localStorage.getItem(taskStorageKey)
    if (!restoredTaskId) return

    const controller = new AbortController()
    setTaskId(restoredTaskId)
    setMonitoringPhase('starting')

    loadMetrics(restoredTaskId, controller.signal).then((restoredMetrics) => {
      if (!restoredMetrics && !controller.signal.aborted) {
        window.localStorage.removeItem(taskStorageKey)
        setTaskId('')
        setMonitoringPhase('idle')
      }
    })

    return () => controller.abort()
  }, [loadMetrics, taskStorageKey])

  useEffect(() => {
    if (monitoringPhase !== 'running' || !taskId) return

    const inFlightControllers = new Set<AbortController>()

    const poll = () => {
      if (inFlightControllers.size >= 3) return

      const controller = new AbortController()
      inFlightControllers.add(controller)
      loadMetrics(taskId, controller.signal).finally(() => {
        inFlightControllers.delete(controller)
      })
    }

    poll()
    const interval = window.setInterval(poll, 1_000)

    return () => {
      window.clearInterval(interval)
      inFlightControllers.forEach((controller) => controller.abort())
      inFlightControllers.clear()
    }
  }, [loadMetrics, monitoringPhase, taskId])

  const handleStartMonitoring = useCallback(async () => {
    if (!selectedPackage) return

    setStartChecking(true)
    setMonitoringError('')
    setMonitoringSummary(null)
    setMetrics(null)
    setLastMetricsSyncedAt(null)
    setLastMetricsCostMs(null)
    setAppExitNotice(false)
    setDeviceDisconnectNotice(false)
    autoStoppingRef.current = false

    try {
      const startedTask = await startMonitoring(
        device.device_id,
        selectedPackage,
        selectedApp?.executable,
        device.product_type || device.model,
      )
      setTaskId(startedTask.task_id)
      window.localStorage.setItem(taskStorageKey, startedTask.task_id)
      setMonitoringPhase('running')
    } catch (requestError) {
      setMonitoringError(requestError instanceof Error ? requestError.message : '启动性能采集失败')
      setMonitoringPhase('idle')
    } finally {
      setStartChecking(false)
    }
  }, [device.device_id, device.model, device.product_type, selectedApp?.executable, selectedPackage, taskStorageKey])

  const handleStopMonitoring = useCallback(async () => {
    if (!taskId) return

    setMonitoringPhase('stopping')
    setMonitoringError('')

    try {
      await stopMonitoring(taskId)
      let finalMetrics = metrics

      try {
        finalMetrics = await getMonitoringMetrics(taskId)
        setMetrics(finalMetrics)
      } catch {
        // 停止已成功时，最终查询失败不应该将任务恢复为采集中。
      }

      if (finalMetrics) setMonitoringSummary(createMonitoringSummary(finalMetrics))
      window.localStorage.removeItem(taskStorageKey)
      setMonitoringPhase('stopped')
    } catch (requestError) {
      setMonitoringError(requestError instanceof Error ? requestError.message : '停止性能采集失败')
      setMonitoringPhase('running')
    }
  }, [metrics, taskId, taskStorageKey])

  const handleExportCsv = useCallback(async () => {
    const exportTaskId = metrics?.task_id || taskId
    if (!exportTaskId || !isFinishedMonitoringPhase(monitoringPhase)) return

    try {
      await downloadPerfHistoryCsv(exportTaskId)
      setMonitoringError('')
    } catch (requestError) {
      setMonitoringError(requestError instanceof Error ? requestError.message : '导出 CSV 失败')
    }
  }, [metrics?.task_id, monitoringPhase, taskId])

  const handleExitMonitoring = useCallback(() => {
    setMonitoringPhase('idle')
    setMetrics(null)
    setMonitoringSummary(null)
    setMonitoringError('')
    setLastMetricsSyncedAt(null)
    setLastMetricsCostMs(null)
    setDeviceDisconnectNotice(false)
    setTaskId('')
  }, [])

  const monitoringLocked = startChecking || monitoringPhase === 'starting' || monitoringPhase === 'running' || monitoringPhase === 'stopping'
  const monitoringViewActive = monitoringPhase === 'starting'
    || monitoringPhase === 'running'
    || monitoringPhase === 'stopping'
    || monitoringPhase === 'stopped'
    || monitoringPhase === 'interrupted'
    || monitoringPhase === 'error'
  const appExitModal = appExitNotice ? (
    <div className="monitoring-modal-layer" role="alertdialog" aria-modal="true" aria-label="应用已退出">
      <div className="monitoring-modal-backdrop" />
      <div className="monitoring-modal">
        <div className="monitoring-modal-icon"><CircleAlert size={24} /></div>
        <h3>采集已自动停止</h3>
        <p>检测到应用已退出或被杀掉，性能数据采集已停止。</p>
        <div className="monitoring-modal-package"><span>应用包名</span><strong className="mono">{metrics?.package_name || selectedPackage}</strong></div>
        <button onClick={() => setAppExitNotice(false)}>我知道了</button>
      </div>
    </div>
  ) : null
  const deviceDisconnectModal = deviceDisconnectNotice ? (
    <div className="monitoring-modal-layer" role="alertdialog" aria-modal="true" aria-label="设备已经断开">
      <div className="monitoring-modal-backdrop" />
      <div className="monitoring-modal">
        <div className="monitoring-modal-icon"><CircleAlert size={24} /></div>
        <h3>设备已经断开</h3>
        <p>检测到设备已离线，性能采集已自动中断。</p>
        <div className="monitoring-modal-package"><span>设备 ID</span><strong className="mono">{metrics?.device_id || device.device_id}</strong></div>
        <button onClick={() => setDeviceDisconnectNotice(false)}>我知道了</button>
      </div>
    </div>
  ) : null

  if (monitoringViewActive) {
    return (
      <>
        <MonitoringWorkbench
          device={device}
          packageName={selectedPackage}
          metrics={metrics}
          phase={monitoringPhase}
          summary={monitoringSummary}
          error={monitoringError}
          lastMetricsSyncedAt={lastMetricsSyncedAt}
          lastMetricsCostMs={lastMetricsCostMs}
          onStop={handleStopMonitoring}
          onExit={handleExitMonitoring}
          onExportCsv={handleExportCsv}
        />
        {appExitModal}
        {deviceDisconnectModal}
      </>
    )
  }

  return (
    <div className="content device-detail-page">
      <section className="detail-page-header">
        <button className="back-button" onClick={onBack}><ArrowLeft size={16} />返回设备中心</button>
        <div className="detail-page-heading">
          <div>
            <span className="eyebrow">设备详情</span>
            <h2>{getDeviceName(device)}</h2>
            <p>查看设备信息并选择需要测试的应用。</p>
          </div>
        </div>
      </section>

      <div className="device-detail-grid">
        <section className="device-summary-card" aria-label="设备基本信息">
          <div className="detail-device-hero">
          <PlatformIcon platform={device.platform} />
          <div>
            <span className={`status-pill ${meta.className}`}><span className="status-dot" />{meta.label}</span>
            <p>{device.platform.toUpperCase()} · {device.version}</p>
          </div>
          </div>

          <div className="detail-list">
            <div><span>设备型号</span><strong>{device.model || '—'}</strong></div>
            <div><span>设备 ID</span><strong className="mono">{device.device_id}</strong></div>
            <div><span>连接方式</span><strong><Wifi size={15} /> 本地连接</strong></div>
            <div><span>服务状态</span><strong><ShieldCheck size={15} /> 响应正常</strong></div>
          </div>

          <div className={`selected-app-summary ${selectedPackage ? 'has-selection' : ''}`}>
            <span>当前选择</span>
            <strong className="mono">{selectedPackage || '请从右侧选择一个应用'}</strong>
          </div>

          {monitoringError && monitoringPhase === 'idle' && <div className="start-monitoring-error"><CircleAlert size={14} />{monitoringError}</div>}

          <button
            className="start-button detail-page-action"
            disabled={device.status !== 'available' || !selectedPackage || monitoringLocked}
            onClick={handleStartMonitoring}
          >
            {startChecking ? <RefreshCw size={16} className="spin" /> : <Play size={16} fill="currentColor" />}
            {startChecking ? '正在检查应用运行状态' : '开始采集'}
          </button>
        </section>

        <section className="device-apps-card device-apps" aria-label="设备应用">
          <div className="device-apps-header">
            <div>
              <strong>已安装应用</strong>
              <span>{appsLoading ? '正在读取设备…' : appsError ? '读取失败' : appsQuery.trim() ? `${visibleApps.length} / ${apps.length} 个应用` : `${apps.length} 个第三方应用`}</span>
            </div>
            <button className="icon-button" onClick={() => loadApps()} disabled={appsLoading} aria-label="刷新应用列表">
              <RefreshCw size={16} className={appsLoading ? 'spin' : ''} />
            </button>
          </div>

          <label className="app-search-box">
            <Search size={16} />
            <input
              value={appsQuery}
              onChange={(event) => setAppsQuery(event.target.value)}
              placeholder="搜索应用或包名"
              disabled={appsLoading || Boolean(appsError)}
            />
          </label>

          {appsLoading ? (
            <div className="app-list-loading" aria-label="正在加载应用列表">
              {[0, 1, 2].map((item) => <div className="app-row" key={item}><span className="skeleton app-icon-skeleton" /><span className="skeleton app-copy-skeleton" /></div>)}
            </div>
          ) : appsError ? (
            <div className="apps-message error">
              <CircleAlert size={18} />
              <div><strong>应用列表加载失败</strong><span>{appsError}</span></div>
              <button onClick={() => loadApps()}>重试</button>
            </div>
          ) : apps.length === 0 ? (
            <div className="apps-message"><AppWindow size={20} /><span>设备中没有发现第三方应用</span></div>
          ) : visibleApps.length === 0 ? (
            <div className="apps-message"><Search size={20} /><span>没有找到匹配的应用或包名</span></div>
          ) : (
            <div className="app-list" role="listbox" aria-label="选择监控应用">
              {visibleApps.map((app) => (
                <button
                  type="button"
                  className={`app-row ${selectedPackage === app.package_name ? 'selected' : ''}`}
                  key={app.package_name}
                  onClick={() => setSelectedPackage(app.package_name)}
                  disabled={monitoringLocked}
                  role="option"
                  aria-selected={selectedPackage === app.package_name}
                >
                  <span className="app-icon"><AppWindow size={17} /></span>
                  <div><strong>{app.app_name || app.package_name}</strong><span className="mono">{app.package_name}</span></div>
                  {selectedPackage === app.package_name && <span className="app-selected-check"><Check size={13} /></span>}
                </button>
              ))}
            </div>
          )}
        </section>
      </div>

      {appExitModal}
    </div>
  )
}

function formatHistoryTime(value?: string) {
  if (!value) return '—'
  const date = new Date(value.replace(' ', 'T'))
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString('zh-CN', { hour12: false })
}

function historyStatusLabel(status?: string) {
  const normalized = (status || '').toLowerCase()
  if (normalized === 'collecting' || normalized === 'running') return '采集中'
  if (normalized === 'stopped') return '已停止'
  if (normalized === 'interrupted') return '已中断'
  if (normalized === 'error') return '异常'
  return status || '未知'
}

function isExportableHistoryStatus(status?: string) {
  const normalized = (status || '').toLowerCase()
  return normalized === 'stopped' || normalized === 'interrupted' || normalized === 'error'
}

function HistoryPage({
  selectedTaskId,
  onOpenTask,
  onCloseTask,
}: {
  selectedTaskId: string
  onOpenTask: (taskId: string) => void
  onCloseTask: () => void
}) {
  const [items, setItems] = useState<PerfHistoryItem[]>([])
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [error, setError] = useState('')
  const [deletingTaskId, setDeletingTaskId] = useState('')
  const [exportingTaskId, setExportingTaskId] = useState('')
  const [detailMetrics, setDetailMetrics] = useState<MonitoringMetrics | null>(null)
  const [detailLoading, setDetailLoading] = useState(false)
  const [detailError, setDetailError] = useState('')

  const loadHistory = useCallback(async (quiet = false, signal?: AbortSignal) => {
    if (quiet) setRefreshing(true)
    else setLoading(true)
    setError('')

    try {
      setItems(await getPerfHistory(signal))
    } catch (requestError) {
      if (requestError instanceof DOMException && requestError.name === 'AbortError') return
      setError(requestError instanceof Error ? requestError.message : '历史记录加载失败')
    } finally {
      if (!signal?.aborted) {
        setLoading(false)
        setRefreshing(false)
      }
    }
  }, [])

  useEffect(() => {
    const controller = new AbortController()
    loadHistory(false, controller.signal)
    return () => controller.abort()
  }, [loadHistory])

  useEffect(() => {
    if (!selectedTaskId) {
      setDetailMetrics(null)
      setDetailError('')
      return
    }

    const controller = new AbortController()
    setDetailLoading(true)
    setDetailError('')
    setDetailMetrics(null)

    getPerfHistoryTask(selectedTaskId, controller.signal)
      .then(setDetailMetrics)
      .catch((requestError) => {
        if (requestError instanceof DOMException && requestError.name === 'AbortError') return
        setDetailError(requestError instanceof Error ? requestError.message : '历史详情加载失败')
      })
      .finally(() => {
        if (!controller.signal.aborted) setDetailLoading(false)
      })

    return () => controller.abort()
  }, [selectedTaskId])

  const handleDelete = useCallback(async (taskId: string) => {
    if (!window.confirm(`确定删除历史记录 ${taskId} 吗？`)) return

    setDeletingTaskId(taskId)
    try {
      await deletePerfHistory(taskId)
      setItems((current) => current.filter((item) => item.task_id !== taskId))
      if (selectedTaskId === taskId) onCloseTask()
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : '删除历史记录失败')
    } finally {
      setDeletingTaskId('')
    }
  }, [onCloseTask, selectedTaskId])

  const handleExportCsv = useCallback(async (taskId: string) => {
    setExportingTaskId(taskId)
    setError('')
    setDetailError('')

    try {
      await downloadPerfHistoryCsv(taskId)
    } catch (requestError) {
      const message = requestError instanceof Error ? requestError.message : '导出 CSV 失败'
      if (selectedTaskId === taskId) setDetailError(message)
      else setError(message)
    } finally {
      setExportingTaskId('')
    }
  }, [selectedTaskId])

  if (selectedTaskId) {
    if (detailMetrics) {
      const historyDevice: Device = {
        device_id: detailMetrics.device_id,
        device_name: detailMetrics.device_id,
        platform: 'android',
        version: '历史记录',
        model: detailMetrics.device_id,
        status: 'offline',
        error_message: '',
      }

      return (
        <MonitoringWorkbench
          device={historyDevice}
          packageName={detailMetrics.package_name}
          metrics={detailMetrics}
          phase={detailMetrics.status === 'running' ? 'running' : detailMetrics.status === 'interrupted' ? 'interrupted' : detailMetrics.status === 'error' ? 'error' : 'stopped'}
          summary={createMonitoringSummary(detailMetrics)}
          error={detailError || detailMetrics.last_error || ''}
          lastMetricsSyncedAt={null}
          lastMetricsCostMs={null}
          exitLabel="返回历史列表"
          showWindowControls={false}
          onStop={() => undefined}
          onExit={onCloseTask}
          onExportCsv={isExportableHistoryStatus(detailMetrics.status) ? () => handleExportCsv(detailMetrics.task_id) : undefined}
        />
      )
    }

    return (
      <div className="content history-detail-loading">
        <button className="back-button" onClick={onCloseTask}><ArrowLeft size={16} />返回历史列表</button>
        <div className={`empty-state ${detailError ? 'history-error-state' : ''}`}>
          <div>{detailLoading ? <RefreshCw size={25} className="spin" /> : <CircleAlert size={25} />}</div>
          <h3>{detailLoading ? '正在加载历史详情' : '历史详情加载失败'}</h3>
          <p>{detailError || selectedTaskId}</p>
          {detailError && <button onClick={() => onOpenTask(selectedTaskId)}>重试</button>}
        </div>
      </div>
    )
  }

  return (
    <div className="content history-page">
      <section className="page-intro">
        <div>
          <h1>历史记录</h1>
          <p>查看已经完成的性能采集任务，回放指标曲线或删除不需要的记录。</p>
        </div>
        <button className="history-refresh-button" onClick={() => loadHistory(true)} disabled={refreshing}>
          <RefreshCw size={16} className={refreshing ? 'spin' : ''} />刷新历史
        </button>
      </section>

      {error && (
        <div className="error-banner"><CircleAlert size={19} /><div><strong>历史记录操作失败</strong><span>{error}</span></div><button onClick={() => loadHistory()}>重新加载</button></div>
      )}

      <section className="history-panel" aria-label="性能采集历史">
        {loading ? (
          <div className="history-list">
            {[0, 1, 2].map((item) => <div className="history-row skeleton-history-row" key={item}><span className="skeleton title" /><span className="skeleton line" /></div>)}
          </div>
        ) : items.length ? (
          <div className="history-list">
            {items.map((item) => {
              const sampleCount = item.total_samples ?? item.sample_count ?? 0
              const statusLabel = historyStatusLabel(item.status)
              const canExport = isExportableHistoryStatus(item.status)
              return (
                <article className="history-row" key={item.task_id} onClick={() => onOpenTask(item.task_id)} tabIndex={0} onKeyDown={(event) => event.key === 'Enter' && onOpenTask(item.task_id)}>
                  <div className="history-row-icon"><Clock3 size={18} /></div>
                  <div className="history-row-main">
                    <div><strong className="mono">{item.task_id}</strong><span className={`history-status ${statusLabel === '采集中' ? 'running' : statusLabel === '异常' || statusLabel === '已中断' ? 'error' : ''}`}>{statusLabel}</span></div>
                    <p><span className="mono">{item.package_name || '—'}</span><span>{item.device_id || '—'}</span></p>
                  </div>
                  <div className="history-row-meta">
                    <span>{formatHistoryTime(item.start_time)}</span>
                    <strong>{sampleCount} 个采样</strong>
                  </div>
                  <div className="history-row-actions">
                    {canExport && (
                      <button
                        className="history-export-button"
                        onClick={(event) => { event.stopPropagation(); handleExportCsv(item.task_id) }}
                        disabled={exportingTaskId === item.task_id}
                        aria-label={`导出历史记录 ${item.task_id} CSV`}
                      >
                        {exportingTaskId === item.task_id ? <RefreshCw size={16} className="spin" /> : <Download size={16} />}
                      </button>
                    )}
                    <button
                      className="history-delete-button"
                      onClick={(event) => { event.stopPropagation(); handleDelete(item.task_id) }}
                      disabled={deletingTaskId === item.task_id}
                      aria-label={`删除历史记录 ${item.task_id}`}
                    >
                      {deletingTaskId === item.task_id ? <RefreshCw size={16} className="spin" /> : <Trash2 size={16} />}
                    </button>
                  </div>
                </article>
              )
            })}
          </div>
        ) : (
          <div className="empty-state">
            <div><History size={25} /></div>
            <h3>还没有历史记录</h3>
            <p>停止一次性能采集后，历史任务会显示在这里。</p>
            <button onClick={() => loadHistory()}>重新加载</button>
          </div>
        )}
      </section>
    </div>
  )
}

function App() {
  const [devices, setDevices] = useState<Device[]>([])
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [error, setError] = useState('')
  const [query, setQuery] = useState('')
  const [filter, setFilter] = useState<StatusFilter>('all')
  const [autoRefresh, setAutoRefresh] = useState(true)
  const [lastUpdated, setLastUpdated] = useState<Date | null>(null)
  const [selectedDevice, setSelectedDevice] = useState<Device | null>(null)
  const [currentPage, setCurrentPage] = useState<'devices' | 'reports'>('devices')
  const [selectedHistoryTaskId, setSelectedHistoryTaskId] = useState('')
  const [sidebarOpen, setSidebarOpen] = useState(false)

  const loadDevices = useCallback(async (quiet = false, signal?: AbortSignal) => {
    if (quiet) setRefreshing(true)
    else setLoading(true)
    setError('')

    try {
      const nextDevices = await getDevices(signal)
      setDevices(nextDevices)
      setLastUpdated(new Date())
    } catch (requestError) {
      if (requestError instanceof DOMException && requestError.name === 'AbortError') return
      setError(requestError instanceof Error ? requestError.message : '加载设备失败，请稍后重试')
    } finally {
      setLoading(false)
      setRefreshing(false)
    }
  }, [])

  useEffect(() => {
    const controller = new AbortController()
    loadDevices(false, controller.signal)
    return () => controller.abort()
  }, [loadDevices])

  useEffect(() => {
    if (!autoRefresh) return
    const interval = window.setInterval(() => loadDevices(true), 15_000)
    return () => window.clearInterval(interval)
  }, [autoRefresh, loadDevices])

  const counts = useMemo(() => ({
    all: devices.length,
    available: devices.filter((device) => device.status === 'available').length,
    busy: devices.filter((device) => device.status === 'busy').length,
  }), [devices])

  const visibleDevices = useMemo(() => {
    const normalizedQuery = query.trim().toLowerCase()
    return devices.filter((device) => {
      const matchesFilter = filter === 'all' || device.status === filter
      const matchesQuery = !normalizedQuery || [getDeviceName(device), device.model, device.device_id, device.version]
        .some((value) => value?.toLowerCase().includes(normalizedQuery))
      return matchesFilter && matchesQuery
    })
  }, [devices, filter, query])

  const selectDevice = useCallback((device: Device) => {
    setCurrentPage('devices')
    setSelectedHistoryTaskId('')
    setSelectedDevice(device)
    window.history.pushState({ perfRabbitDevice: device.device_id }, '', `#device/${encodeURIComponent(device.device_id)}`)
    window.scrollTo({ top: 0, behavior: 'smooth' })
  }, [])

  const returnToDevices = useCallback(() => {
    if (window.history.state?.perfRabbitDevice) {
      window.history.back()
      return
    }
    setSelectedDevice(null)
    setCurrentPage('devices')
    setSelectedHistoryTaskId('')
    window.history.replaceState({}, '', '#devices')
    window.scrollTo({ top: 0, behavior: 'smooth' })
  }, [])

  const openReportTask = useCallback((taskId: string) => {
    setCurrentPage('reports')
    setSelectedDevice(null)
    setSelectedHistoryTaskId(taskId)
    window.history.pushState({ perfRabbitReport: taskId }, '', `#reports/${encodeURIComponent(taskId)}`)
    window.scrollTo({ top: 0, behavior: 'smooth' })
  }, [])

  const returnToReports = useCallback(() => {
    setCurrentPage('reports')
    setSelectedDevice(null)
    setSelectedHistoryTaskId('')
    window.history.replaceState({}, '', '#reports')
    window.scrollTo({ top: 0, behavior: 'smooth' })
  }, [])

  useEffect(() => {
    const syncPageFromUrl = () => {
      if (window.location.hash === '#reports' || window.location.hash === '#history') {
        setCurrentPage('reports')
        setSelectedDevice(null)
        setSelectedHistoryTaskId('')
        return
      }

      const encodedReportTaskId = window.location.hash.startsWith('#reports/')
        ? window.location.hash.slice('#reports/'.length)
        : window.location.hash.startsWith('#history/')
          ? window.location.hash.slice('#history/'.length)
          : ''

      if (encodedReportTaskId) {
        setCurrentPage('reports')
        setSelectedDevice(null)
        setSelectedHistoryTaskId(decodeURIComponent(encodedReportTaskId))
        return
      }

      const encodedDeviceId = window.location.hash.startsWith('#device/')
        ? window.location.hash.slice('#device/'.length)
        : ''

      if (!encodedDeviceId) {
        setCurrentPage('devices')
        setSelectedDevice(null)
        setSelectedHistoryTaskId('')
        return
      }

      const deviceId = decodeURIComponent(encodedDeviceId)
      setCurrentPage('devices')
      setSelectedHistoryTaskId('')
      setSelectedDevice(devices.find((device) => device.device_id === deviceId) ?? null)
    }

    window.addEventListener('popstate', syncPageFromUrl)
    window.addEventListener('hashchange', syncPageFromUrl)
    syncPageFromUrl()
    return () => {
      window.removeEventListener('popstate', syncPageFromUrl)
      window.removeEventListener('hashchange', syncPageFromUrl)
    }
  }, [devices])

  return (
    <div className="app-shell">
      <aside className={`sidebar ${sidebarOpen ? 'open' : ''}`}>
        <div className="brand">
          <div className="brand-mark"><Rabbit size={24} strokeWidth={2.2} /></div>
          <div><strong>PerfRabbit</strong><span>性能工作台</span></div>
        </div>

        <nav>
          <span className="nav-label">工作空间</span>
          <a className={`nav-item ${currentPage === 'devices' ? 'active' : ''}`} href="#devices"><MonitorSmartphone size={19} />设备中心<span>{counts.all}</span></a>
          <a className="nav-item" href="#tasks"><Gauge size={19} />测试任务</a>
          <a className={`nav-item ${currentPage === 'reports' ? 'active' : ''}`} href="#reports"><LayoutDashboard size={19} />性能报告</a>
        </nav>

        <div className="service-card">
          <div className="service-title"><span className="live-dot" />本地服务正常</div>
          <p>{SERVICE_ADDRESS}</p>
          <div><span>API</span><strong><Check size={13} /> 已连接</strong></div>
        </div>
      </aside>

      {sidebarOpen && <button className="mobile-backdrop" onClick={() => setSidebarOpen(false)} aria-label="关闭导航" />}

      <main>
        <header className="topbar">
          <button className="icon-button menu-button" onClick={() => setSidebarOpen(true)} aria-label="打开导航"><Menu size={20} /></button>
          <div className="breadcrumbs">
            <span>工作空间</span><ChevronRight size={14} />
            {currentPage === 'reports' ? (
              selectedHistoryTaskId
                ? <><button onClick={returnToReports}>性能报告</button><ChevronRight size={14} /><strong className="mono">{selectedHistoryTaskId}</strong></>
                : <strong>性能报告</strong>
            ) : selectedDevice ? (
              <><button onClick={returnToDevices}>设备中心</button><ChevronRight size={14} /><strong>{getDeviceName(selectedDevice)}</strong></>
            ) : <strong>设备中心</strong>}
          </div>
          <div className="topbar-actions">
            <span className="sync-copy">{lastUpdated ? `更新于 ${lastUpdated.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })}` : '正在同步'}</span>
            <button className="icon-button" onClick={() => loadDevices(true)} aria-label="刷新设备" disabled={refreshing}>
              <RefreshCw size={18} className={refreshing ? 'spin' : ''} />
            </button>
            <div className="avatar">PR</div>
          </div>
        </header>

        {currentPage === 'reports' ? (
          <HistoryPage selectedTaskId={selectedHistoryTaskId} onOpenTask={openReportTask} onCloseTask={returnToReports} />
        ) : selectedDevice ? <DeviceDetailPage device={selectedDevice} onBack={returnToDevices} /> : <div className="content">
          <section className="page-intro">
            <div>
              <h1>设备中心</h1>
              <p>连接设备，开启一次轻快而清晰的性能测试。</p>
            </div>
          </section>

          <section className="stats-grid" aria-label="设备概览">
            <div className="stat-card"><div className="stat-icon violet"><MonitorSmartphone size={20} /></div><div><span>设备总数</span><strong>{counts.all}</strong><small>已发现设备</small></div></div>
            <div className="stat-card"><div className="stat-icon green"><Zap size={20} /></div><div><span>当前可用</span><strong>{counts.available}</strong><small>可立即开始测试</small></div></div>
            <div className="stat-card"><div className="stat-icon amber"><Cpu size={20} /></div><div><span>测试中</span><strong>{counts.busy}</strong><small>正在采集数据</small></div></div>
          </section>

          <section className="device-panel" id="devices">
            <div className="panel-toolbar">
              <div className="filter-tabs" role="tablist" aria-label="设备状态筛选">
                {([
                  ['all', '全部', counts.all],
                  ['available', '可用', counts.available],
                  ['busy', '测试中', counts.busy],
                ] as const).map(([value, label, count]) => (
                  <button key={value} className={filter === value ? 'active' : ''} onClick={() => setFilter(value)}>
                    {label}<span>{count}</span>
                  </button>
                ))}
              </div>
              <div className="toolbar-right">
                <label className="search-box"><Search size={17} /><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索设备、型号或 ID" /></label>
                <label className="auto-refresh"><input type="checkbox" checked={autoRefresh} onChange={(event) => setAutoRefresh(event.target.checked)} /><span />自动刷新</label>
              </div>
            </div>

            {error && (
              <div className="error-banner"><CircleAlert size={19} /><div><strong>设备列表加载失败</strong><span>{error}</span></div><button onClick={() => loadDevices()}>重新加载</button></div>
            )}

            {loading ? (
              <div className="device-grid" aria-label="正在加载设备">
                {[0, 1, 2].map((item) => <div className="device-card skeleton-card" key={item}><div className="skeleton icon" /><div className="skeleton title" /><div className="skeleton line" /><div className="skeleton button" /></div>)}
              </div>
            ) : visibleDevices.length > 0 ? (
              <div className="device-grid">
                {visibleDevices.map((device) => <DeviceCard key={device.device_id} device={device} onOpen={() => selectDevice(device)} />)}
              </div>
            ) : !error && (
              <div className="empty-state">
                <div><Search size={25} /></div>
                <h3>{devices.length ? '没有找到匹配的设备' : '还没有发现设备'}</h3>
                <p>{devices.length ? '换一个关键词或筛选条件试试。' : '连接 Android 或 iOS 设备后，它会出现在这里。'}</p>
                {devices.length ? <button onClick={() => { setQuery(''); setFilter('all') }}>清除筛选</button> : <button onClick={() => loadDevices()}>重新扫描</button>}
              </div>
            )}
          </section>

          <footer><Rabbit size={15} /> PerfRabbit 本地性能工作台 <span>·</span> 数据仅在本机流转</footer>
        </div>}
      </main>
    </div>
  )
}

export default App
