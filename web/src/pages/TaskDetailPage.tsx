import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Ban, Download, RefreshCw, Wifi, WifiOff } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { Link, useParams } from 'react-router-dom'

import { cancelTask, exportTaskURL, getTask, getTaskResults } from '../api/client'
import type { AccountResultsResponse } from '../api/client'
import StatusBadge from '../components/StatusBadge'
import useTaskProgress from '../hooks/useTaskProgress'
import { formatDateTime, formatNumber, getErrorMessage } from '../utils/format'

const EMPTY_GROUPS: AccountResultsResponse[] = []

export default function TaskDetailPage() {
  const { id } = useParams<{ id: string }>()
  const queryClient = useQueryClient()
  const [selectedAccountID, setSelectedAccountID] = useState('')

  const taskQuery = useQuery({
    queryKey: ['task', id],
    queryFn: () => getTask(id ?? ''),
    enabled: Boolean(id),
    refetchInterval: (query) => (query.state.data?.status === 'running' ? 2_000 : false),
  })
  const task = taskQuery.data

  const resultsQuery = useQuery({
    queryKey: ['task-results', id],
    queryFn: () => getTaskResults(id ?? ''),
    enabled: Boolean(id),
    refetchInterval: task?.status === 'running' ? 3_000 : false,
  })
  const groups = resultsQuery.data ?? EMPTY_GROUPS

  useEffect(() => {
    if (groups.length === 0) {
      setSelectedAccountID('')
      return
    }
    if (!groups.some((group) => group.account_id === selectedAccountID)) {
      setSelectedAccountID(groups[0].account_id)
    }
  }, [groups, selectedAccountID])

  const selectedGroup = useMemo(
    () => groups.find((group) => group.account_id === selectedAccountID),
    [groups, selectedAccountID],
  )
  const progress = useTaskProgress(id)

  const cancelMutation = useMutation({
    mutationFn: () => cancelTask(id ?? ''),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['task', id] }),
  })

  if (taskQuery.isError) {
    return (
      <div className="page-shell">
        <div className="panel panel-body text-sm text-red-700">{getErrorMessage(taskQuery.error)}</div>
      </div>
    )
  }

  const completed = (task?.success_count ?? 0) + (task?.fail_count ?? 0)
  const percent = task && task.total_count > 0 ? Math.min(100, Math.round((completed / task.total_count) * 100)) : 0

  return (
    <div className="page-shell">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h2 className="text-lg font-semibold text-slate-950">{task?.name || id}</h2>
          <div className="mt-1 text-sm text-slate-500">{id}</div>
        </div>
        <div className="flex flex-wrap gap-2">
          <button className="btn btn-secondary" type="button" onClick={() => taskQuery.refetch()}>
            <RefreshCw size={16} />
            刷新
          </button>
          {task ? (
            <a className="btn btn-secondary" download href={exportTaskURL(task.id)}>
              <Download size={16} />
              导出
            </a>
          ) : null}
          {task?.status === 'running' ? (
            <button
              className="btn btn-danger"
              disabled={cancelMutation.isPending}
              type="button"
              onClick={() => cancelMutation.mutate()}
            >
              <Ban size={16} />
              取消任务
            </button>
          ) : null}
        </div>
      </div>

      <section className="panel">
        <div className="toolbar">
          <div className="flex items-center gap-3">
            {task ? <StatusBadge status={task.status} /> : null}
            <div className="text-sm text-slate-500">{task?.module_type || 'load_test'}</div>
          </div>
          <Link className="text-sm font-medium text-blue-700 hover:text-blue-800" to="/">
            任务列表
          </Link>
        </div>
        <div className="grid gap-4 p-4 sm:p-5 lg:grid-cols-5">
          <Metric label="并发数" value={formatNumber(task?.concurrency)} />
          <Metric label="成功数" value={formatNumber(task?.success_count)} />
          <Metric label="失败数" value={formatNumber(task?.fail_count)} />
          <Metric label="总数" value={formatNumber(task?.total_count)} />
          <Metric label="创建时间" value={formatDateTime(task?.created_at)} />
          <div className="lg:col-span-5">
            <div className="mb-2 flex items-center justify-between text-xs font-semibold text-slate-500">
              <span>进度</span>
              <span>{percent}%</span>
            </div>
            <div className="h-2 overflow-hidden rounded bg-slate-100">
              <div className="h-full bg-blue-600 transition-all" style={{ width: `${percent}%` }} />
            </div>
          </div>
          {task?.err_msg ? <div className="text-sm text-red-700 lg:col-span-5">{task.err_msg}</div> : null}
          {cancelMutation.isError ? (
            <div className="text-sm text-red-700 lg:col-span-5">{getErrorMessage(cancelMutation.error)}</div>
          ) : null}
        </div>
      </section>

      <section className="panel">
        <div className="toolbar">
          <div>
            <h3 className="text-base font-semibold text-slate-950">实时事件流</h3>
            <div className="mt-1 flex items-center gap-2 text-sm text-slate-500">
              {progress.state === 'open' ? <Wifi size={15} /> : <WifiOff size={15} />}
              {progress.state}
            </div>
          </div>
          <div className="text-sm text-slate-500">{formatNumber(progress.events.length)} / 500</div>
        </div>
        {progress.state === 'closed' ? (
          <div className="border-b border-slate-200 bg-amber-50 px-4 py-3 text-sm text-amber-800">
            连接已断开，刷新页面重连
          </div>
        ) : null}
        <div className="max-h-96 overflow-y-auto">
          {progress.events.length === 0 ? (
            <div className="panel-body text-sm text-slate-500">暂无事件</div>
          ) : (
            <div className="divide-y divide-slate-100">
              {progress.events.map((event, index) => (
                <div
                  className="grid gap-2 px-4 py-3 text-sm sm:grid-cols-[160px_120px_80px_minmax(0,1fr)_80px_90px]"
                  key={`${event.timestamp}-${event.account_id}-${event.seq_no}-${index}`}
                >
                  <div className="text-slate-500">{formatDateTime(event.timestamp)}</div>
                  <div className="font-medium text-slate-900">{event.account_id}</div>
                  <div>#{event.seq_no}</div>
                  <div className="min-w-0 truncate">{event.step_name}</div>
                  <div className={event.success ? 'text-emerald-700' : 'text-red-700'}>
                    {event.success ? 'success' : 'failed'}
                  </div>
                  <div>{formatNumber(event.cost_ms)} ms</div>
                  {event.err_msg ? <div className="text-red-700 sm:col-span-6">{event.err_msg}</div> : null}
                </div>
              ))}
            </div>
          )}
        </div>
      </section>

      <section className="panel">
        <div className="toolbar">
          <div>
            <h3 className="text-base font-semibold text-slate-950">结果查看</h3>
            <div className="mt-1 text-sm text-slate-500">{formatNumber(groups.length)} 个账号</div>
          </div>
          <select
            className="input sm:w-72"
            disabled={groups.length === 0}
            value={selectedAccountID}
            onChange={(event) => setSelectedAccountID(event.target.value)}
          >
            {groups.map((group) => (
              <option key={group.account_id} value={group.account_id}>
                {group.account_name || group.account_id}
              </option>
            ))}
          </select>
        </div>
        {resultsQuery.isError ? (
          <div className="panel-body text-sm text-red-700">{getErrorMessage(resultsQuery.error)}</div>
        ) : (
          <div className="table-wrap">
            <table className="data-table">
              <thead>
                <tr>
                  <th>序号</th>
                  <th>耗时</th>
                  <th>公式结果</th>
                  <th>是否成功</th>
                  <th>错误信息</th>
                  <th>时间</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 bg-white">
                {resultsQuery.isLoading ? (
                  <tr>
                    <td className="text-slate-500" colSpan={6}>
                      加载中
                    </td>
                  </tr>
                ) : !selectedGroup || selectedGroup.rows.length === 0 ? (
                  <tr>
                    <td className="text-slate-500" colSpan={6}>
                      暂无结果
                    </td>
                  </tr>
                ) : (
                  selectedGroup.rows.map((row) => (
                    <tr key={row.id}>
                      <td>#{row.seq_no}</td>
                      <td>{formatNumber(row.cost_ms)} ms</td>
                      <td>{row.formula_result}</td>
                      <td>
                        <span className={row.success ? 'text-emerald-700' : 'text-red-700'}>
                          {row.success ? 'true' : 'false'}
                        </span>
                      </td>
                      <td className="max-w-md truncate">{row.err_msg || '-'}</td>
                      <td>{formatDateTime(row.created_at)}</td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        )}
      </section>
    </div>
  )
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-md border border-slate-200 bg-slate-50 px-3 py-2">
      <div className="field-label">{label}</div>
      <div className="mt-1 text-sm font-semibold text-slate-950">{value}</div>
    </div>
  )
}
