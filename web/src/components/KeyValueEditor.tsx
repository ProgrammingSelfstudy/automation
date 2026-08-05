import { Plus, Trash2 } from 'lucide-react'
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
  keyPlaceholder = 'key',
  valuePlaceholder = 'value',
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
          className="grid grid-cols-[minmax(0,1fr)_minmax(0,1.4fr)_36px] gap-2"
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
