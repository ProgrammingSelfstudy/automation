import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { CheckCircle2, Plus, Search, XCircle } from 'lucide-react'
import { type FormEvent, useState } from 'react'

import { createAccount, listAccounts } from '../api/client'
import { formatDateTime, getErrorMessage } from '../utils/format'

type AccountForm = {
  group_id: string
  username: string
  password: string
  enabled: boolean
  extra: string
}

const initialForm: AccountForm = {
  group_id: '',
  username: '',
  password: '',
  enabled: true,
  extra: '',
}

export default function AccountsPage() {
  const queryClient = useQueryClient()
  const [groupInput, setGroupInput] = useState('')
  const [activeGroup, setActiveGroup] = useState('')
  const [form, setForm] = useState<AccountForm>(initialForm)
  const [formError, setFormError] = useState('')

  const accountsQuery = useQuery({
    queryKey: ['accounts', activeGroup],
    queryFn: () => listAccounts(activeGroup),
    enabled: activeGroup !== '',
  })

  const createMutation = useMutation({
    mutationFn: createAccount,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['accounts', activeGroup] })
      setForm({
        ...initialForm,
        group_id: activeGroup || groupInput.trim(),
      })
      setFormError('')
    },
    onError: (error) => setFormError(getErrorMessage(error)),
  })

  function runSearch(event: FormEvent) {
    event.preventDefault()
    const group = groupInput.trim()
    setActiveGroup(group)
    setForm((current) => ({
      ...current,
      group_id: group || current.group_id,
    }))
  }

  function submitAccount(event: FormEvent) {
    event.preventDefault()
    const groupID = form.group_id.trim() || groupInput.trim()
    let extra: unknown

    if (form.extra.trim() !== '') {
      try {
        extra = JSON.parse(form.extra)
      } catch {
        setFormError('extra 必须是合法 JSON')
        return
      }
    }

    createMutation.mutate({
      group_id: groupID,
      username: form.username,
      password: form.password,
      enabled: form.enabled,
      ...(extra === undefined ? {} : { extra }),
    })
  }

  const accounts = accountsQuery.data ?? []

  return (
    <div className="page-shell">
      <section className="panel">
        <form className="toolbar" onSubmit={runSearch}>
          <div>
            <h2 className="text-base font-semibold text-slate-950">账号管理</h2>
            <div className="mt-1 text-sm text-slate-500">{activeGroup || 'group_id'}</div>
          </div>
          <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
            <input
              className="input sm:w-72"
              placeholder="group_id"
              value={groupInput}
              onChange={(event) => setGroupInput(event.target.value)}
            />
            <button className="btn btn-primary" disabled={groupInput.trim() === ''} type="submit">
              <Search size={16} />
              查询
            </button>
          </div>
        </form>

        {accountsQuery.isError ? (
          <div className="panel-body text-sm text-red-700">{getErrorMessage(accountsQuery.error)}</div>
        ) : (
          <div className="table-wrap">
            <table className="data-table">
              <thead>
                <tr>
                  <th>username</th>
                  <th>enabled</th>
                  <th>created_at</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 bg-white">
                {accountsQuery.isFetching ? (
                  <tr>
                    <td className="text-slate-500" colSpan={3}>
                      加载中
                    </td>
                  </tr>
                ) : accounts.length === 0 ? (
                  <tr>
                    <td className="text-slate-500" colSpan={3}>
                      暂无账号
                    </td>
                  </tr>
                ) : (
                  accounts.map((account) => (
                    <tr key={account.id}>
                      <td className="font-medium text-slate-950">{account.username}</td>
                      <td>
                        <span
                          className={`inline-flex items-center gap-1 rounded px-2 py-1 text-xs font-semibold ring-1 ${
                            account.enabled
                              ? 'bg-emerald-50 text-emerald-700 ring-emerald-200'
                              : 'bg-slate-100 text-slate-600 ring-slate-200'
                          }`}
                        >
                          {account.enabled ? <CheckCircle2 size={14} /> : <XCircle size={14} />}
                          {account.enabled ? 'true' : 'false'}
                        </span>
                      </td>
                      <td>{formatDateTime(account.created_at)}</td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        )}
      </section>

      <form className="panel" onSubmit={submitAccount}>
        <div className="toolbar">
          <h2 className="text-base font-semibold text-slate-950">新建账号</h2>
          <button className="btn btn-primary" disabled={createMutation.isPending} type="submit">
            <Plus size={16} />
            提交
          </button>
        </div>
        <div className="grid gap-4 p-4 sm:p-5 lg:grid-cols-2">
          <label className="space-y-1">
            <span className="field-label">group_id</span>
            <input
              className="input"
              value={form.group_id}
              onChange={(event) => setForm({ ...form, group_id: event.target.value })}
            />
          </label>
          <label className="space-y-1">
            <span className="field-label">username</span>
            <input
              className="input"
              value={form.username}
              onChange={(event) => setForm({ ...form, username: event.target.value })}
            />
          </label>
          <label className="space-y-1">
            <span className="field-label">password</span>
            <input
              className="input"
              type="password"
              value={form.password}
              onChange={(event) => setForm({ ...form, password: event.target.value })}
            />
          </label>
          <label className="flex h-10 items-center gap-3 self-end text-sm font-medium text-slate-700">
            <input
              checked={form.enabled}
              className="h-4 w-4 rounded border-slate-300 text-blue-600 focus:ring-blue-200"
              type="checkbox"
              onChange={(event) => setForm({ ...form, enabled: event.target.checked })}
            />
            enabled
          </label>
          <label className="space-y-1 lg:col-span-2">
            <span className="field-label">extra</span>
            <textarea
              className="textarea"
              value={form.extra}
              onChange={(event) => setForm({ ...form, extra: event.target.value })}
            />
          </label>
          {formError ? <div className="text-sm text-red-700 lg:col-span-2">{formError}</div> : null}
        </div>
      </form>
    </div>
  )
}
