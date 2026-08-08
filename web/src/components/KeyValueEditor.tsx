import { Lock, Plus, Trash2 } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'

type Row = {
  id: string
  key: string
  value: string
}

type KeyValueEditorProps = {
  value: Record<string, string>
  onChange: (next: Record<string, string>) => void
  keyPlaceholder?: string
  valuePlaceholder?: string
  // onEncryptValue：可选。传了就在每一行值输入框旁边多一个加密按钮，点一下
  // 用这个函数把当前值原地替换成加密后的结果（比如接口要求密码传 MD5，不用
  // 自己手算再贴进来）。不传就是原来的样子——这个组件还被请求头、提取变量
  // 两处复用，那两处不需要这个按钮。
  onEncryptValue?: (value: string) => string
  encryptLabel?: string
}

function makeID() {
  return Math.random().toString(36).slice(2)
}

function rowsFromValue(value: Record<string, string>): Row[] {
  return Object.entries(value).map(([key, rowValue]) => ({
    id: makeID(),
    key,
    value: rowValue,
  }))
}

function valueFromRows(rows: Row[]) {
  const next: Record<string, string> = {}
  for (const row of rows) {
    const key = row.key.trim()
    if (key !== '') {
      next[key] = row.value
    }
  }
  return next
}

export default function KeyValueEditor({
  value,
  onChange,
  keyPlaceholder = '键',
  valuePlaceholder = '值',
  onEncryptValue,
  encryptLabel = '加密',
}: KeyValueEditorProps) {
  const incomingSignature = useMemo(() => JSON.stringify(value), [value])
  const [rows, setRows] = useState<Row[]>(() => rowsFromValue(value))

  useEffect(() => {
    if (JSON.stringify(valueFromRows(rows)) !== incomingSignature) {
      setRows(rowsFromValue(value))
    }
  }, [incomingSignature, rows, value])

  function commit(nextRows: Row[]) {
    setRows(nextRows)
    onChange(valueFromRows(nextRows))
  }

  function updateRow(index: number, patch: Partial<Row>) {
    commit(rows.map((row, rowIndex) => (rowIndex === index ? { ...row, ...patch } : row)))
  }

  function addRow() {
    setRows([...rows, { id: makeID(), key: '', value: '' }])
  }

  function removeRow(index: number) {
    commit(rows.filter((_, rowIndex) => rowIndex !== index))
  }

  return (
    <div className="space-y-2">
      {rows.map((row, index) => (
        <div
          className={`grid gap-2 ${
            onEncryptValue ? 'grid-cols-[minmax(0,1fr)_minmax(0,1.4fr)_36px_36px]' : 'grid-cols-[minmax(0,1fr)_minmax(0,1.4fr)_36px]'
          }`}
          key={row.id}
        >
          <input
            className="input"
            placeholder={keyPlaceholder}
            value={row.key}
            onChange={(event) => updateRow(index, { key: event.target.value })}
          />
          <input
            className="input"
            placeholder={valuePlaceholder}
            value={row.value}
            onChange={(event) => updateRow(index, { value: event.target.value })}
          />
          {onEncryptValue ? (
            <button
              aria-label={encryptLabel}
              className="icon-btn"
              title={`${encryptLabel}（原地替换当前值）`}
              type="button"
              disabled={row.value === ''}
              onClick={() => updateRow(index, { value: onEncryptValue(row.value) })}
            >
              <Lock size={16} />
            </button>
          ) : null}
          <button
            aria-label="删除"
            className="icon-btn"
            title="删除"
            type="button"
            onClick={() => removeRow(index)}
          >
            <Trash2 size={16} />
          </button>
        </div>
      ))}
      <button className="btn btn-secondary" type="button" onClick={addRow}>
        <Plus size={16} />
        添加
      </button>
    </div>
  )
}
