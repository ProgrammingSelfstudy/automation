import { Activity, ListChecks, Plus, Users } from 'lucide-react'
import { NavLink, Navigate, Route, Routes } from 'react-router-dom'

import AccountsPage from './pages/AccountsPage'
import CreateTaskPage from './pages/CreateTaskPage'
import TaskDetailPage from './pages/TaskDetailPage'
import TaskListPage from './pages/TaskListPage'

const navItems = [
  { to: '/', label: '任务', icon: ListChecks },
  { to: '/accounts', label: '账号', icon: Users },
  { to: '/tasks/new', label: '新建', icon: Plus },
]

export default function App() {
  return (
    <div className="min-h-screen bg-slate-100">
      <header className="border-b border-slate-200 bg-white">
        <div className="mx-auto flex w-full max-w-7xl flex-col gap-3 px-4 py-4 sm:px-6 lg:flex-row lg:items-center lg:justify-between lg:px-8">
          <div className="flex items-center gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-blue-600 text-white">
              <Activity size={20} />
            </div>
            <div>
              <h1 className="text-lg font-semibold text-slate-950">接口压测控制台</h1>
              <div className="text-xs text-slate-500">interface-load-test</div>
            </div>
          </div>

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
        </div>
      </header>

      <main>
        <Routes>
          <Route element={<TaskListPage />} path="/" />
          <Route element={<AccountsPage />} path="/accounts" />
          <Route element={<CreateTaskPage />} path="/tasks/new" />
          <Route element={<TaskDetailPage />} path="/tasks/:id" />
          <Route element={<Navigate replace to="/" />} path="*" />
        </Routes>
      </main>
    </div>
  )
}
