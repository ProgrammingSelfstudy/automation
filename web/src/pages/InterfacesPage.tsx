import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Pencil, Plus, RefreshCw, Save, Send } from 'lucide-react'
import { type FormEvent, type ReactNode, useState } from 'react'

import { createInterface, listEnvironments, listInterfaces, trySendInterface, updateInterface } from '../api/client'
import type { EnvironmentResponse, InterfaceResponse, ScenarioStep, TrySendInterfaceResponse } from '../api/client'
import SlideOver from '../components/SlideOver'
import StepFields from '../components/StepFields'
import { formatDateTime, formatNumber, getErrorMessage } from '../utils/format'
import { loadGlobalHeaders, loadGlobalSignKey, loadGlobalToken } from '../utils/globalVariables'
import { newScenarioStep } from '../utils/scenario'
import { computeAppServerSign, parseFormBody, randomNonce } from '../utils/sign'

export default function InterfacesPage() {
  const queryClient = useQueryClient()
  const [name, setName] = useState('')
  const [step, setStep] = useState<ScenarioStep>(() => newScenarioStep(0))
  const [createOpen, setCreateOpen] = useState(false)
  const [selectedInterface, setSelectedInterface] = useState<InterfaceResponse | null>(null)
  const [detailEditing, setDetailEditing] = useState(false)
  const [editName, setEditName] = useState('')
  const [editStep, setEditStep] = useState<ScenarioStep>(() => newScenarioStep(0))
  const [editError, setEditError] = useState('')
  const [previewEnvironmentID, setPreviewEnvironmentID] = useState('')
  const [signSecretKey, setSignSecretKey] = useState('')
  const [trySendResult, setTrySendResult] = useState<TrySendInterfaceResponse | null>(null)
  const [trySendError, setTrySendError] = useState('')
  const [error, setError] = useState('')

  const interfacesQuery = useQuery({
    queryKey: ['interfaces'],
    queryFn: listInterfaces,
  })

  const environmentsQuery = useQuery({
    queryKey: ['environments'],
    queryFn: listEnvironments,
  })

  const createMutation = useMutation({
    mutationFn: createInterface,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['interfaces'] })
      setName('')
      setStep(newScenarioStep(0))
      setError('')
      setCreateOpen(false)
    },
    onError: (mutationError) => setError(getErrorMessage(mutationError)),
  })

  const updateMutation = useMutation({
    mutationFn: ({ id, name, step }: { id: string; name: string; step: ScenarioStep }) =>
      updateInterface(id, { name, step }),
    onSuccess: async (updated) => {
      await queryClient.invalidateQueries({ queryKey: ['interfaces'] })
      setSelectedInterface((current) => ({
        ...updated,
        created_at: current?.created_at || updated.created_at,
      }))
      setEditError('')
      setDetailEditing(false)
    },
    onError: (mutationError) => setEditError(getErrorMessage(mutationError)),
  })

  const trySendMutation = useMutation({
    mutationFn: ({ step, environmentID }: { step: ScenarioStep; environmentID: string }) =>
      trySendInterface({
        step,
        environment_id: environmentID || undefined,
      }),
    onSuccess: (result) => {
      setTrySendResult(result)
      setTrySendError('')
    },
    onError: (mutationError) => {
      setTrySendResult(null)
      setTrySendError(getErrorMessage(mutationError))
    },
  })

  function submitInterface(event: FormEvent) {
    event.preventDefault()
    setError('')
    createMutation.mutate({ name, step })
  }

  function submitInterfaceUpdate(event: FormEvent) {
    event.preventDefault()
    if (!selectedInterface) {
      return
    }
    setEditError('')
    updateMutation.mutate({
      id: selectedInterface.id,
      name: editName,
      step: editStep,
    })
  }

  function openInterfaceDetail(item: InterfaceResponse) {
    setSelectedInterface(item)
    setDetailEditing(false)
    setEditName(item.name)
    setEditStep(item.step)
    setEditError('')
    setSignSecretKey(loadGlobalSignKey())
    setTrySendResult(null)
    setTrySendError('')
  }

  function closeInterfaceDetail() {
    setSelectedInterface(null)
    setDetailEditing(false)
    setEditError('')
    setSignSecretKey('')
    setTrySendResult(null)
    setTrySendError('')
  }

  function startInterfaceEdit() {
    if (!selectedInterface) {
      return
    }
    setEditName(selectedInterface.name)
    setEditStep(selectedInterface.step)
    setEditError('')
    setTrySendResult(null)
    setTrySendError('')
    setDetailEditing(true)
  }

  function changePreviewEnvironment(id: string) {
    setPreviewEnvironmentID(id)
    setTrySendResult(null)
    setTrySendError('')
  }

  function sendInterfaceStep(stepToSend: ScenarioStep) {
    setTrySendResult(null)
    setTrySendError('')

    // {{.global.X}} only exists on the client (the backend template engine has
    // no "global" scope), so it has to be resolved here before sending no
    // matter what. {{.env.X}} is deliberately left untouched — the backend
    // still resolves those itself via environment_id, same as before.
    // Global headers (Authorization/deviceId/os/... shared across every
    // interface) apply as a base layer — the interface's own headers win on
    // key collision, same override rule as environment default headers.
    const globalOnlyScopes: TemplateScopes = { env: {}, global: globalScope() }
    let stepPayload: ScenarioStep = {
      ...stepToSend,
      url: resolveEnvRefs(stepToSend.url, globalOnlyScopes),
      body_tpl: resolveEnvRefs(stepToSend.body_tpl, globalOnlyScopes),
      headers: resolveEnvRecord({ ...loadGlobalHeaders(), ...stepToSend.headers }, globalOnlyScopes),
    }

    // X-Timestamp/X-Nonce are always generated fresh at send time, signing
    // key or not — plenty of APIs want these headers without a full request
    // signature. Generated once here so the signing block below (if it runs)
    // signs the exact same values that are actually sent.
    const timestamp = Date.now().toString()
    const nonce = randomNonce()
    stepPayload = {
      ...stepPayload,
      headers: { ...stepPayload.headers, 'X-Timestamp': timestamp, 'X-Nonce': nonce },
    }

    // Authorization is unconditionally overwritten with the cached global
    // token on every send when a token is set — deliberately chosen this way
    // even though it means an interface that needs its own Authorization
    // (e.g. Basic client-id/secret on a login endpoint) gets clobbered while
    // a global token is configured; clear the Token field in "全局变量" before
    // testing those.
    const globalToken = loadGlobalToken()
    if (globalToken !== '') {
      stepPayload = {
        ...stepPayload,
        headers: { ...stepPayload.headers, Authorization: `Bearer ${globalToken}` },
      }
    }

    if (signSecretKey.trim() !== '') {
      // Legacy app-server device signature: the secret key only ever lives in
      // this component's state (never persisted, never sent to our backend).
      // We compute X-Sign here and send it as a literal header value,
      // matching AppServerSigner.sign() on the target service.
      const envVariables = environments.find((env) => env.id === previewEnvironmentID)?.variables ?? {}
      const resolvedBody = resolveEnvRefs(stepPayload.body_tpl, { env: envVariables, global: globalScope() })
      const sign = computeAppServerSign(parseFormBody(resolvedBody), nonce, timestamp, signSecretKey)
      stepPayload = {
        ...stepPayload,
        headers: {
          ...stepPayload.headers,
          'X-Sign': sign,
        },
      }
    }

    trySendMutation.mutate({
      step: stepPayload,
      environmentID: previewEnvironmentID,
    })
  }

  const interfaces = interfacesQuery.data ?? []
  const environments = environmentsQuery.data ?? []

  return (
    <div className="page-shell">
      <section className="panel">
        <div className="toolbar">
          <div>
            <h2 className="text-base font-semibold text-slate-950">接口库</h2>
            <div className="mt-1 text-sm text-slate-500">{interfaces.length} 个接口</div>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <button className="btn btn-secondary" type="button" onClick={() => interfacesQuery.refetch()}>
              <RefreshCw size={16} />
              刷新
            </button>
            <button className="btn btn-primary" type="button" onClick={() => setCreateOpen(true)}>
              <Plus size={16} />
              新建接口
            </button>
          </div>
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
                    <tr
                      className="cursor-pointer transition hover:bg-blue-50/70"
                      key={item.id}
                      onClick={() => openInterfaceDetail(item)}
                    >
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

      <SlideOver open={createOpen} title="新建接口" onClose={() => setCreateOpen(false)}>
        <form className="space-y-5" onSubmit={submitInterface}>
          <label className="block space-y-1">
            <span className="field-label">接口名称</span>
            <input className="input" value={name} onChange={(event) => setName(event.target.value)} />
          </label>
          <StepFields step={step} onChange={setStep} />

          {error ? <div className="rounded-md bg-red-50 px-4 py-3 text-sm text-red-700">{error}</div> : null}

          <button className="btn btn-primary w-full" disabled={createMutation.isPending} type="submit">
            <Save size={16} />
            保存
          </button>
        </form>
      </SlideOver>

      <SlideOver
        open={selectedInterface !== null}
        title={detailEditing ? '编辑接口' : selectedInterface?.name || '接口详情'}
        onClose={closeInterfaceDetail}
      >
        {selectedInterface ? (
          detailEditing ? (
            <form className="space-y-5" onSubmit={submitInterfaceUpdate}>
              <InterfaceDetailActions
                environments={environments}
                isSending={trySendMutation.isPending}
                selectedEnvironmentID={previewEnvironmentID}
                signSecretKey={signSecretKey}
                onEnvironmentChange={changePreviewEnvironment}
                onSend={() => sendInterfaceStep(editStep)}
                onSignSecretKeyChange={setSignSecretKey}
              />

              <label className="block space-y-1">
                <span className="field-label">接口名称</span>
                <input className="input" value={editName} onChange={(event) => setEditName(event.target.value)} />
              </label>
              <StepFields step={editStep} onChange={setEditStep} />

              <TrySendResultPanel error={trySendError} result={trySendResult} />

              {editError ? <div className="rounded-md bg-red-50 px-4 py-3 text-sm text-red-700">{editError}</div> : null}

              <div className="flex flex-wrap items-center gap-2">
                <button className="btn btn-primary" disabled={updateMutation.isPending} type="submit">
                  <Save size={16} />
                  保存
                </button>
                <button className="btn btn-secondary" type="button" onClick={() => setDetailEditing(false)}>
                  取消
                </button>
              </div>
            </form>
          ) : (
            <InterfaceDetail
              environments={environments}
              isSending={trySendMutation.isPending}
              item={selectedInterface}
              selectedEnvironmentID={previewEnvironmentID}
              signSecretKey={signSecretKey}
              trySendError={trySendError}
              trySendResult={trySendResult}
              onEdit={startInterfaceEdit}
              onEnvironmentChange={changePreviewEnvironment}
              onSend={() => sendInterfaceStep(selectedInterface.step)}
              onSignSecretKeyChange={setSignSecretKey}
            />
          )
        ) : null}
      </SlideOver>
    </div>
  )
}

function InterfaceDetail({
  item,
  environments,
  isSending,
  selectedEnvironmentID,
  signSecretKey,
  trySendError,
  trySendResult,
  onEnvironmentChange,
  onEdit,
  onSend,
  onSignSecretKeyChange,
}: {
  item: InterfaceResponse
  environments: EnvironmentResponse[]
  isSending: boolean
  selectedEnvironmentID: string
  signSecretKey: string
  trySendError: string
  trySendResult: TrySendInterfaceResponse | null
  onEnvironmentChange: (id: string) => void
  onEdit: () => void
  onSend: () => void
  onSignSecretKeyChange: (value: string) => void
}) {
  const previewEnvironment = environments.find((env) => env.id === selectedEnvironmentID)
  const previewScopes: TemplateScopes = { env: previewEnvironment?.variables ?? {}, global: globalScope() }
  const previewStep = {
    ...item.step,
    url: resolveRequestURL(resolveEnvRefs(item.step.url, previewScopes), previewEnvironment?.variables.BASE_URL ?? ''),
    body_tpl: resolveEnvRefs(item.step.body_tpl, previewScopes),
    headers: resolveEnvRecord({ ...loadGlobalHeaders(), ...item.step.headers }, previewScopes),
  }

  return (
    <div className="space-y-5">
      <InterfaceDetailActions
        environments={environments}
        isSending={isSending}
        selectedEnvironmentID={selectedEnvironmentID}
        signSecretKey={signSecretKey}
        onEdit={onEdit}
        onEnvironmentChange={onEnvironmentChange}
        onSend={onSend}
        onSignSecretKeyChange={onSignSecretKeyChange}
      />

      <DetailBlock label="名称">
        <div className="break-words text-sm font-semibold text-slate-950">{item.name}</div>
      </DetailBlock>

      <DetailBlock label="方式">
        <MethodBadge method={item.step.method} />
      </DetailBlock>

      <DetailBlock label="URL">
        <div className="break-all rounded-md bg-slate-50 px-3 py-2 font-mono text-sm text-slate-800 ring-1 ring-slate-200">
          {previewStep.url || '-'}
        </div>
      </DetailBlock>

      <DetailBlock label="请求体模板">
        <pre className="max-h-72 overflow-auto whitespace-pre-wrap break-words rounded-md bg-slate-950 px-3 py-3 text-sm text-slate-100">
          <code>{previewStep.body_tpl || '-'}</code>
        </pre>
      </DetailBlock>

      <DetailBlock label="请求头">
        <KeyValueList values={previewStep.headers} />
      </DetailBlock>

      <DetailBlock label="提取变量">
        <KeyValueList values={item.step.extract} />
      </DetailBlock>

      <DetailBlock label="创建时间">
        <div className="text-sm text-slate-700">{formatDateTime(item.created_at)}</div>
      </DetailBlock>

      <TrySendResultPanel error={trySendError} result={trySendResult} />
    </div>
  )
}

function InterfaceDetailActions({
  environments,
  isSending,
  selectedEnvironmentID,
  signSecretKey,
  onEnvironmentChange,
  onEdit,
  onSend,
  onSignSecretKeyChange,
}: {
  environments: EnvironmentResponse[]
  isSending: boolean
  selectedEnvironmentID: string
  signSecretKey: string
  onEnvironmentChange: (id: string) => void
  onEdit?: () => void
  onSend: () => void
  onSignSecretKeyChange: (value: string) => void
}) {
  return (
    <div className="flex flex-col gap-2 sm:flex-row sm:flex-wrap sm:items-end sm:justify-between">
      <div className="flex min-w-0 flex-1 flex-col gap-2 sm:flex-row">
        <label className="block min-w-0 flex-1 space-y-1">
          <span className="field-label">预览环境</span>
          <select
            className="input"
            value={selectedEnvironmentID}
            onChange={(event) => onEnvironmentChange(event.target.value)}
          >
            <option value="">不使用环境</option>
            {environments.map((env) => (
              <option key={env.id} value={env.id}>
                {env.name}
              </option>
            ))}
          </select>
        </label>
        <label className="block min-w-0 flex-1 space-y-1">
          <span className="field-label">签名密钥</span>
          <input
            autoComplete="off"
            className="input"
            placeholder="仅本地计算 X-Sign，不会发送或保存"
            type="password"
            value={signSecretKey}
            onChange={(event) => onSignSecretKeyChange(event.target.value)}
          />
        </label>
      </div>
      <div className="flex shrink-0 flex-wrap items-center gap-2">
        {onEdit ? (
          <button className="btn btn-secondary" type="button" onClick={onEdit}>
            <Pencil size={16} />
            编辑
          </button>
        ) : null}
        <button className="btn btn-primary" disabled={isSending} type="button" onClick={onSend}>
          <Send size={16} />
          {isSending ? '发送中' : '发送'}
        </button>
      </div>
    </div>
  )
}

function DetailBlock({ label, children }: { label: string; children: ReactNode }) {
  return (
    <section className="space-y-2">
      <h3 className="field-label">{label}</h3>
      {children}
    </section>
  )
}

function MethodBadge({ method }: { method: string }) {
  return (
    <span className="inline-flex h-6 items-center rounded bg-blue-50 px-2 text-xs font-semibold text-blue-700 ring-1 ring-blue-200">
      {method}
    </span>
  )
}

function HTTPStatusBadge({ code }: { code: number }) {
  const colorClass =
    code >= 200 && code < 300
      ? 'bg-emerald-50 text-emerald-700 ring-emerald-200'
      : code >= 300 && code < 400
        ? 'bg-blue-50 text-blue-700 ring-blue-200'
        : code >= 400 && code < 500
          ? 'bg-amber-50 text-amber-700 ring-amber-200'
          : 'bg-red-50 text-red-700 ring-red-200'

  return (
    <span className={`inline-flex h-6 items-center rounded px-2 text-xs font-semibold ring-1 ${colorClass}`}>
      {code || '-'}
    </span>
  )
}

function KeyValueList({ values }: { values: Record<string, string> }) {
  const entries = Object.entries(values)

  if (entries.length === 0) {
    return <div className="rounded-md bg-slate-50 px-3 py-2 text-sm text-slate-500 ring-1 ring-slate-200">-</div>
  }

  return (
    <div className="divide-y divide-slate-200 rounded-md bg-slate-50 ring-1 ring-slate-200">
      {entries.map(([key, value]) => (
        <div className="grid gap-1 px-3 py-2 text-sm sm:grid-cols-[minmax(0,160px)_minmax(0,1fr)]" key={key}>
          <div className="break-all font-mono font-semibold text-slate-800">{key}</div>
          <div className="break-all font-mono text-slate-700">{value}</div>
        </div>
      ))}
    </div>
  )
}

function TrySendResultPanel({ error, result }: { error: string; result: TrySendInterfaceResponse | null }) {
  if (!error && !result) {
    return null
  }

  const hasRequest =
    result !== null &&
    (result.request.method !== '' ||
      result.request.url !== '' ||
      result.request.body !== '' ||
      Object.keys(result.request.headers).length > 0)
  const hasResponse = result !== null && !result.error && result.response.status_code > 0

  return (
    <div className="space-y-5 border-t border-white/60 pt-5">
      {error ? <div className="rounded-md bg-red-50 px-4 py-3 text-sm text-red-700">{error}</div> : null}

      {hasRequest && result ? (
        <DetailBlock label="请求信息">
          <div className="space-y-3">
            <div className="flex flex-wrap items-center gap-2">
              <MethodBadge method={result.request.method} />
              <span className="break-all font-mono text-sm text-slate-800">{result.request.url || '-'}</span>
            </div>
            <KeyValueList values={result.request.headers} />
            <pre className="max-h-72 overflow-auto whitespace-pre-wrap break-words rounded-md bg-slate-950 px-3 py-3 text-sm text-slate-100">
              <code>{result.request.body || '-'}</code>
            </pre>
          </div>
        </DetailBlock>
      ) : null}

      {result?.error ? (
        <div className="rounded-md bg-red-50 px-4 py-3 text-sm text-red-700">{result.error}</div>
      ) : null}

      {hasResponse && result ? (
        <DetailBlock label="响应结果">
          <div className="space-y-3">
            <div className="flex flex-wrap items-center gap-3 text-sm text-slate-700">
              <HTTPStatusBadge code={result.response.status_code} />
              <span>{formatNumber(result.response.cost_ms)} 毫秒</span>
              {result.response.truncated ? <span className="text-amber-700">响应体已截断到 1MB</span> : null}
            </div>
            <pre className="max-h-96 overflow-auto whitespace-pre-wrap break-words rounded-md bg-slate-950 px-3 py-3 text-sm text-slate-100">
              <code>{result.response.body || '-'}</code>
            </pre>
          </div>
        </DetailBlock>
      ) : null}
    </div>
  )
}

type TemplateScopes = {
  env: Record<string, string>
  global: Record<string, string>
}

function globalScope(): Record<string, string> {
  return { TOKEN: loadGlobalToken() }
}

// resolveEnvRefs is a client-side preview/pre-send resolver — it only
// understands the plain {{.env.KEY}}/{{.global.KEY}} data-lookup form, not
// the full Go text/template syntax the backend renders (function calls like
// {{ sortedFormParams ... }} are left untouched and resolved server-side).
function resolveEnvRefs(text: string, scopes: TemplateScopes) {
  return text.replace(/\{\{\s*\.(env|global)\.(\w+)\s*\}\}/g, (match, scope: 'env' | 'global', key: string) => {
    const source = scopes[scope]
    return key in source ? source[key] : match
  })
}

function resolveEnvRecord(values: Record<string, string>, scopes: TemplateScopes) {
  return Object.fromEntries(Object.entries(values).map(([key, value]) => [key, resolveEnvRefs(value, scopes)]))
}

// resolveRequestURL mirrors the backend's scenario.ResolveURL — lets an
// interface be authored as just "/oauth/token" and pick up the selected
// environment's BASE_URL automatically instead of every interface having to
// spell out {{.env.BASE_URL}}. Only a preview convenience: the actual send
// goes through the backend, which applies the same rule server-side.
function resolveRequestURL(url: string, baseURL: string) {
  if (url.startsWith('http://') || url.startsWith('https://') || baseURL === '') {
    return url
  }
  return `${baseURL.replace(/\/+$/, '')}/${url.replace(/^\/+/, '')}`
}
