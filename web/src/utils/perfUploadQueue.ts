import type { CreatePerfTaskRequest } from '../api/client'

// 性能采集停止后，最终记录要由浏览器转发 POST 给中心平台落 MySQL（见
// docs/architecture-perf-rabbit-merge.md 的"反向思考"第 1 点）。这段窗口期
// 如果断网或者标签页被关掉，记录本来会直接丢——这里用 localStorage 落一份
// 待上报队列：拿到 Agent 返回的完整记录后立刻入队，上报成功再出队；就算
// 上报请求失败或者标签页被关掉重开，下次打开这个页面时队列还在，可以重试。
// 这解决不了"换了台电脑/清了浏览器数据"这种更极端的场景——那需要 Agent
// 直接上报 + 独立鉴权，是架构文档里标注为后续可选的更大改动，这里先做
// 成本最低、不改鉴权模型的这版。

const STORAGE_KEY = 'perf-upload-queue-v1'

export type QueuedPerfUpload = {
  id: string
  payload: CreatePerfTaskRequest
  queuedAt: string
}

function readStorage(): QueuedPerfUpload[] {
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY)
    if (!raw) return []
    const parsed = JSON.parse(raw) as unknown
    return Array.isArray(parsed) ? (parsed as QueuedPerfUpload[]) : []
  } catch {
    return []
  }
}

function writeStorage(queue: QueuedPerfUpload[]): void {
  try {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(queue))
  } catch {
    // 存储满了或者被禁用（隐私模式等）：退化为"这次不重试"，不影响主流程。
  }
}

export function loadQueuedPerfUploads(): QueuedPerfUpload[] {
  return readStorage()
}

export function enqueuePerfUpload(payload: CreatePerfTaskRequest): QueuedPerfUpload {
  const item: QueuedPerfUpload = {
    id: `${Date.now()}-${Math.random().toString(36).slice(2, 10)}`,
    payload,
    queuedAt: new Date().toISOString(),
  }
  writeStorage([...readStorage(), item])
  return item
}

export function removeQueuedPerfUpload(id: string): void {
  writeStorage(readStorage().filter((item) => item.id !== id))
}
