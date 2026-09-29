import { MockEventSource } from '@/mocks/event-stream'

// The appliance runtime is the default. Set VITE_USE_MOCKS=true for the standalone UI demo.
export const useMocks = import.meta.env.VITE_USE_MOCKS === 'true'

export interface EventSourceLike {
  readonly readyState?: number
  addEventListener(type: 'message', listener: (event: { data: string }) => void): void
  addEventListener(type: 'open' | 'error', listener: (event: Event) => void): void
  removeEventListener(type: 'message', listener: (event: { data: string }) => void): void
  removeEventListener(type: 'open' | 'error', listener: (event: Event) => void): void
  close(): void
}

export const EVENT_STREAM_PATH = '/api/v1/events/stream'

export function connectEventStream(): EventSourceLike {
	const url = EVENT_STREAM_PATH
  if (useMocks) {
    return new MockEventSource(url)
  }
  return new EventSource(url)
}
