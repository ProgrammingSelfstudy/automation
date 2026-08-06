import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Plus, Save, X } from 'lucide-react'
import { type FormEvent, useEffect, useMemo, useState } from 'react'

import {
  createEnvironment,
  listEnvironments,
  updateEnvironment,
  type EnvironmentResponse,
} from '../api/client'
import { getErrorMessage } from '../utils/format'
import KeyValueEditor from './KeyValueEditor'

type EnvironmentModalProps = {
  open: boolean
  onClose: () => void
}

const EMPTY_ENVIRONMENTS: EnvironmentResponse[] = []

export default function EnvironmentModal({ open, onClose }: EnvironmentModalProps) {
  const queryClient = useQueryClient()
  const [selectedID, setSelectedID] = useState('')
  const [newName, setNewName] = useState('')
  const [draftName, setDraftName] = useState('')
  const [draftVariables, setDraftVariables] = useState<Record<string, string>>({})
  const [draftDefaultHeaders, setDraftDefaultHeaders] = useState<Record<string, string>>({})
  const [error, setError] = useState('')

  const environmentsQuery = useQuery({
    queryKey: ['environments'],
    queryFn: listEnvironments,
    enabled: open,
  })

  const environments = environmentsQuery.data ?? EMPTY_ENVIRONMENTS
  const selectedEnvironment = useMemo(
    () => environments.find((item) => item.id === selectedID) ?? null,
    [environments, selectedID],
  )

  useEffect(() => {
    if (!open) {
      return
    }
    if (selectedID === '' && environments.length > 0) {
      setSelectedID(environments[0].id)
    }
  }, [environments, open, selectedID])

  useEffect(() => {
    if (!selectedEnvironment) {
      setDraftName('')
      setDraftVariables({})
      setDraftDefaultHeaders({})
      return
    }
    setDraftName(selectedEnvironment.name)
    setDraftVariables(selectedEnvironment.variables)
    setDraftDefaultHeaders(selectedEnvironment.default_headers)
    setError('')
  }, [selectedEnvironment])

  const createMutation = useMutation({
    mutationFn: createEnvironment,
    onSuccess: async (created) => {
      await queryClient.invalidateQueries({ queryKey: ['environments'] })
      setSelectedID(created.id)
      setNewName('')
      setError('')
    },
    onError: (caught) => setError(getErrorMessage(caught)),
  })

  const updateMutation = useMutation({
    mutationFn: ({
      id,
      name,
      variables,
      defaultHeaders,
    }: {
      id: string
      name: string
      variables: Record<string, string>
      defaultHeaders: Record<string, string>
    }) =>
      updateEnvironment(id, {
        name,
        variables,
        default_headers: defaultHeaders,
      }),
    onSuccess: async (updated) => {
      await queryClient.invalidateQueries({ queryKey: ['environments'] })
      setSelectedID(updated.id)
      setDraftName(updated.name)
      setDraftVariables(updated.variables)
      setDraftDefaultHeaders(updated.default_headers)
      setError('')
    },
    onError: (caught) => setError(getErrorMessage(caught)),
  })

  function submitCreate(event: FormEvent) {
    event.preventDefault()
    setError('')
    createMutation.mutate({ name: newName, variables: {}, default_headers: {} })
  }

  function submitUpdate(event: FormEvent) {
    event.preventDefault()
    if (!selectedEnvironment) {
      return
    }
    setError('')
    updateMutation.mutate({
      id: selectedEnvironment.id,
      name: draftName,
      variables: draftVariables,
      defaultHeaders: draftDefaultHeaders,
    })
  }

  return (
    <div className={`fixed inset-0 z-50 ${open ? 'pointer-events-auto' : 'pointer-events-none'}`}>
      <button
        aria-label="关闭环境管理"
        className={`absolute inset-0 bg-slate-950/40 transition-opacity duration-300 ${open ? 'opacity-100' : 'opacity-0'}`}
        type="button"
        onClick={onClose}
      />
      <section
        aria-modal="true"
        className={`glass-panel absolute left-1/2 top-1/2 flex h-[min(760px,calc(100vh-2rem))] w-[min(1120px,calc(100vw-2rem))] -translate-x-1/2 flex-col overflow-hidden shadow-2xl transition-all duration-300 ${
          open ? '-translate-y-1/2 opacity-100' : '-translate-y-[46%] opacity-0'
        }`}
        role="dialog"
      >
        <div className="flex items-center justify-between border-b border-white/50 px-5 py-4">
          <div>
            <h2 className="text-base font-semibold text-slate-950">环境变量</h2>
            <div className="mt-1 text-sm text-slate-500">维护接口模板预览用的变量表</div>
          </div>
          <button aria-label="关闭" className="icon-btn" type="button" onClick={onClose}>
            <X size={16} />
          </button>
        </div>

        <div className="grid min-h-0 flex-1 gap-0 md:grid-cols-[280px_minmax(0,1fr)]">
          <aside className="min-h-0 border-b border-white/50 p-4 md:border-b-0 md:border-r">
            <form className="space-y-2" onSubmit={submitCreate}>
              <label className="block space-y-1">
                <span className="field-label">新环境</span>
                <input
                  className="input"
                  placeholder="例如 dev"
                  value={newName}
                  onChange={(event) => setNewName(event.target.value)}
                />
              </label>
              <button className="btn btn-primary w-full" disabled={createMutation.isPending} type="submit">
                <Plus size={16} />
                创建
              </button>
            </form>

            <div className="mt-4 space-y-2 overflow-y-auto">
              {environmentsQuery.isLoading ? (
                <div className="rounded-md bg-white/50 px-3 py-2 text-sm text-slate-500">加载中</div>
              ) : environments.length === 0 ? (
                <div className="rounded-md bg-white/50 px-3 py-2 text-sm text-slate-500">暂无环境</div>
              ) : (
                environments.map((env) => (
                  <button
                    className={`flex h-10 w-full items-center rounded-md px-3 text-left text-sm font-medium transition ${
                      env.id === selectedID
                        ? 'bg-gradient-to-r from-blue-600 to-cyan-500 text-white shadow-lg shadow-blue-500/20'
                        : 'text-slate-700 hover:bg-white/70 hover:text-blue-700'
                    }`}
                    key={env.id}
                    type="button"
                    onClick={() => setSelectedID(env.id)}
                  >
                    <span className="min-w-0 truncate">{env.name}</span>
                  </button>
                ))
              )}
            </div>
          </aside>

          <main className="min-h-0 overflow-y-auto p-5">
            {selectedEnvironment ? (
              <form className="space-y-5" onSubmit={submitUpdate}>
                <label className="block max-w-xl space-y-1">
                  <span className="field-label">环境名称</span>
                  <input className="input" value={draftName} onChange={(event) => setDraftName(event.target.value)} />
                </label>

                <section className="space-y-2">
                  <h3 className="field-label">变量</h3>
                  <KeyValueEditor
                    keyPlaceholder="KEY"
                    value={draftVariables}
                    valuePlaceholder="值"
                    onChange={setDraftVariables}
                  />
                </section>

                <section className="space-y-2">
                  <h3 className="field-label">默认请求头</h3>
                  <KeyValueEditor
                    keyPlaceholder="请求头名"
                    value={draftDefaultHeaders}
                    onChange={setDraftDefaultHeaders}
                  />
                </section>

                {error ? <div className="rounded-md bg-red-50 px-4 py-3 text-sm text-red-700">{error}</div> : null}

                <button className="btn btn-primary" disabled={updateMutation.isPending} type="submit">
                  <Save size={16} />
                  保存
                </button>
              </form>
            ) : (
              <div className="space-y-3">
                <div className="rounded-md bg-white/50 px-4 py-3 text-sm text-slate-500">请选择或新建一个环境。</div>
                {error ? <div className="rounded-md bg-red-50 px-4 py-3 text-sm text-red-700">{error}</div> : null}
              </div>
            )}
          </main>
        </div>
      </section>
    </div>
  )
}
