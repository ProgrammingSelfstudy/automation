import { Plus, Trash2 } from 'lucide-react'

import type { ScenarioStep } from '../api/client'
import KeyValueEditor from './KeyValueEditor'

const METHODS: ScenarioStep['method'][] = ['GET', 'POST', 'PUT', 'DELETE', 'PATCH']

type ScenarioStepEditorProps = {
  steps: ScenarioStep[]
  onStepsChange: (next: ScenarioStep[]) => void
  formula: string
  onFormulaChange: (next: string) => void
  perAccountCount?: number
  onPerAccountCountChange?: (next: number) => void
  showPerAccountCount?: boolean
}

function newStep(index: number): ScenarioStep {
  return {
    name: `step-${index + 1}`,
    method: 'GET',
    url: '',
    body_tpl: '',
    headers: {},
    extract: {},
  }
}

export default function ScenarioStepEditor({
  steps,
  onStepsChange,
  formula,
  onFormulaChange,
  perAccountCount = 1,
  onPerAccountCountChange,
  showPerAccountCount = true,
}: ScenarioStepEditorProps) {
  function updateStep(index: number, patch: Partial<ScenarioStep>) {
    onStepsChange(steps.map((step, stepIndex) => (stepIndex === index ? { ...step, ...patch } : step)))
  }

  function addStep() {
    onStepsChange([...steps, newStep(steps.length)])
  }

  function removeStep(index: number) {
    onStepsChange(steps.filter((_, stepIndex) => stepIndex !== index))
  }

  return (
    <div className="space-y-5">
      <div className={showPerAccountCount ? 'grid gap-4 sm:grid-cols-[minmax(0,1fr)_220px]' : 'grid gap-4'}>
        <label className="space-y-1">
          <span className="field-label">formula</span>
          <input
            className="input"
            value={formula}
            onChange={(event) => onFormulaChange(event.target.value)}
          />
        </label>
        {showPerAccountCount ? (
          <label className="space-y-1">
            <span className="field-label">per_account_count</span>
            <input
              className="input"
              min={1}
              type="number"
              value={perAccountCount}
              onChange={(event) => onPerAccountCountChange?.(Number(event.target.value))}
            />
          </label>
        ) : null}
      </div>

      <div className="space-y-4">
        {steps.map((step, index) => (
          <section className="rounded-lg border border-slate-200 bg-white" key={index}>
            <div className="flex items-center justify-between border-b border-slate-200 px-4 py-3">
              <div className="text-sm font-semibold text-slate-900">Step {index + 1}</div>
              <button
                aria-label="删除 Step"
                className="icon-btn"
                disabled={steps.length <= 1}
                title="删除 Step"
                type="button"
                onClick={() => removeStep(index)}
              >
                <Trash2 size={16} />
              </button>
            </div>

            <div className="grid gap-4 p-4">
              <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_140px_minmax(0,2fr)]">
                <label className="space-y-1">
                  <span className="field-label">name</span>
                  <input
                    className="input"
                    value={step.name}
                    onChange={(event) => updateStep(index, { name: event.target.value })}
                  />
                </label>
                <label className="space-y-1">
                  <span className="field-label">method</span>
                  <select
                    className="input"
                    value={step.method}
                    onChange={(event) =>
                      updateStep(index, { method: event.target.value as ScenarioStep['method'] })
                    }
                  >
                    {METHODS.map((method) => (
                      <option key={method} value={method}>
                        {method}
                      </option>
                    ))}
                  </select>
                </label>
                <label className="space-y-1">
                  <span className="field-label">url</span>
                  <input
                    className="input"
                    value={step.url}
                    onChange={(event) => updateStep(index, { url: event.target.value })}
                  />
                </label>
              </div>

              <label className="space-y-1">
                <span className="field-label">body_tpl</span>
                <textarea
                  className="textarea"
                  value={step.body_tpl}
                  onChange={(event) => updateStep(index, { body_tpl: event.target.value })}
                />
              </label>

              <div className="grid gap-4 lg:grid-cols-2">
                <div className="space-y-2">
                  <div className="field-label">headers</div>
                  <KeyValueEditor
                    keyPlaceholder="header"
                    value={step.headers}
                    onChange={(headers) => updateStep(index, { headers })}
                  />
                </div>
                <div className="space-y-2">
                  <div className="field-label">extract</div>
                  <KeyValueEditor
                    keyPlaceholder="name"
                    valuePlaceholder="gjson path"
                    value={step.extract}
                    onChange={(extract) => updateStep(index, { extract })}
                  />
                </div>
              </div>
            </div>
          </section>
        ))}
      </div>

      <button className="btn btn-secondary" type="button" onClick={addStep}>
        <Plus size={16} />
        添加 Step
      </button>
    </div>
  )
}
