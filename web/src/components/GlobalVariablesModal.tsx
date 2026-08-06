import { Check, X } from 'lucide-react'
import { type FormEvent, useEffect, useState } from 'react'

import {
  loadGlobalHeaders,
  loadGlobalSignKey,
  loadGlobalToken,
  saveGlobalHeaders,
  saveGlobalSignKey,
  saveGlobalToken,
} from '../utils/globalVariables'
import KeyValueEditor from './KeyValueEditor'

type GlobalVariablesModalProps = {
  open: boolean
  onClose: () => void
}

export default function GlobalVariablesModal({ open, onClose }: GlobalVariablesModalProps) {
  const [token, setToken] = useState('')
  const [signKey, setSignKey] = useState('')
  const [headers, setHeaders] = useState<Record<string, string>>({})
  const [saved, setSaved] = useState(false)

  useEffect(() => {
    if (!open) {
      return
    }
    setToken(loadGlobalToken())
    setSignKey(loadGlobalSignKey())
    setHeaders(loadGlobalHeaders())
    setSaved(false)
  }, [open])

  function submit(event: FormEvent) {
    event.preventDefault()
    saveGlobalToken(token)
    saveGlobalSignKey(signKey)
    saveGlobalHeaders(headers)
    setSaved(true)
  }

  return (
    <div className={`fixed inset-0 z-50 flex items-center justify-center px-4 py-6 ${open ? 'pointer-events-auto' : 'pointer-events-none'}`}>
      <button
        aria-label="关闭全局变量"
        className={`absolute inset-0 bg-slate-950/40 transition-opacity duration-300 ${open ? 'opacity-100' : 'opacity-0'}`}
        type="button"
        onClick={onClose}
      />
      <section
        aria-modal="true"
        className={`glass-panel relative flex max-h-[85vh] w-full max-w-xl flex-col shadow-2xl transition-all duration-300 ${
          open ? 'translate-y-0 opacity-100' : 'translate-y-2 opacity-0'
        }`}
        role="dialog"
      >
        <div className="flex items-center justify-between border-b border-white/50 px-5 py-4">
          <div>
            <h2 className="text-base font-semibold text-slate-950">全局变量</h2>
            <div className="mt-1 text-sm text-slate-500">只存在这台浏览器本地（localStorage），不会上传到服务端</div>
          </div>
          <button aria-label="关闭" className="icon-btn" type="button" onClick={onClose}>
            <X size={16} />
          </button>
        </div>

        <form className="min-h-0 flex-1 space-y-4 overflow-y-auto p-5" onSubmit={submit}>
          <label className="block space-y-1">
            <span className="field-label">Token</span>
            <input
              autoComplete="off"
              className="input"
              placeholder="例如登录接口返回的 accessToken，接口里用 {{.global.TOKEN}} 引用"
              type="password"
              value={token}
              onChange={(event) => {
                setToken(event.target.value)
                setSaved(false)
              }}
            />
          </label>
          <label className="block space-y-1">
            <span className="field-label">密钥</span>
            <input
              autoComplete="off"
              className="input"
              placeholder="签名密钥，接口详情里的“签名密钥”框会用这里保存的值预填"
              type="password"
              value={signKey}
              onChange={(event) => {
                setSignKey(event.target.value)
                setSaved(false)
              }}
            />
          </label>

          <section className="space-y-2">
            <h3 className="field-label">全局请求头</h3>
            <div className="text-xs text-slate-500">
              每个接口发送时都会带上，比如 Authorization/deviceId/os/X-Lang；接口自己配了同名请求头会覆盖这里的值
            </div>
            <KeyValueEditor
              keyPlaceholder="请求头名"
              value={headers}
              onChange={(next) => {
                setHeaders(next)
                setSaved(false)
              }}
            />
          </section>

          {saved ? (
            <div className="flex items-center gap-2 rounded-md bg-emerald-50 px-4 py-3 text-sm text-emerald-700">
              <Check size={16} />
              已保存到本地
            </div>
          ) : null}

          <button className="btn btn-primary w-full" type="submit">
            保存
          </button>
        </form>
      </section>
    </div>
  )
}
