import { RefreshCw } from 'lucide-react'

import type { QueuedPerfUpload } from '../utils/perfUploadQueue'

export default function PendingPerfUploadsBanner({
  pendingUploads,
  retrying,
  onRetry,
}: {
  pendingUploads: QueuedPerfUpload[]
  retrying: boolean
  onRetry: () => void
}) {
  if (pendingUploads.length === 0) {
    return null
  }

  return (
    <div className="flex flex-wrap items-center justify-between gap-3 rounded-md bg-amber-50 px-4 py-3 text-sm text-amber-800 ring-1 ring-amber-200">
      <span>有 {pendingUploads.length} 条采集记录还没成功上报到中心平台，已保存在本地，不会丢。</span>
      <button className="btn btn-secondary" disabled={retrying} type="button" onClick={onRetry}>
        <RefreshCw size={16} className={retrying ? 'animate-spin' : ''} />
        重试上报
      </button>
    </div>
  )
}
