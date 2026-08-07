import * as echarts from 'echarts/core'
import { LineChart } from 'echarts/charts'
import { GridComponent, LegendComponent, TooltipComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { useEffect, useRef } from 'react'

import type { PerfMonitoringSample } from '../api/perfAgent'

echarts.use([LineChart, GridComponent, LegendComponent, TooltipComponent, CanvasRenderer])

export type PerfChartSeriesConfig = {
  metric: keyof PerfMonitoringSample
  label: string
  color: string
}

const MAX_SAMPLES = 60

function formatSampleTime(value: string) {
  const date = new Date(value.includes('T') ? value : value.replace(' ', 'T'))
  return Number.isNaN(date.getTime()) ? value || '—' : date.toLocaleTimeString('zh-CN', { hour12: false })
}

function formatSeriesValue(value: number, unit: string) {
  if (unit === '%') return `${value.toFixed(1)}%`
  if (unit === 'MB') return `${value.toFixed(1)} MB`
  if (unit === 'fps') return `${value.toFixed(1)} FPS`
  if (unit === 'count') return value.toFixed(0)
  return `${value.toFixed(1)} ${unit}`
}

// PerfRealtimeChart：性能采集实时曲线图，用 ECharts 渲染（按需引入
// LineChart/Grid/Legend/Tooltip + Canvas renderer，不引入完整包，控制
// 体积）。crosshair + tooltip 是 ECharts 折线图自带的（trigger: 'axis'），
// 不用手写 hover 逻辑。
export default function PerfRealtimeChart({
  title,
  samples,
  series,
  unit,
  fixedRange,
  maxSamples = MAX_SAMPLES,
}: {
  title: string
  samples: PerfMonitoringSample[]
  series: PerfChartSeriesConfig[]
  unit: string
  fixedRange?: [number, number]
  // maxSamples：只保留最近 N 个采样点。实时采集页面用默认值 60（滚动窗口，
  // 图表不会随采集时间越拉越挤）；历史记录回放整段数据想看全貌，传 0 表示
  // 不做窗口裁剪。
  maxSamples?: number
}) {
  const containerRef = useRef<HTMLDivElement | null>(null)
  const chartRef = useRef<echarts.ECharts | null>(null)

  useEffect(() => {
    if (!containerRef.current) return
    const chart = echarts.init(containerRef.current)
    chartRef.current = chart

    const resize = () => chart.resize()
    window.addEventListener('resize', resize)
    return () => {
      window.removeEventListener('resize', resize)
      chart.dispose()
      chartRef.current = null
    }
  }, [])

  useEffect(() => {
    const chart = chartRef.current
    if (!chart) return

    const recentSamples = maxSamples > 0 ? samples.slice(-maxSamples) : samples
    const categories = recentSamples.map((sample) => formatSampleTime(sample.collected_at))

    chart.setOption(
      {
        animation: false,
        grid: { left: 40, right: 16, top: series.length > 1 ? 28 : 12, bottom: 24 },
        tooltip: {
          trigger: 'axis',
          valueFormatter: (value: number | string) => formatSeriesValue(Number(value) || 0, unit),
          textStyle: { fontSize: 11 },
        },
        legend: series.length > 1 ? { top: 0, right: 0, itemWidth: 12, itemHeight: 2, textStyle: { fontSize: 11, color: '#64748b' } } : undefined,
        xAxis: {
          type: 'category',
          data: categories,
          boundaryGap: false,
          axisLine: { lineStyle: { color: '#e2e8f0' } },
          axisLabel: { color: '#94a3b8', fontSize: 10 },
          axisTick: { show: false },
        },
        yAxis: {
          type: 'value',
          min: fixedRange?.[0],
          max: fixedRange?.[1],
          splitLine: { lineStyle: { color: '#f1f5f9' } },
          axisLabel: { color: '#94a3b8', fontSize: 10 },
          name: unit,
          nameTextStyle: { color: '#94a3b8', fontSize: 10 },
        },
        series: series.map((item) => ({
          name: item.label,
          type: 'line',
          data: recentSamples.map((sample) => Number(sample[item.metric]) || 0),
          showSymbol: false,
          lineStyle: { width: 2, color: item.color },
          itemStyle: { color: item.color },
        })),
      },
      { notMerge: true },
    )
  }, [samples, series, unit, fixedRange, maxSamples])

  const hasSamples = samples.length > 0
  const shownCount = maxSamples > 0 ? Math.min(samples.length, maxSamples) : samples.length

  return (
    <div className="rounded-lg border border-white/60 bg-white/70 p-4 shadow-sm backdrop-blur-xl" aria-label={title}>
      <span className="text-xs font-semibold uppercase text-slate-500">{title}</span>
      <div className="relative mt-2">
        <div className="h-[160px] w-full" ref={containerRef} role="img" aria-label={`${title}，共 ${shownCount} 个采样点`} />
        {!hasSamples ? (
          <div className="absolute inset-0 flex items-center justify-center text-sm text-slate-400">等待性能采样数据</div>
        ) : null}
      </div>
    </div>
  )
}
