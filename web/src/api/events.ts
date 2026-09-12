import { MockEventSource } from '@/mocks/event-stream'

export const useMocks = import.meta.env.VITE_USE_MOCKS !== 'false'

export interface EventSourceLike {
  addEventListener(type: 'message', listener: (event: { data: string }) => void): void
  removeEventListener(type: 'message', listener: (event: { data: string }) => void): void
  close(): void
}

export function connectEventStream(): EventSourceLike {
  const url = '/api/v1/events/stream'
  if (useMocks) {
    return new MockEventSource(url)
  }
  return new EventSource(url)
}
