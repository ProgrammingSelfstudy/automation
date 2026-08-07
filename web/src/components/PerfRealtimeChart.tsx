import { useState, type MouseEvent as ReactMouseEvent } from 'react'

import type { PerfMonitoringSample } from '../api/perfAgent'

export type PerfChartSeriesConfig = {
  metric: keyof PerfMonitoringSample
  label: string
  color: string
}

const CHART_LEFT = 43
const CHART_RIGHT = 592
const CHART_TOP = 14
const CHART_BOTTOM = 160
const CHART_WIDTH = CHART_RIGHT - CHART_LEFT
const CHART_HEIGHT = CHART_BOTTOM - CHART_TOP
const VIEWBOX_WIDTH = 610
const VIEWBOX_HEIGHT = 190
const MAX_SAMPLES = 60

function formatAxisValue(value: number) {
  return value.toFixed(0)
}

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

// PerfRealtimeChart：性能采集实时曲线图。手写 SVG 而不是引入图表库——
// 只需要多条折线 + hover 十字线/tooltip，数据量小（60 个采样点封顶），
// 用一个组件换一整个图表库的依赖不划算。悬浮交互（crosshair + tooltip）
// 是折线图的标配，不是可选项，见 dataviz 技能的 interaction 规范。
export default function PerfRealtimeChart({
  title,
  samples,
  series,
  unit,
  fixedRange,
}: {
  title: string
  samples: PerfMonitoringSample[]
  series: PerfChartSeriesConfig[]
  unit: string
  fixedRange?: [number, number]
}) {
  const [hoverIndex, setHoverIndex] = useState<number | null>(null)
  const recentSamples = samples.slice(-MAX_SAMPLES)

  const allValues = recentSamples.flatMap((sample) => series.map(({ metric }) => Number(sample[metric]) || 0))
  const dataMin = allValues.length ? Math.min(...allValues) : 0
  const dataMax = allValues.length ? Math.max(...allValues) : 1
  const rangeMin = fixedRange?.[0] ?? (dataMin > 0 ? dataMin * 0.94 : 0)
  const rawRangeMax = fixedRange?.[1] ?? Math.max(dataMax * 1.06, rangeMin + 1)
  const rangeMax = rawRangeMax > rangeMin ? rawRangeMax : rangeMin + 1

  const valueToY = (value: number) => CHART_BOTTOM - ((value - rangeMin) / (rangeMax - rangeMin)) * CHART_HEIGHT
  const sampleToX = (index: number) =>
    recentSamples.length <= 1 ? CHART_RIGHT : CHART_LEFT + (index / (recentSamples.length - 1)) * CHART_WIDTH

  const chartSeries = series.map((item) => {
    const values = recentSamples.map((sample) => Number(sample[item.metric]) || 0)
    const points =
      values.length === 1
        ? `${CHART_LEFT},${valueToY(values[0])} ${CHART_RIGHT},${valueToY(values[0])}`
        : values.map((value, index) => `${sampleToX(index)},${valueToY(value)}`).join(' ')
    return { ...item, values, points }
  })

  const activeHoverIndex = hoverIndex == null || !recentSamples[hoverIndex] ? null : hoverIndex
  const hoverSample = activeHoverIndex == null ? null : recentSamples[activeHoverIndex]
  const hoverX = activeHoverIndex == null ? null : sampleToX(activeHoverIndex)
  const hoverRows = hoverSample
    ? chartSeries.map((item) => {
        const value = Number(hoverSample[item.metric]) || 0
        return { ...item, value, y: valueToY(value) }
      })
    : []

  const tooltipWidth = 150
  const tooltipHeight = 26 + hoverRows.length * 15
  const hoverMinY = hoverRows.length ? Math.min(...hoverRows.map((row) => row.y)) : CHART_TOP
  const tooltipX =
    hoverX == null
      ? 0
      : Math.max(
          8,
          Math.min(
            VIEWBOX_WIDTH - tooltipWidth - 4,
            hoverX + 12 > VIEWBOX_WIDTH - tooltipWidth ? hoverX - tooltipWidth - 12 : hoverX + 12,
          ),
        )
  const tooltipY = Math.max(8, Math.min(VIEWBOX_HEIGHT - tooltipHeight - 8, hoverMinY - tooltipHeight / 2))

  function handleChartMouseMove(event: ReactMouseEvent<SVGSVGElement>) {
    if (!recentSamples.length) return
    const rect = event.currentTarget.getBoundingClientRect()
    if (!rect.width) return

    const viewBoxX = ((event.clientX - rect.left) / rect.width) * VIEWBOX_WIDTH
    const ratio = (viewBoxX - CHART_LEFT) / CHART_WIDTH
    const nextIndex = Math.max(0, Math.min(recentSamples.length - 1, Math.round(ratio * (recentSamples.length - 1))))
    setHoverIndex((current) => (current === nextIndex ? current : nextIndex))
  }

  const firstTime = recentSamples[0]?.collected_at
  const lastTime = recentSamples.at(-1)?.collected_at

  return (
    <div className="rounded-lg border border-white/60 bg-white/70 p-4 shadow-sm backdrop-blur-xl" aria-label={title}>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <span className="text-xs font-semibold uppercase text-slate-500">{title}</span>
        {series.length > 1 ? (
          <div className="flex flex-wrap items-center gap-3">
            {series.map((item) => (
              <span className="flex items-center gap-1.5 text-xs text-slate-500" key={item.metric}>
                <i className="inline-block h-0.5 w-2.5 rounded-full" style={{ background: item.color }} />
                {item.label}
              </span>
            ))}
          </div>
        ) : null}
      </div>

      <div className="relative mt-2">
        <svg
          className="h-[140px] w-full overflow-visible"
          viewBox={`0 0 ${VIEWBOX_WIDTH} ${VIEWBOX_HEIGHT}`}
          preserveAspectRatio="none"
          role="img"
          aria-label={`${title}，最近 ${recentSamples.length} 个采样点`}
          onMouseMove={handleChartMouseMove}
          onMouseLeave={() => setHoverIndex(null)}
        >
          {[0, 0.25, 0.5, 0.75, 1].map((position) => {
            const y = CHART_TOP + position * CHART_HEIGHT
            const value = rangeMax - position * (rangeMax - rangeMin)
            return (
              <g key={position}>
                <line x1={CHART_LEFT} y1={y} x2={CHART_RIGHT} y2={y} stroke="#e2e8f0" strokeWidth={1} vectorEffect="non-scaling-stroke" />
                <text x={CHART_LEFT - 7} y={y + 3} textAnchor="end" fontSize={9} fill="#94a3b8">
                  {formatAxisValue(value)}
                </text>
              </g>
            )
          })}
          <text x={8} y={12} fontSize={9} fill="#94a3b8">
            {unit}
          </text>

          {chartSeries.map((item) => (
            <polyline
              key={item.metric}
              points={item.points}
              fill="none"
              stroke={item.color}
              strokeWidth={2.4}
              strokeLinecap="round"
              strokeLinejoin="round"
              vectorEffect="non-scaling-stroke"
            />
          ))}

          <rect x={CHART_LEFT} y={CHART_TOP} width={CHART_WIDTH} height={CHART_HEIGHT} fill="transparent" className="cursor-crosshair" />

          {hoverSample && hoverX != null ? (
            <g className="pointer-events-none">
              <line
                x1={hoverX}
                y1={CHART_TOP}
                x2={hoverX}
                y2={CHART_BOTTOM}
                stroke="#64748b"
                strokeWidth={1}
                strokeDasharray="4 4"
                vectorEffect="non-scaling-stroke"
              />
              {hoverRows.map((item) => (
                <circle key={item.metric} cx={hoverX} cy={item.y} r={3.2} fill={item.color} stroke="#fff" strokeWidth={1.4} vectorEffect="non-scaling-stroke" />
              ))}
              <g transform={`translate(${tooltipX}, ${tooltipY})`}>
                <rect width={tooltipWidth} height={tooltipHeight} rx={8} fill="rgba(255,255,255,0.97)" stroke="#e2e8f0" strokeWidth={1} />
                <text x={10} y={15} fontSize={9} fontWeight={700} fill="#334155">
                  {formatSampleTime(hoverSample.collected_at)}
                </text>
                {hoverRows.map((item, index) => (
                  <g key={item.metric} transform={`translate(0, ${26 + index * 15})`}>
                    <circle cx={16} cy={-3} r={3} fill={item.color} />
                    <text x={22} y={0} fontSize={8.5} fill="#64748b">
                      {item.label}
                    </text>
                    <text x={tooltipWidth - 10} y={0} textAnchor="end" fontSize={8.5} fontWeight={700} fill="#0f172a">
                      {formatSeriesValue(item.value, unit)}
                    </text>
                  </g>
                ))}
              </g>
            </g>
          ) : null}

          {firstTime ? (
            <text x={CHART_LEFT} y={182} fontSize={8.5} fill="#94a3b8">
              {formatSampleTime(firstTime)}
            </text>
          ) : null}
          {lastTime ? (
            <text x={CHART_RIGHT} y={182} textAnchor="end" fontSize={8.5} fill="#94a3b8">
              {formatSampleTime(lastTime)}
            </text>
          ) : null}
        </svg>

        {!recentSamples.length ? (
          <div className="absolute inset-0 flex items-center justify-center text-sm text-slate-400">等待性能采样数据</div>
        ) : null}
      </div>
    </div>
  )
}
