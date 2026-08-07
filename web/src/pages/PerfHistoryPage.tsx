import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Eye, RefreshCw, Trash2 } from 'lucide-react'
import { useState } from 'react'

import { deletePerfTask, getPerfTask, listPerfTasks, type PerfTaskResponse } from '../api/client'
import type { PerfMonitoringSample } from '../api/perfAgent'
import PendingPerfUploadsBanner from '../components/PendingPerfUploadsBanner'
import PerfMetricCharts from '../components/PerfMetricCharts'
import SlideOver from '../components/SlideOver'
import usePerfUploadRetryQueue from '../hooks/usePerfUploadRetryQueue'
import { formatDateTime, getErrorMessage } from '../utils/format'

// 历史数据存在中心平台 MySQL 里，跟本地有没有装/启动 Agent 完全无关——
// 这个页面不依赖 probePerfAgent，不该被"未检测到本地采集工具"挡住
// （之前跟"性能采集"挤在同一个页面时出过这个问题）。
export default function PerfHistoryPage() {
  const queryClient = useQueryClient()
  const [detailID, setDetailID] = useState<number | null>(null)

  const { pendingUploads, retryingUploads, retryPendingUploads } = usePerfUploadRetryQueue()

  const historyQuery = useQuery({
    queryKey: ['perf-tasks'],
    queryFn: listPerfTasks,
  })

  const detailQuery = useQuery({
    queryKey: ['perf-tasks', detailID],
    queryFn: () => getPerfTask(detailID as number),
    enabled: detailID !== null,
  })

  const deleteMutation = useMutation({
    mutationFn: deletePerfTask,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['perf-tasks'] })
      setDetailID(null)
    },
  })

  return (
    <div className="page-shell">
      <PendingPerfUploadsBanner pendingUploads={pendingUploads} retrying={retryingUploads} onRetry={retryPendingUploads} />

      <section className="panel">
        <div className="toolbar">
          <div>
            <h2 className="text-base font-semibold text-slate-950">历史数据</h2>
            <div className="mt-1 text-sm text-slate-500">采集结束后自动上报到平台，所有登录用户可见，不需要本地 Agent</div>
          </div>
          <button className="btn btn-secondary" type="button" onClick={() => historyQuery.refetch()}>
            <RefreshCw size={16} className={historyQuery.isFetching ? 'animate-spin' : ''} />
            刷新
          </button>
        </div>

        {historyQuery.isError ? (
          <div className="panel-body text-sm text-red-700">{getErrorMessage(historyQuery.error)}</div>
        ) : (
          <div className="table-wrap">
            <table className="data-table">
              <thead>
                <tr>
                  <th>App</th>
                  <th>设备</th>
                  <th>平台</th>
                  <th>状态</th>
                  <th>样本数</th>
                  <th>开始时间</th>
                  <th>操作</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-white/50 bg-white/35">
                {historyQuery.isFetching ? (
                  <tr>
                    <td className="text-slate-500" colSpan={7}>
                      加载中
                    </td>
                  </tr>
                ) : (historyQuery.data ?? []).length === 0 ? (
                  <tr>
                    <td className="text-slate-500" colSpan={7}>
                      暂无采集记录
                    </td>
                  </tr>
                ) : (
                  (historyQuery.data ?? []).map((task) => (
                    <tr key={task.id}>
                      <td className="font-medium text-slate-950">{task.package_name}</td>
                      <td>{task.device_model || task.device_id}</td>
                      <td>{task.platform}</td>
                      <td>{task.status}</td>
                      <td>{task.sample_count}</td>
                      <td>{formatDateTime(task.start_time)}</td>
                      <td>
                        <div className="flex items-center gap-2">
                          <button className="icon-btn" type="button" onClick={() => setDetailID(task.id)}>
                            <Eye size={14} />
                          </button>
                          <button
                            className="icon-btn"
                            disabled={deleteMutation.isPending}
                            type="button"
                            onClick={() => deleteMutation.mutate(task.id)}
                          >
                            <Trash2 size={14} />
                          </button>
                        </div>
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        )}
      </section>

      <SlideOver open={detailID !== null} title="采集记录详情" onClose={() => setDetailID(null)}>
        <PerfHistoryDetail detail={detailQuery.data} loading={detailQuery.isLoading} />
      </SlideOver>
    </div>
  )
}

// toChartSamples：detail.samples 落库前经过 createPerfTask 请求体，类型
// 是 unknown（perfstore 原样存 JSON，不强绑定具体样本结构，见
// internal/perfstore/store.go 的注释）。这里防御性地转成 PerfRealtimeChart
// 需要的形状，字段缺失/类型不对就按 0 处理，不让个别脏数据整个图表崩掉。
function toChartSamples(raw: unknown): PerfMonitoringSample[] {
  if (!Array.isArray(raw)) return []
  return raw.map((item) => {
    const record = (item ?? {}) as Record<string, unknown>
    const num = (value: unknown) => (typeof value === 'number' && Number.isFinite(value) ? value : Number(value) || 0)
    return {
      collected_at: typeof record.collected_at === 'string' ? record.collected_at : '',
      total_cpu: num(record.total_cpu),
      app_cpu: num(record.app_cpu),
      memory_pss: num(record.memory_pss),
      java_heap: num(record.java_heap),
      native_heap: num(record.native_heap),
      fps: num(record.fps),
      jank: num(record.jank),
      big_jank: num(record.big_jank),
    }
  })
}

function PerfHistoryDetail({ detail, loading }: { detail?: PerfTaskResponse; loading: boolean }) {
  if (loading) {
    return <div className="text-sm text-slate-500">加载中</div>
  }
  if (!detail) {
    return <div className="text-sm text-slate-500">未找到记录</div>
  }
  const rawSamples = Array.isArray(detail.samples) ? detail.samples : []
  const chartSamples = toChartSamples(detail.samples)
  return (
    <div className="space-y-4">
      <div className="grid grid-cols-2 gap-3 text-sm">
        <div>
          <div className="field-label">App</div>
          <div className="mt-1 text-slate-950">{detail.package_name}</div>
        </div>
        <div>
          <div className="field-label">设备</div>
          <div className="mt-1 text-slate-950">{detail.device_model || detail.device_id}</div>
        </div>
        <div>
          <div className="field-label">状态</div>
          <div className="mt-1 text-slate-950">{detail.status}</div>
        </div>
        <div>
          <div className="field-label">样本数</div>
          <div className="mt-1 text-slate-950">{detail.sample_count}</div>
        </div>
        <div>
          <div className="field-label">开始时间</div>
          <div className="mt-1 text-slate-950">{formatDateTime(detail.start_time)}</div>
        </div>
        <div>
          <div className="field-label">结束时间</div>
          <div className="mt-1 text-slate-950">{formatDateTime(detail.stop_time)}</div>
        </div>
      </div>
      {detail.last_error ? (
        <div className="rounded-md bg-red-50 px-4 py-3 text-sm text-red-700">{detail.last_error}</div>
      ) : null}

      {chartSamples.length > 0 ? (
        <div>
          <div className="field-label mb-2">曲线回放（完整这次采集，不做滚动窗口裁剪）</div>
          <PerfMetricCharts samples={chartSamples} maxSamples={0} />
        </div>
      ) : null}

      <div>
        <div className="field-label mb-2">原始样本（JSON）</div>
        <pre className="max-h-96 overflow-auto rounded-md bg-slate-950/90 p-4 text-xs text-slate-100">
          {JSON.stringify(rawSamples, null, 2)}
        </pre>
      </div>
    </div>
  )
}
