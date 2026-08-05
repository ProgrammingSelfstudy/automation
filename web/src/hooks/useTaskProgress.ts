import { useEffect, useState } from 'react'

import type { ProgressEvent } from '../api/client'
import { taskProgressURL } from '../api/client'

export const MAX_PROGRESS_EVENTS = 500

export type ProgressConnectionState = 'idle' | 'connecting' | 'open' | 'closed' | 'error'

export function appendProgressEvent(events: ProgressEvent[], event: ProgressEvent) {
  const next = [...events, event]
  if (next.length > MAX_PROGRESS_EVENTS) {
    return next.slice(next.length - MAX_PROGRESS_EVENTS)
  }
  return next
}

export default function useTaskProgress(taskID: string | undefined) {
  const [events, setEvents] = useState<ProgressEvent[]>([])
  const [state, setState] = useState<ProgressConnectionState>(taskID ? 'connecting' : 'idle')

  useEffect(() => {
    if (!taskID) {
      setEvents([])
      setState('idle')
      return
    }

    let closedByCleanup = false
    setEvents([])
    setState('connecting')

    const socket = new WebSocket(taskProgressURL(taskID))
    socket.onopen = () => setState('open')
    socket.onmessage = (message) => {
      try {
        const event = JSON.parse(message.data as string) as ProgressEvent
        setEvents((current) => appendProgressEvent(current, event))
      } catch {
        setState('error')
      }
    }
    socket.onerror = () => setState('error')
    socket.onclose = () => {
      if (!closedByCleanup) {
        setState('closed')
      }
    }

    return () => {
      closedByCleanup = true
      socket.close()
    }
  }, [taskID])

  return { events, state }
}
