import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { RefreshCw, Save } from 'lucide-react'
import { type FormEvent, useState } from 'react'

import { createInterface, listInterfaces } from '../api/client'
import type { ScenarioStep } from '../api/client'
import StepFields from '../components/StepFields'
import { formatDateTime, getErrorMessage } from '../utils/format'
import { newScenarioStep } from '../utils/scenario'

export default function InterfacesPage() {
  const queryClient = useQueryClient()
  const [name, setName] = useState('')
  const [step, setStep] = useState<ScenarioStep>(() => newScenarioStep(0))
  const [error, setError] = useState('')

  const interfacesQuery = useQuery({
    queryKey: ['interfaces'],
    queryFn: listInterfaces,
  })

  const createMutation = useMutation({
    mutationFn: createInterface,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['interfaces'] })
      setName('')
      setStep(newScenarioStep(0))
      setError('')
    },
    onError: (mutationError) => setError(getErrorMessage(mutationError)),
  })

  function submitInterface(event: FormEvent) {
    event.preventDefault()
    setError('')
    createMutation.mutate({ name, step })
  }

  const interfaces = interfacesQuery.data ?? []

  return (
    <div className="page-shell">
      <section className="panel">
        <div className="toolbar">
          <div>
            <h2 className="text-base font-semibold text-slate-950">接口库</h2>
            <div className="mt-1 text-sm text-slate-500">{interfaces.length} 个接口</div>
          </div>
          <button className="btn btn-secondary" type="button" onClick={() => interfacesQuery.refetch()}>
            <RefreshCw size={16} />
            刷新
          </button>
        </div>

        {interfacesQuery.isError ? (
          <div className="panel-body text-sm text-red-700">{getErrorMessage(interfacesQuery.error)}</div>
        ) : (
          <div className="table-wrap">
            <table className="data-table">
              <thead>
                <tr>
                  <th>名称</th>
                  <th>方式</th>
                  <th>URL</th>
                  <th>创建时间</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-white/50 bg-white/35">
                {interfacesQuery.isLoading ? (
                  <tr>
                    <td className="text-slate-500" colSpan={4}>
                      加载中
                    </td>
                  </tr>
                ) : interfaces.length === 0 ? (
                  <tr>
                    <td className="text-slate-500" colSpan={4}>
                      暂无接口
                    </td>
                  </tr>
                ) : (
                  interfaces.map((item) => (
                    <tr key={item.id}>
                      <td className="font-medium text-slate-950">{item.name}</td>
                      <td>
                        <span className="inline-flex h-6 items-center rounded bg-blue-50 px-2 text-xs font-semibold text-blue-700 ring-1 ring-blue-200">
                          {item.step.method}
                        </span>
                      </td>
                      <td className="max-w-2xl truncate">{item.step.url}</td>
                      <td>{formatDateTime(item.created_at)}</td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        )}
      </section>

      <form className="panel" onSubmit={submitInterface}>
        <div className="toolbar">
          <h2 className="text-base font-semibold text-slate-950">新建接口</h2>
          <button className="btn btn-primary" disabled={createMutation.isPending} type="submit">
            <Save size={16} />
            保存
          </button>
        </div>
        <div className="space-y-5 p-4 sm:p-5">
          <label className="block max-w-2xl space-y-1">
            <span className="field-label">接口名称</span>
            <input className="input" value={name} onChange={(event) => setName(event.target.value)} />
          </label>
          <StepFields step={step} onChange={setStep} />

          {error ? <div className="rounded-md bg-red-50 px-4 py-3 text-sm text-red-700">{error}</div> : null}
        </div>
      </form>
    </div>
  )
}
