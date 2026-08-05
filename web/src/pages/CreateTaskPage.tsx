import { useMutation } from '@tanstack/react-query'
import { ArrowLeft, Send } from 'lucide-react'
import { type FormEvent, useState } from 'react'
import { useNavigate } from 'react-router-dom'

import { createTask } from '../api/client'
import type { ScenarioStep } from '../api/client'
import ScenarioStepEditor from '../components/ScenarioStepEditor'
import { getErrorMessage } from '../utils/format'

type TaskForm = {
  name: string
  account_group_id: string
  concurrency: number
  created_by: string
}

const initialForm: TaskForm = {
  name: '',
  account_group_id: '',
  concurrency: 1,
  created_by: '',
}

function newScenarioStep(index: number): ScenarioStep {
  return {
    name: `step-${index + 1}`,
    method: 'GET',
    url: '',
    body_tpl: '',
    headers: {},
    extract: {},
  }
}

export default function CreateTaskPage() {
  const navigate = useNavigate()
  const [form, setForm] = useState<TaskForm>(initialForm)
  const [steps, setSteps] = useState<ScenarioStep[]>([newScenarioStep(0)])
  const [formula, setFormula] = useState('')
  const [perAccountCount, setPerAccountCount] = useState(1)
  const [error, setError] = useState('')

  const createMutation = useMutation({
    mutationFn: createTask,
    onSuccess: (task) => navigate(`/tasks/${task.id}`),
    onError: (mutationError) => setError(getErrorMessage(mutationError)),
  })

  function submitTask(event: FormEvent) {
    event.preventDefault()
    if (steps.length === 0) {
      setError('至少需要一个 step')
      return
    }
    if (perAccountCount <= 0) {
      setError('per_account_count must be positive')
      return
    }
    if (form.concurrency <= 0) {
      setError('concurrency must be positive')
      return
    }

    setError('')
    createMutation.mutate({
      module_type: 'load_test',
      name: form.name,
      account_group_id: form.account_group_id,
      concurrency: form.concurrency,
      created_by: form.created_by,
      config: {
        scenario: {
          steps,
          formula,
        },
        per_account_count: perAccountCount,
      },
    })
  }

  return (
    <form className="page-shell" onSubmit={submitTask}>
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h2 className="text-lg font-semibold text-slate-950">新建任务</h2>
          <div className="mt-1 text-sm text-slate-500">load_test</div>
        </div>
        <div className="flex gap-2">
          <button className="btn btn-secondary" type="button" onClick={() => navigate('/')}>
            <ArrowLeft size={16} />
            返回
          </button>
          <button className="btn btn-primary" disabled={createMutation.isPending} type="submit">
            <Send size={16} />
            提交
          </button>
        </div>
      </div>

      <section className="panel">
        <div className="toolbar">
          <h3 className="text-base font-semibold text-slate-950">基础字段</h3>
        </div>
        <div className="grid gap-4 p-4 sm:p-5 lg:grid-cols-2">
          <label className="space-y-1">
            <span className="field-label">name</span>
            <input
              className="input"
              value={form.name}
              onChange={(event) => setForm({ ...form, name: event.target.value })}
            />
          </label>
          <label className="space-y-1">
            <span className="field-label">account_group_id</span>
            <input
              className="input"
              value={form.account_group_id}
              onChange={(event) => setForm({ ...form, account_group_id: event.target.value })}
            />
          </label>
          <label className="space-y-1">
            <span className="field-label">concurrency</span>
            <input
              className="input"
              min={1}
              type="number"
              value={form.concurrency}
              onChange={(event) => setForm({ ...form, concurrency: Number(event.target.value) })}
            />
          </label>
          <label className="space-y-1">
            <span className="field-label">created_by</span>
            <input
              className="input"
              value={form.created_by}
              onChange={(event) => setForm({ ...form, created_by: event.target.value })}
            />
          </label>
        </div>
      </section>

      <section className="space-y-4">
        <div>
          <h3 className="text-base font-semibold text-slate-950">Scenario</h3>
        </div>
        <ScenarioStepEditor
          formula={formula}
          perAccountCount={perAccountCount}
          steps={steps}
          onFormulaChange={setFormula}
          onPerAccountCountChange={setPerAccountCount}
          onStepsChange={setSteps}
        />
      </section>

      {error ? <div className="rounded-md bg-red-50 px-4 py-3 text-sm text-red-700">{error}</div> : null}
    </form>
  )
}
