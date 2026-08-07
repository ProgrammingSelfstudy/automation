import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Activity,
  BookOpen,
  Braces,
  ChevronDown,
  ChevronRight,
  FileCode2,
  Gauge,
  Globe2,
  KeyRound,
  ListChecks,
  LogOut,
  RefreshCw,
  Users,
  X,
} from 'lucide-react'
import { type FormEvent, useState } from 'react'
import { NavLink, Navigate, Route, Routes, useLocation, useNavigate } from 'react-router-dom'

import { logout, me, regenerateBackupCodes } from './api/client'
import EnvironmentModal from './components/EnvironmentModal'
import GlobalVariablesModal from './components/GlobalVariablesModal'
import AccountsPage from './pages/AccountsPage'
import CreateTaskPage from './pages/CreateTaskPage'
import InterfacesPage from './pages/InterfacesPage'
import LoginPage from './pages/LoginPage'
import PerfTestPage from './pages/PerfTestPage'
import ScenariosPage from './pages/ScenariosPage'
import TaskDetailPage from './pages/TaskDetailPage'
import TaskListPage from './pages/TaskListPage'
import { getErrorMessage } from './utils/format'

const moduleSections = [
  {
    key: 'interface-test',
    title: '接口测试',
    items: [
      { to: '/interfaces', label: '接口', icon: FileCode2 },
      { to: '/scenarios', label: '场景列表', icon: BookOpen },
      {
        to: '/runs',
        label: '运行记录',
        icon: ListChecks,
        match: (pathname: string) => pathname === '/runs' || pathname.startsWith('/tasks/'),
      },
    ],
  },
  {
    key: 'perf-test',
    title: '性能测试',
    items: [{ to: '/perf', label: '性能采集', icon: Gauge }],
  },
]

export default function App() {
  const location = useLocation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [backupModalOpen, setBackupModalOpen] = useState(false)
  const [backupPassword, setBackupPassword] = useState('')
  const [newBackupCodes, setNewBackupCodes] = useState<string[]>([])
  const [backupError, setBackupError] = useState('')
  const [expandedModules, setExpandedModules] = useState<Record<string, boolean>>(
    Object.fromEntries(moduleSections.map((section) => [section.key, true])),
  )
  const [environmentModalOpen, setEnvironmentModalOpen] = useState(false)
  const [globalVariablesModalOpen, setGlobalVariablesModalOpen] = useState(false)

  const meQuery = useQuery({
    queryKey: ['auth', 'me'],
    queryFn: me,
    retry: false,
  })

  const logoutMutation = useMutation({
    mutationFn: logout,
    onSuccess: async () => {
      await queryClient.removeQueries({ queryKey: ['auth', 'me'] })
      navigate('/login', { replace: true })
    },
  })

  const regenerateMutation = useMutation({
    mutationFn: regenerateBackupCodes,
    onSuccess: (response) => {
      setNewBackupCodes(response.backup_codes)
      setBackupPassword('')
      setBackupError('')
    },
    onError: (caught) => setBackupError(getErrorMessage(caught)),
  })

  function submitBackupRegeneration(event: FormEvent) {
    event.preventDefault()
    setBackupError('')
    setNewBackupCodes([])
    regenerateMutation.mutate({ password: backupPassword })
  }

  function closeBackupModal() {
    setBackupModalOpen(false)
    setBackupPassword('')
    setNewBackupCodes([])
    setBackupError('')
    regenerateMutation.reset()
  }

  if (location.pathname === '/login') {
    return <LoginPage />
  }

  if (meQuery.isLoading) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-slate-950 px-4 text-sm text-slate-300">
        加载中
      </div>
    )
  }

  if (meQuery.isError) {
    return <Navigate replace to="/login" />
  }

  const currentUser = meQuery.data?.user
  if (!currentUser) {
    return <Navigate replace to="/login" />
  }

  return (
    <div className="relative min-h-screen overflow-hidden text-slate-900">
      <div className="app-backdrop" aria-hidden="true" />

      <header className="sticky top-0 z-30 border-b border-white/50 bg-white/65 backdrop-blur-xl">
        <div className="flex w-full flex-col gap-3 px-4 py-3 sm:px-6 lg:flex-row lg:items-center lg:justify-between lg:px-8">
          <div className="flex items-center gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-gradient-to-br from-[#1677ff] via-[#1aa8ff] to-[#22d7c7] text-white shadow-lg shadow-blue-500/20">
              <Activity size={20} />
            </div>
            <div>
              <h1 className="text-lg font-semibold text-slate-950">接口压测控制台</h1>
              <div className="text-xs text-slate-500">API 设计、开发、测试一体化协作</div>
            </div>
          </div>

          <div className="flex flex-wrap items-center gap-2">
            <NavLink
              className={({ isActive }) =>
                [
                  'btn btn-secondary',
                  isActive ? 'border-blue-200 bg-blue-50/80 text-blue-700' : '',
                ].join(' ')
              }
              to="/accounts"
            >
              <Users size={16} />
              账号
            </NavLink>
            <button className="btn btn-secondary" type="button" onClick={() => setEnvironmentModalOpen(true)}>
              <Globe2 size={16} />
              环境
            </button>
            <button className="btn btn-secondary" type="button" onClick={() => setGlobalVariablesModalOpen(true)}>
              <Braces size={16} />
              全局变量
            </button>
            <div className="inline-flex h-10 items-center rounded-md border border-white/60 bg-white/55 px-3 text-sm font-medium text-slate-700 backdrop-blur-xl">
              {currentUser.username}
            </div>
            <button className="btn btn-secondary" type="button" onClick={() => setBackupModalOpen(true)}>
              <KeyRound size={16} />
              备用码
            </button>
            <button
              className="btn btn-secondary"
              disabled={logoutMutation.isPending}
              type="button"
              onClick={() => logoutMutation.mutate()}
            >
              <LogOut size={16} />
              登出
            </button>
          </div>
        </div>
      </header>

      <div className="grid w-full gap-5 px-4 py-5 sm:px-6 lg:grid-cols-[260px_minmax(0,1fr)] lg:px-8">
        <aside className="glass-panel h-fit space-y-4 p-3 lg:sticky lg:top-20">
          {moduleSections.map((section) => {
            const expanded = expandedModules[section.key] ?? true
            return (
              <div key={section.key}>
                <div className="mb-2 flex items-center justify-between px-2 py-2">
                  <div>
                    <div className="text-xs font-semibold text-slate-500">模块</div>
                    <div className="mt-1 text-base font-semibold text-slate-950">{section.title}</div>
                  </div>
                  <button
                    aria-expanded={expanded}
                    aria-label={expanded ? `收起${section.title}模块` : `展开${section.title}模块`}
                    className="flex h-8 w-8 items-center justify-center rounded-full border border-cyan-200/70 bg-cyan-50/80 text-cyan-600"
                    type="button"
                    onClick={() =>
                      setExpandedModules((current) => ({ ...current, [section.key]: !expanded }))
                    }
                  >
                    {expanded ? <ChevronDown size={16} /> : <ChevronRight size={16} />}
                  </button>
                </div>

                {expanded ? (
                  <nav className="space-y-2">
                    {section.items.map((item) => {
                      const Icon = item.icon
                      return (
                        <NavLink
                          className={({ isActive }) =>
                            [
                              'flex h-11 items-center gap-3 rounded-lg px-3 text-sm font-medium transition',
                              isActive || item.match?.(location.pathname)
                                ? 'bg-gradient-to-r from-blue-600 to-cyan-500 text-white shadow-lg shadow-blue-500/20'
                                : 'text-slate-600 hover:bg-white/70 hover:text-blue-700',
                            ].join(' ')
                          }
                          end={item.to === '/runs'}
                          key={item.to}
                          to={item.to}
                        >
                          <Icon size={16} />
                          {item.label}
                        </NavLink>
                      )
                    })}
                  </nav>
                ) : null}
              </div>
            )
          })}
        </aside>

        <main className="min-w-0">
          <Routes>
            <Route element={<Navigate replace to="/interfaces" />} path="/" />
            <Route element={<AccountsPage />} path="/accounts" />
            <Route element={<InterfacesPage />} path="/interfaces" />
            <Route element={<ScenariosPage />} path="/scenarios" />
            <Route element={<TaskListPage />} path="/runs" />
            <Route element={<CreateTaskPage />} path="/tasks/new" />
            <Route element={<TaskDetailPage />} path="/tasks/:id" />
            <Route element={<PerfTestPage />} path="/perf" />
            <Route element={<Navigate replace to="/interfaces" />} path="*" />
          </Routes>
        </main>
      </div>

      {backupModalOpen ? (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/40 px-4 py-6">
          <section className="glass-panel w-full max-w-lg shadow-xl">
            <div className="flex items-center justify-between border-b border-white/50 px-5 py-4">
              <div>
                <h2 className="text-base font-semibold text-slate-950">重新生成备用码</h2>
                <div className="mt-1 text-sm text-slate-500">旧的未使用备用码会失效。</div>
              </div>
              <button aria-label="关闭" className="icon-btn" type="button" onClick={closeBackupModal}>
                <X size={16} />
              </button>
            </div>
            {newBackupCodes.length === 0 ? (
              <form className="space-y-4 p-5" onSubmit={submitBackupRegeneration}>
                <label className="block space-y-1">
                  <span className="field-label">当前密码</span>
                  <input
                    autoComplete="current-password"
                    className="input"
                    type="password"
                    value={backupPassword}
                    onChange={(event) => setBackupPassword(event.target.value)}
                  />
                </label>
                {backupError ? (
                  <div className="rounded-md bg-red-50 px-4 py-3 text-sm text-red-700">{backupError}</div>
                ) : null}
                <button className="btn btn-primary w-full" disabled={regenerateMutation.isPending} type="submit">
                  <RefreshCw size={16} />
                  重新生成
                </button>
              </form>
            ) : (
              <div className="space-y-5 p-5">
                <div className="grid gap-2 sm:grid-cols-2">
                  {newBackupCodes.map((code) => (
                    <code className="rounded-md bg-slate-100 px-3 py-2 text-center text-sm text-slate-800" key={code}>
                      {code}
                    </code>
                  ))}
                </div>
                <button className="btn btn-primary w-full" type="button" onClick={closeBackupModal}>
                  完成
                </button>
              </div>
            )}
          </section>
        </div>
      ) : null}

      <EnvironmentModal open={environmentModalOpen} onClose={() => setEnvironmentModalOpen(false)} />
      <GlobalVariablesModal open={globalVariablesModalOpen} onClose={() => setGlobalVariablesModalOpen(false)} />
    </div>
  )
}
