import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Activity, BookOpen, KeyRound, ListChecks, LogOut, Plus, RefreshCw, Users, X } from 'lucide-react'
import { type FormEvent, useState } from 'react'
import { NavLink, Navigate, Route, Routes, useLocation, useNavigate } from 'react-router-dom'

import { logout, me, regenerateBackupCodes } from './api/client'
import AccountsPage from './pages/AccountsPage'
import CreateTaskPage from './pages/CreateTaskPage'
import LoginPage from './pages/LoginPage'
import ScenariosPage from './pages/ScenariosPage'
import TaskDetailPage from './pages/TaskDetailPage'
import TaskListPage from './pages/TaskListPage'
import { getErrorMessage } from './utils/format'

const navItems = [
  { to: '/', label: '任务', icon: ListChecks },
  { to: '/accounts', label: '账号', icon: Users },
  { to: '/scenarios', label: '场景', icon: BookOpen },
  { to: '/tasks/new', label: '新建', icon: Plus },
]

export default function App() {
  const location = useLocation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [backupModalOpen, setBackupModalOpen] = useState(false)
  const [backupPassword, setBackupPassword] = useState('')
  const [newBackupCodes, setNewBackupCodes] = useState<string[]>([])
  const [backupError, setBackupError] = useState('')

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
      <div className="flex min-h-screen items-center justify-center bg-slate-100 px-4 text-sm text-slate-600">
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
    <div className="min-h-screen bg-slate-100">
      <header className="border-b border-slate-200 bg-white">
        <div className="mx-auto flex w-full max-w-7xl flex-col gap-4 px-4 py-4 sm:px-6 lg:px-8">
          <div className="flex items-center gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-blue-600 text-white">
              <Activity size={20} />
            </div>
            <div>
              <h1 className="text-lg font-semibold text-slate-950">接口压测控制台</h1>
              <div className="text-xs text-slate-500">interface-load-test</div>
            </div>
          </div>

          <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
            <nav className="flex flex-wrap gap-2">
              {navItems.map((item) => {
                const Icon = item.icon
                return (
                  <NavLink
                    className={({ isActive }) =>
                      [
                        'inline-flex h-10 items-center gap-2 rounded-md px-3 text-sm font-medium transition',
                        isActive
                          ? 'bg-slate-900 text-white'
                          : 'border border-slate-300 bg-white text-slate-700 hover:bg-slate-50',
                      ].join(' ')
                    }
                    end={item.to === '/'}
                    key={item.to}
                    to={item.to}
                  >
                    <Icon size={16} />
                    {item.label}
                  </NavLink>
                )
              })}
            </nav>
            <div className="flex flex-wrap items-center gap-2">
              <div className="inline-flex h-10 items-center rounded-md border border-slate-200 bg-slate-50 px-3 text-sm font-medium text-slate-700">
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
        </div>
      </header>

      <main>
        <Routes>
          <Route element={<TaskListPage />} path="/" />
          <Route element={<AccountsPage />} path="/accounts" />
          <Route element={<ScenariosPage />} path="/scenarios" />
          <Route element={<CreateTaskPage />} path="/tasks/new" />
          <Route element={<TaskDetailPage />} path="/tasks/:id" />
          <Route element={<Navigate replace to="/" />} path="*" />
        </Routes>
      </main>

      {backupModalOpen ? (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/40 px-4 py-6">
          <section className="w-full max-w-lg rounded-lg border border-slate-200 bg-white shadow-xl">
            <div className="flex items-center justify-between border-b border-slate-200 px-5 py-4">
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
                  <span className="field-label">password</span>
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
    </div>
  )
}
