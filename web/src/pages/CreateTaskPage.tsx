import { useMutation, useQuery } from '@tanstack/react-query'
import { ArrowLeft, Send } from 'lucide-react'
import { type FormEvent, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'

import { createAccount, createTask, listScenarios } from '../api/client'
import type { ScenarioStep } from '../api/client'
import ScenarioStepEditor from '../components/ScenarioStepEditor'
import { getErrorMessage } from '../utils/format'
import { cloneScenarioDefinition, newScenarioStep } from '../utils/scenario'

type TaskForm = {
  name: string
  account_group_id: string
  concurrency: number
}

type AccountSource = 'existing' | 'paste'

type ParsedAccount = {
  line: number
  username: string
  password: string
}

const initialForm: TaskForm = {
  name: '',
  account_group_id: '',
  concurrency: 1,
}

function parseBulkAccounts(raw: string): { accounts: ParsedAccount[]; error: string } {
  const accounts: ParsedAccount[] = []
  const lines = raw.split(/\r?\n/)

  for (let index = 0; index < lines.length; index += 1) {
    const line = lines[index]
    if (line.trim() === '') {
      continue
    }

    const commaIndex = line.indexOf(',')
    if (commaIndex < 0) {
      return { accounts, error: `第 ${index + 1} 行格式不正确：缺少逗号` }
    }

    const username = line.slice(0, commaIndex).trim()
    const password = line.slice(commaIndex + 1).trim()
    if (username === '') {
      return { accounts, error: `第 ${index + 1} 行格式不正确：用户名为空` }
    }
    if (password === '') {
      return { accounts, error: `第 ${index + 1} 行格式不正确：密码为空` }
    }
    accounts.push({ line: index + 1, username, password })
  }

  return { accounts, error: '' }
}

// account.group_id is VARCHAR(36) in the database — a generated id longer than
// that fails every bulk-paste task creation with a raw MySQL error surfaced as
// a 500. crypto.randomUUID() is exactly 36 characters, so it fits with zero
// room to spare; do not prefix it with anything.
function newTaskAccountGroupID() {
  if (typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID()
  }
  return `t${Date.now().toString(36)}${Math.random().toString(36).slice(2, 10)}`
}

export default function CreateTaskPage() {
  const navigate = useNavigate()
  const [form, setForm] = useState<TaskForm>(initialForm)
  const [steps, setSteps] = useState<ScenarioStep[]>([newScenarioStep(0)])
  const [formula, setFormula] = useState('')
  const [perAccountCount, setPerAccountCount] = useState(1)
  const [selectedScenarioID, setSelectedScenarioID] = useState('')
  const [accountSource, setAccountSource] = useState<AccountSource>('existing')
  const [bulkAccountsText, setBulkAccountsText] = useState('')
  const [isCreatingAccounts, setIsCreatingAccounts] = useState(false)
  const [error, setError] = useState('')
  const parsedAccounts = useMemo(() => parseBulkAccounts(bulkAccountsText), [bulkAccountsText])

  const scenariosQuery = useQuery({
    queryKey: ['scenarios'],
    queryFn: listScenarios,
  })

  const createMutation = useMutation({
    mutationFn: createTask,
    onSuccess: (task) => navigate(`/tasks/${task.id}`),
  })

  async function submitTask(event: FormEvent) {
    event.preventDefault()
    if (steps.length === 0) {
      setError('至少需要一个步骤')
      return
    }
    if (perAccountCount <= 0) {
      setError('每账号执行次数必须大于 0')
      return
    }
    if (form.concurrency <= 0) {
      setError('并发数必须大于 0')
      return
    }

    let accountGroupID = form.account_group_id.trim()
    if (accountSource === 'existing') {
      if (accountGroupID === '') {
        setError('账号组 ID 必填')
        return
      }
    } else {
      if (parsedAccounts.error) {
        setError(parsedAccounts.error)
        return
      }
      if (parsedAccounts.accounts.length === 0) {
        setError('请至少粘贴一个账号')
        return
      }
      accountGroupID = newTaskAccountGroupID()
      setError('')
      setIsCreatingAccounts(true)
      try {
        for (let index = 0; index < parsedAccounts.accounts.length; index += 1) {
          const account = parsedAccounts.accounts[index]
          try {
            await createAccount({
              group_id: accountGroupID,
              username: account.username,
              password: account.password,
              enabled: true,
            })
          } catch (caught) {
            setError(
              `第 ${account.line} 行账号（${account.username}）创建失败：${getErrorMessage(caught)}。已成功创建 ${index} 个账号，账号组 ID：${accountGroupID}`,
            )
            return
          }
        }
      } finally {
        setIsCreatingAccounts(false)
      }
    }

    setError('')
    try {
      await createMutation.mutateAsync({
        module_type: 'load_test',
        name: form.name,
        account_group_id: accountGroupID,
        concurrency: form.concurrency,
        config: {
          scenario: {
            steps,
            formula,
          },
          per_account_count: perAccountCount,
        },
      })
    } catch (caught) {
      const message = getErrorMessage(caught)
      if (accountSource === 'paste') {
        setError(`账号已创建到账号组 ${accountGroupID}，但任务创建失败：${message}`)
        return
      }
      setError(message)
    }
  }

  function loadSavedScenario(scenarioID: string) {
    setSelectedScenarioID(scenarioID)
    const saved = scenariosQuery.data?.find((item) => item.id === scenarioID)
    if (!saved) {
      return
    }

    const definition = cloneScenarioDefinition(saved.definition)
    setSteps(definition.steps.length > 0 ? definition.steps : [newScenarioStep(0)])
    setFormula(definition.formula)
  }

  return (
    <form className="page-shell" onSubmit={submitTask}>
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h2 className="text-lg font-semibold text-slate-950">新建任务</h2>
          <div className="mt-1 text-sm text-slate-500">接口压测</div>
        </div>
        <div className="flex gap-2">
          <button className="btn btn-secondary" type="button" onClick={() => navigate('/runs')}>
            <ArrowLeft size={16} />
            返回
          </button>
          <button className="btn btn-primary" disabled={createMutation.isPending || isCreatingAccounts} type="submit">
            <Send size={16} />
            {isCreatingAccounts ? '创建账号中' : '提交'}
          </button>
        </div>
      </div>

      <section className="panel">
        <div className="toolbar">
          <h3 className="text-base font-semibold text-slate-950">基础字段</h3>
        </div>
        <div className="grid gap-4 p-4 sm:p-5 lg:grid-cols-2">
          <label className="space-y-1">
            <span className="field-label">任务名称</span>
            <input
              className="input"
              value={form.name}
              onChange={(event) => setForm({ ...form, name: event.target.value })}
            />
          </label>
          <label className="space-y-1">
            <span className="field-label">并发数</span>
            <input
              className="input"
              min={1}
              type="number"
              value={form.concurrency}
              onChange={(event) => setForm({ ...form, concurrency: Number(event.target.value) })}
            />
          </label>
        </div>
      </section>

      <section className="panel">
        <div className="toolbar">
          <h3 className="text-base font-semibold text-slate-950">账号来源</h3>
          <div className="inline-flex rounded-md border border-slate-300 bg-white p-1">
            <button
              className={`rounded px-3 py-1.5 text-sm font-medium transition ${
                accountSource === 'existing' ? 'bg-slate-900 text-white' : 'text-slate-600 hover:bg-slate-50'
              }`}
              type="button"
              onClick={() => setAccountSource('existing')}
            >
              已有账号组
            </button>
            <button
              className={`rounded px-3 py-1.5 text-sm font-medium transition ${
                accountSource === 'paste' ? 'bg-slate-900 text-white' : 'text-slate-600 hover:bg-slate-50'
              }`}
              type="button"
              onClick={() => setAccountSource('paste')}
            >
              批量粘贴新建
            </button>
          </div>
        </div>
        <div className="space-y-4 p-4 sm:p-5">
          {accountSource === 'existing' ? (
            <label className="block max-w-2xl space-y-1">
              <span className="field-label">账号组 ID</span>
              <input
                className="input"
                value={form.account_group_id}
                onChange={(event) => setForm({ ...form, account_group_id: event.target.value })}
              />
            </label>
          ) : (
            <>
              <label className="block space-y-1">
                <span className="field-label">账号文本</span>
                <textarea
                  className="textarea min-h-40"
                  placeholder={'用户名,密码\nalice,secret\nbob,p@ss,with,comma'}
                  value={bulkAccountsText}
                  onChange={(event) => setBulkAccountsText(event.target.value)}
                />
              </label>
              <div
                className={`rounded-md px-4 py-3 text-sm ${
                  parsedAccounts.error ? 'bg-red-50 text-red-700' : 'bg-blue-50 text-blue-700'
                }`}
              >
                {parsedAccounts.error || `解析到 ${parsedAccounts.accounts.length} 个账号`}
              </div>
            </>
          )}
        </div>
      </section>

      <section className="space-y-4">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <h3 className="text-base font-semibold text-slate-950">执行场景</h3>
          </div>
          <select
            className="input sm:w-80"
            disabled={scenariosQuery.isLoading || (scenariosQuery.data ?? []).length === 0}
            value={selectedScenarioID}
            onChange={(event) => loadSavedScenario(event.target.value)}
          >
            <option value="">从已保存场景加载</option>
            {(scenariosQuery.data ?? []).map((saved) => (
              <option key={saved.id} value={saved.id}>
                {saved.name}
              </option>
            ))}
          </select>
        </div>
        {scenariosQuery.isError ? (
          <div className="rounded-md bg-amber-50 px-4 py-3 text-sm text-amber-800">
            {getErrorMessage(scenariosQuery.error)}
          </div>
        ) : null}
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
