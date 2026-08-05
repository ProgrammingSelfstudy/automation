import { Plus, Trash2 } from 'lucide-react'

import type { ScenarioStep } from '../api/client'
import StepFields from './StepFields'

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
    name: `步骤-${index + 1}`,
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
          <span className="field-label">公式</span>
          <input
            className="input"
            value={formula}
            onChange={(event) => onFormulaChange(event.target.value)}
          />
        </label>
        {showPerAccountCount ? (
          <label className="space-y-1">
            <span className="field-label">每账号执行次数</span>
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
              <div className="text-sm font-semibold text-slate-900">步骤 {index + 1}</div>
              <button
                aria-label="删除步骤"
                className="icon-btn"
                disabled={steps.length <= 1}
                title="删除步骤"
                type="button"
                onClick={() => removeStep(index)}
              >
                <Trash2 size={16} />
              </button>
            </div>

            <div className="p-4">
              <StepFields step={step} onChange={(next) => updateStep(index, next)} />
            </div>
          </section>
        ))}
      </div>

      <button className="btn btn-secondary" type="button" onClick={addStep}>
        <Plus size={16} />
        添加步骤
      </button>
    </div>
  )
}
