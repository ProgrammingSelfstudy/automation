import { ClipboardPaste, Wand2 } from 'lucide-react'
import { useState } from 'react'

import type { ScenarioStep } from '../api/client'
import { decodeFormBody, encodeFormBody } from '../utils/formBody'
import { parseHeaderText } from '../utils/headers'
import KeyValueEditor from './KeyValueEditor'

const METHODS: ScenarioStep['method'][] = ['GET', 'POST', 'PUT', 'DELETE', 'PATCH']
const FORM_URLENCODED_CONTENT_TYPE = 'application/x-www-form-urlencoded'

type BodyMode = 'raw' | 'form'

// 有 Content-Type: application/x-www-form-urlencoded 头的话，大概率是之前
// 就用键值对模式编辑过的——重新打开这个步骤时默认展示键值对模式，不用每次
// 手动切换。
function detectBodyMode(headers: Record<string, string>): BodyMode {
  const contentType = Object.entries(headers).find(([key]) => key.toLowerCase() === 'content-type')?.[1] ?? ''
  return contentType.toLowerCase().includes(FORM_URLENCODED_CONTENT_TYPE) ? 'form' : 'raw'
}

type StepFieldsProps = {
  step: ScenarioStep
  onChange: (next: ScenarioStep) => void
}

export default function StepFields({ step, onChange }: StepFieldsProps) {
  const [headerImportOpen, setHeaderImportOpen] = useState(false)
  const [headerImportText, setHeaderImportText] = useState('')
  const [bodyFormatError, setBodyFormatError] = useState('')
  const [bodyMode, setBodyMode] = useState<BodyMode>(() => detectBodyMode(step.headers))

  function updateStep(patch: Partial<ScenarioStep>) {
    onChange({ ...step, ...patch })
  }

  function formatBodyJSON() {
    setBodyFormatError('')
    try {
      updateStep({ body_tpl: JSON.stringify(JSON.parse(step.body_tpl), null, 2) })
    } catch {
      setBodyFormatError('不是合法 JSON')
    }
  }

  // 切到键值对模式时顺手把 Content-Type 设好，省得再手动去请求头那边加一遍；
  // 只在点击切换的这一下设置，不在每次编辑键值对时都重复设置，不然会跟用户
  // 自己手动改过的 Content-Type（比如带了 charset）打架。
  function switchToFormMode() {
    setBodyFormatError('')
    setBodyMode('form')
    const hasContentType = Object.keys(step.headers).some((key) => key.toLowerCase() === 'content-type')
    if (!hasContentType) {
      updateStep({ headers: { ...step.headers, 'Content-Type': FORM_URLENCODED_CONTENT_TYPE } })
    }
  }

  function importHeaders() {
    const parsed = parseHeaderText(headerImportText)
    updateStep({ headers: { ...step.headers, ...parsed } })
    setHeaderImportText('')
    setHeaderImportOpen(false)
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

      <div className="space-y-1">
        <div className="flex flex-wrap items-center gap-2">
          <span className="field-label">请求体模板</span>
          <div className="flex overflow-hidden rounded-md ring-1 ring-white/60">
            <button
              className={`h-8 px-2 text-xs font-medium ${bodyMode === 'raw' ? 'bg-blue-600 text-white' : 'bg-white/60 text-slate-600'}`}
              type="button"
              onClick={() => {
                setBodyFormatError('')
                setBodyMode('raw')
              }}
            >
              原始文本
            </button>
            <button
              className={`h-8 px-2 text-xs font-medium ${bodyMode === 'form' ? 'bg-blue-600 text-white' : 'bg-white/60 text-slate-600'}`}
              type="button"
              onClick={switchToFormMode}
            >
              x-www-form-urlencoded
            </button>
          </div>
          {bodyMode === 'raw' ? (
            <button className="btn btn-secondary h-8 px-2" type="button" onClick={formatBodyJSON}>
              <Wand2 size={14} />
              格式化 JSON
            </button>
          ) : null}
          {bodyFormatError ? <span className="text-sm text-red-700">{bodyFormatError}</span> : null}
        </div>
        {bodyMode === 'form' ? (
          <KeyValueEditor
            keyPlaceholder="字段名"
            valuePlaceholder="值，支持 {{.account.xxx}} 变量"
            value={decodeFormBody(step.body_tpl)}
            onChange={(pairs) => updateStep({ body_tpl: encodeFormBody(pairs) })}
          />
        ) : (
          <textarea
            className="textarea"
            value={step.body_tpl}
            onChange={(event) => {
              setBodyFormatError('')
              updateStep({ body_tpl: event.target.value })
            }}
          />
        )}
      </div>

      <div className="grid gap-4 lg:grid-cols-2">
        <div className="space-y-2">
          <div className="flex flex-wrap items-center gap-2">
            <div className="field-label">请求头</div>
            <button
              className="btn btn-secondary h-8 px-2"
              type="button"
              onClick={() => setHeaderImportOpen((value) => !value)}
            >
              <ClipboardPaste size={14} />
              粘贴导入
            </button>
          </div>
          {headerImportOpen ? (
            <div className="space-y-2 rounded-md bg-white/50 p-3 ring-1 ring-white/60">
              <textarea
                className="textarea"
                placeholder={'Content-Type: application/json\nAuthorization: Bearer xxx\n-H "X-Custom: 1"'}
                value={headerImportText}
                onChange={(event) => setHeaderImportText(event.target.value)}
              />
              <div className="flex flex-wrap items-center gap-2">
                <button className="btn btn-primary" type="button" onClick={importHeaders}>
                  确认导入
                </button>
                <button
                  className="btn btn-secondary"
                  type="button"
                  onClick={() => {
                    setHeaderImportText('')
                    setHeaderImportOpen(false)
                  }}
                >
                  取消
                </button>
              </div>
            </div>
          ) : null}
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
