import type { TaskStatus } from '../api/client'
import { formatTaskStatus } from '../utils/format'

const STATUS_STYLE: Record<TaskStatus, string> = {
  pending: 'bg-amber-50 text-amber-700 ring-amber-200',
  running: 'bg-blue-50 text-blue-700 ring-blue-200',
  success: 'bg-emerald-50 text-emerald-700 ring-emerald-200',
  failed: 'bg-red-50 text-red-700 ring-red-200',
  canceled: 'bg-slate-100 text-slate-600 ring-slate-200',
}

export default function StatusBadge({ status }: { status: TaskStatus }) {
  return (
    <span
      className={`inline-flex h-6 items-center rounded px-2 text-xs font-semibold ring-1 ${STATUS_STYLE[status]}`}
    >
      {formatTaskStatus(status)}
    </span>
  )
}
