import { useQueryClient } from '@tanstack/react-query'
import { Check, KeyRound, LogIn, ShieldCheck } from 'lucide-react'
import { type FormEvent, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import * as QRCode from 'qrcode'

import { ApiError, confirmTOTP, login, setupTOTP } from '../api/client'
import { getErrorMessage } from '../utils/format'

type LoginStep = 'credentials' | 'setup-totp' | 'verify-totp' | 'backup-codes'

export default function LoginPage() {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [step, setStep] = useState<LoginStep>('credentials')
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [code, setCode] = useState('')
  const [backupCode, setBackupCode] = useState('')
  const [useBackupCode, setUseBackupCode] = useState(false)
  const [secret, setSecret] = useState('')
  const [qrDataURL, setQRDataURL] = useState('')
  const [backupCodes, setBackupCodes] = useState<string[]>([])
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
        try {
          await startTOTPSetup()
        } catch (setupError) {
          setError(getErrorMessage(setupError))
        }
      } else if (caught instanceof ApiError && caught.message === 'totp_code_required') {
        setStep('verify-totp')
      } else {
        setError(getErrorMessage(caught))
      }
    } finally {
      setIsSubmitting(false)
    }
  }

  async function startTOTPSetup() {
    const setup = await setupTOTP({ username, password })
    setSecret(setup.secret)
    setQRDataURL(await QRCode.toDataURL(setup.otpauth_url, { margin: 1, width: 224 }))
    setCode('')
    setStep('setup-totp')
  }

  async function submitTOTPSetup(event: FormEvent) {
    event.preventDefault()
    setError('')
    setIsSubmitting(true)
    try {
      const response = await confirmTOTP({ username, password, code })
      setBackupCodes(response.backup_codes)
      setStep('backup-codes')
      await queryClient.invalidateQueries({ queryKey: ['auth', 'me'] })
    } catch (caught) {
      setError(getErrorMessage(caught))
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

  async function finishSetup() {
    await queryClient.invalidateQueries({ queryKey: ['auth', 'me'] })
    navigate('/')
  }

  return (
    <main className="flex min-h-screen items-center justify-center bg-slate-100 px-4 py-8">
      <section className="w-full max-w-md rounded-lg border border-slate-200 bg-white shadow-sm">
        <div className="border-b border-slate-200 px-5 py-5">
          <div className="flex items-center gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-blue-600 text-white">
              <ShieldCheck size={20} />
            </div>
            <div>
              <h1 className="text-lg font-semibold text-slate-950">接口压测控制台</h1>
              <div className="mt-1 text-sm text-slate-500">账号密码 + 2FA</div>
            </div>
          </div>
        </div>

        {step === 'credentials' ? (
          <form className="space-y-4 p-5" onSubmit={submitCredentials}>
            <label className="block space-y-1">
              <span className="field-label">username</span>
              <input
                autoComplete="username"
                className="input"
                value={username}
                onChange={(event) => setUsername(event.target.value)}
              />
            </label>
            <label className="block space-y-1">
              <span className="field-label">password</span>
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

        {step === 'setup-totp' ? (
          <form className="space-y-5 p-5" onSubmit={submitTOTPSetup}>
            <div className="rounded-md border border-slate-200 bg-slate-50 p-4">
              {qrDataURL ? (
                <img alt="TOTP QR code" className="mx-auto h-56 w-56 rounded-md bg-white p-2" src={qrDataURL} />
              ) : null}
              <div className="mt-3 break-all rounded bg-white px-3 py-2 font-mono text-xs text-slate-700 ring-1 ring-slate-200">
                {secret}
              </div>
            </div>
            <label className="block space-y-1">
              <span className="field-label">verification code</span>
              <input
                autoComplete="one-time-code"
                className="input"
                inputMode="numeric"
                value={code}
                onChange={(event) => setCode(event.target.value)}
              />
            </label>
            {error ? <div className="rounded-md bg-red-50 px-4 py-3 text-sm text-red-700">{error}</div> : null}
            <button className="btn btn-primary w-full" disabled={isSubmitting} type="submit">
              <KeyRound size={16} />
              确认 2FA
            </button>
          </form>
        ) : null}

        {step === 'verify-totp' ? (
          <form className="space-y-4 p-5" onSubmit={submitVerification}>
            {useBackupCode ? (
              <label className="block space-y-1">
                <span className="field-label">backup code</span>
                <input
                  autoComplete="one-time-code"
                  className="input"
                  value={backupCode}
                  onChange={(event) => setBackupCode(event.target.value)}
                />
              </label>
            ) : (
              <label className="block space-y-1">
                <span className="field-label">verification code</span>
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

        {step === 'backup-codes' ? (
          <div className="space-y-5 p-5">
            <div>
              <h2 className="text-base font-semibold text-slate-950">备用恢复码</h2>
              <div className="mt-1 text-sm text-slate-500">只显示这一次。</div>
            </div>
            <div className="grid gap-2 sm:grid-cols-2">
              {backupCodes.map((item) => (
                <code className="rounded-md bg-slate-100 px-3 py-2 text-center text-sm text-slate-800" key={item}>
                  {item}
                </code>
              ))}
            </div>
            <button className="btn btn-primary w-full" type="button" onClick={finishSetup}>
              <Check size={16} />
              我已保存，继续
            </button>
          </div>
        ) : null}
      </section>
    </main>
  )
}
