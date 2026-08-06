import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Plus, RefreshCw, Save } from 'lucide-react'
import { type FormEvent, useState } from 'react'

import { createScenario, listScenarios } from '../api/client'
import type { ScenarioStep } from '../api/client'
import ScenarioStepEditor from '../components/ScenarioStepEditor'
import SlideOver from '../components/SlideOver'
import { formatDateTime, getErrorMessage } from '../utils/format'
import { newScenarioStep } from '../utils/scenario'

export default function ScenariosPage() {
  const queryClient = useQueryClient()
  const [name, setName] = useState('')
  const [steps, setSteps] = useState<ScenarioStep[]>([newScenarioStep(0)])
  const [formula, setFormula] = useState('')
  const [createOpen, setCreateOpen] = useState(false)
  const [error, setError] = useState('')

  const scenariosQuery = useQuery({
    queryKey: ['scenarios'],
    queryFn: listScenarios,
  })

  const createMutation = useMutation({
    mutationFn: createScenario,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['scenarios'] })
      setName('')
      setSteps([newScenarioStep(0)])
      setFormula('')
      setError('')
      setCreateOpen(false)
    },
    onError: (mutationError) => setError(getErrorMessage(mutationError)),
  })

  function submitScenario(event: FormEvent) {
    event.preventDefault()
    setError('')
    createMutation.mutate({
      name,
      definition: {
        steps,
        formula,
      },
    })
  }

  const scenarios = scenariosQuery.data ?? []

  return (
    <div className="page-shell">
      <section className="panel">
        <div className="toolbar">
          <div>
            <h2 className="text-base font-semibold text-slate-950">场景库</h2>
            <div className="mt-1 text-sm text-slate-500">{scenarios.length} 个模板</div>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <button className="btn btn-secondary" type="button" onClick={() => scenariosQuery.refetch()}>
              <RefreshCw size={16} />
              刷新
            </button>
            <button className="btn btn-primary" type="button" onClick={() => setCreateOpen(true)}>
              <Plus size={16} />
              新建场景
            </button>
          </div>
        </div>

        {scenariosQuery.isError ? (
          <div className="panel-body text-sm text-red-700">{getErrorMessage(scenariosQuery.error)}</div>
        ) : (
          <div className="table-wrap">
            <table className="data-table">
              <thead>
                <tr>
                  <th>名称</th>
                  <th>步骤数</th>
                  <th>创建时间</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-white/50 bg-white/35">
                {scenariosQuery.isLoading ? (
                  <tr>
                    <td className="text-slate-500" colSpan={3}>
                      加载中
                    </td>
                  </tr>
                ) : scenarios.length === 0 ? (
                  <tr>
                    <td className="text-slate-500" colSpan={3}>
                      暂无场景
                    </td>
                  </tr>
                ) : (
                  scenarios.map((scenario) => (
                    <tr key={scenario.id}>
                      <td className="font-medium text-slate-950">{scenario.name}</td>
                      <td>
                        <span className="inline-flex h-6 items-center rounded bg-blue-50 px-2 text-xs font-semibold text-blue-700 ring-1 ring-blue-200">
                          {scenario.definition.steps.length}
                        </span>
                      </td>
                      <td>{formatDateTime(scenario.created_at)}</td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        )}
      </section>

      <SlideOver open={createOpen} title="新建场景" onClose={() => setCreateOpen(false)}>
        <form className="space-y-5" onSubmit={submitScenario}>
          <label className="block max-w-2xl space-y-1">
            <span className="field-label">场景名称</span>
            <input className="input" value={name} onChange={(event) => setName(event.target.value)} />
          </label>

          <ScenarioStepEditor
            formula={formula}
            showPerAccountCount={false}
            steps={steps}
            onFormulaChange={setFormula}
            onStepsChange={setSteps}
          />

          {error ? <div className="rounded-md bg-red-50 px-4 py-3 text-sm text-red-700">{error}</div> : null}

          <button className="btn btn-primary w-full" disabled={createMutation.isPending} type="submit">
            <Save size={16} />
            保存
          </button>
        </form>
      </SlideOver>
    </div>
  )
}
