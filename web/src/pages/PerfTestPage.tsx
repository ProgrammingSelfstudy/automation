import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  AlertTriangle,
  Download,
  Eye,
  Play,
  RefreshCw,
  Smartphone,
  Square,
  Trash2,
} from 'lucide-react'
import { useEffect, useRef, useState } from 'react'

import {
  createPerfTask,
  deletePerfTask,
  getPerfTask,
  listPerfAgentDownloads,
  listPerfTasks,
  perfAgentDownloadURL,
  type PerfTaskResponse,
} from '../api/client'
import {
  connectPerfMonitoringWS,
  getPerfMonitoringTask,
  listPerfDeviceApps,
  listPerfDevices,
  MIN_COMPATIBLE_AGENT_VERSION,
  probePerfAgent,
  startPerfMonitoring,
  stopPerfMonitoring,
  type PerfMonitoringTask,
} from '../api/perfAgent'
import SlideOver from '../components/SlideOver'
import { formatDateTime, getErrorMessage } from '../utils/format'

// Agent 侧时间戳是 "2006-01-02 15:04:05"（Go 的本地时间格式，非 RFC3339），
// 中心平台的 time.Time 字段按 RFC3339 解析——上报前必须转一次，否则每次
// createPerfTask 都会因为 JSON 解析失败而 400。
function toRFC3339(raw: string): string {
  if (!raw) {
    return new Date().toISOString()
  }
  const isoLike = raw.includes('T') ? raw : raw.replace(' ', 'T')
  const parsed = new Date(isoLike)
  return Number.isNaN(parsed.getTime()) ? new Date().toISOString() : parsed.toISOString()
}

type AgentStatus = 'checking' | 'unreachable' | 'incompatible' | 'ready'

export default function PerfTestPage() {
  const queryClient = useQueryClient()
  const [agentStatus, setAgentStatus] = useState<AgentStatus>('checking')
  const [agentVersion, setAgentVersion] = useState<string | null>(null)
  const [probing, setProbing] = useState(false)

  const [selectedDeviceId, setSelectedDeviceId] = useState('')
  const [packageName, setPackageName] = useState('')
  const [processName, setProcessName] = useState('')

  const [activeTask, setActiveTask] = useState<PerfMonitoringTask | null>(null)
  const [startError, setStartError] = useState('')
  const [uploadError, setUploadError] = useState('')
  const wsCleanup = useRef<(() => void) | null>(null)

  const [detailID, setDetailID] = useState<string | null>(null)

  async function probe() {
    setProbing(true)
    const result = await probePerfAgent()
    if (!result.reachable) {
      setAgentStatus('unreachable')
      setAgentVersion(null)
    } else {
      setAgentStatus(result.compatible ? 'ready' : 'incompatible')
      setAgentVersion(result.version)
    }
    setProbing(false)
  }

  useEffect(() => {
    probe()
  }, [])

  const agentDownloadsQuery = useQuery({
    queryKey: ['perf-agent', 'downloads'],
    queryFn: listPerfAgentDownloads,
    enabled: agentStatus === 'unreachable' || agentStatus === 'incompatible',
  })

  const devicesQuery = useQuery({
    queryKey: ['perf-agent', 'devices'],
    queryFn: ({ signal }) => listPerfDevices(signal),
    enabled: agentStatus === 'ready',
    refetchInterval: agentStatus === 'ready' ? 5000 : false,
  })

  const appsQuery = useQuery({
    queryKey: ['perf-agent', 'apps', selectedDeviceId],
    queryFn: ({ signal }) => listPerfDeviceApps(selectedDeviceId, signal),
    enabled: agentStatus === 'ready' && selectedDeviceId !== '',
  })

  const historyQuery = useQuery({
    queryKey: ['perf-tasks'],
    queryFn: listPerfTasks,
  })

  const detailQuery = useQuery({
    queryKey: ['perf-tasks', detailID],
    queryFn: () => getPerfTask(detailID as string),
    enabled: detailID !== null,
  })

  const deleteMutation = useMutation({
    mutationFn: deletePerfTask,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['perf-tasks'] })
      setDetailID(null)
    },
  })

  const uploadMutation = useMutation({
    mutationFn: createPerfTask,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['perf-tasks'] })
      setUploadError('')
    },
    onError: (error) => setUploadError(getErrorMessage(error)),
  })

  function closeWS() {
    if (wsCleanup.current !== null) {
      wsCleanup.current()
      wsCleanup.current = null
    }
  }

  useEffect(() => closeWS, [])

  const selectedDevice = devicesQuery.data?.find((device) => device.device_id === selectedDeviceId)

  async function handleStart() {
    if (!selectedDevice || packageName.trim() === '') {
      return
    }
    setStartError('')
    try {
      const started = await startPerfMonitoring({
        deviceId: selectedDevice.device_id,
        packageName: packageName.trim(),
        processName: processName.trim() || packageName.trim(),
        deviceModel: selectedDevice.model,
      })
      const initial: PerfMonitoringTask = {
        task_id: started.task_id,
        status: 'collecting',
        device_id: selectedDevice.device_id,
        package_name: packageName.trim(),
        process_name: processName.trim() || packageName.trim(),
        platform: selectedDevice.platform,
        device_model: selectedDevice.model,
        start_time: started.start_time,
        sample_interval_ms: started.sample_interval_ms,
        last_error: '',
        samples: [],
      }
      setActiveTask(initial)

      wsCleanup.current = connectPerfMonitoringWS(started.task_id, {
        onUpdate: (update) => {
          setActiveTask((current) => {
            if (!current || current.task_id !== started.task_id) {
              return current
            }
            return {
              ...current,
              status: update.status,
              last_error: update.lastError,
              samples: [...current.samples, ...update.newSamples],
            }
          })
        },
        onError: (message) => setStartError(message),
      })
    } catch (error) {
      setStartError(getErrorMessage(error))
    }
  }

  async function handleStop() {
    if (!activeTask) {
      return
    }
    closeWS()
    try {
      const stopped = await stopPerfMonitoring(activeTask.task_id)
      // WS 在任务结束时会推最后一条更新再自己关闭连接，但这里不依赖那条
      // 消息的到达时机——停止后直接再拉一次完整数据，确保上报的是最终、
      // 完整的样本集合，不会因为消息顺序问题漏掉最后几个样本。
      const final = await getPerfMonitoringTask(activeTask.task_id)
      setActiveTask(null)
      uploadMutation.mutate({
        device_id: final.device_id,
        package_name: final.package_name,
        process_name: final.process_name,
        platform: final.platform,
        device_model: final.device_model,
        status: final.status,
        start_time: toRFC3339(final.start_time),
        stop_time: toRFC3339(stopped.stop_time),
        sample_interval_ms: final.sample_interval_ms,
        sample_count: final.samples.length,
        last_error: final.last_error,
        samples: final.samples,
      })
    } catch (error) {
      setStartError(getErrorMessage(error))
    }
  }

  if (agentStatus !== 'ready') {
    const heading =
      agentStatus === 'checking'
        ? '正在检测本地采集工具…'
        : agentStatus === 'incompatible'
          ? '本地采集工具版本过旧，请升级'
          : '未检测到本地采集工具'

    const description =
      agentStatus === 'incompatible' ? (
        <>
          当前本地 Agent 版本是 <code className="rounded bg-slate-100 px-1.5 py-0.5 text-xs">{agentVersion}</code>
          ，低于这版页面要求的最低版本{' '}
          <code className="rounded bg-slate-100 px-1.5 py-0.5 text-xs">{MIN_COMPATIBLE_AGENT_VERSION}</code>
          ，部分接口字段可能对不上。请退出旧版 Agent，下载安装下面的新版本后重新启动。
        </>
      ) : (
        <>
          性能测试需要在你自己的电脑上运行本地采集 Agent（perf-rabbit），浏览器通过{' '}
          <code className="rounded bg-slate-100 px-1.5 py-0.5 text-xs">127.0.0.1:9527</code>{' '}
          与它通信来控制手机采集。请先启动 Agent；Android 设备还需要本机装好 adb 并加入 PATH，
          iOS 设备需要本机预装 Python 3.8+。
        </>
      )

    return (
      <div className="page-shell">
        <section className="panel panel-body flex flex-col items-center gap-4 py-12 text-center">
          <div className="flex h-14 w-14 items-center justify-center rounded-full bg-amber-50 text-amber-600 ring-1 ring-amber-200">
            <AlertTriangle size={26} />
          </div>
          <div>
            <h2 className="text-base font-semibold text-slate-950">{heading}</h2>
            <p className="mt-2 max-w-md text-sm text-slate-500">{description}</p>
          </div>
          <button className="btn btn-secondary" disabled={probing} type="button" onClick={probe}>
            <RefreshCw size={16} className={probing ? 'animate-spin' : ''} />
            重新检测
          </button>

          {agentStatus === 'unreachable' || agentStatus === 'incompatible' ? (
            <div className="mt-2 flex flex-wrap items-center justify-center gap-2">
              {(agentDownloadsQuery.data ?? []).map((option) => (
                <a
                  className={`btn ${option.available ? 'btn-primary' : 'btn-secondary pointer-events-none opacity-50'}`}
                  download
                  href={option.available ? perfAgentDownloadURL(option.filename) : undefined}
                  key={option.platform}
                >
                  <Download size={16} />
                  {option.label}
                  {!option.available ? '（暂未提供）' : ''}
                </a>
              ))}
            </div>
          ) : null}
        </section>
      </div>
    )
  }

  return (
    <div className="page-shell">
      <section className="panel">
        <div className="toolbar">
          <div>
            <h2 className="text-base font-semibold text-slate-950">性能采集</h2>
            <div className="mt-1 text-sm text-slate-500">选择设备和 App，开始实时性能采集</div>
          </div>
          <button className="btn btn-secondary" type="button" onClick={() => devicesQuery.refetch()}>
            <RefreshCw size={16} className={devicesQuery.isFetching ? 'animate-spin' : ''} />
            刷新设备
          </button>
        </div>

        <div className="panel-body space-y-4">
          <div className="grid gap-4 lg:grid-cols-3">
            <label className="space-y-1">
              <span className="field-label">设备</span>
              <select
                className="input"
                disabled={activeTask !== null}
                value={selectedDeviceId}
                onChange={(event) => setSelectedDeviceId(event.target.value)}
              >
                <option value="">请选择设备</option>
                {(devicesQuery.data ?? []).map((device) => (
                  <option disabled={device.status !== 'available'} key={device.device_id} value={device.device_id}>
                    {device.device_name}（{device.platform} {device.version}）
                    {device.status !== 'available' ? ' - 离线' : ''}
                  </option>
                ))}
              </select>
            </label>
            <label className="space-y-1">
              <span className="field-label">包名</span>
              <input
                className="input"
                disabled={activeTask !== null}
                list="perf-device-apps"
                placeholder="com.example.app"
                value={packageName}
                onChange={(event) => setPackageName(event.target.value)}
              />
              <datalist id="perf-device-apps">
                {(appsQuery.data ?? []).map((app) => (
                  <option key={app.package_name} value={app.package_name}>
                    {app.app_name}
                  </option>
                ))}
              </datalist>
            </label>
            <label className="space-y-1">
              <span className="field-label">进程名（留空同包名）</span>
              <input
                className="input"
                disabled={activeTask !== null}
                placeholder={packageName}
                value={processName}
                onChange={(event) => setProcessName(event.target.value)}
              />
            </label>
          </div>

          {startError ? <div className="rounded-md bg-red-50 px-4 py-3 text-sm text-red-700">{startError}</div> : null}
          {uploadError ? (
            <div className="rounded-md bg-red-50 px-4 py-3 text-sm text-red-700">上报采集记录失败：{uploadError}</div>
          ) : null}

          {activeTask ? (
            <div className="space-y-3 rounded-lg border border-blue-100 bg-blue-50/60 p-4">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <div className="flex items-center gap-2 text-sm font-semibold text-blue-700">
                  <Smartphone size={16} />
                  {activeTask.status === 'collecting' ? '正在采集' : '采集已中断'} · {activeTask.package_name}
                  <span className="inline-flex h-6 items-center rounded bg-blue-100 px-2 text-xs font-semibold text-blue-700 ring-1 ring-blue-200">
                    已采集 {activeTask.samples.length} 个样本
                  </span>
                </div>
                <button className="btn btn-danger" type="button" onClick={handleStop}>
                  <Square size={16} />
                  停止采集
                </button>
              </div>
              {activeTask.status !== 'collecting' ? (
                <div className="rounded-md bg-amber-50 px-4 py-3 text-sm text-amber-700">
                  设备已断开，采集自动中断{activeTask.last_error ? `：${activeTask.last_error}` : ''}
                  。点击"停止采集"确认并上报这段记录。
                </div>
              ) : null}
              <PerfLiveMetrics task={activeTask} />
            </div>
          ) : (
            <button
              className="btn btn-primary"
              disabled={!selectedDevice || packageName.trim() === ''}
              type="button"
              onClick={handleStart}
            >
              <Play size={16} />
              开始采集
            </button>
          )}
        </div>
      </section>

      <section className="panel">
        <div className="toolbar">
          <div>
            <h2 className="text-base font-semibold text-slate-950">历史记录</h2>
            <div className="mt-1 text-sm text-slate-500">采集结束后自动上报到平台，多端可见</div>
          </div>
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

function PerfLiveMetrics({ task }: { task: PerfMonitoringTask }) {
  const latest = task.samples[task.samples.length - 1]
  if (!latest) {
    return <div className="text-sm text-blue-700/70">等待第一个样本…</div>
  }
  const stats = [
    { label: 'App CPU', value: `${latest.app_cpu.toFixed(1)}%` },
    { label: '总 CPU', value: `${latest.total_cpu.toFixed(1)}%` },
    { label: '内存 PSS', value: `${latest.memory_pss.toFixed(1)} MB` },
    { label: 'FPS', value: latest.fps.toFixed(0) },
    { label: 'Jank', value: latest.jank.toFixed(0) },
    { label: 'Big Jank', value: latest.big_jank.toFixed(0) },
  ]
  return (
    <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-6">
      {stats.map((stat) => (
        <div className="rounded-md bg-white/70 px-3 py-2 text-center shadow-sm" key={stat.label}>
          <div className="text-xs text-slate-500">{stat.label}</div>
          <div className="mt-1 text-sm font-semibold text-slate-950">{stat.value}</div>
        </div>
      ))}
    </div>
  )
}

function PerfHistoryDetail({ detail, loading }: { detail?: PerfTaskResponse; loading: boolean }) {
  if (loading) {
    return <div className="text-sm text-slate-500">加载中</div>
  }
  if (!detail) {
    return <div className="text-sm text-slate-500">未找到记录</div>
  }
  const samples = Array.isArray(detail.samples) ? detail.samples : []
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
      <div>
        <div className="field-label mb-2">原始样本（JSON）</div>
        <pre className="max-h-96 overflow-auto rounded-md bg-slate-950/90 p-4 text-xs text-slate-100">
          {JSON.stringify(samples, null, 2)}
        </pre>
      </div>
    </div>
  )
}
