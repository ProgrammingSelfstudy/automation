import { useQueryClient } from '@tanstack/react-query'
import { LogIn, ShieldCheck } from 'lucide-react'
import { type FormEvent, useState } from 'react'
import { useNavigate } from 'react-router-dom'

import { ApiError, login } from '../api/client'
import { getErrorMessage } from '../utils/format'

type LoginStep = 'credentials' | 'verify-totp'

export default function LoginPage() {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [step, setStep] = useState<LoginStep>('credentials')
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [code, setCode] = useState('')
  const [backupCode, setBackupCode] = useState('')
  const [useBackupCode, setUseBackupCode] = useState(false)
  const [error, setError] = useState('')
  const [isSubmitting, setIsSubmitting] = useState(false)

  async function submitCredentials(event: FormEvent) {
    event.preventDefault()
    setError('')
    setIsSubmitting(true)
    try {
      await login({ username, password })
      await queryClient.invalidateQueries({ queryKey: ['auth', 'me'] })
      navigate('/')
    } catch (caught) {
      if (caught instanceof ApiError && caught.message === 'totp_setup_required') {
        setError('该账号尚未配置双因素认证，请联系管理员创建')
      } else if (caught instanceof ApiError && caught.message === 'totp_code_required') {
        setStep('verify-totp')
      } else {
        setError(getErrorMessage(caught))
      }
    } finally {
      setIsSubmitting(false)
    }
  }

  async function submitVerification(event: FormEvent) {
    event.preventDefault()
    setError('')
    setIsSubmitting(true)
    try {
      await login({
        username,
        password,
        code: useBackupCode ? undefined : code,
        backup_code: useBackupCode ? backupCode : undefined,
      })
      await queryClient.invalidateQueries({ queryKey: ['auth', 'me'] })
      navigate('/')
    } catch (caught) {
      setError(getErrorMessage(caught))
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <main className="relative flex min-h-screen items-center justify-center overflow-hidden px-4 py-8">
      <div className="app-backdrop" aria-hidden="true" />
      <section className="glass-panel w-full max-w-md">
        <div className="border-b border-white/50 px-5 py-5">
          <div className="flex items-center gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-gradient-to-br from-[#1677ff] via-[#1aa8ff] to-[#22d7c7] text-white shadow-lg shadow-blue-500/20">
              <ShieldCheck size={20} />
            </div>
            <div>
              <h1 className="text-lg font-semibold text-slate-950">接口压测控制台</h1>
              <div className="mt-1 text-sm text-slate-500">账号密码 + 双因素认证</div>
            </div>
          </div>
        </div>

        {step === 'credentials' ? (
          <form className="space-y-4 p-5" onSubmit={submitCredentials}>
            <label className="block space-y-1">
              <span className="field-label">用户名</span>
              <input
                autoComplete="username"
                className="input"
                value={username}
                onChange={(event) => setUsername(event.target.value)}
              />
            </label>
            <label className="block space-y-1">
              <span className="field-label">密码</span>
              <input
                autoComplete="current-password"
                className="input"
                type="password"
                value={password}
                onChange={(event) => setPassword(event.target.value)}
              />
            </label>
            {error ? <div className="rounded-md bg-red-50 px-4 py-3 text-sm text-red-700">{error}</div> : null}
            <button className="btn btn-primary w-full" disabled={isSubmitting} type="submit">
              <LogIn size={16} />
              登录
            </button>
          </form>
        ) : null}

        {step === 'verify-totp' ? (
          <form className="space-y-4 p-5" onSubmit={submitVerification}>
            {useBackupCode ? (
              <label className="block space-y-1">
                <span className="field-label">备用码</span>
                <input
                  autoComplete="one-time-code"
                  className="input"
                  value={backupCode}
                  onChange={(event) => setBackupCode(event.target.value)}
                />
              </label>
            ) : (
              <label className="block space-y-1">
                <span className="field-label">验证码</span>
                <input
                  autoComplete="one-time-code"
                  className="input"
                  inputMode="numeric"
                  value={code}
                  onChange={(event) => setCode(event.target.value)}
                />
              </label>
            )}
            <button
              className="text-sm font-medium text-blue-700 hover:text-blue-800"
              type="button"
              onClick={() => setUseBackupCode((value) => !value)}
            >
              {useBackupCode ? '使用验证码登录' : '使用备用码登录'}
            </button>
            {error ? <div className="rounded-md bg-red-50 px-4 py-3 text-sm text-red-700">{error}</div> : null}
            <button className="btn btn-primary w-full" disabled={isSubmitting} type="submit">
              <LogIn size={16} />
              登录
            </button>
          </form>
        ) : null}
      </section>
    </main>
  )
}
