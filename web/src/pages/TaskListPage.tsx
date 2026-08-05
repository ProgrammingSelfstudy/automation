import { useQuery } from '@tanstack/react-query'
import { Plus, RefreshCw } from 'lucide-react'
import { useState } from 'react'
import { useNavigate } from 'react-router-dom'

import { listTasks } from '../api/client'
import type { TaskStatus } from '../api/client'
import StatusBadge from '../components/StatusBadge'
import { formatDateTime, formatModuleType, formatNumber, getErrorMessage } from '../utils/format'

const STATUS_OPTIONS: Array<{ value: '' | TaskStatus; label: string }> = [
  { value: '', label: '全部状态' },
  { value: 'pending', label: '等待中' },
  { value: 'running', label: '运行中' },
  { value: 'success', label: '成功' },
  { value: 'failed', label: '失败' },
  { value: 'canceled', label: '已取消' },
]

export default function TaskListPage() {
  const navigate = useNavigate()
  const [status, setStatus] = useState<'' | TaskStatus>('')
  const tasksQuery = useQuery({
    queryKey: ['tasks', status],
    queryFn: () => listTasks({ status }),
    refetchInterval: (query) =>
      query.state.data?.some((item) => item.status === 'running') ? 3_000 : false,
  })

  const tasks = tasksQuery.data ?? []

  return (
    <div className="page-shell">
      <section className="panel">
        <div className="toolbar">
          <div>
            <h2 className="text-base font-semibold text-slate-950">任务列表</h2>
            <div className="mt-1 text-sm text-slate-500">{formatNumber(tasks.length)} 条</div>
          </div>
          <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
            <select
              className="input sm:w-44"
              value={status}
              onChange={(event) => setStatus(event.target.value as '' | TaskStatus)}
            >
              {STATUS_OPTIONS.map((option) => (
                <option key={option.value || 'all'} value={option.value}>
                  {option.label}
                </option>
              ))}
            </select>
            <button className="btn btn-secondary" type="button" onClick={() => tasksQuery.refetch()}>
              <RefreshCw size={16} />
              刷新
            </button>
            <button className="btn btn-primary" type="button" onClick={() => navigate('/tasks/new')}>
              <Plus size={16} />
              新建任务
            </button>
          </div>
        </div>

        {tasksQuery.isError ? (
          <div className="panel-body text-sm text-red-700">{getErrorMessage(tasksQuery.error)}</div>
        ) : (
          <div className="table-wrap">
            <table className="data-table">
              <thead>
                <tr>
                  <th>名称</th>
                  <th>模块类型</th>
                  <th>状态</th>
                  <th>并发数</th>
                  <th>成功数</th>
                  <th>失败数</th>
                  <th>总数</th>
                  <th>创建时间</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-white/50 bg-white/35">
                {tasksQuery.isLoading ? (
                  <tr>
                    <td className="text-slate-500" colSpan={8}>
                      加载中
                    </td>
                  </tr>
                ) : tasks.length === 0 ? (
                  <tr>
                    <td className="text-slate-500" colSpan={8}>
                      暂无任务
                    </td>
                  </tr>
                ) : (
                  tasks.map((item) => (
                    <tr
                      className="cursor-pointer transition hover:bg-blue-50"
                      key={item.id}
                      onClick={() => navigate(`/tasks/${item.id}`)}
                    >
                      <td className="font-medium text-slate-950">{item.name || item.id}</td>
                      <td>{formatModuleType(item.module_type)}</td>
                      <td>
                        <StatusBadge status={item.status} />
                      </td>
                      <td>{formatNumber(item.concurrency)}</td>
                      <td>{formatNumber(item.success_count)}</td>
                      <td>{formatNumber(item.fail_count)}</td>
                      <td>{formatNumber(item.total_count)}</td>
                      <td>{formatDateTime(item.created_at)}</td>
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
