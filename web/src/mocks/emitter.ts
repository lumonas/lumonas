import type { LumoEvent } from '@/api/types'
import { runtime } from '@/mocks/db'

export type EventListener = (event: { data: string }) => void

export const listeners = new Set<EventListener>()

export function emit(
  type: string,
  severity: LumoEvent['severity'],
  resource: LumoEvent['resource'],
  data: Record<string, unknown>,
) {
  const envelope: LumoEvent = {
    schemaVersion: 1,
    id: `evt-${++runtime.eventCounter}`,
    type,
    timestamp: new Date().toISOString(),
    severity,
    resource,
    data,
  }
  const payload = JSON.stringify(envelope)
  for (const listener of listeners) {
    listener({ data: payload })
  }
}
