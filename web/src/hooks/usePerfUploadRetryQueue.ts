import { useQueryClient } from '@tanstack/react-query'
import { useCallback, useEffect, useState } from 'react'

import { createPerfTask, type CreatePerfTaskRequest } from '../api/client'
import { getErrorMessage } from '../utils/format'
import {
  enqueuePerfUpload,
  loadQueuedPerfUploads,
  removeQueuedPerfUpload,
  type QueuedPerfUpload,
} from '../utils/perfUploadQueue'

// 采集停止后最终记录靠浏览器转发 POST 给中心平台落库，这段窗口期断网
// 或标签页被关掉会丢数据（见 docs/architecture-perf-rabbit-merge.md
// "反向思考"第 1 点）。这个 hook 被"性能采集"和"历史数据"两个页面共用：
// 不管用户停止采集后先去哪个页面，挂载时都会自动重试一次积压的记录，
// 不用绑定在某一个页面的生命周期上。
export default function usePerfUploadRetryQueue() {
  const queryClient = useQueryClient()
  const [pendingUploads, setPendingUploads] = useState<QueuedPerfUpload[]>(() => loadQueuedPerfUploads())
  const [retryingUploads, setRetryingUploads] = useState(false)
  const [uploadError, setUploadError] = useState('')

  const attemptUpload = useCallback(
    async (item: QueuedPerfUpload) => {
      try {
        await createPerfTask(item.payload)
        removeQueuedPerfUpload(item.id)
        setPendingUploads((current) => current.filter((pending) => pending.id !== item.id))
        setUploadError('')
        queryClient.invalidateQueries({ queryKey: ['perf-tasks'] })
      } catch (error) {
        setUploadError(getErrorMessage(error))
      }
    },
    [queryClient],
  )

  const retryPendingUploads = useCallback(async () => {
    setRetryingUploads(true)
    for (const item of loadQueuedPerfUploads()) {
      await attemptUpload(item)
    }
    setRetryingUploads(false)
  }, [attemptUpload])

  useEffect(() => {
    retryPendingUploads()
  }, [retryPendingUploads])

  const enqueueUpload = useCallback((payload: CreatePerfTaskRequest) => {
    const queued = enqueuePerfUpload(payload)
    setPendingUploads((current) => [...current, queued])
    return queued
  }, [])

  return { pendingUploads, retryingUploads, uploadError, attemptUpload, retryPendingUploads, enqueueUpload }
}
