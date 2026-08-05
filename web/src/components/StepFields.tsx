import type { ScenarioStep } from '../api/client'
import KeyValueEditor from './KeyValueEditor'

const METHODS: ScenarioStep['method'][] = ['GET', 'POST', 'PUT', 'DELETE', 'PATCH']

type StepFieldsProps = {
  step: ScenarioStep
  onChange: (next: ScenarioStep) => void
}

export default function StepFields({ step, onChange }: StepFieldsProps) {
  function updateStep(patch: Partial<ScenarioStep>) {
    onChange({ ...step, ...patch })
  }

  return (
    <div className="grid gap-4">
      <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_140px_minmax(0,2fr)]">
        <label className="space-y-1">
          <span className="field-label">步骤名称</span>
          <input className="input" value={step.name} onChange={(event) => updateStep({ name: event.target.value })} />
        </label>
        <label className="space-y-1">
          <span className="field-label">请求方法</span>
          <select
            className="input"
            value={step.method}
            onChange={(event) => updateStep({ method: event.target.value as ScenarioStep['method'] })}
          >
            {METHODS.map((method) => (
              <option key={method} value={method}>
                {method}
              </option>
            ))}
          </select>
        </label>
        <label className="space-y-1">
          <span className="field-label">URL</span>
          <input className="input" value={step.url} onChange={(event) => updateStep({ url: event.target.value })} />
        </label>
      </div>

      <label className="space-y-1">
        <span className="field-label">请求体模板</span>
        <textarea
          className="textarea"
          value={step.body_tpl}
          onChange={(event) => updateStep({ body_tpl: event.target.value })}
        />
      </label>

      <div className="grid gap-4 lg:grid-cols-2">
        <div className="space-y-2">
          <div className="field-label">请求头</div>
          <KeyValueEditor
            keyPlaceholder="请求头名"
            value={step.headers}
            onChange={(headers) => updateStep({ headers })}
          />
        </div>
        <div className="space-y-2">
          <div className="field-label">提取变量</div>
          <KeyValueEditor
            keyPlaceholder="变量名"
            valuePlaceholder="GJSON 路径"
            value={step.extract}
            onChange={(extract) => updateStep({ extract })}
          />
        </div>
      </div>
    </div>
  )
}
